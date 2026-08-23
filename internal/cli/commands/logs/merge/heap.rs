//! Pure k-way merge by `(ts, seq)`.
//!
//! No I/O here. Each input slice must already be in source-order (which
//! is also chronological: log files are append-only), so a linear
//! selection against the current head of each slice yields the
//! chronologically-next entry. Ties on `(ts, seq)` keep the output
//! stable per source — entries emitted with identical timestamps
//! preserve insertion order so a debug reader can spot the "what
//! landed first" question.

use std::cmp::Ordering;

use super::super::entry::Entry;

/// Compare two entries by `(ts, seq)` for merge ordering.
fn entry_order(a: &Entry, b: &Entry) -> Ordering {
	match (a.ts_unix, b.ts_unix) {
		(Some(x), Some(y)) => match x.cmp(&y) {
			Ordering::Equal => a.seq.cmp(&b.seq),
			other => other,
		},
		(Some(_), None) => Ordering::Less,
		(None, Some(_)) => Ordering::Greater,
		(None, None) => a.seq.cmp(&b.seq),
	}
}

#[allow(dead_code)]
fn debug_assert_idx_in_range(idx: &[usize], sources: &[Vec<Entry>], context: &str) {
	for (i, src) in sources.iter().enumerate() {
		assert!(
			idx[i] <= src.len(),
			"idx[{i}] = {} out of bounds for src.len() = {} at {context}",
			idx[i],
			src.len()
		);
	}
}

/// Stable k-way merge by `(ts, seq)`. Each input slice must already be
/// in source-order (which is also chronological — log files are
/// append-only).
#[must_use]
pub fn merge_by_ts(sources: &[Vec<Entry>]) -> Vec<Entry> {
	let total: usize = sources.iter().map(|s| s.len()).sum();
	let mut out: Vec<Entry> = Vec::with_capacity(total);
	let mut idx = vec![0usize; sources.len()];
	loop {
		// Find the earliest entry across all sources. Reset best every
		// iteration so a source that ran out in a previous iteration
		// does not become a stale reference.
		let mut best_idx: Option<usize> = None;
		for i in 0..sources.len() {
			if idx[i] >= sources[i].len() {
				continue;
			}
			match best_idx {
				None => best_idx = Some(i),
				Some(j) => {
					// The check on idx[i] above guarantees
					// `sources[i][idx[i]]` is in range; the check
					// on idx[j] (which we set during this same
					// pass when we picked j as best) similarly
					// guarantees `sources[j][idx[j]]` is in range.
					let a = &sources[i][idx[i]];
					let b = &sources[j][idx[j]];
					if entry_order(a, b) == Ordering::Less {
						best_idx = Some(i);
					}
				}
			}
		}
		match best_idx {
			None => break,
			Some(i) => {
				out.push(sources[i][idx[i]].clone());
				idx[i] += 1;
			}
		}
	}
	out
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::cli::commands::logs::entry::Entry as E;

	fn entry(ts: i64, body: &str, seq: u64) -> E {
		E {
			ts_unix: Some(ts),
			label: "STDOUT".into(),
			body: body.into(),
			seq,
			has_ts: true,
		}
	}

	#[test]
	fn merge_by_ts_empty() {
		assert_eq!(merge_by_ts(&[]).len(), 0);
	}

	#[test]
	fn merge_by_ts_single_source() {
		let src = vec![entry(1, "a", 0), entry(2, "b", 1)];
		let got = merge_by_ts(&[src]);
		assert_eq!(got.len(), 2);
		assert_eq!(got[0].body, "a");
		assert_eq!(got[1].body, "b");
	}

	#[test]
	fn merge_by_ts_chronological_across_sources() {
		let stdout = vec![entry(1, "ok 1", 0), entry(3, "ok 2", 1)];
		let stderr = vec![entry(2, "err 1", 2), entry(4, "err 2", 3)];
		let merged = merge_by_ts(&[stdout, stderr]);
		assert_eq!(merged.len(), 4);
		assert_eq!(merged[0].body, "ok 1");
		assert_eq!(merged[1].body, "err 1");
		assert_eq!(merged[2].body, "ok 2");
		assert_eq!(merged[3].body, "err 2");
	}

	#[test]
	fn merge_by_ts_tie_break_by_seq() {
		let a = vec![entry(0, "a", 0)];
		let b = vec![entry(0, "b", 1)];
		let got = merge_by_ts(&[a, b]);
		assert_eq!(got[0].body, "a");
		assert_eq!(got[1].body, "b");
	}
}
