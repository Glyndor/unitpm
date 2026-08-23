//! Tests for the `show` command. Mirrors the test cases that landed
//! inside `internal/cli/commands/show/cmd_test.go`.

use std::collections::BTreeMap;

use crate::ipc::protocol::{AppResources, AppRestart, AppStop, AppWatch, RunAsPolicy};

use super::super::tests_helpers::{empty_info, spec_with_logs};
use super::super::{empty_spec, parse_args, spec};
use super::{
	bool_dimmed, color_state, cpu_or_unlimited, git_str, int_or_dash, int_or_unlimited, join_args,
	join_log_path, mask_secret, mem_or_unlimited, non_empty, pid_str, render_env, render_exec,
	render_isolation, render_logs, render_process, render_resources, render_restart,
	render_schedule, render_stop, render_watch, watch_str,
};
use crate::cli::format;
use crate::ipc::protocol::AppSpec;
use crate::types::ProcessInfo;

#[test]
fn parse_args_missing_target() {
	let opts = parse_args(&[]).unwrap();
	assert!(opts.target.is_none());
}

#[test]
fn parse_args_target_set() {
	let opts = parse_args(&["abc-123".into()]).unwrap();
	assert_eq!(opts.target.as_deref(), Some("abc-123"));
}

#[test]
fn parse_args_json_flag() {
	let opts = parse_args(&["--json".into(), "abc".into()]).unwrap();
	assert!(opts.json);
	assert_eq!(opts.target.as_deref(), Some("abc"));
}

#[test]
fn spec_name_and_aliases() {
	let s = spec();
	assert_eq!(s.name, "show");
	assert!(s.aliases.contains(&"info".to_string()));
	assert!(s.aliases.contains(&"describe".to_string()));
}

#[test]
fn help_renders_without_panic() {
	let mut buf = Vec::new();
	super::super::render_help(&mut buf).unwrap();
	let plain = format::strip_ansi(&String::from_utf8(buf).unwrap());
	assert!(plain.contains("Usage:"));
	assert!(plain.contains("--json"));
}

#[test]
fn color_state_categorises() {
	assert!(color_state(crate::types::ProcessState::Running).contains("running"));
	assert!(color_state(crate::types::ProcessState::Online).contains("online"));
	assert!(color_state(crate::types::ProcessState::Stopped).contains("stopped"));
	assert!(color_state(crate::types::ProcessState::Failed).contains("failed"));
	assert!(color_state(crate::types::ProcessState::Restarting).contains("restarting"));
	assert!(color_state(crate::types::ProcessState::Exited).contains("exited"));
}

#[test]
fn pid_str_formats() {
	assert!(pid_str(0).contains("-"));
	assert_eq!(pid_str(42), "42");
}

#[test]
fn git_str_branch_commit_dirty() {
	let plain = git_str(&ProcessInfo {
		git_branch: Some("main".into()),
		git_commit: Some("abc".into()),
		..empty_info()
	});
	assert!(plain.contains("main"));
	assert!(plain.contains("abc"));

	let dirty = git_str(&ProcessInfo {
		git_branch: Some("main".into()),
		git_commit: Some("abc".into()),
		git_dirty: true,
		..empty_info()
	});
	assert!(dirty.contains('*'));

	let empty = git_str(&empty_info());
	assert!(empty.contains('-'));
}

#[test]
fn watch_str_formats() {
	assert!(watch_str(true).contains("enabled"));
	assert!(watch_str(false).contains("disabled"));
}

#[test]
fn bool_dimmed_formats() {
	assert!(bool_dimmed(true).contains("true"));
	assert!(bool_dimmed(false).contains("false"));
}

#[test]
fn join_args_quotes_whitespace() {
	assert_eq!(join_args(&[]), "");
	assert_eq!(join_args(&["a".into(), "b".into()]), "a b");
	assert_eq!(join_args(&["a b".into(), "c".into()]), "\"a b\" c");
}

#[test]
fn join_log_path_resolves_relative() {
	assert_eq!(join_log_path("", ""), "");
	assert_eq!(join_log_path("/var/log", ""), "");
	assert_eq!(join_log_path("", "stdout.log"), "stdout.log");
	assert_eq!(join_log_path("/var/log", "/etc/abs.log"), "/etc/abs.log");
	assert_eq!(
		join_log_path("/var/log", "stdout.log"),
		"/var/log/stdout.log"
	);
}

#[test]
fn int_or_dash_formats() {
	assert!(int_or_dash(0).contains('-'));
	assert_eq!(int_or_dash(5), "5");
}

#[test]
fn int_or_unlimited_formats() {
	assert!(int_or_unlimited(0).contains("unlimited"));
	assert_eq!(int_or_unlimited(7), "7");
}

#[test]
fn mem_or_unlimited_formats() {
	assert!(mem_or_unlimited(0).contains("unlimited"));
	let s = mem_or_unlimited(2 * 1024 * 1024);
	assert!(!s.is_empty());
}

