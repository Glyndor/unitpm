//! Box-drawing tables sized to the terminal width.
//!
//! 7 cases ported from `internal/cli/table/table_test.go`.
//!
//! Two shapes are supported:
//!
//!   - [`Table`]: a classic column table (headers + rows), auto-wrapping
//!     long cells and shrinking the widest column when the total exceeds
//!     the terminal width.
//!   - [`kv`]: a compact two-column layout with an optional section title,
//!     used by commands like `show` to render AppSpec sections.
//!
//! The terminal width is **injected** by the caller via the writer-bound
//! render functions; the global stdout-locking [`Table::render`] is a
//! thin wrapper that probes the real width. Tests must use the writer form
//! so they do not depend on the runner's terminal — and so they do not
//! drift when a window is resized in CI.

mod wrap;

use std::io::{self, Write};

use crate::term;

use wrap::{visible_len, wrap_text};

/// A printable column table.
#[derive(Debug, Clone)]
pub struct Table {
	headers: Vec<String>,
	rows: Vec<Vec<String>>,
	max_col_widths: Option<Vec<usize>>,
}

impl Table {
	/// Construct a table with the given headers.
	#[must_use]
	pub fn new(headers: &[&str]) -> Self {
		Self {
			headers: headers.iter().map(|s| (*s).to_string()).collect(),
			rows: Vec::new(),
			max_col_widths: None,
		}
	}

	/// Append a row. Length is not checked — the caller is expected to
	/// pass rows whose length matches `headers`.
	pub fn add_row(&mut self, row: &[&str]) {
		self.rows
			.push(row.iter().map(|s| (*s).to_string()).collect());
	}

	/// Configure per-column maximum widths. Columns wider than their max
	/// are wrapped. Length must match the header count, otherwise the
	/// value is silently ignored.
	pub fn set_max_col_widths(&mut self, widths: &[usize]) {
		if widths.len() == self.headers.len() {
			self.max_col_widths = Some(widths.to_vec());
		}
	}

	/// Render to stdout. The width comes from the current terminal probe;
	/// tests must use [`Self::render_to`] instead.
	pub fn render(&self) {
		let width = crate::term::get_terminal_width();
		let stdout = io::stdout();
		let mut out = stdout.lock();
		let _ = self.render_to(&mut out, width);
	}

	/// Render to `w` sized for a `width`-column terminal. The width is
	/// caller-injected so tests can pin it; in production it's whatever
	/// [`crate::term::get_terminal_width`] reports.
	pub fn render_to<W: Write>(&self, w: &mut W, width: usize) -> io::Result<()> {
		let widths = self.calculate_widths(width);
		self.print_border(w, "┌", "┬", "┐", &widths)?;
		self.print_row(w, &self.headers, &widths)?;
		self.print_border(w, "├", "┼", "┤", &widths)?;
		for row in &self.rows {
			self.print_row(w, row, &widths)?;
		}
		self.print_border(w, "└", "┴", "┘", &widths)
	}

	fn calculate_widths(&self, term_width: usize) -> Vec<usize> {
		let mut widths: Vec<usize> = self.headers.iter().map(|h| visible_len(h)).collect();
		for row in &self.rows {
			for (i, cell) in row.iter().enumerate() {
				if i >= widths.len() {
					break;
				}
				let l = visible_len(cell);
				if l > widths[i] {
					widths[i] = l;
				}
			}
		}
		if let Some(max) = self.max_col_widths.as_ref() {
			if max.len() == widths.len() {
				for (i, &m) in max.iter().enumerate() {
					if widths[i] > m {
						widths[i] = m;
					}
				}
			}
		}

		// Shrink the widest column one cell at a time until the total
		// fits. `minColWidth = 3` is the Go constant.
		const MIN_COL_WIDTH: usize = 3;
		loop {
			let total_width: usize = 1 + widths.iter().map(|w| *w + 3).sum::<usize>();
			if total_width <= term_width {
				break;
			}
			let mut widest_idx: Option<usize> = None;
			for (i, &w) in widths.iter().enumerate() {
				if w <= MIN_COL_WIDTH {
					continue;
				}
				match widest_idx {
					None => widest_idx = Some(i),
					Some(j) if w > widths[j] => widest_idx = Some(i),
					_ => {}
				}
			}
			if let Some(i) = widest_idx {
				widths[i] -= 1;
			} else {
				break;
			}
		}
		widths
	}

