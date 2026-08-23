//! Self-update path: check GitHub for new releases, verify the embedded
//! release key against the published signature, then swap the binary.
//!
//! Phase 5b of the Go -> Rust rewrite (#51). The fail-closed posture of the
//! Go original is preserved verbatim:
//!
//! 1. Resolve the release from GitHub.
//! 2. Require a `.sig` asset (and a configured release key).
//! 3. Download the new binary to a temp file.
//! 4. Verify the ed25519 signature before swapping.
//! 5. Rename the temp file over the running binary.
//!
//! Skipping any step that refuses — or rearranging them — turns the updater
//! into an arbitrary-code-execution surface. The controls that get tested
//! are the ones above. The trust anchor is [`keys::RELEASE_PUBLIC_KEY_B64`];
//! it is an embedded constant and must never become configurable.
//!
//! [`keys::RELEASE_PUBLIC_KEY_B64`]: keys::RELEASE_PUBLIC_KEY_B64

mod cache;
mod keys;

use std::env;
use std::fs::{self, File, OpenOptions, Permissions};
use std::io::{Read, Write};
use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};
use std::process::Command;
#[cfg(test)]
use std::sync::{Mutex, MutexGuard, RwLock};
use std::time::Duration;

pub use cache::{check_cached as CheckCached, CacheEntry, Release as CachedRelease};
pub use keys::{decode_signature, load_release_public_key, verify_signature, ErrSignatureRequired};

pub const REPO_OWNER: &str = "Glyndor";
pub const REPO_NAME: &str = "unitpm";

/// Maximum size of a downloaded binary (500MB).
pub const MAX_DOWNLOAD_SIZE: u64 = 500 * 1024 * 1024;

const HTTP_READ_TIMEOUT: Duration = Duration::from_secs(10);
const HTTP_DOWNLOAD_TIMEOUT: Duration = Duration::from_secs(10 * 60);

/// A GitHub release.
#[derive(Debug, Clone, Default, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct Release {
	#[serde(rename = "tag_name")]
	pub tag_name: String,
	pub assets: Vec<Asset>,
	pub body: String,
	#[serde(rename = "html_url")]
	pub html_url: String,
}

/// A single file in a GitHub release.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct Asset {
	pub name: String,
	#[serde(rename = "browser_download_url")]
	pub browser_download_url: String,
}

/// Customise update application.
#[derive(Debug, Clone, Default)]
pub struct ApplyOptions {
	/// Permit updates when no signature is present. Must be set explicitly by
	/// the caller — there is no environment switch and no CLI flag.
	pub allow_unsigned: bool,
}

/// Errors surfaced by the updater.
#[derive(Debug)]
pub enum Error {
	/// Refusal: the release is not signed, or the build does not ship a key.
	SignatureRequired,
	/// Refusal with extra context (e.g. "no .sig asset in release v1.2.3").
	SignatureRequiredWith(String),
	NoCompatibleBinary {
		os: String,
		arch: String,
		tag: String,
	},
	Http(String),
	Json(String),
	BadPublicKey(String),
	SignatureDownload(String),
	SignatureMalformed {
		bytes: usize,
	},
	SignatureInvalid,
	Io(std::io::Error),
	ExePath(String),
	SymlinkResolve(String),
	TempFile(String),
	DownloadStatus(String),
	DownloadWrite(String),
	DownloadTooLarge {
		max: u64,
	},
	TempClose(String),
	Chmod(String),
	Rename(String),
}

impl Error {
	/// True when the error is or wraps `ErrSignatureRequired`.
	pub fn is_signature_required(&self) -> bool {
		matches!(
			self,
			Error::SignatureRequired | Error::SignatureRequiredWith(_)
		)
	}
}

