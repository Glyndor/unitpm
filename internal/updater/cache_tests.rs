//! Tests for the on-disk cache used by [`check_cached`].
//!
//! Mirrors `cache_test.go` (5 cases). The cache reads env vars and writes
//! to a path on disk, so the tests take the cache-path mutex via
//! [`CachePathGuard`] and the URL mutex via [`UrlGuard`].
//!
//! [`check_cached`]: crate::updater::check_cached
//! [`CachePathGuard`]: crate::updater::cache::tests::CachePathGuard

use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use super::tests::{setup_release_server, up_to_date_release};
use super::*;
use crate::updater::cache::tests::CachePathGuard;
use crate::updater::test_server::{build_response, TestServer};

fn write_cache_for_test(path: &Path, entry: &crate::updater::CacheEntry) {
	let data = serde_json::to_vec(entry).expect("json");
	std::fs::create_dir_all(path.parent().unwrap()).expect("mkdir");
	let tmp = path.with_extension("json.tmp");
	std::fs::write(&tmp, &data).expect("write");
	std::fs::rename(&tmp, path).expect("rename");
}

fn release_for_cache(tag: &str) -> crate::updater::CachedRelease {
	crate::updater::CachedRelease {
		tag_name: tag.into(),
		html_url: "https://example.com/r".into(),
	}
}

fn cache_fixture() -> (PathBuf, CachePathGuard) {
	let dir = tempfile::tempdir().expect("tempdir");
	let cache_path = dir.path().join("update-check.json");
	let guard = CachePathGuard::new(&cache_path);
	(cache_path, guard)
}

#[test]
fn check_cached_fresh_hit_skips_network() {
	let (_path, _guard) = cache_fixture();
	write_cache_for_test(
		&_path,
		&crate::updater::CacheEntry {
			checked_at: std::time::SystemTime::now() - std::time::Duration::from_secs(3600),
			version: crate::version::VERSION.into(),
			release: Some(release_for_cache("v1.2.3")),
		},
	);
	let server = TestServer::new(vec![build_response(500, "text/plain", b"")]);
	let _g = UrlGuard::new(&server.url("/releases/latest"));
	let live_hit = Arc::new(Mutex::new(false));
	let live_hit_clone = live_hit.clone();
	let got = CheckCached(
		&|| {
			*live_hit_clone.lock().unwrap() = true;
			Ok(None)
		},
		Duration::from_secs(6 * 3600),
	)
	.expect("cached");
	assert!(!*live_hit.lock().unwrap(), "fresh hit must skip network");
	assert_eq!(got.unwrap().tag_name, "v1.2.3");
}

#[test]
fn check_cached_stale_triggers_refresh() {
	let (path, _guard) = cache_fixture();
	write_cache_for_test(
		&path,
		&crate::updater::CacheEntry {
			checked_at: std::time::SystemTime::now() - std::time::Duration::from_secs(25 * 3600),
			version: crate::version::VERSION.into(),
			release: Some(release_for_cache("v0.0.1")),
		},
	);
	let fresh = Release {
		tag_name: "v99.99.99".into(),
		html_url: "https://example.com/new".into(),
		..Release::default()
	};
	let (_s, _g) = setup_release_server(&fresh, 200);
	let got = CheckCached(&check, Duration::from_secs(6 * 3600)).expect("refresh");
	assert_eq!(got.unwrap().tag_name, "v99.99.99");
	let entry: crate::updater::CacheEntry =
		serde_json::from_slice(&std::fs::read(&path).expect("read")).expect("parse");
	assert_eq!(entry.release.unwrap().tag_name, "v99.99.99");
	assert_eq!(entry.version, crate::version::VERSION);
}

#[test]
fn check_cached_version_mismatch_invalidates_cache() {
	let (_path, _guard) = cache_fixture();
	write_cache_for_test(
		&_path,
		&crate::updater::CacheEntry {
			checked_at: std::time::SystemTime::now(),
			version: "0.0.0-old".into(),
			release: Some(release_for_cache("v0.0.1")),
		},
	);
	let release = up_to_date_release();
	let (_s, _g) = setup_release_server(&release, 200);
	let got = CheckCached(&check, Duration::from_secs(6 * 3600)).expect("live");
	assert!(got.is_none());
}

#[test]
fn check_cached_no_cache_performs_live_check() {
	let (_path, _guard) = cache_fixture();
	let release = up_to_date_release();
	let (_s, _g) = setup_release_server(&release, 200);
	let got = CheckCached(&check, Duration::from_secs(6 * 3600)).expect("live");
	assert!(got.is_none());
}

#[test]
fn check_cached_future_clock_skew_treated_as_stale() {
	let (_path, _guard) = cache_fixture();
	write_cache_for_test(
		&_path,
		&crate::updater::CacheEntry {
			checked_at: std::time::SystemTime::now() + std::time::Duration::from_secs(3600),
			version: crate::version::VERSION.into(),
			release: Some(release_for_cache("v0.0.1")),
		},
	);
	let release = up_to_date_release();
	let (_s, _g) = setup_release_server(&release, 200);
	let got = CheckCached(&check, Duration::from_secs(6 * 3600)).expect("live");
	assert!(got.is_none());
}
