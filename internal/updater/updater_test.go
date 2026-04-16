package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestCheck_UpToDate(t *testing.T) {
	// Return the same version we're currently running — Check should
	// report nil (no update available).
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
	// Server offers an older version — Check must not recommend downgrade.
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
	// Bump major so we always test a newer version regardless of
	// current version.Version at build time.
	current := parseVersion(strings.TrimPrefix(version.Version, "v"))
	newer := Release{
		TagName: "v99.99.99",
		HTMLURL: "https://example.com/release",
		Assets: []Asset{
			{Name: "lynx_linux_amd64", BrowserDownloadURL: "https://example.com/bin"},
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
	_ = current
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
		{"1.10.0", "1.2.0", true}, // semver, not lexical
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
		{"1.0", [3]int{1, 0, 0}}, // short is OK
		{"abc", [3]int{0, 0, 0}}, // non-numeric falls back
	}
	for _, c := range cases {
		got := parseVersion(c.in)
		if got != c.want {
			t.Errorf("parseVersion(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestIsManagedByPackageSystem_NoPanic(t *testing.T) {
	// We can't deterministically control what dpkg-query returns on the
	// test host, but we can confirm the function does not panic and
	// returns a bool.
	_ = IsManagedByPackageSystem()
}
