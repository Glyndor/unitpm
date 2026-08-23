//! Target-selector resolution for lifecycle commands.
//!
//! 14 cases ported from `internal/cli/expand/expand_test.go`.
//!
//! The module owns three kinds of selector: literal IDs/names (passed
//! through unchanged), `<namespace>:*` (expands via the daemon), and
//! `*`/`*:*` (expands to every managed process). A `--namespace` flag is
//! equivalent to `<namespace>:*` but cannot be combined with positional
//! targets — that mix is rejected with a usage error so the operator gets
//! a deterministic error rather than an ambiguous union.

use std::collections::HashSet;

use crate::cli::errs::UsageError;
use crate::types::{ProcessInfo, DEFAULT_NAMESPACE};

/// Long flag for the `--namespace` selector.
pub const NAMESPACE_FLAG: &str = "namespace";
/// Single-token wildcard ("every process").
pub const WILDCARD_ALL: &str = "*";
/// Two-token wildcard ("every process in any namespace").
pub const WILDCARD_ALL_PAIR: &str = "*:*";
/// Separator between a namespace and a name in a single token.
pub const NAMESPACE_SEPARATOR: &str = ":";

/// A classified positional target token.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Selector {
	pub namespace: String,
	pub all_in_ns: bool,
	pub all_procs: bool,
}

/// Parse a single positional selector. Tokens that aren't wildcards are
/// returned with both wildcard flags false; the caller passes them straight
/// through to the daemon's name/prefix resolution.
#[must_use]
pub fn parse_selector(tok: &str) -> Selector {
	if tok == WILDCARD_ALL || tok == WILDCARD_ALL_PAIR {
		return Selector {
			namespace: String::new(),
			all_in_ns: false,
			all_procs: true,
		};
	}
	if let Some(idx) = tok.find(NAMESPACE_SEPARATOR) {
		let ns = &tok[..idx];
		let name = &tok[idx + 1..];
		if name == WILDCARD_ALL && !ns.is_empty() && ns != WILDCARD_ALL {
			return Selector {
				namespace: ns.to_string(),
				all_in_ns: true,
				all_procs: false,
			};
		}
	}
	Selector {
		namespace: String::new(),
		all_in_ns: false,
		all_procs: false,
	}
}

/// Minimal client surface this module needs from the IPC layer. Defined
/// locally so the test can mock it without re-implementing the full
/// transport trait (which is generic over Serialize/Deserialize). Phase 6b
/// will add an `impl ListClient for transport::Client` adapter.
pub trait ListClient {
	fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, ListError>;
}

/// Error returned by [`ListClient::list_processes`]. The string payload is
/// what bubbles up to the operator — the batch module's `--json` shape
/// matches the Go side, and the human-readable path renders the
/// `Display` form verbatim.
#[derive(Debug)]
pub enum ListError {
	Protocol(String),
	Network(String),
}

impl std::fmt::Display for ListError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			ListError::Protocol(m) => write!(f, "{m}"),
			ListError::Network(m) => write!(f, "{m}"),
		}
	}
}

impl std::error::Error for ListError {}

/// Resolve the positional `ids` and optional `--namespace` value into a
/// deduplicated slice of process IDs. Literal targets (no wildcard, no
/// `--namespace` flag) are passed through unchanged so the daemon can
/// resolve names/prefixes the same way as before.
///
/// Wildcard expansion calls [`ListClient::list_processes`]. The client is
/// required as soon as any wildcard or `--namespace` flag is present;
/// callers that pass only literals can pass `None` and `targets` will
/// return early without an IPC round-trip.
pub fn targets<C: ListClient + ?Sized>(
	client: Option<&mut C>,
	ids: &[String],
	namespace: &str,
) -> Result<Vec<String>, ExpandError> {
	if !namespace.is_empty() {
		if !ids.is_empty() {
			return Err(ExpandError::Usage(UsageError::new(
				"cannot combine --namespace with positional targets — use one or the other",
			)));
		}
		return expand_namespace(client, namespace);
	}

	let selectors: Vec<Selector> = ids.iter().map(|s| parse_selector(s)).collect();
	let has_wildcard = selectors.iter().any(|s| s.all_in_ns || s.all_procs);

	if !has_wildcard {
		return Ok(ids.to_vec());
	}

	let procs = fetch_list(client)?;
	let mut seen: HashSet<String> = HashSet::new();
	let mut out: Vec<String> = Vec::with_capacity(procs.len());

	let add = |id: &str, seen: &mut HashSet<String>, out: &mut Vec<String>| {
		if seen.insert(id.to_string()) {
			out.push(id.to_string());
		}
	};

	for (tok, sel) in ids.iter().zip(selectors.iter()) {
		match () {
			_ if sel.all_procs => {
				if procs.is_empty() {
					return Err(ExpandError::Other("no managed processes".into()));
				}
				for p in &procs {
					add(&p.id, &mut seen, &mut out);
				}
			}
			_ if sel.all_in_ns => {
				let mut matched = false;
				for p in &procs {
					if process_ns(p) == sel.namespace {
						add(&p.id, &mut seen, &mut out);
						matched = true;
					}
				}
				if !matched {
					return Err(ExpandError::Other(format!(
						"no processes in namespace {:?}",
						sel.namespace
					)));
				}
			}
			_ => {
				add(tok, &mut seen, &mut out);
			}
		}
	}

	Ok(out)
}

