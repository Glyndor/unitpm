//! Command-line tokenizer for the `start` command.
//!
//! Mirrors the Go `lexer.go` state machine: normal, single-quoted, and
//! double-quoted states; backslash escapes only inside double quotes.
//! Used to detect "command and args in one token" — when an explicit
//! runtime is *not* given, the spec parser tokenizes the lone token to
//! decide whether it is a single file (extension-inferred entry) or a
//! quoted command.

/// Tokenize a command-line string. Quoted spans are preserved verbatim;
/// backslash escapes (`\"`, `\\`) are honoured inside double quotes.
/// Returns [`CmdLineError::UnclosedQuote`] for an unterminated quote
/// and [`CmdLineError::InvalidEscape`] for an unknown escape sequence.
pub fn tokenize(input: &str) -> Result<Vec<String>, CmdLineError> {
	let chars: Vec<char> = input.chars().collect();
	let mut out: Vec<String> = Vec::new();
	let mut cur = String::new();
	let mut state = State::Normal;
	let mut pos = 0;
	while pos < chars.len() {
		let c = chars[pos];
		match state {
			State::Normal => {
				if c.is_whitespace() {
					if !cur.is_empty() {
						out.push(cur.clone());
						cur.clear();
					}
				} else if c == '\'' {
					state = State::Single;
				} else if c == '"' {
					state = State::Double;
				} else {
					cur.push(c);
				}
			}
			State::Single => {
				if c == '\'' {
					state = State::Normal;
				} else {
					cur.push(c);
				}
			}
			State::Double => {
				if c == '"' {
					state = State::Normal;
				} else if c == '\\' {
					if pos + 1 >= chars.len() {
						return Err(CmdLineError::InvalidEscape(
							"trailing backslash".to_string(),
						));
					}
					let next = chars[pos + 1];
					match next {
						'"' | '\\' => {
							cur.push(next);
							pos += 1;
						}
						_ => {
							return Err(CmdLineError::InvalidEscape(format!("\\{next}")));
						}
					}
				} else {
					cur.push(c);
				}
			}
		}
		pos += 1;
	}
	if state != State::Normal {
		return Err(CmdLineError::UnclosedQuote);
	}
	if !cur.is_empty() {
		out.push(cur);
	}
	Ok(out)
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum State {
	Normal,
	Single,
	Double,
}

/// Errors returned by [`tokenize`].
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum CmdLineError {
	UnclosedQuote,
	InvalidEscape(String),
}

impl std::fmt::Display for CmdLineError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			CmdLineError::UnclosedQuote => f.write_str("unclosed quote"),
			CmdLineError::InvalidEscape(s) => {
				write!(f, "invalid escape sequence: {s}")
			}
		}
	}
}

impl std::error::Error for CmdLineError {}

#[cfg(test)]
mod tests {
	use super::*;

	fn tok(s: &str) -> Vec<String> {
		tokenize(s).expect("tokenize")
	}

	#[test]
	fn tokenize_splits_on_whitespace() {
		assert_eq!(tok("a b c"), vec!["a", "b", "c"]);
	}

	#[test]
	fn tokenize_single_quoted_preserves_spaces() {
		assert_eq!(tok("a 'b c' d"), vec!["a", "b c", "d"]);
	}

	#[test]
	fn tokenize_double_quoted_preserves_spaces() {
		assert_eq!(tok("a \"b c\""), vec!["a", "b c"]);
	}

	#[test]
	fn tokenize_nested_quotes_preserve_other_quote_kind() {
		assert_eq!(tok("a 'b \"c\" d'"), vec!["a", "b \"c\" d"]);
		assert_eq!(tok("a \"b 'c' d\""), vec!["a", "b 'c' d"]);
	}

	#[test]
	fn tokenize_backslash_outside_quotes_is_literal() {
		// Outside double quotes, backslash is preserved as-is. Splits on the
		// whitespace boundary that follows.
		assert_eq!(tok("a\\ b"), vec!["a\\", "b"]);
	}

	#[test]
	fn tokenize_unclosed_single_quote_errors() {
		let err = tokenize("'a b").unwrap_err();
		assert_eq!(err, CmdLineError::UnclosedQuote);
	}

	#[test]
	fn tokenize_unclosed_double_quote_errors() {
		let err = tokenize("\"a b").unwrap_err();
		assert_eq!(err, CmdLineError::UnclosedQuote);
	}

	#[test]
	fn tokenize_invalid_escape_errors() {
		let err = tokenize("\"invalid escape \\z\"").unwrap_err();
		assert!(matches!(err, CmdLineError::InvalidEscape(_)));
	}

	#[test]
	fn tokenize_valid_double_quote_escapes() {
		assert_eq!(tok("\"valid escape \\\" \""), vec!["valid escape \" "]);
		assert_eq!(tok("\"valid escape \\\\ \""), vec!["valid escape \\ "]);
	}
}
