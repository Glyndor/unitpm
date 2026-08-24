//! Tests for the `monit` command. Mirrors cases from
//! `internal/cli/commands/monit/{cmd,helpers}_test.go`.

use std::time::Duration;

use super::state::MAX_HISTORY as MAX_HISTORY_VAL;
use super::*;

#[test]
fn parse_args_default() {
	let p = parse_args(&[]);
	assert!(!p.json);
	assert!(p.target.is_none());
}

#[test]
fn parse_args_with_target() {
	let p = parse_args(&["api".into()]);
	assert_eq!(p.target.as_deref(), Some("api"));
}

#[test]
fn parse_args_with_json() {
	let p = parse_args(&["--json".into(), "api".into()]);
	assert!(p.json);
	assert_eq!(p.target.as_deref(), Some("api"));
}

#[test]
fn spec_includes_aliases() {
	let s = spec();
	assert_eq!(s.name, "monit");
	assert!(s.aliases.contains(&"top".to_string()));
	assert!(s.aliases.contains(&"monitor".to_string()));
}

#[test]
fn render_frame_does_not_panic_full_state() {
	let mut s = MonitState::default();
	s.info.name = "svc".into();
	s.info.pid = 1234;
	s.info.state = ProcessState::Running;
	s.info.cpu = 12.5;
	s.info.memory = 4 * 1024 * 1024;
	s.info.uptime = 3_725_000;
	s.info.restarts = 3;
	s.info.git_branch = Some("main".into());
	s.info.git_commit = Some("abc1234".into());
	s.info.version = "1.0".into();
	s.info.mode = "cluster".into();
	s.info.user = "root".into();
	s.spec.exec.command = Some("/usr/bin/node".into());
	s.spec.exec.args = Some(vec!["server.js".into()]);
	s.cpu_hist = vec![0.0, 5.0, 10.0, 15.0, 20.0, 25.0, 12.5];
	s.mem_hist = vec![0, 1024 * 1024, 2 * 1024 * 1024, 4 * 1024 * 1024];
	s.mem_max = 4 * 1024 * 1024;
	let mut buf = Vec::new();
	render_frame_to(&mut buf, &s, 120).unwrap();
	assert!(!buf.is_empty());
}

#[test]
fn render_frame_with_process_tree() {
	let mut s = MonitState::default();
	s.info.name = "svc".into();
	s.info.pid = 42;
	s.info.state = ProcessState::Running;
	s.cpu_hist = vec![10.0; 10];
	s.mem_hist = vec![1024 * 1024; 10];
	s.mem_max = 2 * 1024 * 1024;
	s.tree = vec![
		crate::metrics::ChildStat {
			pid: 42,
			comm: "node".into(),
			depth: 0,
			memory_bytes: 1024 * 1024,
		},
		crate::metrics::ChildStat {
			pid: 43,
			comm: "worker".into(),
			depth: 1,
			memory_bytes: 512 * 1024,
		},
	];
	let mut buf = Vec::new();
	render_frame_to(&mut buf, &s, 80).unwrap();
}

#[test]
fn render_frame_stopped_state() {
	let mut s = MonitState::default();
	s.info.name = "svc".into();
	s.info.state = ProcessState::Stopped;
	s.cpu_hist = vec![0.0; 5];
	s.mem_hist = vec![0; 5];
	let mut buf = Vec::new();
	render_frame_to(&mut buf, &s, 80).unwrap();
}

#[test]
fn render_frame_failed_state() {
	let mut s = MonitState::default();
	s.info.name = "svc".into();
	s.info.state = ProcessState::Failed;
	let mut buf = Vec::new();
	render_frame_to(&mut buf, &s, 80).unwrap();
}

#[test]
fn render_frame_empty_history() {
	let mut s = MonitState::default();
	s.info.name = "empty".into();
	s.info.state = ProcessState::Running;
	let mut buf = Vec::new();
	render_frame_to(&mut buf, &s, 80).unwrap();
}

#[test]
fn render_frame_no_git() {
	let mut s = MonitState::default();
	s.info.name = "svc".into();
	s.info.state = ProcessState::Running;
	s.spec.exec.command = Some("/bin/true".into());
	let mut buf = Vec::new();
	render_frame_to(&mut buf, &s, 80).unwrap();
}

#[test]
fn run_loop_quits_on_event() {
	let mut state = MonitState::default();
	state.info.name = "svc".into();
	let events: Vec<Event> = vec![Event::Quit];
	let mut it = events.into_iter();
	run_loop(&mut state, &mut it, |_| Ok(()));
}

#[test]
fn run_loop_quits_on_q_key() {
	let mut state = MonitState::default();
	state.info.name = "svc".into();
	let events: Vec<Event> = vec![Event::Key(b'q'), Event::Tick];
	let mut it = events.into_iter();
	run_loop(&mut state, &mut it, |_| Ok(()));
}

#[test]
fn run_loop_quits_on_ctrl_c() {
	let mut state = MonitState::default();
	state.info.name = "svc".into();
	let events: Vec<Event> = vec![Event::Key(3)];
	let mut it = events.into_iter();
	run_loop(&mut state, &mut it, |_| Ok(()));
}

#[test]
fn run_loop_handles_other_keys() {
	let mut state = MonitState::default();
	state.info.name = "svc".into();
	let events: Vec<Event> = vec![Event::Key(b'x'), Event::Resize, Event::Quit];
	let mut it = events.into_iter();
	run_loop(&mut state, &mut it, |_| Ok(()));
}

