//go:build linux

package landlock

import (
	"strings"
	"testing"

	"golang.org/x/sys/unix"
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

func TestLandlockFSMask_ABI1(t *testing.T) {
	mask := landlockFSMask(1)
	if mask == 0 {
		t.Error("ABI 1 mask should be non-zero")
	}
	// REFER is ABI >= 2; must not appear in ABI 1 mask.
	if mask&unix.LANDLOCK_ACCESS_FS_REFER != 0 {
		t.Error("ABI 1 mask must not include LANDLOCK_ACCESS_FS_REFER")
	}
}

func TestLandlockFSMask_ABI2IncludesRefer(t *testing.T) {
	mask := landlockFSMask(2)
	if mask&unix.LANDLOCK_ACCESS_FS_REFER == 0 {
		t.Error("ABI 2 mask must include LANDLOCK_ACCESS_FS_REFER")
	}
}

func TestLandlockFSMask_ABI3IncludesTruncate(t *testing.T) {
	mask := landlockFSMask(3)
	if mask&unix.LANDLOCK_ACCESS_FS_TRUNCATE == 0 {
		t.Error("ABI 3 mask must include LANDLOCK_ACCESS_FS_TRUNCATE")
	}
}

func TestLandlockFSMask_MonotonicallyGrows(t *testing.T) {
	m1 := landlockFSMask(1)
	m2 := landlockFSMask(2)
	m3 := landlockFSMask(3)
	if m2 < m1 {
		t.Errorf("ABI 2 mask (%x) < ABI 1 mask (%x)", m2, m1)
	}
	if m3 < m2 {
		t.Errorf("ABI 3 mask (%x) < ABI 2 mask (%x)", m3, m2)
	}
}

func TestAddPathRule_RelativePath(t *testing.T) {
	err := addPathRule(0, PathAccess{Path: "relative/path", Read: true}, 0xffffffff)
	if err == nil {
		t.Fatal("expected error for relative path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApply_EmptyRuleset_SupportedKernel(t *testing.T) {
	if !Supported() {
		t.Skip("Landlock not supported on this kernel")
	}
	// Empty ruleset: Landlock creates a ruleset with no rules, then restricts.
	// This is a valid (if strict) sandbox. We cannot un-restrict, so skip in
	// this process — the test just verifies no error path is triggered before
	// restrict_self.
	t.Skip("applying Landlock would restrict the test runner process permanently")
}
