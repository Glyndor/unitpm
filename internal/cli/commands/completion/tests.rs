//! Tests for the completion command.
//!
//! 8 cases ported from `internal/cli/commands/completion/cmd_test.go`.

use crate::cli::commands::completion;
use crate::cli::help::CommandSpec;
use crate::cli::registry;

fn lock_term() -> crate::term::tests::TermGuard {
	crate::term::tests::lock_term()
}

/// Insert predictable test commands into the registry so the
/// completion-script assertions don't depend on whatever else got
/// registered earlier in the test session.
fn install_test_commands() {
	let mut s1 = CommandSpec {
		name: "__test_apply__".into(),
		aliases: Vec::new(),
		usage: String::new(),
		description: String::new(),
		options: Vec::new(),
		examples: Vec::new(),
		hidden: false,
	};
	let mut s2 = CommandSpec {
		name: "__test_stop__".into(),
		aliases: Vec::new(),
		usage: String::new(),
		description: String::new(),
		options: Vec::new(),
		examples: Vec::new(),
		hidden: false,
	};
	let mut s3 = CommandSpec {
		name: "__test_hidden__".into(),
		aliases: Vec::new(),
		usage: String::new(),
		description: String::new(),
		options: Vec::new(),
		examples: Vec::new(),
		hidden: true,
	};
	registry::register(s1.clone());
	registry::register(s2.clone());
	registry::register(s3.clone());
	// Keep the unused-mut warnings away.
	let _ = (&mut s1, &mut s2, &mut s3);
}

#[test]
fn run_help_does_not_panic() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = completion::run(&mut buf, &["--help".to_string()]);
	rc.expect("ok");
}

#[test]
fn run_missing_shell_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = completion::run(&mut buf, &[]);
	let err = rc.expect_err("missing shell");
	assert!(
		err.to_string().contains("usage:"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_unsupported_shell_errors() {
	let _g = lock_term();
	let mut buf = Vec::new();
	let rc = completion::run(&mut buf, &["tcsh".to_string()]);
	let err = rc.expect_err("unsupported shell");
	assert!(
		err.to_string().contains("unsupported shell"),
		"unexpected error: {err}"
	);
}

#[test]
fn run_bash_includes_visible_and_excludes_hidden() {
	let _g = lock_term();
	install_test_commands();

	let mut buf = Vec::new();
	completion::run(&mut buf, &["bash".to_string()]).expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	for want in ["_unitpm_completions", "complete -F", "__test_stop__"] {
		assert!(out.contains(want), "bash script missing {want:?}");
	}
	assert!(
		!out.contains("__test_hidden__"),
		"bash script leaked hidden command"
	);
}

#[test]
fn run_zsh_compdef_and_visible_present() {
	let _g = lock_term();
	install_test_commands();

	let mut buf = Vec::new();
	completion::run(&mut buf, &["zsh".to_string()]).expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	assert!(
		out.contains("#compdef unitpm"),
		"zsh missing #compdef directive"
	);
	assert!(out.contains("__test_apply__"), "zsh missing visible apply");
}

#[test]
fn run_fish_static_markers_present() {
	let _g = lock_term();
	install_test_commands();

	let mut buf = Vec::new();
	completion::run(&mut buf, &["fish".to_string()]).expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	for want in ["__unitpm_list_names", "unitpm list --long", "completion"] {
		assert!(out.contains(want), "fish script missing {want:?}");
	}
}

#[test]
fn run_bash_hides_internals() {
	let _g = lock_term();
	let mut buf = Vec::new();
	completion::run(&mut buf, &["bash".to_string()]).expect("ok");
	let out = String::from_utf8(buf).expect("utf8");
	for bad in ["_exec-env", "_exec-sandbox"] {
		assert!(
			!out.contains(bad),
			"bash script leaked hidden command {bad:?}"
		);
	}
}

#[test]
fn get_spec_matches_name() {
	let s = completion::spec();
	assert_eq!(s.name, "completion");
}
