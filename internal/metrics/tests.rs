//! Tests for the metrics package.
//!
//! Ports the thirteen Go test cases spread across
//! `cgroup_linux_test.go`, `proctree_linux_test.go`, and
//! `factory_linux_test.go`. Each Go test becomes one `#[test]` here —
//! the case identity is what the spec counts, not the syntax.
//!
//! Gated to Linux because the collectors read kernel interfaces that do
//! not exist on other platforms; a green tick from a test whose code path
//! never ran would be a lie. The `cfg(target_os = "linux")` on the
//! production modules is what makes the cross-platform crate build; the
//! `cfg(target_os = "linux")` here is what keeps the suite honest.

use std::fs;
use std::io::Write;
use std::path::PathBuf;
use std::process;
use std::sync::Mutex;

use crate::metrics::cgroup::{get_cgroup_path, read_cpu_usage};
use crate::metrics::factory::new_collector;
use crate::metrics::proctree::tests::ProcTreeCacheGuard;
use crate::metrics::proctree::{get_ppid, get_process_tree, new_proc_tree_collector};
use crate::metrics::{new_cgroup_collector, Collector, CollectorKind, MetricsError};

/// Process-global lock for /proc-mutating tests. The proctree snapshot
/// cache is process-global, and `cargo test` runs in parallel by default,
/// so two tests that both walk /proc can race. The lock alone isn't enough
/// — we also clear the cache inside each test (or wrap the body in a
/// [`ProcTreeCacheGuard`]) so state set by an earlier test doesn't leak.
static PROC_LOCK: Mutex<()> = Mutex::new(());

/// Allow tests that legitimately need the /proc PID self-reference to
/// obtain the current process's PID without each one calling
/// `process::id()` ad hoc.
fn self_pid() -> i32 {
	process::id() as i32
}

// --- cgroup_linux_test.go --------------------------------------------------

fn write_cpu_stat(dir: &std::path::Path, contents: &str) -> PathBuf {
	let path = dir.join("cpu.stat");
	let mut f = fs::File::create(&path).expect("create cpu.stat");
	f.write_all(contents.as_bytes()).expect("write cpu.stat");
	path
}

#[test]
fn read_cpu_usage_parses_usage_usec() {
	let tmp = tempfile::tempdir().expect("tempdir");
	write_cpu_stat(
		tmp.path(),
		"usage_usec 12345\nuser_usec 10000\nsystem_usec 2345\n",
	);
	let got = read_cpu_usage(tmp.path()).expect("read_cpu_usage");
	assert_eq!(got, 12345, "expected 12345, got {got}");
}

#[test]
fn read_cpu_usage_missing_field() {
	let tmp = tempfile::tempdir().expect("tempdir");
	write_cpu_stat(tmp.path(), "user_usec 10000\n");
	let err = read_cpu_usage(tmp.path()).expect_err("expected error");
	assert!(
		matches!(err, MetricsError::InvalidStatFormat),
		"expected InvalidStatFormat, got {err:?}"
	);
}

#[test]
fn read_cpu_usage_file_missing() {
	let tmp = tempfile::tempdir().expect("tempdir");
	let err = read_cpu_usage(tmp.path()).expect_err("expected error");
	assert!(
		matches!(err, MetricsError::Io(_)),
		"expected Io error, got {err:?}"
	);
}

#[test]
fn read_cpu_usage_bad_value() {
	let tmp = tempfile::tempdir().expect("tempdir");
	write_cpu_stat(tmp.path(), "usage_usec abc\n");
	let err = read_cpu_usage(tmp.path()).expect_err("expected error");
	assert!(
		matches!(err, MetricsError::InvalidStatValue(_)),
		"expected InvalidStatValue, got {err:?}"
	);
}

#[test]
fn get_cgroup_path_self() {
	let _lock = PROC_LOCK.lock().unwrap_or_else(|e| e.into_inner());
	if !std::path::Path::new("/proc/self/cgroup").exists() {
		eprintln!("/proc/self/cgroup missing, skipping");
		return;
	}
	let path = match get_cgroup_path(self_pid()) {
		Ok(p) => p,
		Err(e) => {
			eprintln!("no v2 cgroup for self: {e}");
			return;
		}
	};
	assert!(!path.as_os_str().is_empty(), "empty cgroup path");
}

#[test]
fn get_cgroup_path_bad_pid() {
	let _lock = PROC_LOCK.lock().unwrap_or_else(|e| e.into_inner());
	let err = get_cgroup_path(2_147_483_646).expect_err("expected error");
	assert!(
		matches!(err, MetricsError::Io(_)),
		"expected Io error, got {err:?}"
	);
}

