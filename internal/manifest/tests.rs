//! Tests ported from the legacy manifest package's test file.
//!
//! Twenty-four cases in scope. The behaviour under test is preserved — only
//! the syntax changes. Test functions follow the Rust snake_case convention
//! rather than the Go CamelCase; the case identity is what matters.
//!
//! Gated to Linux to match the `//go:build linux` on the original file. The
//! manifest itself is platform-neutral, but this phase's brief is the
//! Linux-only rewrite and the wider CI runs Linux.

use std::io::Cursor;

use super::{parse, File, LogsConfig, RestartConfig};
use crate::manifest::convert::ToAppSpecs;
use crate::manifest::ManifestError;

const MINIMAL_YAML: &str = "\
version: \"1\"
apps:
  - name: myapp
    command: node server.js
";

fn parse_ok(yaml: &str) -> File {
	parse(Cursor::new(yaml.as_bytes())).expect("parse")
}

fn parse_err(yaml: &str) -> Result<File, ManifestError> {
	parse(Cursor::new(yaml.as_bytes()))
}

// --- Parse: shape tests ---------------------------------------------------

#[test]
fn parse_minimal() {
	let f = parse_ok(MINIMAL_YAML);
	assert_eq!(f.apps.len(), 1);
	assert_eq!(f.apps[0].name, "myapp");
}

#[test]
fn parse_entry_app() {
	let yaml = "\
version: \"1\"
apps:
  - name: server
    entry: main.go
    runtime: go run
";
	let f = parse_ok(yaml);
	assert_eq!(f.apps[0].entry, "main.go");
	assert_eq!(f.apps[0].runtime, "go run");
}

#[test]
fn parse_namespace_inherited() {
	let yaml = "\
version: \"1\"
namespace: production
apps:
  - name: api
    command: ./api
";
	let f = parse_ok(yaml);
	assert_eq!(f.namespace, "production");
}

#[test]
fn parse_multiple_apps() {
	let yaml = "\
version: \"1\"
apps:
  - name: app1
    command: cmd1
  - name: app2
    command: cmd2
  - name: app3
    entry: main.py
    runtime: python3
";
	let f = parse_ok(yaml);
	assert_eq!(f.apps.len(), 3);
}

#[test]
fn parse_restart_config() {
	let yaml = "\
version: \"1\"
apps:
  - name: worker
    command: ./worker
    restart:
      policy: on-failure
      max_restarts: 5
      delay_ms: 1000
      backoff: expo
      stop_on_exit: [0, 2]
";
	let f = parse_ok(yaml);
	let r: &RestartConfig = &f.apps[0].restart;
	assert_eq!(r.policy, "on-failure");
	assert_eq!(r.max_restarts, 5);
	assert_eq!(r.delay_ms, 1000);
	assert_eq!(r.backoff, "expo");
	assert_eq!(r.stop_on_exit, vec![0, 2]);
}

#[test]
fn parse_logs_config() {
	let yaml = "\
version: \"1\"
apps:
  - name: svc
    command: ./svc
    logs:
      dir: /var/log/glyndor/myapp
      stdout: out.log
      stderr: err.log
      format: json
      timestamp: rfc3339
";
	let f = parse_ok(yaml);
	let l: &LogsConfig = &f.apps[0].logs;
	assert_eq!(l.dir, "/var/log/glyndor/myapp");
	assert_eq!(l.format, "json");
	assert_eq!(l.timestamp, "rfc3339");
}

#[test]
fn parse_env_vars() {
	let yaml = "\
version: \"1\"
apps:
  - name: api
    command: ./api
    env:
      PORT: \"8080\"
      DEBUG: \"true\"
";
	let f = parse_ok(yaml);
	assert_eq!(f.apps[0].env.get("PORT").map(String::as_str), Some("8080"));
	assert_eq!(f.apps[0].env.get("DEBUG").map(String::as_str), Some("true"));
}

#[test]
fn parse_instances() {
	let yaml = "\
version: \"1\"
apps:
  - name: worker
    command: ./worker
    instances: 3
";
	let f = parse_ok(yaml);
	assert_eq!(f.apps[0].instances, 3);
}

// --- Parse: error cases ---------------------------------------------------

#[test]
fn parse_no_apps() {
	let yaml = "version: \"1\"\napps: []\n";
	let err = parse_err(yaml).expect_err("expected error");
	assert!(matches!(err, ManifestError::NoApps));
}

#[test]
fn parse_empty_name() {
	let yaml = "\
version: \"1\"
apps:
  - name: \"\"
    command: ./cmd
";
	let err = parse_err(yaml).expect_err("expected error");
	assert!(matches!(err, ManifestError::EmptyAppName));
}

