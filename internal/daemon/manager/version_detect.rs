//! Project version detector.
//!
//! Mirrors `internal/daemon/manager/version_detect.go`. The detector reads
//! one of five common manifest files at the root of an application's working
//! directory and pulls out the `version` field. We keep the same priority
//! order (`package.json` first, `Cargo.toml` second, ...) so behaviour matches
//! the Go implementation exactly; a regression that swaps two sources is the
//! kind of thing a unit test catches and an operator would not.
//!
//! The parsers are deliberately line-scoped matches rather than full
//! JSON/TOML decoders — the original implementation picked the cheapest
//! approach that still handles the common shapes, and the cost of swapping
//! in `serde_json` / `toml` is not worth the dependencies for one field.

use std::fs;
use std::path::Path;

/// A single detector — file name and a one-shot extractor.
struct Detector {
	file: &'static str,
	extract: fn(&[u8]) -> String,
}

/// Walk the lines and find one whose first field is `key = "value"`. Used
/// for `Cargo.toml`, `pyproject.toml`, and similar TOML manifests.
fn extract_toml_value(data: &[u8], key: &str) -> String {
	for line in data.split(|b| *b == b'\n') {
		let line = strip_ascii_space_left(line);
		let Some(after) = line.strip_prefix(key.as_bytes()) else {
			continue;
		};
		let after = strip_ascii_space_left(after);
		let Some(after) = after.strip_prefix(b"=") else {
			continue;
		};
		let after = strip_ascii_space_left(after);
		// Quoted: strip surrounding double quotes if present.
		let bytes = strip_ascii_space_right(after);
		if let Some(inner) = bytes
			.strip_prefix(b"\"")
			.and_then(|b| b.strip_suffix(b"\""))
		{
			return std::str::from_utf8(inner).unwrap_or("").trim().to_string();
		}
		// Unquoted (INI-style): take the whole rest of the line.
		if !bytes.is_empty() {
			return std::str::from_utf8(bytes).unwrap_or("").trim().to_string();
		}
	}
	String::new()
}

/// Walk the data looking for `"version": "..."`. Used for `package.json`.
fn extract_json_value(data: &[u8], key: &str) -> String {
	let needle = format!("\"{key}\"");
	let bytes = data;
	let mut i = 0;
	while i + needle.len() < bytes.len() {
		if &bytes[i..i + needle.len()] == needle.as_bytes() {
			let mut j = i + needle.len();
			// Skip whitespace and the colon.
			while j < bytes.len() && (bytes[j] == b' ' || bytes[j] == b'\t') {
				j += 1;
			}
			if j >= bytes.len() || bytes[j] != b':' {
				i += 1;
				continue;
			}
			j += 1;
			while j < bytes.len() && (bytes[j] == b' ' || bytes[j] == b'\t') {
				j += 1;
			}
			if j >= bytes.len() || bytes[j] != b'"' {
				return String::new();
			}
			let start = j + 1;
			let mut end = start;
			while end < bytes.len() && bytes[end] != b'"' {
				end += 1;
			}
			if end >= bytes.len() {
				return String::new();
			}
			return std::str::from_utf8(&bytes[start..end])
				.unwrap_or("")
				.trim()
				.to_string();
		}
		i += 1;
	}
	String::new()
}

/// Walk the lines for `version: "..."` (Elixir-style).
fn extract_elixir_value(data: &[u8]) -> String {
	// The version string can appear anywhere on a line in a Mix project
	// (`def project, do: [version: "..."]`, `version: "..."`, or even a
	// `@version "..."` macro). Scan for the literal `version:` token.
	let bytes = data;
	let token = b"version:";
	let mut i = 0;
	while i + token.len() <= bytes.len() {
		if &bytes[i..i + token.len()] == token {
			let mut j = i + token.len();
			// Optional whitespace.
			while j < bytes.len() && (bytes[j] == b' ' || bytes[j] == b'\t') {
				j += 1;
			}
			if j < bytes.len() && bytes[j] == b'"' {
				let start = j + 1;
				let mut end = start;
				while end < bytes.len() && bytes[end] != b'"' {
					end += 1;
				}
				if end < bytes.len() {
					return std::str::from_utf8(&bytes[start..end])
						.unwrap_or("")
						.trim()
						.to_string();
				}
			}
		}
		i += 1;
	}
	String::new()
}

fn extract_package_json(data: &[u8]) -> String {
	extract_json_value(data, "version")
}

fn extract_cargo_toml(data: &[u8]) -> String {
	extract_toml_value(data, "version")
}

fn extract_pyproject_toml(data: &[u8]) -> String {
	extract_toml_value(data, "version")
}

