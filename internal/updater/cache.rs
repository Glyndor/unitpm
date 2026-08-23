//! On-disk cache for update-check results.
//!
//! Mirrors `cache.go`. The cache file lives under `XDG_CACHE_HOME/unitpm` when
//! that variable is set, otherwise under `$HOME/.cache/unitpm`. Tests override
//! the path with [`with_cache_path`] which installs a `Drop`-restoring guard.

use std::fs;
use std::path::PathBuf;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};

use crate::version;

/// On-disk cache entry.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct CacheEntry {
	#[serde(rename = "checked_at")]
	pub checked_at: SystemTime,
	pub version: String,
	#[serde(skip_serializing_if = "Option::is_none")]
	pub release: Option<Release>,
}

/// Subset of [`crate::updater::Release`] the cache is allowed to hold. The
/// cache only needs the fields a future `CheckCached` call cares about, so
/// the dependency on `crate::updater` stays one-way.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Release {
	pub tag_name: String,
	pub html_url: String,
}

const CACHE_BASENAME: &str = "update-check.json";
const XDG_SUBDIR: &str = "unitpm";
const HOME_SUBDIR: &str = ".cache/unitpm";

/// Compute the cache file path. Honors the `UNITPM_CACHE_PATH` override
/// (used by tests) before falling back to the XDG rules.
pub fn cache_path() -> Option<PathBuf> {
	if let Ok(p) = std::env::var("UNITPM_CACHE_PATH") {
		if !p.is_empty() {
			return Some(PathBuf::from(p));
		}
	}
	if let Ok(xdg) = std::env::var("XDG_CACHE_HOME") {
		if !xdg.is_empty() {
			return Some(PathBuf::from(xdg).join(XDG_SUBDIR).join(CACHE_BASENAME));
		}
	}
	if let Ok(home) = std::env::var("HOME") {
		if !home.is_empty() {
			return Some(PathBuf::from(home).join(HOME_SUBDIR).join(CACHE_BASENAME));
		}
	}
	None
}

fn read_cache_inner() -> Result<Option<CacheEntry>, CacheError> {
	let Some(p) = cache_path() else {
		return Ok(None);
	};
	match fs::read(&p) {
		Ok(data) => {
			let entry: CacheEntry = serde_json::from_slice(&data)?;
			Ok(Some(entry))
		}
		Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(None),
		Err(e) => Err(CacheError::Io(e)),
	}
}

fn write_cache_inner(entry: &CacheEntry) -> Result<(), CacheError> {
	let Some(p) = cache_path() else {
		return Err(CacheError::NoPath);
	};
	if let Some(parent) = p.parent() {
		fs::create_dir_all(parent)?;
	}
	let data = serde_json::to_vec(entry)?;
	let tmp = p.with_extension("json.tmp");
	fs::write(&tmp, &data)?;
	fs::rename(&tmp, &p)?;
	Ok(())
}

/// Behaviourally identical to `Check`, but consults an on-disk cache first.
/// Returns `Ok(None)` when the running version matches the latest (in either
/// path), and `Ok(Some(release))` when an update is available.
pub fn check_cached(
	check: &dyn Fn() -> Result<Option<crate::updater::Release>, crate::updater::Error>,
	ttl: Duration,
) -> Result<Option<Release>, CacheError> {
	if let Some(entry) = read_cache_inner()? {
		if entry.version == version::VERSION {
			let now = SystemTime::now()
				.duration_since(UNIX_EPOCH)
				.unwrap_or_default();
			let age = entry.checked_at.duration_since(UNIX_EPOCH).unwrap_or(now);
			// Future clock skew: age > now → treat as stale by going to the
			// live check. Mirror the Go `age >= 0` guard.
			if age <= now {
				let age_since = now.saturating_sub(age);
				if age_since < ttl {
					return Ok(entry.release);
				}
			}
		}
	}
	let live = check()?;
	let entry = CacheEntry {
		checked_at: SystemTime::now(),
		version: version::VERSION.to_string(),
		release: live.as_ref().map(|r| Release {
			tag_name: r.tag_name.clone(),
			html_url: r.html_url.clone(),
		}),
	};
	let _ = write_cache_inner(&entry); // best-effort
	Ok(entry.release)
}

/// Errors surfaced by the cache helpers.
#[derive(Debug)]
pub enum CacheError {
	Io(std::io::Error),
	Json(serde_json::Error),
	NoPath,
	Updater(String),
}

impl std::fmt::Display for CacheError {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			CacheError::Io(e) => write!(f, "cache io error: {e}"),
			CacheError::Json(e) => write!(f, "cache json error: {e}"),
			CacheError::NoPath => f.write_str("no cache path resolvable"),
			CacheError::Updater(s) => write!(f, "updater error: {s}"),
		}
	}
}

impl std::error::Error for CacheError {}

impl From<std::io::Error> for CacheError {
	fn from(e: std::io::Error) -> Self {
		CacheError::Io(e)
	}
}

impl From<serde_json::Error> for CacheError {
	fn from(e: serde_json::Error) -> Self {
		CacheError::Json(e)
	}
}

impl From<crate::updater::Error> for CacheError {
	fn from(e: crate::updater::Error) -> Self {
		CacheError::Updater(e.to_string())
	}
}

#[cfg(test)]
pub(crate) mod tests {
	use std::path::Path;

	/// Install a per-test cache path. Holds the global env lock and restores
	/// the previous values on `Drop` — the cache module reads process-global
	/// environment variables, so without serialisation two tests can race.
	pub(crate) struct CachePathGuard {
		_held: std::sync::MutexGuard<'static, ()>,
		prev_path: Option<String>,
	}

	pub(crate) static ENV_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());

	impl CachePathGuard {
		pub(crate) fn new(path: &Path) -> Self {
			let held = ENV_LOCK.lock().unwrap_or_else(|e| e.into_inner());
			let prev = std::env::var("UNITPM_CACHE_PATH").ok();
			std::env::set_var("UNITPM_CACHE_PATH", path.as_os_str());
			Self {
				_held: held,
				prev_path: prev,
			}
		}
	}

	impl Drop for CachePathGuard {
		fn drop(&mut self) {
			match self.prev_path.as_deref() {
				Some(v) => std::env::set_var("UNITPM_CACHE_PATH", v),
				None => std::env::remove_var("UNITPM_CACHE_PATH"),
			}
		}
	}
}
