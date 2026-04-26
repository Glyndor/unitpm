//go:build linux

package paths

import "testing"

func TestIsRoot(t *testing.T) {
	prev := currentEuid
	t.Cleanup(func() { currentEuid = prev })

	currentEuid = func() int { return 0 }
	if !IsRoot() {
		t.Error("IsRoot() = false for euid 0, want true")
	}

	currentEuid = func() int { return 1000 }
	if IsRoot() {
		t.Error("IsRoot() = true for euid 1000, want false")
	}
}

func TestWithinRoot(t *testing.T) {
	cases := []struct {
		name string
		root string
		path string
		want bool
	}{
		{"inside", "/var/log/lynx-pm", "/var/log/lynx-pm/app/stdout.log", true},
		{"equal", "/var/log/lynx-pm", "/var/log/lynx-pm", true},
		{"escape", "/var/log/lynx-pm", "/etc/passwd", false},
		{"sibling", "/var/log/lynx-pm", "/var/log/other", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := WithinRoot(c.root, c.path); got != c.want {
				t.Errorf("WithinRoot(%q,%q)=%v, want %v", c.root, c.path, got, c.want)
			}
		})
	}
}