#[test]
fn cpu_or_unlimited_formats() {
	assert!(cpu_or_unlimited(0).contains("unlimited"));
	assert!(cpu_or_unlimited(150).contains("150%"));
}

#[test]
fn non_empty_picks_first() {
	assert_eq!(non_empty("", "b"), "b");
	assert_eq!(non_empty("a", "b"), "a");
}

#[test]
fn mask_secret_hides_sensitive() {
	assert!(mask_secret("API_TOKEN", "abc").contains('*'));
	assert_eq!(mask_secret("PORT", ""), "");
	assert_eq!(mask_secret("PORT", "8080"), "8080");
	for k in ["PASSWORD", "PASSWD", "MY_KEY", "CREDENTIALS", "PRIVATE_KEY"] {
		assert!(mask_secret(k, "v").contains('*'));
	}
}

#[test]
fn render_restart_full_and_nil() {
	let spec = AppSpec {
		restart: Some(Box::new(AppRestart {
			policy: "always".into(),
			max_retries: Some(3),
			backoff_ms: Some(1000),
			backoff_type: Some("expo".into()),
			stop_on_exit: Some(vec![0, 2]),
		})),
		..empty_spec()
	};
	let mut buf = Vec::new();
	render_restart(&mut buf, &spec);
	render_restart(&mut buf, &empty_spec());
}

#[test]
fn render_env_full_and_nil() {
	let mut env = BTreeMap::new();
	env.insert("FOO".into(), "bar".into());
	env.insert("API_TOKEN".into(), "xyz".into());
	let mut spec = empty_spec();
	spec.env_file = Some("/tmp/env".into());
	spec.env = Some(env);
	let mut buf = Vec::new();
	render_env(&mut buf, &spec);
	render_env(&mut buf, &empty_spec());
}

#[test]
fn render_logs_present_and_absent() {
	let mut buf = Vec::new();
	render_logs(&mut buf, &spec_with_logs());
	render_logs(&mut buf, &empty_spec());
}

#[test]
fn render_resources_present_and_absent() {
	let mut buf = Vec::new();
	render_resources(
		&mut buf,
		&AppSpec {
			resources: Some(Box::new(AppResources {
				memory_max_bytes: Some(512 * 1024 * 1024),
				cpu_max_percent: Some(200),
				tasks_max: Some(100),
			})),
			..empty_spec()
		},
	);
	render_resources(
		&mut buf,
		&AppSpec {
			resources: Some(Box::new(AppResources {
				memory_max_bytes: None,
				cpu_max_percent: None,
				tasks_max: None,
			})),
			..empty_spec()
		},
	);
	render_resources(&mut buf, &empty_spec());
}

#[test]
fn render_stop_present_and_absent() {
	let mut buf = Vec::new();
	render_stop(
		&mut buf,
		&AppSpec {
			stop: Some(Box::new(AppStop {
				signal: Some("SIGTERM".into()),
				timeout_ms: Some(1000),
			})),
			..empty_spec()
		},
	);
	render_stop(&mut buf, &empty_spec());
}

#[test]
fn render_isolation_present_and_absent() {
	let mut buf = Vec::new();
	render_isolation(
		&mut buf,
		&AppSpec {
			run_as: Some(Box::new(RunAsPolicy {
				mode: "self".into(),
			})),
			..empty_spec()
		},
	);
	render_isolation(&mut buf, &empty_spec());
}

#[test]
fn render_schedule_present_and_absent() {
	let mut buf = Vec::new();
	render_schedule(
		&mut buf,
		&AppSpec {
			cron: Some("* * * * *".into()),
			..empty_spec()
		},
	);
	render_schedule(&mut buf, &empty_spec());
}

#[test]
fn render_watch_present_and_absent() {
	let mut buf = Vec::new();
	render_watch(
		&mut buf,
		&AppSpec {
			watch: Some(Box::new(AppWatch {
				enabled: true,
				ignore: Some(vec!["node_modules".into()]),
			})),
			..empty_spec()
		},
	);
	render_watch(
		&mut buf,
		&AppSpec {
			watch: Some(Box::new(AppWatch {
				enabled: false,
				ignore: Some(vec![]),
			})),
			..empty_spec()
		},
	);
	render_watch(&mut buf, &empty_spec());
}

#[test]
fn render_exec_uses_entry() {
	let mut spec = empty_spec();
	spec.exec.kind = "entry".into();
	spec.exec.entry = Some("npm:start".into());
	let mut buf = Vec::new();
	render_exec(&mut buf, &spec);
}

#[test]
fn render_process_uses_info_namespace_fallback() {
	let mut info = empty_info();
	info.namespace = String::new();
	let mut spec = empty_spec();
	spec.namespace = Some("prod".into());
	let mut buf = Vec::new();
	render_process(&mut buf, &info, &spec);
}