#[test]
fn run_loop_tick_triggers_refresh_and_render() {
	let mut state = MonitState::default();
	state.info.name = "svc".into();
	state.info.cpu = 1.0;
	let mut calls = 0;
	let events: Vec<Event> = vec![Event::Tick, Event::Quit];
	let mut it = events.into_iter();
	run_loop(&mut state, &mut it, |s| {
		calls += 1;
		s.info.cpu += 1.0;
		Ok(())
	});
	assert!(calls >= 1, "on_tick was not invoked");
	assert!(state.info.cpu > 1.0);
}

#[test]
fn max_history_locked() {
	assert_eq!(MAX_HISTORY_VAL, 120);
	assert_eq!(REFRESH_RATE, Duration::from_secs(1));
}

#[test]
fn print_json_writes_payload() {
	let mut state = MonitState::default();
	state.info.name = "svc".into();
	state.info.pid = 999;
	print_json(&state).unwrap();
}

#[test]
fn write_all_processes_prints_rows() {
	let procs = vec![ProcessInfo {
		name: "api".into(),
		namespace: "default".into(),
		pid: 1,
		state: ProcessState::Running,
		cpu: 0.5,
		memory: 1024,
		..empty_info()
	}];
	let mut buf = Vec::new();
	write_all_processes(&mut buf, &procs);
	let plain = String::from_utf8_lossy(&buf);
	assert!(plain.contains("api"));
	assert!(plain.contains("default"));
}

/// Test-only mock that returns canned responses.
struct MockMonitClient {
	show: Option<ShowResponse>,
	list: Vec<ProcessInfo>,
	proctree: Vec<crate::metrics::ChildStat>,
	list_err: Option<String>,
	show_err: Option<String>,
}

impl MonitClient for MockMonitClient {
	fn call_show(&mut self, _id: &str, resp: &mut ShowResponse) -> Result<(), String> {
		if let Some(e) = &self.show_err {
			return Err(e.clone());
		}
		if let Some(s) = &self.show {
			*resp = s.clone();
		}
		Ok(())
	}
	fn call_list(&mut self, out: &mut Vec<ProcessInfo>) -> Result<(), String> {
		if let Some(e) = &self.list_err {
			return Err(e.clone());
		}
		*out = self.list.clone();
		Ok(())
	}
	fn call_proctree(
		&mut self,
		_id: &str,
		out: &mut Vec<crate::metrics::ChildStat>,
	) -> Result<(), String> {
		*out = self.proctree.clone();
		Ok(())
	}
}

#[test]
fn run_with_mock_json_mode() {
	let info = ProcessInfo {
		name: "svc".into(),
		pid: 999,
		state: ProcessState::Running,
		..empty_info()
	};
	let mut client = MockMonitClient {
		show: Some(ShowResponse {
			info,
			spec: empty_spec(),
		}),
		list: Vec::new(),
		proctree: Vec::new(),
		list_err: None,
		show_err: None,
	};
	let args = vec!["svc".to_string(), "--json".to_string()];
	let events: Vec<Event> = vec![Event::Quit];
	let mut it = events.into_iter();
	run(Some(&mut client), &args, &mut it).unwrap();
}

#[test]
fn fetch_state_records_history() {
	let info = ProcessInfo {
		name: "testproc".into(),
		pid: 12345,
		state: ProcessState::Running,
		cpu: 1.5,
		memory: 1024 * 1024,
		..empty_info()
	};
	let mut client = MockMonitClient {
		show: Some(ShowResponse {
			info,
			spec: empty_spec(),
		}),
		list: Vec::new(),
		proctree: Vec::new(),
		list_err: None,
		show_err: None,
	};
	let mut state = MonitState::default();
	fetch_state(&mut client, "testproc", &mut state).unwrap();
	assert_eq!(state.info.name, "testproc");
	assert_eq!(state.cpu_hist.last().copied(), Some(1.5));
	assert_eq!(state.mem_max, 1024 * 1024);
}

#[test]
fn fetch_state_trims_history_at_max() {
	let info = ProcessInfo {
		cpu: 50.0,
		..empty_info()
	};
	let mut client = MockMonitClient {
		show: Some(ShowResponse {
			info,
			spec: empty_spec(),
		}),
		list: Vec::new(),
		proctree: Vec::new(),
		list_err: None,
		show_err: None,
	};
	let mut state = MonitState::default();
	for _ in 0..(MAX_HISTORY + 10) {
		fetch_state(&mut client, "x", &mut state).unwrap();
	}
	assert_eq!(state.cpu_hist.len(), MAX_HISTORY);
	assert_eq!(state.mem_hist.len(), MAX_HISTORY);
}

#[test]
fn run_list_all_processes() {
	let mut client = MockMonitClient {
		show: None,
		list: vec![ProcessInfo {
			name: "api".into(),
			pid: 1,
			..empty_info()
		}],
		proctree: Vec::new(),
		list_err: None,
		show_err: None,
	};
	let args = vec![];
	let events: Vec<Event> = vec![Event::Quit];
	let mut it = events.into_iter();
	run(Some(&mut client), &args, &mut it).unwrap();
}

#[test]
fn run_ipc_error_propagates() {
	let mut client = MockMonitClient {
		show: None,
		list: Vec::new(),
		proctree: Vec::new(),
		list_err: Some("connection refused".into()),
		show_err: None,
	};
	let args = vec![];
	let events: Vec<Event> = vec![Event::Quit];
	let mut it = events.into_iter();
	let err = run(Some(&mut client), &args, &mut it).unwrap_err();
	assert!(err.to_string().contains("monit failed"), "got {err}");
}
