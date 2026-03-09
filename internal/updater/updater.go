// Package updater handles checking and applying updates from GitHub Releases.
package updater

import (
	"context"
	"encoding/json"
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
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// #nosec G704 // URL is hardcoded with constants repoOwner and repoName
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

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

// Apply downloads and applies the update.
func Apply(ctx context.Context, release *Release) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}

	// Resolve symlinks (e.g., if running from /usr/bin/lynx -> /opt/lynx/lynx)
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// Find compatible asset
	assetURL := ""
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Expected pattern: lynx_linux_amd64 or lynx_linux_arm64
	target := fmt.Sprintf("lynx_%s_%s", osName, arch)

	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, target) {
			assetURL = asset.BrowserDownloadURL
			break
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

	// Download and replace
	return downloadAndReplace(ctx, assetURL, exePath)
}

func downloadAndReplace(ctx context.Context, assetURL, exePath string) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(exePath), "lynx-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file (check permissions): %w", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up on error, but we'll rename if successful

	// Download
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to create download request: %w", err)
	}

	// Use a client with a timeout to prevent indefinite hangs.
	downloadClient := &http.Client{Timeout: 10 * time.Minute}

	// #nosec G704 // assetURL comes from the Github API response
	resp, err := downloadClient.Do(req)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Limit body size to prevent disk exhaustion.
	_, err = io.Copy(tmpFile, io.LimitReader(resp.Body, maxDownloadSize))
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close() // Close before chmod/rename
	if err != nil {
		return fmt.Errorf("failed to write update file: %w", err)
	}

	// Make executable (use os.Chmod on path since file is already closed).
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	// Replace binary
	// On Linux, we can rename over a running binary (it stays open for the running process, new process gets new file)
	cleanExePath := filepath.Clean(exePath)
	// #nosec G703 // cleanExePath is derived from os.Executable() and sanitized
	if err := os.Rename(tmpFile.Name(), cleanExePath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	return nil
}

// IsManagedByPackageSystem tries to detect if the binary is managed by apt/dpkg.
func IsManagedByPackageSystem() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return false
	}

	// Common system paths
	if strings.HasPrefix(exePath, "/usr/bin") || strings.HasPrefix(exePath, "/bin") {
		// Check if dpkg knows about it
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// #nosec G204 // exePath is safely derived from os.Executable
		cmd := exec.CommandContext(ctx, "dpkg", "-S", exePath)
		if err := cmd.Run(); err == nil {
			return true
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
