// Package updater handles checking and applying updates from GitHub Releases.
package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/version"
)

const (
	repoOwner = "Jaro-c"
	repoName  = "Lynx"

	// maxDownloadSize is the maximum size of a downloaded binary (500MB).
	maxDownloadSize = 500 * 1024 * 1024
)

// releasesURL is the endpoint Check queries. Package-level var so tests
// can point it at an httptest.Server.
var releasesURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)

// releasePublicKeyB64 is the ed25519 public key used to verify release
// signatures. Base64 (std) encoding of the 32-byte public key.
const releasePublicKeyB64 = "HFv7vg5FCY7YyKUDbJhaQSfB9SboJGSblJtFbLmLHzM="

// ErrSignatureRequired is returned when signature verification is required
// but the release does not ship a signature asset.
var ErrSignatureRequired = errors.New("update refused: release is not signed")

// ApplyOptions customizes update application.
type ApplyOptions struct {
	// AllowUnsigned permits updates even when no release signing key is
	// configured or when the release ships without a signature. This is the
	// only way to update today (until the project publishes a signing key)
	// and must be set explicitly by the caller.
	AllowUnsigned bool
}

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
	Body    string  `json:"body"`
	HTMLURL string  `json:"html_url"`
}

// Asset represents a file in a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Check checks for updates on GitHub.
// Returns the release info if a new version is available, or nil if up to date.
func Check(ctx context.Context) (*Release, error) {
	body, err := httpGet(ctx, releasesURL, 10*time.Second, 0)
	if err != nil {
		return nil, fmt.Errorf("github api returned status: %w", err)
	}

	var release Release
	if err := jsonx.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("failed to decode release info: %w", err)
	}

	current := strings.TrimPrefix(version.Version, "v")
	latest := strings.TrimPrefix(release.TagName, "v")

	if current == latest {
		return nil, nil
	}

	if !isNewer(latest, current) {
		return nil, nil
	}

	return &release, nil
}

// Apply downloads, verifies, and applies the update.
func Apply(ctx context.Context, release *Release, opts ApplyOptions) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}

	// Resolve symlinks so dpkg diversions (/usr/bin/lynxpm -> /opt/lynxpm/lynxpm) are followed.
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	osName := runtime.GOOS
	arch := runtime.GOARCH
	target := fmt.Sprintf("lynxpm_%s_%s", osName, arch)
	sigTarget := target + ".sig"

	var assetURL, sigURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case target:
			assetURL = asset.BrowserDownloadURL
		case sigTarget:
			sigURL = asset.BrowserDownloadURL
		}
	}

	if assetURL == "" {
		return fmt.Errorf(
			"no compatible binary found for %s/%s in release %s",
			osName,
			arch,
			release.TagName,
		)
	}

	pubKey, err := loadReleasePublicKey()
	if err != nil {
		return fmt.Errorf("release public key invalid: %w", err)
	}

	switch {
	case len(pubKey) == 0:
		if !opts.AllowUnsigned {
			return fmt.Errorf(
				"%w: release signing key is not configured in this build "+
					"(use AllowUnsigned=true / --insecure-skip-signature to override)",
				ErrSignatureRequired,
			)
		}
	case sigURL == "":
		if !opts.AllowUnsigned {
			return fmt.Errorf("%w: no %s asset in release %s", ErrSignatureRequired, sigTarget, release.TagName)
		}
	}

	return downloadAndReplace(ctx, assetURL, sigURL, exePath, pubKey)
}

