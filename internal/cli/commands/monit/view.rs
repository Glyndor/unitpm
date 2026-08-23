//! `monit`'s rendering primitives.
//!
//! Every helper takes an `&mut String` so the renderer composes a single
//! frame in one allocation pass, then the caller writes the buffer to a
//! [`std::io::Write`].

use crate::cli::format;
use crate::term;
use crate::types::ProcessState;

use super::state::MonitState;

/// Height of each CPU/memory sparkline in cells.
pub const GRAPH_HEIGHT: usize = 6;

/// Block-element runes used by the sparklines, lowest → highest density.
pub const BLOCK_RUNES: &[char] = &[' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'];

/// Build the entire frame into `out`. `width` is the column budget
/// (caller-injected so tests can pin it).
pub fn build_frame(out: &mut String, s: &MonitState, width: usize) {
	let w = if width < 40 { 80 } else { width };
	out.push_str("\x1b[H\x1b[2J");
	write_header(out, s, w);
	write_graphs(out, s, w);
	write_details(out, s, w);
	if !s.tree.is_empty() {
		write_process_tree(out, s, w);
	}
	write_footer(out);
}

/// Visible width of `s` ignoring ANSI escapes. The Go version does the
/// same thing via byte-counting; we count UTF-8 leading bytes so multibyte
/// runes don't get split.
pub fn vis_len(s: &str) -> usize {
	let mut n = 0usize;
	let mut in_esc = false;
	for &b in s.as_bytes() {
		if in_esc {
			if b == b'm' {
				in_esc = false;
			}
			continue;
		}
		if b == 0x1b {
			in_esc = true;
			continue;
		}
		if !(0x80..0xC0).contains(&b) {
			n += 1;
		}
	}
	n
}

/// Pad `s` to `inner_width` visible cells.
pub fn pad_to(s: &str, inner_width: usize) -> String {
	let vl = vis_len(s);
	if vl >= inner_width {
		return s.to_string();
	}
	let mut out = String::with_capacity(s.len() + (inner_width - vl));
	out.push_str(s);
	for _ in 0..(inner_width - vl) {
		out.push(' ');
	}
	out
}

/// Top border with a section title baked in.
pub fn border_top(width: usize, title: &str) -> String {
	let inner = width.saturating_sub(2);
	let title_part = format!("─{title}─");
	let rem = inner.saturating_sub(title_part.chars().count());
	let mut out = String::with_capacity(width + title.len());
	out.push('╭');
	out.push_str(&title_part);
	for _ in 0..rem {
		out.push('─');
	}
	out.push('╮');
	out
}

/// Bottom border.
pub fn border_bot(width: usize) -> String {
	let mut out = String::with_capacity(width);
	out.push('╰');
	for _ in 0..width.saturating_sub(2) {
		out.push('─');
	}
	out.push('╯');
	out
}

fn write_header(out: &mut String, s: &MonitState, w: usize) {
	let header_text = format!(
		"  {}  •  {}  •  pid {}  •  {}  •  restarts {}  ",
		term::bold(format_args!("{}", s.info.name)),
		state_str(s.info.state),
		s.info.pid,
		fmt_uptime(s.info.uptime),
		s.info.restarts,
	);
	out.push_str(&border_top(w, " unitpm monit "));
	out.push('\n');
	let line = format!("│{}│\n", pad_to(&header_text, w.saturating_sub(2)));
	out.push_str(&line);
	out.push_str(&border_bot(w));
	out.push('\n');
}

