//go:build linux

package main

import (
	"os/user"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/paths"
)

func TestAuditPath(t *testing.T) {
	if got := auditPath(false); got != "" {
		t.Errorf("auditPath(false)=%q want empty", got)
	}
	got := auditPath(true)
	if !strings.HasPrefix(got, paths.LogRoot) || !strings.HasSuffix(got, "audit.log") {
		t.Errorf("auditPath(true)=%q", got)
	}
}

func TestIsSystemDaemon(t *testing.T) {
	got := isSystemDaemon()
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current: %v", err)
	}
	want := paths.IsRoot() || cur.Username == "lynx"
	if got != want {
		t.Errorf("isSystemDaemon=%v want %v (root=%v user=%q)", got, want, paths.IsRoot(), cur.Username)
	}
}