impl std::fmt::Display for Error {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		match self {
			Error::SignatureRequired => f.write_str("update refused: release is not signed"),
			Error::SignatureRequiredWith(s) => {
				write!(f, "update refused: release is not signed: {s}")
			}
			Error::NoCompatibleBinary { os, arch, tag } => {
				write!(
					f,
					"no compatible binary found for {os}/{arch} in release {tag}"
				)
			}
			Error::Http(s) => write!(f, "github api returned status: {s}"),
			Error::Json(s) => write!(f, "failed to decode release info: {s}"),
			Error::BadPublicKey(s) => write!(f, "release public key invalid: {s}"),
			Error::SignatureDownload(s) => write!(f, "signature download: {s}"),
			Error::SignatureMalformed { bytes } => {
				write!(f, "signature malformed: {bytes} bytes")
			}
			Error::SignatureInvalid => {
				f.write_str("ed25519 signature does not match downloaded binary")
			}
			Error::Io(e) => write!(f, "io error: {e}"),
			Error::ExePath(s) => write!(f, "failed to determine executable path: {s}"),
			Error::SymlinkResolve(s) => write!(f, "failed to resolve symlinks: {s}"),
			Error::TempFile(s) => write!(f, "failed to create temp file (check permissions): {s}"),
			Error::DownloadStatus(s) => write!(f, "download failed with status: {s}"),
			Error::DownloadWrite(s) => write!(f, "failed to write update file: {s}"),
			Error::DownloadTooLarge { max } => {
				write!(f, "update file exceeded max download size of {max} bytes")
			}
			Error::TempClose(s) => write!(f, "failed to close update file: {s}"),
			Error::Chmod(s) => write!(f, "failed to set executable permissions: {s}"),
			Error::Rename(s) => write!(f, "failed to replace binary: {s}"),
		}
	}
}

impl std::error::Error for Error {}

impl From<std::io::Error> for Error {
	fn from(e: std::io::Error) -> Self {
		Error::Io(e)
	}
}

// --- URL override (test seam) ---------------------------------------------

#[cfg(test)]
static RELEASES_URL_OVERRIDE: RwLock<Option<String>> = RwLock::new(None);

fn releases_url() -> String {
	#[cfg(test)]
	if let Some(url) = RELEASES_URL_OVERRIDE.read().unwrap().clone() {
		return url;
	}
	format!("https://api.github.com/repos/{REPO_OWNER}/{REPO_NAME}/releases/latest")
}

/// Test-only helper: override `releases_url` and acquire the test mutex so
/// concurrent tests cannot race. The returned guard clears the override and
/// releases the mutex on `Drop`.
#[cfg(test)]
pub(crate) struct UrlGuard {
	_held: MutexGuard<'static, ()>,
}

#[cfg(test)]
static URL_LOCK: Mutex<()> = Mutex::new(());

#[cfg(test)]
impl UrlGuard {
	pub(crate) fn new(url: &str) -> Self {
		let held = URL_LOCK.lock().unwrap_or_else(|e| e.into_inner());
		*RELEASES_URL_OVERRIDE.write().unwrap() = Some(url.to_string());
		Self { _held: held }
	}
}

#[cfg(test)]
impl Drop for UrlGuard {
	fn drop(&mut self) {
		*RELEASES_URL_OVERRIDE.write().unwrap() = None;
	}
}

// --- Asset naming ---------------------------------------------------------

fn asset_basename() -> String {
	format!("unitpm_{}_{}", env::consts::OS, env::consts::ARCH)
}

// --- HTTP -----------------------------------------------------------------

fn http_get(url: &str, timeout: Duration, max_bytes: u64) -> Result<Vec<u8>, Error> {
	let agent = ureq::AgentBuilder::new()
		.timeout_read(timeout)
		.timeout_write(timeout)
		.build();
	let resp = agent
		.get(url)
		.call()
		.map_err(|e| Error::Http(e.to_string()))?;
	let status = resp.status();
	if status != 200 {
		return Err(Error::Http(status.to_string()));
	}
	let mut buf = Vec::new();
	let mut reader = resp.into_reader();
	if max_bytes > 0 {
		let mut limited = (&mut reader).take(max_bytes);
		limited
			.read_to_end(&mut buf)
			.map_err(|e| Error::Http(e.to_string()))?;
	} else {
		reader
			.read_to_end(&mut buf)
			.map_err(|e| Error::Http(e.to_string()))?;
	}
	Ok(buf)
}

// --- Public API -----------------------------------------------------------

/// Check for updates. Returns `Ok(Some(release))` when the server's tag is
/// strictly newer than the running version, or `Ok(None)` when up to date.
pub fn check() -> Result<Option<Release>, Error> {
	let body = http_get(&releases_url(), HTTP_READ_TIMEOUT, 0)?;
	let release: Release = serde_json::from_slice(&body).map_err(|e| Error::Json(e.to_string()))?;
	let current = crate::version::VERSION.trim_start_matches('v');
	let latest = release.tag_name.trim_start_matches('v');
	if current == latest {
		return Ok(None);
	}
	if !is_newer(latest, current) {
		return Ok(None);
	}
	Ok(Some(release))
}