fn expand_namespace<C: ListClient + ?Sized>(
	client: Option<&mut C>,
	namespace: &str,
) -> Result<Vec<String>, ExpandError> {
	let procs = fetch_list(client)?;
	let mut out: Vec<String> = Vec::with_capacity(procs.len());
	for p in &procs {
		if process_ns(p) == namespace {
			out.push(p.id.clone());
		}
	}
	if out.is_empty() {
		return Err(ExpandError::Other(format!(
			"no processes in namespace {namespace:?}"
		)));
	}
	Ok(out)
}

fn fetch_list<C: ListClient + ?Sized>(
	client: Option<&mut C>,
) -> Result<Vec<ProcessInfo>, ExpandError> {
	let client = client.ok_or_else(|| {
		ExpandError::Other("internal error: expand requires an IPC client".into())
	})?;
	client
		.list_processes()
		.map_err(|e| ExpandError::List(e.to_string()))
}

fn process_ns(p: &ProcessInfo) -> String {
	if p.namespace.is_empty() {
		DEFAULT_NAMESPACE.to_string()
	} else {
		p.namespace.clone()
	}
}

/// Errors surfaced by [`targets`]. The split mirrors the Go side: usage
/// errors for "you typed the flags wrong", list errors for daemon
/// problems, and other errors for "the cluster is empty in this slice".
#[derive(Debug)]
pub enum ExpandError {
	Usage(UsageError),
	List(String),
	Other(String),
}

impl std::fmt::Display for ExpandError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			ExpandError::Usage(e) => write!(f, "{e}"),
			ExpandError::List(msg) => write!(f, "list failed: {msg}"),
			ExpandError::Other(msg) => write!(f, "{msg}"),
		}
	}
}

impl std::error::Error for ExpandError {}

impl From<UsageError> for ExpandError {
	fn from(e: UsageError) -> Self {
		ExpandError::Usage(e)
	}
}

#[cfg(test)]
mod tests {
	use std::cell::Cell;

	use super::*;

	struct ListMock {
		procs: Vec<ProcessInfo>,
		err: Option<String>,
		calls: Cell<u32>,
	}

	impl ListClient for ListMock {
		fn list_processes(&mut self) -> Result<Vec<ProcessInfo>, ListError> {
			self.calls.set(self.calls.get() + 1);
			if let Some(ref e) = self.err {
				return Err(ListError::Protocol(e.clone()));
			}
			Ok(self.procs.clone())
		}
	}

	fn sample() -> Vec<ProcessInfo> {
		vec![
			ProcessInfo {
				id: "id-prod-api".into(),
				name: "api".into(),
				namespace: "prod".into(),
				..sample_empty()
			},
			ProcessInfo {
				id: "id-prod-worker".into(),
				name: "worker".into(),
				namespace: "prod".into(),
				..sample_empty()
			},
			ProcessInfo {
				id: "id-dev-api".into(),
				name: "api".into(),
				namespace: "dev".into(),
				..sample_empty()
			},
			ProcessInfo {
				id: "id-default-cron".into(),
				name: "cron".into(),
				namespace: String::new(), // empty → default
				..sample_empty()
			},
		]
	}

	fn sample_empty() -> ProcessInfo {
		ProcessInfo {
			id: String::new(),
			name: String::new(),
			namespace: String::new(),
			version: String::new(),
			mode: String::new(),
			pid: 0,
			uptime: 0,
			restarts: 0,
			state: crate::types::ProcessState::Running,
			cpu: 0.0,
			memory: 0,
			user: String::new(),
			watch: false,
			git_branch: None,
			git_commit: None,
			git_dirty: false,
			created_at: None,
		}
	}

	#[test]
	fn parse_selector_literals_are_not_wildcards() {
		for tok in ["api", "id-abc", "prod:api"] {
			let s = parse_selector(tok);
			assert!(
				!s.all_in_ns && !s.all_procs,
				"literal {tok:?} misclassified: {s:?}"
			);
		}
	}

	#[test]
	fn parse_selector_wildcards() {
		let cases = [
			("*", "", false, true),
			("*:*", "", false, true),
			("prod:*", "prod", true, false),
		];
		for (tok, ns, all_in_ns, all_procs) in cases {
			let s = parse_selector(tok);
			assert_eq!(s.namespace, ns, "{tok:?}: namespace mismatch");
			assert_eq!(s.all_in_ns, all_in_ns, "{tok:?}: all_in_ns mismatch");
			assert_eq!(s.all_procs, all_procs, "{tok:?}: all_procs mismatch");
		}
	}

