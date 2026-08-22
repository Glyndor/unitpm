//! CLI error types.
//!
//! `UsageError` signals incorrect user input (bad flags, bad args). The CLI
//! renders the message followed by the command's help text when it sees this.

/// Error caused by incorrect CLI usage — invalid flags or arguments.
///
/// When this is returned, the caller should display the message followed by
/// the command's help text.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct UsageError {
	pub message: String,
}

impl UsageError {
	#[must_use]
	pub fn new(message: impl Into<String>) -> Self {
		Self {
			message: message.into(),
		}
	}
}

impl std::fmt::Display for UsageError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		f.write_str(&self.message)
	}
}

impl std::error::Error for UsageError {}

/// Convenience constructor.
#[must_use]
pub fn new_usage_error(message: impl Into<String>) -> Box<dyn std::error::Error + Send + Sync> {
	Box::new(UsageError::new(message))
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn is_usage_error_via_downcast() {
		let err: Box<dyn std::error::Error + Send + Sync> = Box::new(UsageError::new("test"));
		let usage = err.downcast_ref::<UsageError>();
		assert!(usage.is_some(), "expected downcast to UsageError");
		assert_eq!(usage.unwrap().message, "test");

		let plain: Box<dyn std::error::Error + Send + Sync> =
			Box::new(std::io::Error::other("nope"));
		assert!(plain.downcast_ref::<UsageError>().is_none());
	}

	#[test]
	fn usage_error_displays_message() {
		let err = UsageError::new("test");
		assert_eq!(err.to_string(), "test");
	}

	#[test]
	fn new_usage_error_constructs_boxed_error() {
		let err = new_usage_error("bad flag");
		assert_eq!(err.to_string(), "bad flag");
		let usage = err.downcast_ref::<UsageError>().expect("UsageError");
		assert_eq!(usage.message, "bad flag");
	}
}
