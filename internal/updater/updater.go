// Package updater handles checking and applying updates from GitHub Releases.
package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
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
const releasePublicKeyB64 = "3eSCGskGd4rjnsVcBfKM5a25SNkJayBHcqZ6dpCfWIw="

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
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// #nosec G704 // URL is hardcoded with constants repoOwner and repoName
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status: %s", resp.Status)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode release info: %w", err)
	}

	// Semantic version comparison (assumes vX.Y.Z format).
	current := strings.TrimPrefix(version.Version, "v")
	latest := strings.TrimPrefix(release.TagName, "v")

	if current == latest {
		return nil, nil // Up to date
	}

	// Only report update if latest is actually newer, to prevent downgrades.
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

	// Resolve symlinks (e.g., if running from /usr/bin/lynx -> /opt/lynx/lynx)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sigURL, nil)
	if err != nil {
		return nil, fmt.Errorf("signature request: %w", err)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	// #nosec G107 // sigURL is from the GitHub API response
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("signature download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signature download status: %s", resp.Status)
	}
	// 4KB is way more than enough for a raw ed25519 sig or a base64-wrapped one.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil, fmt.Errorf("signature read: %w", err)
	}
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == ed25519.SignatureSize {
		return raw, nil
	}
	// Try base64 (std or url-safe, with or without padding).
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
	tmpFile, err := os.CreateTemp(filepath.Dir(exePath), "lynx-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file (check permissions): %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to create download request: %w", err)
	}

	downloadClient := &http.Client{Timeout: 10 * time.Minute}
	// #nosec G107 // assetURL comes from the GitHub API response
	resp, err := downloadClient.Do(req)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	n, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxDownloadSize))
	closeErr := tmpFile.Close()
	if err != nil {
		return fmt.Errorf("failed to write update file: %w", err)
	}
	if n >= maxDownloadSize {
		return fmt.Errorf("update file exceeded max download size of %d bytes", maxDownloadSize)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close update file: %w", closeErr)
	}

	// Verify signature BEFORE chmod/rename. If pubKey is nil we're in the
	// explicit-opt-in unsigned path (Apply already gated on AllowUnsigned).
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
// and symlink-resolved paths so dpkg diversions (e.g. /usr/bin/lynx →
// /opt/lynx/lynx) aren't missed.
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
