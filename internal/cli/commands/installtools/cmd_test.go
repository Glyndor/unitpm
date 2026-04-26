//go:build linux

package installtools_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/installtools"
)

func TestGetSpec(t *testing.T) {
	spec := installtools.GetSpec()
	if spec.Name != "install-tools" {
		t.Errorf("expected name 'install-tools', got %s", spec.Name)
	}
	if spec.Description == "" {
		t.Error("expected non-empty description")
	}
	// Ensure --system option is documented
	found := false
	for _, opt := range spec.Options {
		if strings.Contains(opt.Long, "--system") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected --system flag in options")
	}
}

func TestRun_Help(t *testing.T) {
	err := installtools.Run([]string{"--help"})
	if err != nil {
		t.Errorf("Run(--help) failed: %v", err)
	}
}

func TestRun_SystemWithoutRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test non-root branch when running as root")
	}
	err := installtools.Run([]string{"--system", "-y"})
	if err == nil {
		t.Fatal("expected error when --system used without root")
	}
	if !strings.Contains(err.Error(), "requires root") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_UserMode(t *testing.T) {
	// User mode (default): no root needed. Point HOME to temp dir.
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Auto-yes so it doesn't prompt
	err := installtools.Run([]string{"-y"})
	if err != nil {
		t.Errorf("expected no error in user mode, got %v", err)
	}
	// ~/.local/bin should now exist
	if _, err := os.Stat(home + "/.local/bin"); err != nil {
		t.Errorf("expected ~/.local/bin to be created, got %v", err)
	}
}

func TestRun_UserMode_LongYes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	err := installtools.Run([]string{"--yes"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// stageFakeTools puts a real binary on PATH under each name commonly known to the
// installer, so the planner ends up with non-empty actions to perform.
func stageFakeTools(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	src := "/bin/true"
	if _, err := os.Stat(src); err != nil {
		src = "/usr/bin/true"
	}
	for _, name := range []string{"bun", "node", "python3"} {
		dst := tmp + "/" + name
		if err := os.Symlink(src, dst); err != nil {
			t.Skipf("symlink: %v", err)
		}
	}
	t.Setenv("PATH", tmp+":"+os.Getenv("PATH"))
	return tmp
}

func TestRun_UserMode_LinksTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stageFakeTools(t)

	if err := installtools.Run([]string{"-y"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, name := range []string{"bun", "node", "python3"} {
		link := home + "/.local/bin/" + name
		fi, err := os.Lstat(link)
		if err != nil {
			t.Errorf("missing symlink for %s: %v", name, err)
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is not a symlink", name)
		}
	}
}

func TestRun_UserMode_PromptDeny(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stageFakeTools(t)
	withStdin(t, "n\n")
	if err := installtools.Run(nil); err != nil {
		t.Errorf("Run: %v", err)
	}
	// Nothing should have been linked.
	if entries, _ := os.ReadDir(home + "/.local/bin"); len(entries) != 0 {
		t.Errorf("expected no links after deny, got %d", len(entries))
	}
}

func TestRun_UserMode_PromptChooseAllNo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stageFakeTools(t)
	// "choose" then say "n" enough times to reject every staged tool.
	withStdin(t, "choose\n"+strings.Repeat("n\n", 32))
	if err := installtools.Run(nil); err != nil {
		t.Errorf("Run: %v", err)
	}
	if entries, _ := os.ReadDir(home + "/.local/bin"); len(entries) != 0 {
		t.Errorf("expected no links after rejecting all, got %d", len(entries))
	}
}

func TestRun_UserMode_PromptDefaultYes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stageFakeTools(t)
	// Empty input → default Yes; followed by enough newlines to drain readers.
	withStdin(t, "\n")
	if err := installtools.Run(nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if entries, _ := os.ReadDir(home + "/.local/bin"); len(entries) == 0 {
		t.Error("expected default-yes prompt to create symlinks")
	}
}

func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = w.Close()
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}
