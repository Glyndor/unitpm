package update

import (
	"runtime"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/updater"
)

func TestFindDebAsset_PrefersArchMatch(t *testing.T) {
	rel := &updater.Release{Assets: []updater.Asset{
		{Name: "lynx_1.0.0_other.deb", BrowserDownloadURL: "https://example/other.deb"},
		{Name: "lynx_1.0.0_" + runtime.GOARCH + ".deb", BrowserDownloadURL: "https://example/arch.deb"},
		{Name: "lynx_1.0.0_other2.deb", BrowserDownloadURL: "https://example/other2.deb"},
	}}
	got := findDebAsset(rel)
	if got != "https://example/arch.deb" {
		t.Errorf("expected arch-match URL, got %q", got)
	}
}

func TestFindDebAsset_FallbackAnyDeb(t *testing.T) {
	rel := &updater.Release{Assets: []updater.Asset{
		{Name: "lynx_1.0.0_unknownarch.deb", BrowserDownloadURL: "https://example/any.deb"},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example/checksums.txt"},
	}}
	got := findDebAsset(rel)
	if !strings.HasSuffix(got, ".deb") {
		t.Errorf("expected fallback .deb URL, got %q", got)
	}
}

func TestFindDebAsset_NoneFound(t *testing.T) {
	rel := &updater.Release{Assets: []updater.Asset{
		{Name: "checksums.txt", BrowserDownloadURL: "https://example/checksums.txt"},
		{Name: "lynx_1.0.0_amd64.tar.gz", BrowserDownloadURL: "https://example/tarball"},
	}}
	if got := findDebAsset(rel); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFindDebAsset_EmptyAssets(t *testing.T) {
	if got := findDebAsset(&updater.Release{}); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
