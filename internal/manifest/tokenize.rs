//! Command-line tokenizer used when the manifest declares an app's command.
//!
//! The shape mirrors the Go tree's `start.Tokenize`: a hand-rolled state
//! machine that splits on whitespace and recognises single and double quotes,
//! with `\\"` and `\\\\` recognised inside double quotes. It deliberately does
//! NOT support shell features — no globbing, no env expansion, no variable
//! substitution.
//!
//! Kept private to `manifest` rather than a shared `internal/start` module:
//! this phase's brief is the manifest parser only, and the public `start`
//! command lives on a later phase. When that arrives it can call into this
//! helper or replace it, whichever fits.

const STATE_NORMAL: u8 = 0;
const STATE_SINGLE: u8 = 1;
const STATE_DOUBLE: u8 = 2;

/// Split a command string into its argv-shaped parts. Returns an error on an
/// unclosed quote or an unsupported escape sequence inside double quotes.
pub fn tokenize(input: &str) -> Result<Vec<String>, TokenizeError> {
	let chars: Vec<char> = input.chars().collect();
	let mut pos = 0;
	let mut state = STATE_NORMAL;
	let mut args: Vec<String> = Vec::new();
	let mut cur = String::new();

	while pos < chars.len() {
		let r = chars[pos];
		match state {
			STATE_NORMAL => {
				if r.is_whitespace() {
					if !cur.is_empty() {
						args.push(std::mem::take(&mut cur));
					}
				} else if r == '\'' {
					state = STATE_SINGLE;
				} else if r == '"' {
					state = STATE_DOUBLE;
				} else {
					cur.push(r);
				}
			}
			STATE_SINGLE => {
				if r == '\'' {
					state = STATE_NORMAL;
				} else {
					cur.push(r);
				}
			}
			STATE_DOUBLE => match r {
				'"' => state = STATE_NORMAL,
				'\\' => {
					if pos + 1 >= chars.len() {
						return Err(TokenizeError::TrailingBackslash);
					}
					let next = chars[pos + 1];
					match next {
						'"' | '\\' => {
							cur.push(next);
							pos += 1;
						}
						_ => return Err(TokenizeError::InvalidEscape(next)),
					}
				}
				_ => cur.push(r),
			},
			_ => unreachable!("unknown tokenizer state"),
		}
		pos += 1;
	}

	if state != STATE_NORMAL {
		return Err(TokenizeError::UnclosedQuote);
	}

	if !cur.is_empty() {
		args.push(cur);
	}

	Ok(args)
}

/// Errors surfaced by [`tokenize`].
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TokenizeError {
	UnclosedQuote,
	TrailingBackslash,
	InvalidEscape(char),
}

impl std::fmt::Display for TokenizeError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			TokenizeError::UnclosedQuote => f.write_str("unclosed quote"),
			TokenizeError::TrailingBackslash => {
				f.write_str("invalid escape sequence: trailing backslash")
			}
			TokenizeError::InvalidEscape(c) => write!(f, "invalid escape sequence: \\{c}"),
		}
	}
}

impl std::error::Error for TokenizeError {}

/// Tokenize a manifest command, falling back to a plain whitespace split when
/// the lexer returns zero parts (empty / whitespace-only input). Mirrors the
/// Go `tokenizeCommand` wrapper.
pub fn tokenize_command(cmd: &str) -> Result<Vec<String>, TokenizeError> {
	let parts = tokenize(cmd)?;
	if parts.is_empty() {
		Ok(cmd.split_whitespace().map(str::to_string).collect())
	} else {
		Ok(parts)
	}
}