/// Download, verify, and apply the update against the given target path.
/// Tests inject `exe_path`; production callers use [`apply`].
pub fn apply_to_path(exe_path: &Path, release: &Release, opts: ApplyOptions) -> Result<(), Error> {
	let pub_key = load_release_public_key().map_err(|e| Error::BadPublicKey(e.to_string()))?;
	apply_to_path_with_key(exe_path, release, opts, &pub_key)
}

/// Same as [`apply_to_path`] but takes the public key directly. Production
/// code always loads the embedded key via [`apply_to_path`] — this entry
/// point exists so tests can verify the apply path end-to-end against a
/// throwaway keypair without needing the org signing secret.
pub fn apply_to_path_with_key(
	exe_path: &Path,
	release: &Release,
	opts: ApplyOptions,
	pub_key: &ed25519_dalek::VerifyingKey,
) -> Result<(), Error> {
	let target = asset_basename();
	let sig_target = format!("{target}.sig");

	let mut asset_url: Option<&str> = None;
	let mut sig_url: Option<&str> = None;
	for a in &release.assets {
		if a.name == target {
			asset_url = Some(a.browser_download_url.as_str());
		} else if a.name == sig_target {
			sig_url = Some(a.browser_download_url.as_str());
		}
	}

	let Some(asset_url) = asset_url else {
		return Err(Error::NoCompatibleBinary {
			os: env::consts::OS.into(),
			arch: env::consts::ARCH.into(),
			tag: release.tag_name.clone(),
		});
	};

	match (pub_key_is_present(), sig_url, opts.allow_unsigned) {
		(false, _, false) => {
			return Err(Error::SignatureRequiredWith(
				"release signing key is not configured in this build".into(),
			));
		}
		(true, None, false) => {
			return Err(Error::SignatureRequiredWith(format!(
				"no {sig_target} asset in release {}",
				release.tag_name
			)));
		}
		_ => {}
	}

	download_and_replace(asset_url, sig_url, exe_path, pub_key, opts.allow_unsigned)
}

/// Production entry point. Resolves the running executable (following
/// symlinks so dpkg diversions are handled) and calls [`apply_to_path`].
pub fn apply(release: &Release, opts: ApplyOptions) -> Result<(), Error> {
	let exe = env::current_exe().map_err(|e| Error::ExePath(e.to_string()))?;
	let resolved = fs::canonicalize(&exe).map_err(|e| Error::SymlinkResolve(e.to_string()))?;
	apply_to_path(&resolved, release, opts)
}

/// `true` iff the embedded release key is non-empty. Mirrors the Go
/// `len(pubKey) == 0` check that gates the `AllowUnsigned` branch.
fn pub_key_is_present() -> bool {
	!keys::RELEASE_PUBLIC_KEY_B64.is_empty()
}

fn download_and_replace(
	asset_url: &str,
	sig_url: Option<&str>,
	exe_path: &Path,
	pub_key: &ed25519_dalek::VerifyingKey,
	allow_unsigned: bool,
) -> Result<(), Error> {
	let parent = exe_path.parent().unwrap_or_else(|| Path::new("."));
	let (_tmp_file, tmp_path) = tempfile_in(parent)?;
	let mut dest = OpenOptions::new()
		.write(true)
		.truncate(true)
		.open(&tmp_path)
		.map_err(|e| Error::TempFile(e.to_string()))?;
	download_into(asset_url, &mut dest)?;
	drop(dest);
	let sig_present = pub_key_is_present() && sig_url.is_some() && !allow_unsigned;
	if sig_present {
		let sig_url = sig_url.expect("checked above");
		let sig_bytes = http_get(sig_url, Duration::from_secs(30), 4096)?;
		let sig = decode_signature(&sig_bytes).map_err(|e| match e {
			keys::SigError::Malformed { bytes } => Error::SignatureMalformed { bytes },
		})?;
		let body = fs::read(&tmp_path)?;
		if !verify_signature(pub_key, &body, &sig) {
			fs::remove_file(&tmp_path).ok();
			return Err(Error::SignatureInvalid);
		}
	}
	fs::set_permissions(&tmp_path, Permissions::from_mode(0o755))
		.map_err(|e| Error::Chmod(e.to_string()))?;
	fs::rename(&tmp_path, exe_path).map_err(|e| Error::Rename(e.to_string()))?;
	Ok(())
}