fn write_graphs(out: &mut String, s: &MonitState, w: usize) {
	let left_w = w / 2;
	let right_w = w - left_w;
	let cpu_gw = left_w.saturating_sub(4);
	let mem_gw = right_w.saturating_sub(4);

	let cpu_rows = build_graph(&s.cpu_hist, 100.0, cpu_gw, GRAPH_HEIGHT);
	let mem_max = if s.mem_max == 0 {
		1.0
	} else {
		s.mem_max as f64
	};
	let mem_f: Vec<f64> = s.mem_hist.iter().map(|v| *v as f64).collect();
	let mem_rows = build_graph(&mem_f, mem_max, mem_gw, GRAPH_HEIGHT);

	out.push_str(&border_top(left_w, " CPU "));
	out.push_str(&border_top(right_w, " Memory "));
	out.push('\n');

	let cpu_val = format!("  {:.1}%", s.info.cpu);
	let mem_val = format!(
		"  {} / peak {}",
		fmt_bytes(s.info.memory),
		fmt_bytes(s.mem_max)
	);
	out.push_str(&format!(
		"│{}││{}│\n",
		pad_to(&cpu_val, left_w.saturating_sub(2)),
		pad_to(&mem_val, right_w.saturating_sub(2)),
	));

	for r in 0..GRAPH_HEIGHT {
		let cpu_row = graph_row_str(&cpu_rows, r, cpu_gw);
		let mem_row = graph_row_str(&mem_rows, r, mem_gw);
		out.push_str(&format!(
			"│ {}{} ││ {}{} │\n",
			term::green(format_args!("{}", cpu_row)),
			term::dim(format_args!("{}", " ")),
			term::cyan(format_args!("{}", mem_row)),
			term::dim(format_args!("{}", " ")),
		));
	}
	out.push_str(&border_bot(left_w));
	out.push_str(&border_bot(right_w));
	out.push('\n');
}

fn write_details(out: &mut String, s: &MonitState, w: usize) {
	let mut git = s.info.git_branch.clone().unwrap_or_default();
	if !git.is_empty() && s.info.git_commit.is_some() {
		git.push('@');
		git.push_str(&s.info.git_commit.clone().unwrap_or_default());
	}
	if git.is_empty() {
		git = "—".to_string();
	}
	let mut cmd = s.spec.exec.command.clone().unwrap_or_default();
	if let Some(args) = &s.spec.exec.args {
		if !args.is_empty() {
			cmd.push(' ');
			cmd.push_str(&args.join(" "));
		}
	}

	out.push_str(&border_top(w, " Details "));
	out.push('\n');

	let ns = if !s.info.namespace.is_empty() {
		s.info.namespace.clone()
	} else {
		s.spec.namespace.clone().unwrap_or_default()
	};
	for row in [
		detail_row(&[("namespace", &ns), ("version", &s.info.version)]),
		detail_row(&[("mode", &s.info.mode), ("git", &git)]),
		detail_row(&[("user", &s.info.user), ("cmd", &cmd)]),
	] {
		out.push_str(&format!("│{}│\n", pad_to(&row, w.saturating_sub(2))));
	}
	out.push_str(&border_bot(w));
	out.push('\n');
}

fn write_process_tree(out: &mut String, s: &MonitState, w: usize) {
	out.push_str(&border_top(w, " Process Tree "));
	out.push('\n');
	let hdr = detail_row(&[("PID", "Process"), ("Memory", "")]);
	out.push_str(&format!(
		"│{}│\n",
		pad_to(&term::dim(format_args!("{}", hdr)), w.saturating_sub(2))
	));
	for entry in &s.tree {
		let indent = "  ".repeat(entry.depth as usize);
		let prefix = if entry.depth > 0 { "└─ " } else { "" };
		let proc_name = format!("{}{}{}", indent, prefix, entry.comm);
		let row = format!(
			"  {:<8}  {:<24}  {}",
			entry.pid,
			proc_name,
			fmt_bytes(entry.memory_bytes)
		);
		out.push_str(&format!("│{}│\n", pad_to(&row, w.saturating_sub(2))));
	}
	out.push_str(&border_bot(w));
	out.push('\n');
}

fn write_footer(out: &mut String) {
	out.push_str(&format!(
		"  {}   refresh: 1s\n",
		term::dim(format_args!("{}", "[q] quit"))
	));
}