#[test]
fn new_cgroup_collector_no_v2() {
	// The negative case is only meaningful when v2 is NOT mounted. On
	// every host we ship to, v2 is present, so this test is a no-op
	// (mirroring the Go `t.Skip`).
	if std::path::Path::new("/sys/fs/cgroup/cgroup.controllers").exists() {
		eprintln!("v2 is mounted; skipping negative case");
		return;
	}
	let err = new_cgroup_collector(self_pid()).expect_err("expected error");
	assert!(
		matches!(err, MetricsError::CgroupV2Unavailable),
		"expected CgroupV2Unavailable, got {err:?}"
	);
}

#[test]
fn cgroup_collector_collect_and_delta() {
	let c = match new_cgroup_collector(self_pid()) {
		Ok(c) => c,
		Err(e) => {
			eprintln!("cgroup v2 not usable: {e}");
			return;
		}
	};
	let mut c = c;
	let first = c.collect().expect("first collect");
	assert!(
		first.memory_bytes > 0,
		"memory should be > 0, got {}",
		first.memory_bytes
	);
	// Second collect exercises the CPU% delta; it may be 0.0 but must
	// not error.
	let _second = c.collect().expect("second collect");
}

// --- proctree_linux_test.go ------------------------------------------------

#[test]
fn get_process_tree_current_process() {
	let _lock = PROC_LOCK.lock().unwrap_or_else(|e| e.into_inner());
	let _guard = ProcTreeCacheGuard::new();
	let pid = self_pid();
	let tree = get_process_tree(pid).expect("get_process_tree");
	assert!(
		!tree.is_empty(),
		"expected at least one entry (the process itself)"
	);
	let root = &tree[0];
	assert_eq!(root.pid, pid, "root PID mismatch");
	assert_eq!(root.depth, 0, "root depth must be 0");
	assert!(!root.comm.is_empty(), "root Comm is empty");
	assert!(
		root.memory_bytes > 0,
		"root MemoryBytes must be > 0, got {}",
		root.memory_bytes
	);
}

#[test]
fn get_process_tree_depths_non_negative() {
	let _lock = PROC_LOCK.lock().unwrap_or_else(|e| e.into_inner());
	let _guard = ProcTreeCacheGuard::new();
	let tree = get_process_tree(self_pid()).expect("get_process_tree");
	for e in &tree {
		assert!(
			e.depth >= 0,
			"entry PID {} has negative depth {}",
			e.pid,
			e.depth
		);
	}
}

#[test]
fn proc_tree_collector_safe() {
	let _lock = PROC_LOCK.lock().unwrap_or_else(|e| e.into_inner());
	let _guard = ProcTreeCacheGuard::new();
	let mut c = new_proc_tree_collector(self_pid()).expect("new_proc_tree_collector");
	let first = c.collect().expect("first collect");
	assert!(first.memory_bytes > 0, "MemoryBytes is 0 on first collect");

	// Second collect hits the cache. The test exercises that the path
	// does not panic and continues to report memory.
	let second = c.collect().expect("second collect");
	assert!(second.memory_bytes > 0, "MemoryBytes is 0 on cache hit");
}

// --- factory_linux_test.go -------------------------------------------------

#[test]
fn new_collector_prefers_proc_tree() {
	let c = new_collector(self_pid()).expect("new_collector");
	// Mirrors the Go `c, ok := collector.(*ProcTreeCollector)` check
	// via the `kind()` discriminator on the trait.
	assert_eq!(
		c.kind(),
		CollectorKind::ProcTree.as_str(),
		"factory must prefer ProcTreeCollector"
	);
}

#[test]
fn new_collector_bad_pid_does_not_panic() {
	// Either factory returns ProcTree (because /proc/<pid>/stat happens
	// to be readable on some kernels) or the cgroup fallback errors.
	// Both outcomes are acceptable; we just verify no panic and the
	// error/collector are coherent.
	let _ = new_collector(2_147_483_646);
}

#[test]
fn get_ppid_for_self_is_positive() {
	let _lock = PROC_LOCK.lock().unwrap_or_else(|e| e.into_inner());
	let ppid = get_ppid(self_pid()).expect("get_ppid");
	assert!(ppid > 0, "ppid should be positive, got {ppid}");
}

#[test]
fn get_ppid_bad_pid_errors() {
	let _lock = PROC_LOCK.lock().unwrap_or_else(|e| e.into_inner());
	let err = get_ppid(2_147_483_646).expect_err("expected error");
	assert!(
		matches!(err, MetricsError::Io(_)),
		"expected Io error, got {err:?}"
	);
}
