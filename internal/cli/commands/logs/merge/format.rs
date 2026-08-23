//! Per-entry rendering and the coloured stream-label helper.
//!
//! `format_entry` is the canonical line form: a coloured `[LABEL]` tag,
//! a dimmed `YYYY-MM-DD HH:MM:SS` (or empty placeholder when the entry
//! is header-less), then the body. Multi-line bodies are emitted
//! verbatim so stack traces stay readable.
//!
//! `color_label` is the same helper the legacy tail uses to colour its
//! own per-line prefix. Kept here so both code paths stay in sync.

use std::io::Write;

use crate::cli::commands::logs::entry::{render_ts, Entry, TS_LEN};
use crate::term;

use super::Filter;

/// Format an entry for terminal output. Multi-line bodies are emitted
/// as-is so stack traces stay readable.
pub fn format_entry(e: &Entry) -> String {
	let ts_str = match e.ts_unix {
		Some(t) => render_ts(Some(t)),
		None => " ".repeat(TS_LEN),
	};
	format!(
		"{} {} {}",
		color_label(&e.label),
		term::dim(format_args!("{}", ts_str)),
		e.body
	)
}

/// Coloured stream-label used as the line prefix in both the merged
/// output and the legacy per-stream tail.
pub fn color_label(label: &str) -> String {
	match label {
		"STDOUT" => term::cyan(format_args!("[STDOUT]")),
		"STDERR" => term::red(format_args!("[STDERR]")),
		other => term::dim(format_args!("[{}]", other)),
	}
}

/// Emit `entries` through `w`, applying `fs`.
pub fn format_entries<W: Write>(w: &mut W, entries: &[Entry], fs: &Filter) -> std::io::Result<()> {
	for e in entries {
		if !fs.keep(e) {
			continue;
		}
		writeln!(w, "{}", format_entry(e))?;
	}
	Ok(())
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::cli::commands::logs::entry::Entry as E;

	#[test]
	fn format_entry_no_ts_placeholder() {
		let e = E {
			ts_unix: None,
			label: "STDOUT".into(),
			body: "raw".into(),
			seq: 0,
			has_ts: false,
		};
		let plain = crate::cli::format::strip_ansi(&format_entry(&e));
		assert!(plain.contains(&" ".repeat(TS_LEN)));
		assert!(plain.contains("raw"));
	}

	#[test]
	fn color_label_distinguishes() {
		let a = color_label("STDOUT");
		let b = color_label("STDERR");
		let c = color_label("custom");
		assert_ne!(a, b);
		assert_ne!(a, c);
		assert_ne!(b, c);
	}
}
