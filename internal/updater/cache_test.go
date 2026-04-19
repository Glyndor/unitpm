package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/version"
)

func withCachePath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "update-check.json")
	orig := cachePathOverride
	cachePathOverride = p
	t.Cleanup(func() { cachePathOverride = orig })
	return p
}

func TestCheckCached_FreshHit_SkipsNetwork(t *testing.T) {
	withCachePath(t)

	// Prime cache with a release from 1 hour ago.
	cached := &Release{TagName: "v1.2.3", HTMLURL: "https://example.com/r"}
	if err := writeCache(CacheEntry{
		CheckedAt: time.Now().Add(-1 * time.Hour),
		Version:   version.Version,
		Release:   cached,
	}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	// Point releasesURL at a server that fails the test if it's hit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("network call should not happen on fresh cache")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	orig := releasesURL
	releasesURL = srv.URL
	t.Cleanup(func() { releasesURL = orig })

	rel, err := CheckCached(context.Background(), 6*time.Hour)
	if err != nil {
		t.Fatalf("CheckCached: %v", err)
	}
	if rel == nil || rel.TagName != "v1.2.3" {
		t.Fatalf("expected cached release, got %+v", rel)
	}
}

func TestCheckCached_StaleTriggersRefresh(t *testing.T) {
	p := withCachePath(t)

	// Stale cache (25 hours old, ttl 6h).
	if err := writeCache(CacheEntry{
		CheckedAt: time.Now().Add(-25 * time.Hour),
		Version:   version.Version,
		Release:   &Release{TagName: "v0.0.1"},
	}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	fresh := Release{
		TagName: "v99.99.99",
		HTMLURL: "https://example.com/new",
		Assets:  []Asset{{Name: "lynxpm_linux_amd64", BrowserDownloadURL: "https://example.com/bin"}},
	}
	newServer(t, fresh, 0)

	rel, err := CheckCached(context.Background(), 6*time.Hour)
	if err != nil {
		t.Fatalf("CheckCached: %v", err)
	}
	if rel == nil || rel.TagName != "v99.99.99" {
		t.Fatalf("expected refreshed release, got %+v", rel)
	}

	// Cache updated on disk.
	// #nosec G304 -- test fixture path
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("readfile: %v", err)
	}
	var got CacheEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Release == nil || got.Release.TagName != "v99.99.99" {
		t.Errorf("cache not updated: %+v", got)
	}
	if got.Version != version.Version {
		t.Errorf("version key mismatch: got %q want %q", got.Version, version.Version)
	}
}

func TestCheckCached_VersionMismatchInvalidatesCache(t *testing.T) {
	withCachePath(t)

	// Cache written under a stale running version — e.g. user upgraded.
	if err := writeCache(CacheEntry{
		CheckedAt: time.Now(),
		Version:   "0.0.0-old",
		Release:   &Release{TagName: "v0.0.1"},
	}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	newServer(t, Release{TagName: version.Version}, 0) // server says up-to-date

	rel, err := CheckCached(context.Background(), 6*time.Hour)
	if err != nil {
		t.Fatalf("CheckCached: %v", err)
	}
	if rel != nil {
		t.Errorf("expected nil (up to date) after version invalidation, got %+v", rel)
	}
}

func TestCheckCached_NoCachePerformsLiveCheck(t *testing.T) {
	withCachePath(t) // empty tmpdir — cache file does not yet exist

	newServer(t, Release{TagName: version.Version}, 0)

	rel, err := CheckCached(context.Background(), 6*time.Hour)
	if err != nil {
		t.Fatalf("CheckCached: %v", err)
	}
	if rel != nil {
		t.Errorf("expected nil (up to date), got %+v", rel)
	}
}

func TestCheckCached_FutureClockSkewTreatedAsStale(t *testing.T) {
	withCachePath(t)

	// CheckedAt in the future — clock skew. Treat as stale.
	if err := writeCache(CacheEntry{
		CheckedAt: time.Now().Add(1 * time.Hour),
		Version:   version.Version,
		Release:   &Release{TagName: "v0.0.1"},
	}); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	newServer(t, Release{TagName: version.Version}, 0)

	rel, err := CheckCached(context.Background(), 6*time.Hour)
	if err != nil {
		t.Fatalf("CheckCached: %v", err)
	}
	if rel != nil {
		t.Errorf("expected live refresh (nil up-to-date), got %+v", rel)
	}
}
