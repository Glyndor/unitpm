package updater

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/version"
)

// newServer boots an httptest.Server that returns the given release JSON
// from /releases/latest (path suffix). Swaps releasesURL for the test and
// restores it on cleanup.
func newServer(t *testing.T, release Release, status int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(release)
	})
	srv := httptest.NewServer(mux)
	orig := releasesURL
	releasesURL = srv.URL + "/releases/latest"
	t.Cleanup(func() {
		releasesURL = orig
		srv.Close()
	})
	return srv
}

func TestHTTPGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("hello"))
		case "/big":
			_, _ = w.Write([]byte("0123456789abcdef"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	body, err := httpGet(context.Background(), srv.URL+"/ok", 5e9, 0)
	if err != nil || string(body) != "hello" {
		t.Errorf("ok: body=%q err=%v", body, err)
	}

	body, err = httpGet(context.Background(), srv.URL+"/big", 5e9, 8)
	if err != nil || len(body) != 8 {
		t.Errorf("limited: len=%d err=%v", len(body), err)
	}

	if _, err := httpGet(context.Background(), srv.URL+"/missing", 5e9, 0); err == nil {
		t.Error("expected error on 404, got nil")
	}
}

func TestCheck_UpToDate(t *testing.T) {
	newServer(t, Release{TagName: version.Version}, 0)
	r, err := Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil on up-to-date, got %+v", r)
	}
}

func TestCheck_OlderAvailable_NoDowngrade(t *testing.T) {
	newServer(t, Release{TagName: "v0.0.1"}, 0)
	r, err := Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil for older server version, got %+v", r)
	}
}

func TestCheck_NewerAvailable(t *testing.T) {
	newer := Release{
		TagName: "v99.99.99",
		HTMLURL: "https://example.com/release",
		Assets: []Asset{
			{Name: "lynxpm_linux_amd64", BrowserDownloadURL: "https://example.com/bin"},
		},
	}
	newServer(t, newer, 0)
	r, err := Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if r == nil {
		t.Fatal("expected a release payload")
	}
	if r.TagName != "v99.99.99" {
		t.Errorf("tag: %s", r.TagName)
	}
}

func TestCheck_HTTPError(t *testing.T) {
	newServer(t, Release{}, http.StatusInternalServerError)
	_, err := Check(context.Background())
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestCheck_BadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	srv := httptest.NewServer(mux)
	orig := releasesURL
	releasesURL = srv.URL + "/releases/latest"
	t.Cleanup(func() {
		releasesURL = orig
		srv.Close()
	})
	_, err := Check(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.0", "0.9.9", true},
		{"0.9.9", "1.0.0", false},
		{"1.2.3", "1.2.3", false},
		{"1.0.1", "1.0.0", true},
		{"2.0.0", "1.99.99", true},
		{"1.10.0", "1.2.0", true},
	}
	for _, c := range cases {
		if got := isNewer(c.a, c.b); got != c.want {
			t.Errorf("isNewer(%s,%s)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"0.4.11", [3]int{0, 4, 11}},
		{"1.0", [3]int{1, 0, 0}},
		{"abc", [3]int{0, 0, 0}},
	}
	for _, c := range cases {
		got := parseVersion(c.in)
		if got != c.want {
			t.Errorf("parseVersion(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIsManagedByPackageSystem_NoPanic(t *testing.T) {
	_ = IsManagedByPackageSystem()
}

func TestApply_NoCompatibleBinary(t *testing.T) {
	// Release ships only a foo_bar asset — nothing matches current GOOS/GOARCH.
	release := &Release{
		TagName: "v99.0.0",
		Assets: []Asset{
			{Name: "irrelevant-asset", BrowserDownloadURL: "https://example.com/x"},
		},
	}
	err := Apply(context.Background(), release, ApplyOptions{AllowUnsigned: true})
	if err == nil {
		t.Fatal("expected error when no compatible asset is present")
	}
	if !strings.Contains(err.Error(), "no compatible binary") {
		t.Errorf("expected 'no compatible binary' error, got: %v", err)
	}
}

func TestApply_MissingSignatureRequiresFlag(t *testing.T) {
	// Release has the binary but no .sig asset. Without AllowUnsigned, Apply
	// must refuse with ErrSignatureRequired.
	target := "lynxpm_" + runtime.GOOS + "_" + runtime.GOARCH
	release := &Release{
		TagName: "v99.0.0",
		Assets: []Asset{
			{Name: target, BrowserDownloadURL: "https://example.invalid/bin"},
		},
	}
	err := Apply(context.Background(), release, ApplyOptions{AllowUnsigned: false})
	if err == nil {
		t.Fatal("expected ErrSignatureRequired when .sig asset is missing")
	}
	if !errors.Is(err, ErrSignatureRequired) {
		t.Errorf("expected ErrSignatureRequired, got: %v", err)
	}
}

func TestApply_AllowUnsignedBypassesSigCheck(t *testing.T) {
	// With AllowUnsigned=true the pre-download signature check is skipped.
	// Apply will still fail because the download URL is bogus, but the error
	// must NOT be ErrSignatureRequired.
	target := "lynxpm_" + runtime.GOOS + "_" + runtime.GOARCH
	release := &Release{
		TagName: "v99.0.0",
		Assets: []Asset{
			{Name: target, BrowserDownloadURL: "https://127.0.0.1:1/bin"},
		},
	}
	err := Apply(context.Background(), release, ApplyOptions{AllowUnsigned: true})
	if err == nil {
		t.Fatal("expected network error from bogus download URL")
	}
	if errors.Is(err, ErrSignatureRequired) {
		t.Errorf("ErrSignatureRequired surfaced despite AllowUnsigned=true: %v", err)
	}
}