/// Build a sparkline of `height` rows × `width` columns from `values`.
/// Values are normalized against `max_val`; cells render as the block
/// rune closest to their normalized height. Empty inputs produce rows
/// of spaces.
#[must_use]
pub fn build_graph(values: &[f64], max_val: f64, width: usize, height: usize) -> Vec<String> {
	let mut rows: Vec<String> = Vec::with_capacity(height);
	for r in 0..height {
		let mut line = String::with_capacity(width);
		for c in 0..width {
			let idx = (values.len() as isize) - (width as isize) + (c as isize);
			let v = if idx >= 0 && (idx as usize) < values.len() {
				values[idx as usize]
			} else {
				0.0
			};
			let norm = if max_val > 0.0 { v / max_val } else { 0.0 };
			let row_top = (height - r) as f64 / height as f64;
			let row_bot = (height - r - 1) as f64 / height as f64;
			let ch = if norm >= row_top {
				BLOCK_RUNES[BLOCK_RUNES.len() - 1]
			} else if norm > row_bot {
				let frac = (norm - row_bot) / (row_top - row_bot);
				let bi = (frac * (BLOCK_RUNES.len() - 1) as f64) as usize;
				let bi = bi.min(BLOCK_RUNES.len() - 1);
				BLOCK_RUNES[bi]
			} else {
				' '
			};
			line.push(ch);
		}
		rows.push(line);
	}
	rows
}

fn graph_row_str(rows: &[String], r: usize, width: usize) -> String {
	if let Some(line) = rows.get(r) {
		return line.clone();
	}
	" ".repeat(width)
}

/// Build a 2-column row of label/value pairs.
#[must_use]
pub fn detail_row(pairs: &[(&str, &str)]) -> String {
	const LABEL_W: usize = 12;
	const VAL_W: usize = 20;
	let mut out = String::from("  ");
	let mut i = 0;
	while i < pairs.len() {
		let (label, val) = (pairs[i].0, pairs[i].1);
		out.push_str(&term::dim(format_args!("{}", label)));
		for _ in 0..(LABEL_W.saturating_sub(label.len())) {
			out.push(' ');
		}
		out.push_str(val);
		if i + 1 < pairs.len() {
			let pad = VAL_W.saturating_sub(val.len()).max(1);
			for _ in 0..pad {
				out.push(' ');
			}
		}
		i += 1;
	}
	out
}

/// Colourise a process state string.
#[must_use]
pub fn state_str(state: ProcessState) -> String {
	match state {
		ProcessState::Running | ProcessState::Online => {
			term::green(format_args!("{}", state.as_str()))
		}
		ProcessState::Stopped | ProcessState::Exited => {
			term::yellow(format_args!("{}", state.as_str()))
		}
		ProcessState::Failed => term::red(format_args!("{}", state.as_str())),
		ProcessState::Restarting => term::cyan(format_args!("{}", state.as_str())),
	}
}

/// Format uptime in milliseconds as `"1h 0m"` / `"22m 9s"` / `"5s"`.
#[must_use]
pub fn fmt_uptime(ms: i64) -> String {
	let total_secs = (ms / 1000).max(0);
	let h = total_secs / 3_600;
	let m = (total_secs / 60) % 60;
	let s = total_secs % 60;
	if h > 0 {
		format!("{h}h {m}m")
	} else if m > 0 {
		format!("{m}m {s}s")
	} else {
		format!("{s}s")
	}
}

/// Format a byte count as `"4.0 MB"` etc. (different from
/// [`crate::cli::format::bytes`] — uses decimal units).
#[must_use]
pub fn fmt_bytes(b: i64) -> String {
	const KB: i64 = 1024;
	const MB: i64 = KB * 1024;
	const GB: i64 = MB * 1024;
	if b >= GB {
		format!("{:.1} GB", b as f64 / GB as f64)
	} else if b >= MB {
		format!("{:.1} MB", b as f64 / MB as f64)
	} else if b >= KB {
		format!("{:.1} KB", b as f64 / KB as f64)
	} else {
		format!("{b} B")
	}
}