#[test]
fn parse_both_command_and_entry() {
	let yaml = "\
version: \"1\"
apps:
  - name: app
    command: ./cmd
    entry: main.go
";
	let err = parse_err(yaml).expect_err("expected error");
	assert!(matches!(err, ManifestError::BothCommandAndEntry(ref n) if n == "app"));
}

#[test]
fn parse_neither_command_nor_entry() {
	let yaml = "\
version: \"1\"
apps:
  - name: app
    cwd: /tmp
";
	let err = parse_err(yaml).expect_err("expected error");
	assert!(matches!(err, ManifestError::NeitherCommandNorEntry(ref n) if n == "app"));
}

#[test]
fn parse_invalid_yaml() {
	let err = parse_err("{ invalid yaml:").expect_err("expected error");
	assert!(matches!(err, ManifestError::Yaml(_)));
}

// --- ToAppSpecs tests -----------------------------------------------------

fn check_first(specs: &[crate::ipc::protocol::AppSpec]) -> &crate::ipc::protocol::AppSpec {
	assert!(!specs.is_empty(), "expected at least one spec");
	&specs[0]
}

#[test]
fn to_app_specs_single_app() {
	let f = parse_ok(MINIMAL_YAML);
	let specs = f.to_app_specs().expect("to_app_specs");
	assert_eq!(specs.len(), 1);
	let s = check_first(&specs);
	assert_eq!(s.name, "myapp");
	assert_eq!(s.exec.kind, "command");
	assert_eq!(s.exec.command.as_deref(), Some("node"));
}

#[test]
fn to_app_specs_quoted_command() {
	let yaml = "\
version: \"1\"
apps:
  - name: app
    command: node --eval \"console.log('hi')\"
";
	let f = parse_ok(yaml);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	assert_eq!(s.exec.command.as_deref(), Some("node"));
	let args = s.exec.args.as_ref().expect("args");
	assert_eq!(args.len(), 2);
	assert_eq!(args[0], "--eval");
	assert_eq!(args[1], "console.log('hi')");
}

#[test]
fn to_app_specs_multiple_instances() {
	let yaml = "\
version: \"1\"
apps:
  - name: worker
    command: ./worker
    instances: 3
";
	let f = parse_ok(yaml);
	let specs = f.to_app_specs().expect("to_app_specs");
	assert_eq!(specs.len(), 3);
}

#[test]
fn to_app_specs_default_namespace() {
	let f = parse_ok(MINIMAL_YAML);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	assert_eq!(s.namespace.as_deref(), Some("default"));
}

#[test]
fn to_app_specs_inherited_namespace() {
	let yaml = "\
version: \"1\"
namespace: staging
apps:
  - name: api
    command: ./api
";
	let f = parse_ok(yaml);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	assert_eq!(s.namespace.as_deref(), Some("staging"));
}

#[test]
fn to_app_specs_app_overrides_namespace() {
	let yaml = "\
version: \"1\"
namespace: global
apps:
  - name: api
    namespace: local
    command: ./api
";
	let f = parse_ok(yaml);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	assert_eq!(s.namespace.as_deref(), Some("local"));
}

#[test]
fn to_app_specs_entry_type() {
	let yaml = "\
version: \"1\"
apps:
  - name: pyapp
    entry: app.py
    runtime: python3
";
	let f = parse_ok(yaml);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	assert_eq!(s.exec.kind, "entry");
	assert_eq!(s.exec.entry.as_deref(), Some("app.py"));
}

#[test]
fn to_app_specs_command_args() {
	let yaml = "\
version: \"1\"
apps:
  - name: server
    command: node server.js --port 8080
";
	let f = parse_ok(yaml);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	assert_eq!(s.exec.command.as_deref(), Some("node"));
	let args = s.exec.args.as_ref().expect("args");
	// "node server.js --port 8080" → command="node", args=["server.js", "--port", "8080"]
	assert_eq!(args.len(), 3, "args = {args:?}");
}

#[test]
fn to_app_specs_restart_built() {
	let yaml = "\
version: \"1\"
apps:
  - name: svc
    command: ./svc
    restart:
      policy: always
";
	let f = parse_ok(yaml);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	let r = s.restart.as_ref().expect("restart");
	assert_eq!(r.policy, "always");
}

#[test]
fn to_app_specs_no_restart_config() {
	let f = parse_ok(MINIMAL_YAML);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	assert!(
		s.restart.is_none(),
		"restart should be nil when no restart config provided"
	);
}

#[test]
fn to_app_specs_logs_built() {
	let yaml = "\
version: \"1\"
apps:
  - name: svc
    command: ./svc
    logs:
      dir: /tmp/logs
";
	let f = parse_ok(yaml);
	let specs = f.to_app_specs().expect("to_app_specs");
	let s = check_first(&specs);
	let l = s.logs.as_ref().expect("logs");
	assert_eq!(l.dir.as_deref(), Some("/tmp/logs"));
}
