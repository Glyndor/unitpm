//! Central command registry for the CLI.
//!
//! 2 cases ported from `internal/cli/registry/registry_test.go`.

use std::collections::HashMap;
use std::sync::{Mutex, OnceLock};

use crate::cli::help::CommandSpec;

/// Process-global table of registered commands. Mutated by [`register`] and
/// read by [`get_all`] / [`resolve`]. Tests cannot reset this without
/// calling [`register`] with a fresh name, which is what the Go tests do
/// anyway — they register a stub and assert it appears in `GetAll`.
static SPECS: OnceLock<Mutex<HashMap<String, CommandSpec>>> = OnceLock::new();
static ALIASES: OnceLock<Mutex<HashMap<String, String>>> = OnceLock::new();

fn specs() -> &'static Mutex<HashMap<String, CommandSpec>> {
	SPECS.get_or_init(|| Mutex::new(HashMap::new()))
}

fn aliases() -> &'static Mutex<HashMap<String, String>> {
	ALIASES.get_or_init(|| Mutex::new(HashMap::new()))
}

/// Register `spec` under its canonical name plus every alias. A second
/// registration of the same canonical name overwrites the prior entry —
/// matches the Go behavior, which the tests already rely on for
/// idempotency.
pub fn register(spec: CommandSpec) {
	let norm = normalize(&spec.name);
	let mut specs = specs().lock().expect("specs poisoned");
	let mut aliases = aliases().lock().expect("aliases poisoned");
	specs.insert(norm.clone(), spec.clone());
	for alias in &spec.aliases {
		aliases.insert(normalize(alias), norm.clone());
	}
}

/// Return every registered spec, sorted by canonical name.
#[must_use]
pub fn get_all() -> Vec<CommandSpec> {
	let specs = specs().lock().expect("specs poisoned");
	let mut out: Vec<CommandSpec> = specs.values().cloned().collect();
	out.sort_by(|a, b| a.name.cmp(&b.name));
	out
}

/// Resolve a name or alias to the canonical command name. Returns
/// `(name, true)` on a hit and `("", false)` on a miss.
#[must_use]
pub fn resolve(name: &str) -> (String, bool) {
	let norm = normalize(name);
	let specs = specs().lock().expect("specs poisoned");
	if let Some(spec) = specs.get(&norm) {
		return (spec.name.clone(), true);
	}
	let aliases = aliases().lock().expect("aliases poisoned");
	if let Some(canonical) = aliases.get(&norm) {
		if let Some(spec) = specs.get(canonical) {
			return (spec.name.clone(), true);
		}
	}
	(String::new(), false)
}

fn normalize(s: &str) -> String {
	s.trim().to_lowercase()
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::cli::help::CommandSpec;

	const TEST_CMD: &str = "test-cmd-6a";

	fn test_spec() -> CommandSpec {
		CommandSpec {
			name: TEST_CMD.to_string(),
			aliases: vec!["tc6a".to_string(), "tcmd6a".to_string()],
			usage: String::new(),
			description: String::new(),
			options: vec![],
			examples: vec![],
			hidden: false,
		}
	}

	#[test]
	fn register_and_resolve() {
		register(test_spec());

		// Canonical.
		let (name, ok) = resolve(TEST_CMD);
		assert!(ok, "resolve(canonical) should hit");
		assert_eq!(name, TEST_CMD, "resolve(canonical) returns canonical");

		// Alias.
		let (name, ok) = resolve("tc6a");
		assert!(ok, "resolve(alias) should hit");
		assert_eq!(name, TEST_CMD, "resolve(alias) returns canonical");

		// Case-insensitive — the Go test asserts Resolve("TC") works.
		let (name, ok) = resolve("TC6A");
		assert!(ok, "resolve(uppercase) should hit");
		assert_eq!(name, TEST_CMD, "resolve(uppercase) returns canonical");
	}

	#[test]
	fn get_all_includes_registered() {
		register(test_spec());

		let all = get_all();
		assert!(
			all.iter().any(|s| s.name == TEST_CMD),
			"GetAll() missing registered command {TEST_CMD:?}"
		);
	}
}