fn extract_setup_cfg(data: &[u8]) -> String {
	extract_toml_value(data, "version")
}

fn extract_mix_exs(data: &[u8]) -> String {
	extract_elixir_value(data)
}

fn strip_ascii_space_left(mut bytes: &[u8]) -> &[u8] {
	while let Some((&first, rest)) = bytes.split_first() {
		if first == b' ' || first == b'\t' || first == b'\r' {
			bytes = rest;
		} else {
			break;
		}
	}
	bytes
}

fn strip_ascii_space_right(mut bytes: &[u8]) -> &[u8] {
	while let Some((last, rest)) = bytes.split_last() {
		if *last == b' ' || *last == b'\t' || *last == b'\r' {
			bytes = rest;
		} else {
			break;
		}
	}
	bytes
}

/// Detector table — order matters: the first file that yields a value wins.
const DETECTORS: &[Detector] = &[
	Detector {
		file: "package.json",
		extract: extract_package_json,
	},
	Detector {
		file: "Cargo.toml",
		extract: extract_cargo_toml,
	},
	Detector {
		file: "pyproject.toml",
		extract: extract_pyproject_toml,
	},
	Detector {
		file: "setup.cfg",
		extract: extract_setup_cfg,
	},
	Detector {
		file: "mix.exs",
		extract: extract_mix_exs,
	},
];

/// Read the version declared in `cwd`. Returns the empty string when the
/// directory is unset, missing, or contains no recognisable manifest.
#[must_use]
pub fn detect_project_version(cwd: &str) -> String {
	if cwd.is_empty() {
		return String::new();
	}
	for d in DETECTORS {
		let path = Path::new(cwd).join(d.file);
		let Ok(data) = fs::read(&path) else { continue };
		let v = (d.extract)(&data);
		if !v.is_empty() {
			return v;
		}
	}
	String::new()
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn package_json() {
		let dir = tempfile::tempdir().expect("tempdir");
		fs::write(
			dir.path().join("package.json"),
			br#"{"name":"app","version":"2.1.0"}"#,
		)
		.unwrap();
		assert_eq!(
			detect_project_version(dir.path().to_str().unwrap()),
			"2.1.0"
		);
	}

	#[test]
	fn cargo_toml() {
		let dir = tempfile::tempdir().expect("tempdir");
		fs::write(
			dir.path().join("Cargo.toml"),
			b"[package]\nname = \"app\"\nversion = \"0.3.5\"\n",
		)
		.unwrap();
		assert_eq!(
			detect_project_version(dir.path().to_str().unwrap()),
			"0.3.5"
		);
	}

	#[test]
	fn pyproject_toml() {
		let dir = tempfile::tempdir().expect("tempdir");
		fs::write(
			dir.path().join("pyproject.toml"),
			b"[project]\nname = \"app\"\nversion = \"1.2.3\"\n",
		)
		.unwrap();
		assert_eq!(
			detect_project_version(dir.path().to_str().unwrap()),
			"1.2.3"
		);
	}

	#[test]
	fn setup_cfg() {
		let dir = tempfile::tempdir().expect("tempdir");
		fs::write(
			dir.path().join("setup.cfg"),
			b"[metadata]\nname = app\nversion = 4.0.0\n",
		)
		.unwrap();
		assert_eq!(
			detect_project_version(dir.path().to_str().unwrap()),
			"4.0.0"
		);
	}

	#[test]
	fn mix_exs() {
		let dir = tempfile::tempdir().expect("tempdir");
		fs::write(
			dir.path().join("mix.exs"),
			b"defmodule App.MixProject do\n  use Mix.Project\n  def project, do: [app: :app, version: \"0.0.1\"]\nend\n",
		)
		.unwrap();
		assert_eq!(
			detect_project_version(dir.path().to_str().unwrap()),
			"0.0.1"
		);
	}

	#[test]
	fn priority_is_package_json_first() {
		let dir = tempfile::tempdir().expect("tempdir");
		fs::write(dir.path().join("package.json"), br#"{"version":"1.0.0"}"#).unwrap();
		fs::write(dir.path().join("Cargo.toml"), b"version = \"2.0.0\"\n").unwrap();
		assert_eq!(
			detect_project_version(dir.path().to_str().unwrap()),
			"1.0.0"
		);
	}

	#[test]
	fn no_files_returns_empty() {
		let dir = tempfile::tempdir().expect("tempdir");
		assert_eq!(detect_project_version(dir.path().to_str().unwrap()), "");
	}

	#[test]
	fn empty_cwd_returns_empty() {
		assert_eq!(detect_project_version(""), "");
	}
}