// silence unused warning for format module on linux
#[allow(dead_code)]
fn _format_keepalive() -> &'static str {
	format::uptime_exact(0).leak()
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn fmt_bytes_buckets() {
		assert_eq!(fmt_bytes(0), "0 B");
		assert_eq!(fmt_bytes(500), "500 B");
		assert_eq!(fmt_bytes(1024), "1.0 KB");
		assert_eq!(fmt_bytes(1536), "1.5 KB");
		assert_eq!(fmt_bytes(1024 * 1024), "1.0 MB");
		assert_eq!((1024 * 1024 + 512 * 1024) as i64, 1024 * 1024 + 512 * 1024);
		let m = fmt_bytes((1024 * 1024 + 512 * 1024) as i64);
		assert!(m.starts_with("1.5 MB"), "got {m}");
		assert_eq!(fmt_bytes(1024_i64 * 1024 * 1024), "1.0 GB");
	}

	#[test]
	fn fmt_uptime_buckets() {
		assert_eq!(fmt_uptime(0), "0s");
		assert_eq!(fmt_uptime(5_000), "5s");
		assert_eq!(fmt_uptime(65_000), "1m 5s");
		assert_eq!(fmt_uptime(3_600_000), "1h 0m");
		assert_eq!(fmt_uptime(3_661_000), "1h 1m");
	}

	#[test]
	fn vis_len_ignores_ansi() {
		assert_eq!(vis_len("hello"), 5);
		assert_eq!(vis_len(""), 0);
		assert_eq!(vis_len("\x1b[32mok\x1b[0m"), 2);
		assert_eq!(vis_len("\x1b[1mBold\x1b[0m text"), 9);
	}

	#[test]
	fn pad_to_pads_or_passes_through() {
		assert_eq!(pad_to("hi", 6), "hi    ");
		assert_eq!(pad_to("hello", 3), "hello");
	}

	#[test]
	fn border_top_includes_title() {
		let s = border_top(20, " Title ");
		assert!(s.starts_with('╭'));
		assert!(s.ends_with('╮'));
		assert!(s.contains(" Title "));
	}

	#[test]
	fn border_bot_corners_and_width() {
		let s = border_bot(10);
		assert!(s.starts_with('╰'));
		assert!(s.ends_with('╯'));
		assert_eq!(s.chars().count(), 10);
	}

	#[test]
	fn graph_row_str_in_range() {
		let rows = vec!["abc".to_string(), "def".to_string()];
		assert_eq!(graph_row_str(&rows, 0, 3), "abc");
	}

	#[test]
	fn graph_row_str_out_of_range() {
		let rows = vec!["abc".to_string()];
		assert_eq!(graph_row_str(&rows, 5, 4), "    ");
	}

	#[test]
	fn build_graph_empty_inputs() {
		let rows = build_graph(&[], 100.0, 10, 3);
		assert_eq!(rows.len(), 3);
		for r in &rows {
			assert!(r.trim().is_empty());
		}
	}

	#[test]
	fn build_graph_full_bar_uses_top_block() {
		let vals = vec![100.0; 10];
		let rows = build_graph(&vals, 100.0, 10, 4);
		for r in &rows {
			for ch in r.chars() {
				assert_eq!(ch, '█');
			}
		}
	}

	#[test]
	fn build_graph_respects_width() {
		let rows = build_graph(&[50.0], 100.0, 8, 2);
		for r in &rows {
			assert_eq!(r.chars().count(), 8);
		}
	}

	#[test]
	fn detail_row_includes_values() {
		let row = detail_row(&[("key", "value"), ("k2", "v2")]);
		assert!(row.contains("value"));
		assert!(row.contains("v2"));
	}

	#[test]
	fn state_str_returns_non_empty_for_each_variant() {
		for s in [
			ProcessState::Running,
			ProcessState::Online,
			ProcessState::Stopped,
			ProcessState::Exited,
			ProcessState::Failed,
			ProcessState::Restarting,
		] {
			assert!(!state_str(s).is_empty());
		}
	}
}