	#[test]
	fn targets_literal_passthrough_no_ipc() {
		let mut mc = ListMock {
			procs: sample(),
			err: None,
			calls: Cell::new(0),
		};
		let ids = vec!["api".to_string(), "prod:worker".to_string()];
		let out = targets(Some(&mut mc), &ids, "").expect("targets");
		assert_eq!(mc.calls.get(), 0, "literal-only must not call IPC");
		assert_eq!(out.join(","), "api,prod:worker");
	}

	#[test]
	fn targets_namespace_wildcard_expands_via_ipc() {
		let mut mc = ListMock {
			procs: sample(),
			err: None,
			calls: Cell::new(0),
		};
		let ids = vec!["prod:*".to_string()];
		let out = targets(Some(&mut mc), &ids, "").expect("targets");
		assert_eq!(out.join(","), "id-prod-api,id-prod-worker");
	}

	#[test]
	fn targets_all_procs_wildcard() {
		let mut mc = ListMock {
			procs: sample(),
			err: None,
			calls: Cell::new(0),
		};
		let ids = vec!["*".to_string()];
		let out = targets(Some(&mut mc), &ids, "").expect("targets");
		assert_eq!(out.len(), 4, "expected all 4 procs, got {out:?}");
	}

	#[test]
	fn targets_all_procs_wildcard_empty_cluster_errors() {
		let mut mc = ListMock {
			procs: vec![],
			err: None,
			calls: Cell::new(0),
		};
		let ids = vec!["*".to_string()];
		let err = targets(Some(&mut mc), &ids, "").expect_err("empty cluster must error");
		assert!(
			matches!(err, ExpandError::Other(_)),
			"expected Other error, got {err:?}"
		);
	}

	#[test]
	fn targets_default_namespace_matches_empty_spec() {
		let mut mc = ListMock {
			procs: sample(),
			err: None,
			calls: Cell::new(0),
		};
		let ids = vec!["default:*".to_string()];
		let out = targets(Some(&mut mc), &ids, "").expect("targets");
		assert_eq!(out.join(","), "id-default-cron");
	}

	#[test]
	fn targets_namespace_flag() {
		let mut mc = ListMock {
			procs: sample(),
			err: None,
			calls: Cell::new(0),
		};
		let out = targets(Some(&mut mc), &[], "prod").expect("targets");
		assert_eq!(out.join(","), "id-prod-api,id-prod-worker");
	}

	#[test]
	fn targets_namespace_flag_rejects_mix_with_positional() {
		let mut mc = ListMock {
			procs: sample(),
			err: None,
			calls: Cell::new(0),
		};
		let ids = vec!["api".to_string()];
		let err = targets(Some(&mut mc), &ids, "prod").expect_err("must reject mix");
		let ExpandError::Usage(usage) = err else {
			panic!("expected UsageError, got {err:?}");
		};
		assert_eq!(
			usage.message,
			"cannot combine --namespace with positional targets — use one or the other"
		);
	}

	#[test]
	fn targets_empty_namespace_errors() {
		let mut mc = ListMock {
			procs: sample(),
			err: None,
			calls: Cell::new(0),
		};
		let ids = vec!["ghost:*".to_string()];
		let err = targets(Some(&mut mc), &ids, "").expect_err("must error");
		let msg = err.to_string();
		assert!(
			msg.contains("\"ghost\""),
			"expected quoted namespace in error, got {msg:?}"
		);
	}

	#[test]
	fn targets_dedupes_across_selectors() {
		let mut mc = ListMock {
			procs: sample(),
			err: None,
			calls: Cell::new(0),
		};
		let ids = vec!["prod:*".to_string(), "id-prod-api".to_string()];
		let out = targets(Some(&mut mc), &ids, "").expect("targets");
		let count = out.iter().filter(|id| *id == "id-prod-api").count();
		assert_eq!(
			count, 1,
			"id-prod-api must appear exactly once, got {out:?}"
		);
	}

	#[test]
	fn targets_list_error_propagates_with_prefix() {
		let mut mc = ListMock {
			procs: vec![],
			err: Some("connection refused".into()),
			calls: Cell::new(0),
		};
		let ids = vec!["prod:*".to_string()];
		let err = targets(Some(&mut mc), &ids, "").expect_err("must propagate");
		let msg = err.to_string();
		assert!(
			msg.contains("list failed"),
			"expected 'list failed' prefix, got {msg:?}"
		);
	}

	#[test]
	fn targets_nil_client_rejected_when_wildcard() {
		let ids = vec!["prod:*".to_string()];
		let err = targets::<ListMock>(None, &ids, "").expect_err("must error");
		assert!(
			matches!(err, ExpandError::Other(_)),
			"expected Other error, got {err:?}"
		);
	}

	#[test]
	fn targets_nil_client_ok_for_literal() {
		let ids = vec!["api".to_string()];
		let out = targets::<ListMock>(None, &ids, "").expect("literal-only must not need client");
		assert_eq!(out.len(), 1);
		assert_eq!(out[0], "api");
	}
}
