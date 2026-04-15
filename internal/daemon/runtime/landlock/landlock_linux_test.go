//go:build linux

package landlock

import (
	"testing"
)

func TestSupported(t *testing.T) {
	// Just confirm the probe doesn't crash. Result depends on kernel.
	_ = Supported()
}

func TestSensibleDefaults(t *testing.T) {
	rs := SensibleDefaults("/home/user/app", "/var/log/app")
	if len(rs.Allow) < 10 {
		t.Errorf("expected >=10 allow entries in defaults, got %d", len(rs.Allow))
	}

	// Confirm cwd and logDir are both present.
	var sawCwd, sawLog bool
	for _, a := range rs.Allow {
		if a.Path == "/home/user/app" {
			sawCwd = true
		}
		if a.Path == "/var/log/app" {
			sawLog = true
		}
	}
	if !sawCwd {
		t.Error("cwd not in default allowlist")
	}
	if !sawLog {
		t.Error("logDir not in default allowlist")
	}
}

func TestAccessMask(t *testing.T) {
	const allFlags uint64 = 0xffffffff
	cases := []struct {
		pa   PathAccess
		want bool // non-zero mask expected
	}{
		{PathAccess{Read: true}, true},
		{PathAccess{Write: true}, true},
		{PathAccess{Execute: true}, true},
		{PathAccess{}, false},
	}
	for _, c := range cases {
		m := accessMask(c.pa, allFlags)
		if (m != 0) != c.want {
			t.Errorf("accessMask(%+v) = %x, want non-zero=%v", c.pa, m, c.want)
		}
	}
}

func TestApply_NoOpWhenUnsupported(t *testing.T) {
	// Apply must return nil even on kernels without Landlock.
	// On a supporting kernel this still applies the restriction — but this
	// test runs in-process so we intentionally use an empty allow list that
	// allows nothing. We only verify no panic and no returned error.
	// Skip on supported kernels because restricting the test runner is bad.
	if Supported() {
		t.Skip("would restrict the test runner on supporting kernels")
	}
	if err := Apply(Ruleset{}); err != nil {
		t.Errorf("expected nil on unsupported kernel, got %v", err)
	}
}