	fn print_border<W: Write>(
		&self,
		w: &mut W,
		left: &str,
		mid: &str,
		right: &str,
		widths: &[usize],
	) -> io::Result<()> {
		write!(w, "{}", term::dim(format_args!("{}", left)))?;
		for (i, &cell_w) in widths.iter().enumerate() {
			write!(
				w,
				"{}",
				term::dim(format_args!("{}", "─".repeat(cell_w + 2)))
			)?;
			if i < widths.len() - 1 {
				write!(w, "{}", term::dim(format_args!("{}", mid)))?;
			}
		}
		writeln!(w, "{}", term::dim(format_args!("{}", right)))
	}

	fn print_row<W: Write>(&self, w: &mut W, row: &[String], widths: &[usize]) -> io::Result<()> {
		let cell_lines: Vec<Vec<String>> = row
			.iter()
			.enumerate()
			.map(|(i, cell)| {
				let width = widths.get(i).copied().unwrap_or(0);
				wrap_text(cell, width)
			})
			.collect();
		let max_lines = cell_lines.iter().map(Vec::len).max().unwrap_or(1);

		for line_idx in 0..max_lines {
			write!(w, "{}", term::dim(format_args!("{}", "│")))?;
			for (i, lines) in cell_lines.iter().enumerate() {
				let cell = lines.get(line_idx).cloned().unwrap_or_default();
				let vis = visible_len(&cell);
				let pad = widths.get(i).copied().unwrap_or(0).saturating_sub(vis);
				write!(w, " {cell}{}", " ".repeat(pad))?;
				write!(w, " {}", term::dim(format_args!("{}", "│")))?;
			}
			writeln!(w)?;
		}
		Ok(())
	}
}

/// `[title, value]` pair for use with [`kv`]. Rows with an empty value
/// are dropped before rendering so the caller can supply optional fields
/// unconditionally.
pub type KvRow = [String; 2];

/// Print a 2-column table with the given section title printed above.
/// Empty values are dropped before rendering so callers can supply
/// optional fields unconditionally.
pub fn kv<W: Write>(w: &mut W, title: &str, rows: &[KvRow]) -> io::Result<()> {
	let mut filtered: Vec<&KvRow> = Vec::new();
	for r in rows {
		if !r[1].is_empty() {
			filtered.push(r);
		}
	}
	if filtered.is_empty() {
		return Ok(());
	}
	if !title.is_empty() {
		writeln!(w, "{}", term::bold(format_args!("{}", title)))?;
	}
	let header_field = term::cyan(format_args!("{}", term::bold(format_args!("{}", "field"))));
	let header_value = term::cyan(format_args!("{}", term::bold(format_args!("{}", "value"))));
	let mut t = Table::new(&[&header_field, &header_value]);
	for r in &filtered {
		t.add_row(&[r[0].as_str(), r[1].as_str()]);
	}
	let width = crate::term::get_terminal_width();
	t.render_to(w, width)
}

