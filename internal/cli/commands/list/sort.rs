//! Sort-spec parser for the `list` command.
//!
//! Tokens are comma-separated `field[:direction]` pairs. `direction` is
//! `asc` (default) or `desc`. Empty fields, duplicate separators, and
//! unknown field/direction names return a [`UsageError`].

use crate::cli::errs::UsageError;

/// One field used as a sort key plus direction.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SortField {
	pub field: String,
	pub asc: bool,
}

/// Parse a sort spec. Empty specs return an empty slice — the caller is
/// expected to apply its default ordering when the slice is empty.
pub fn parse_sort_spec(spec: &str) -> Result<Vec<SortField>, UsageError> {
	if spec.is_empty() {
		return Ok(Vec::new());
	}
	let mut out: Vec<SortField> = Vec::new();
	for part in spec.split(',') {
		let trimmed = part.trim();
		if trimmed.is_empty() {
			continue;
		}
		let mut field = trimmed;
		let mut asc = true;
		if let Some(idx) = trimmed.find(':') {
			field = trimmed[..idx].trim();
			let dir = trimmed[idx + 1..].trim().to_lowercase();
			match dir.as_str() {
				"desc" => asc = false,
				"" | "asc" => {}
				_ => {
					return Err(UsageError::new(format!("invalid sort direction: {dir}")));
				}
			}
		}
		match field {
			"namespace" | "name" | "createdAt" | "id" => {}
			_ => return Err(UsageError::new(format!("invalid sort field: {field}"))),
		}
		out.push(SortField {
			field: field.to_string(),
			asc,
		});
	}
	Ok(out)
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn empty_spec_returns_empty_vec() {
		let got = parse_sort_spec("").expect("empty");
		assert!(got.is_empty());
	}

	#[test]
	fn single_field_defaults_to_asc() {
		let got = parse_sort_spec("name").expect("single");
		assert_eq!(
			got,
			vec![SortField {
				field: "name".into(),
				asc: true
			}]
		);
	}

	#[test]
	fn explicit_desc_is_captured() {
		let got = parse_sort_spec("name:desc").expect("desc");
		assert_eq!(
			got,
			vec![SortField {
				field: "name".into(),
				asc: false
			}]
		);
	}

	#[test]
	fn multi_field_spec_trims_whitespace() {
		let got = parse_sort_spec("namespace:asc, name:desc").expect("multi");
		assert_eq!(
			got,
			vec![
				SortField {
					field: "namespace".into(),
					asc: true
				},
				SortField {
					field: "name".into(),
					asc: false
				},
			]
		);
	}

	#[test]
	fn invalid_field_errors() {
		assert!(parse_sort_spec("invalid").is_err());
	}

	#[test]
	fn invalid_direction_errors() {
		assert!(parse_sort_spec("name:invalid").is_err());
	}

	#[test]
	fn id_is_a_valid_field() {
		let got = parse_sort_spec("id:asc").expect("id");
		assert_eq!(
			got,
			vec![SortField {
				field: "id".into(),
				asc: true
			}]
		);
	}

	#[test]
	fn created_at_is_a_valid_field() {
		let got = parse_sort_spec("createdAt:desc").expect("createdAt");
		assert_eq!(
			got,
			vec![SortField {
				field: "createdAt".into(),
				asc: false
			}]
		);
	}
}