func loadReleasePublicKey() (ed25519.PublicKey, error) {
	if releasePublicKeyB64 == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(releasePublicKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode pubkey: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("pubkey wrong size: got %d, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

func downloadSignature(ctx context.Context, sigURL string) ([]byte, error) {
	// #nosec G107 // sigURL is from the GitHub API response
	raw, err := httpGet(ctx, sigURL, 30*time.Second, 4096)
	if err != nil {
		return nil, fmt.Errorf("signature download: %w", err)
	}
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == ed25519.SignatureSize {
		return raw, nil
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if decoded, err := enc.DecodeString(string(raw)); err == nil && len(decoded) == ed25519.SignatureSize {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("signature malformed: %d bytes", len(raw))
}

func downloadAndReplace(ctx context.Context, assetURL, sigURL, exePath string, pubKey ed25519.PublicKey) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(exePath), "lynxpm-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file (check permissions): %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		_ = tmpFile.Close()
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	// #nosec G107 // assetURL comes from the GitHub API response
	resp, err := client.Do(req)
	if err != nil {
		_ = tmpFile.Close()
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}
	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxDownloadSize))
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write update file: %w", err)
	}
	if written >= maxDownloadSize {
		_ = tmpFile.Close()
		return fmt.Errorf("update file exceeded max download size of %d bytes", maxDownloadSize)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close update file: %w", err)
	}

	if len(pubKey) != 0 && sigURL != "" {
		if err := verifyFileSignature(ctx, tmpPath, sigURL, pubKey); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	// #nosec G302 // binary needs to be executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	cleanExePath := filepath.Clean(exePath)
	// #nosec G304 // cleanExePath is derived from os.Executable()
	if err := os.Rename(tmpPath, cleanExePath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}
	return nil
}

// httpGet builds an HTTP GET with the given timeout, executes it, and reads
// up to maxBytes (no limit when maxBytes <= 0). Non-200 responses become an
// error carrying the HTTP status line.
func httpGet(ctx context.Context, url string, timeout time.Duration, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	var r io.Reader = resp.Body
	if maxBytes > 0 {
		r = io.LimitReader(resp.Body, maxBytes)
	}
	return io.ReadAll(r)
}

func verifyFileSignature(ctx context.Context, filePath, sigURL string, pubKey ed25519.PublicKey) error {
	sig, err := downloadSignature(ctx, sigURL)
	if err != nil {
		return err
	}
	// #nosec G304 // filePath is our own CreateTemp output
	body, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read downloaded file: %w", err)
	}
	if !ed25519.Verify(pubKey, body, sig) {
		return errors.New("ed25519 signature does not match downloaded binary")
	}
	return nil
}

// IsManagedByPackageSystem returns true when dpkg/rpm/pacman claim ownership
// of the running binary. Queries each tool directly with both the original
// and symlink-resolved paths so dpkg diversions (e.g. /usr/bin/lynxpm →
// /opt/lynxpm/lynxpm) aren't missed.
func IsManagedByPackageSystem() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		resolved = exePath
	}
	candidates := []string{exePath}
	if resolved != exePath {
		candidates = append(candidates, resolved)
	}

	for _, tool := range []struct {
		bin  string
		args []string
	}{
		{"dpkg", []string{"-S"}},
		{"rpm", []string{"-qf"}},
		{"pacman", []string{"-Qo"}},
	} {
		if _, err := exec.LookPath(tool.bin); err != nil {
			continue
		}
		for _, path := range candidates {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			// #nosec G204 // path is from os.Executable; tool/args are hardcoded
			cmd := exec.CommandContext(ctx, tool.bin, append(tool.args, path)...)
			runErr := cmd.Run()
			cancel()
			if runErr == nil {
				return true
			}
		}
	}
	return false
}

// isNewer reports whether version a is strictly newer than version b.
// Both must be in X.Y.Z format (without 'v' prefix).
func isNewer(a, b string) bool {
	pa := parseVersion(a)
	pb := parseVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return true
		}
		if pa[i] < pb[i] {
			return false
		}
	}
	return false
}

// parseVersion splits "X.Y.Z" into [X, Y, Z]. Returns [0,0,0] on error.
func parseVersion(v string) [3]int {
	var parts [3]int
	segs := strings.SplitN(v, ".", 3)
	for i := 0; i < len(segs) && i < 3; i++ {
		n, err := strconv.Atoi(segs[i])
		if err != nil {
			return [3]int{}
		}
		parts[i] = n
	}
	return parts
}