/// Convenience wrapper: lock stdout and call [`kv`].
pub fn kv_stdout(title: &str, rows: &[KvRow]) -> io::Result<()> {
	let stdout = io::stdout();
	let mut out = stdout.lock();
	kv(&mut out, title, rows)
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::cli::format::strip_ansi;

	#[test]
	fn table_render_basic_contains_headers_rows_and_borders() {
		let mut t = Table::new(&["id", "name", "state"]);
		t.add_row(&["abc12345", "api", "running"]);
		t.add_row(&["def67890", "worker", "stopped"]);
		let mut buf = Vec::new();
		t.render_to(&mut buf, 120).expect("render");
		let plain = strip_ansi(&String::from_utf8(buf).expect("utf8"));

		for want in [
			"id", "name", "state", "abc12345", "api", "running", "worker", "stopped",
		] {
			assert!(plain.contains(want), "missing {want:?} in output:\n{plain}");
		}
		assert!(
			plain.contains("┌") && plain.contains("└"),
			"expected box-drawing beam borders, got:\n{plain}"
		);
	}

	#[test]
	fn kv_with_all_empty_rows_writes_nothing() {
		let rows = vec![
			["a".to_string(), String::new()],
			["b".to_string(), String::new()],
		];
		let mut buf = Vec::new();
		kv(&mut buf, "Hidden", &rows).expect("kv");
		assert!(
			buf.is_empty(),
			"all-empty KV must render nothing, got {:?}",
			String::from_utf8_lossy(&buf)
		);
	}

	#[test]
	fn kv_drops_empty_rows_and_renders_others() {
		let rows = vec![
			["state".to_string(), "running".to_string()],
			["pid".to_string(), "1234".to_string()],
			["omitted".to_string(), String::new()], // dropped
		];
		let mut buf = Vec::new();
		kv(&mut buf, "Process", &rows).expect("kv");
		let plain = strip_ansi(&String::from_utf8(buf).expect("utf8"));
		for want in ["Process", "state", "running", "pid", "1234"] {
			assert!(plain.contains(want), "missing {want:?} in output:\n{plain}");
		}
		assert!(
			!plain.contains("omitted"),
			"empty row leaked into KV output:\n{plain}"
		);
	}

	#[test]
	fn set_max_col_widths_silently_ignores_length_mismatch() {
		// Wrong-length slice must not panic; widths are silently ignored
		// and the table falls back to its natural sizing.
		let mut t = Table::new(&["a", "b"]);
		t.set_max_col_widths(&[5, 5, 5]); // 3 widths for 2 headers
		t.add_row(&["1", "2"]);
		let mut buf = Vec::new();
		t.render_to(&mut buf, 120).expect("render despite mismatch");
	}

	#[test]
	fn max_col_widths_wraps_long_content() {
		let mut t = Table::new(&["col"]);
		t.set_max_col_widths(&[5]);
		t.add_row(&["hello world this is long"]);
		let mut buf = Vec::new();
		t.render_to(&mut buf, 120).expect("render");
		let plain = strip_ansi(&String::from_utf8(buf).expect("utf8"));

		assert!(plain.contains("hello"), "expected wrapped 'hello'");
		assert!(plain.contains("world"), "expected wrapped 'world'");

		// With width=120, the lines stay comfortably under 30 cells.
		let max_line_len = plain
			.lines()
			.map(|line| line.chars().count())
			.max()
			.unwrap_or(0);
		assert!(
			max_line_len <= 30,
			"line width {max_line_len} exceeds expected:\n{plain}"
		);
	}

	#[test]
	fn long_word_is_split_into_chunks() {
		let mut t = Table::new(&["col"]);
		t.set_max_col_widths(&[4]);
		t.add_row(&["abcdefghij"]);
		let mut buf = Vec::new();
		t.render_to(&mut buf, 120).expect("render");
		let plain = strip_ansi(&String::from_utf8(buf).expect("utf8"));

		assert!(plain.contains("abcd"), "expected first 4-char chunk");
		assert!(plain.contains("efgh"), "expected second 4-char chunk");
	}

	#[test]
	fn widest_column_shrinks_to_fit_terminal_width() {
		// 80-column terminal, two columns where the second holds a long
		// string. The shrink pass must reduce `wide-column-header` enough
		// that the whole row fits.
		let mut t = Table::new(&["a", "wide-column-header"]);
		let long = "this is some content that should force shrink due to terminal width constraints applied";
		t.add_row(&["x", long]);
		let mut buf = Vec::new();
		t.render_to(&mut buf, 80).expect("render");
		let plain = strip_ansi(&String::from_utf8(buf).expect("utf8"));
		assert!(
			plain.contains("wide-column-header"),
			"missing header after shrink:\n{plain}"
		);
		// No line should exceed 80 cells once the shrink pass has done
		// its job.
		let max_line_len = plain
			.lines()
			.map(|line| line.chars().count())
			.max()
			.unwrap_or(0);
		assert!(
			max_line_len <= 80,
			"line width {max_line_len} exceeds terminal budget:\n{plain}"
		);
	}
}