fn tempfile_in(parent: &Path) -> Result<(File, PathBuf), Error> {
	let pid = std::process::id();
	let nanos = std::time::SystemTime::now()
		.duration_since(std::time::UNIX_EPOCH)
		.map(|d| d.as_nanos())
		.unwrap_or(0);
	let name = format!("unitpm-update-{pid}-{nanos}");
	let mut path = parent.to_path_buf();
	path.push(&name);
	let f = OpenOptions::new()
		.create_new(true)
		.write(true)
		.read(true)
		.mode(0o600)
		.open(&path)
		.map_err(|e| Error::TempFile(e.to_string()))?;
	Ok((f, path))
}

fn download_into(url: &str, dest: &mut File) -> Result<u64, Error> {
	let agent = ureq::AgentBuilder::new()
		.timeout_read(HTTP_DOWNLOAD_TIMEOUT)
		.timeout_write(HTTP_DOWNLOAD_TIMEOUT)
		.build();
	let resp = agent
		.get(url)
		.call()
		.map_err(|e| Error::DownloadStatus(e.to_string()))?;
	let status = resp.status();
	if status != 200 {
		return Err(Error::DownloadStatus(status.to_string()));
	}
	let mut reader = resp.into_reader().take(MAX_DOWNLOAD_SIZE + 1);
	let mut buf = [0u8; 8192];
	let mut total: u64 = 0;
	loop {
		let n = reader
			.read(&mut buf)
			.map_err(|e| Error::DownloadWrite(e.to_string()))?;
		if n == 0 {
			break;
		}
		total += n as u64;
		if total > MAX_DOWNLOAD_SIZE {
			return Err(Error::DownloadTooLarge {
				max: MAX_DOWNLOAD_SIZE,
			});
		}
		dest.write_all(&buf[..n])
			.map_err(|e| Error::DownloadWrite(e.to_string()))?;
	}
	Ok(total)
}

/// True when dpkg/rpm/pacman claim ownership of the running binary. Stops
/// self-update from clobbering an apt install. Tests exercise the no-panic
/// path; the package-system probes run on every check.
pub fn is_managed_by_package_system() -> bool {
	let exe = match env::current_exe() {
		Ok(p) => p,
		Err(_) => return false,
	};
	let resolved = fs::canonicalize(&exe).unwrap_or_else(|_| exe.clone());
	let mut candidates: Vec<PathBuf> = vec![exe.clone()];
	if resolved != exe {
		candidates.push(resolved);
	}
	for tool in [
		("dpkg", vec!["-S".to_string()]),
		("rpm", vec!["-qf".to_string()]),
		("pacman", vec!["-Qo".to_string()]),
	] {
		let (bin, args) = (tool.0, tool.1);
		if which(bin).is_none() {
			continue;
		}
		for path in &candidates {
			let mut cmd = Command::new(bin);
			cmd.args(&args);
			cmd.arg(path);
			if cmd.status().map(|s| s.success()).unwrap_or(false) {
				return true;
			}
		}
	}
	false
}

fn which(bin: &str) -> Option<PathBuf> {
	let path = env::var_os("PATH")?;
	for dir in env::split_paths(&path) {
		let candidate = dir.join(bin);
		if candidate.is_file() {
			return Some(candidate);
		}
	}
	None
}

/// Strict semver comparison on `X.Y.Z` (no `v` prefix).
pub fn is_newer(a: &str, b: &str) -> bool {
	let pa = parse_version(a);
	let pb = parse_version(b);
	for i in 0..3 {
		if pa[i] > pb[i] {
			return true;
		}
		if pa[i] < pb[i] {
			return false;
		}
	}
	false
}

/// Parse `X.Y.Z` into `[X, Y, Z]`. Returns `[0, 0, 0]` on any parse error
/// or input shorter than three segments — matches the Go fallback.
pub fn parse_version(v: &str) -> [i64; 3] {
	let mut parts = [0i64, 0, 0];
	for (i, seg) in v.split('.').take(3).enumerate() {
		match seg.parse::<i64>() {
			Ok(n) => parts[i] = n,
			Err(_) => return [0, 0, 0],
		}
	}
	parts
}

#[cfg(all(test, target_os = "linux"))]
mod test_server;

#[cfg(all(test, target_os = "linux"))]
mod is_managed_tests;

#[cfg(all(test, target_os = "linux"))]
mod cache_tests;

#[cfg(all(test, target_os = "linux"))]
mod tests;
