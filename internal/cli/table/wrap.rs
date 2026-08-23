//! Word-wrap helpers used by [`super::Table`].
//!
//! Pulled out so the renderer stays focused on box-drawing; the wrap
//! logic is otherwise self-contained.
//!
//! ANSI escapes are stripped before measuring, then assumed not to span
//! a break boundary — the same convention the Go `wrapText` used.

use crate::cli::format::strip_ansi;

/// Wrap `text` to `width`. Splits greedily by word; a single word longer
/// than `width` is split character-by-character.
#[must_use]
pub fn wrap_text(text: &str, width: usize) -> Vec<String> {
	if width == 0 {
		return vec![text.to_string()];
	}
	if visible_len(text) <= width {
		return vec![text.to_string()];
	}
	let plain = strip_ansi(text);
	let words: Vec<&str> = plain.split_whitespace().collect();
	if words.is_empty() {
		return vec![text.to_string()];
	}

	let mut lines: Vec<String> = Vec::new();
	let mut current = String::new();
	let mut current_len = 0usize;

	for word in words {
		let word_len = word.chars().count();
		if word_len > width {
			if current_len > 0 {
				lines.push(std::mem::take(&mut current));
				current_len = 0;
			}
			for chunk in split_long_word(word, width) {
				lines.push(chunk);
			}
			continue;
		}
		match (current_len + word_len + 1 > width, current_len > 0) {
			(true, true) => {
				lines.push(std::mem::take(&mut current));
				current = word.to_string();
				current_len = word_len;
			}
			(_, true) => {
				current.push(' ');
				current.push_str(word);
				current_len += 1 + word_len;
			}
			(_, false) => {
				current = word.to_string();
				current_len = word_len;
			}
		}
	}
	if !current.is_empty() {
		lines.push(current);
	}
	lines
}

/// Split a single whitespace-less block into chunks of at most `width`
/// visible columns. Used when a word is longer than the column width.
fn split_long_word(word: &str, width: usize) -> Vec<String> {
	let mut parts: Vec<String> = Vec::new();
	let mut remaining = word;
	while !remaining.is_empty() {
		let take = visible_len(remaining).min(width);
		// Walk forward `take` code points, then split at that byte offset.
		let mut byte_idx = remaining.len();
		for (count, (i, _)) in remaining.char_indices().enumerate() {
			if count == take {
				byte_idx = i;
				break;
			}
		}
		if byte_idx == 0 {
			// `take` is zero — bail so we don't loop forever.
			break;
		}
		parts.push(remaining[..byte_idx].to_string());
		remaining = &remaining[byte_idx..];
	}
	parts
}

/// Width of `s` after stripping ANSI escapes. Counts Unicode code points;
/// matches the Go `utf8.RuneCountInString` behaviour the original used.
#[must_use]
pub fn visible_len(s: &str) -> usize {
	strip_ansi(s).chars().count()
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn wrap_text_short_passes_through() {
		assert_eq!(wrap_text("hello", 10), vec!["hello".to_string()]);
	}

	#[test]
	fn wrap_text_word_boundary_splits() {
		let lines = wrap_text("hello world this is long", 10);
		for line in &lines {
			assert!(visible_len(line) <= 10, "line wider than 10 cols: {line:?}");
		}
		// All words present, in order.
		let joined = lines.join(" ");
		assert!(joined.contains("hello"));
		assert!(joined.contains("world"));
		assert!(joined.contains("this"));
		assert!(joined.contains("is"));
		assert!(joined.contains("long"));
	}

	#[test]
	fn wrap_text_long_word_is_character_split() {
		let lines = wrap_text("abcdefghij", 4);
		assert_eq!(lines, vec!["abcd", "efgh", "ij"]);
	}

	#[test]
	fn wrap_text_strips_ansi_before_measuring() {
		// The red escape should not count towards width. With 11 visible
		// columns and width 10, splitting kicks in once.
		let lines = wrap_text("\u{1b}[31mhello world\u{1b}[0m", 5);
		for line in &lines {
			let vis = visible_len(line);
			assert!(vis <= 5 + 11, "line {line:?} has vis={vis}");
		}
	}
}
