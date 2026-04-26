package updater

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/version"
)

// CacheEntry is the on-disk update-check result.
type CacheEntry struct {
	CheckedAt time.Time `json:"checked_at"`
	Version   string    `json:"version"`
	Release   *Release  `json:"release,omitempty"`
}

// cachePathOverride is non-empty in tests to redirect cache I/O away from
// the user's real XDG_CACHE_HOME.
var cachePathOverride string

func cachePath() (string, error) {
	if cachePathOverride != "" {
		return cachePathOverride, nil
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "lynx-pm", "update-check.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "lynx-pm", "update-check.json"), nil
}

func readCache() (*CacheEntry, error) {
	p, err := cachePath()
	if err != nil {
		return nil, err
	}
	// #nosec G304 -- path is inside user cache dir we own
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var e CacheEntry
	if err := jsonx.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func writeCache(e CacheEntry) error {
	p, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := jsonx.Marshal(e)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// CheckCached behaves like Check but persists the result in a user-cache file.
// Returns the cached release when it is fresh (age < ttl) and matches the
// running binary's version. Otherwise it triggers a live Check, writes the
// result, and returns it. A nil release means "up to date" in either path.
func CheckCached(ctx context.Context, ttl time.Duration) (*Release, error) {
	if e, err := readCache(); err == nil && e.Version == version.Version {
		age := time.Since(e.CheckedAt)
		if age >= 0 && age < ttl {
			return e.Release, nil
		}
	}
	rel, err := Check(ctx)
	if err != nil {
		return nil, err
	}
	_ = writeCache(CacheEntry{CheckedAt: time.Now(), Version: version.Version, Release: rel})
	return rel, nil
}
