package registry

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/help"
)

func TestRegisterAndResolve(t *testing.T) {
	// Reset state (although global state is bad for tests, this is a simple registry)
	// We can't easily reset unexported variables without export_test.go
	// But we can just add new commands.

	spec := help.CommandSpec{
		Name:    "test-cmd",
		Aliases: []string{"tc", "tcmd"},
	}

	Register(spec)

	// Resolve canonical
	name, ok := Resolve("test-cmd")
	if !ok || name != "test-cmd" {
		t.Errorf("Resolve(test-cmd) = %s, %v; want test-cmd, true", name, ok)
	}

	// Resolve alias
	name, ok = Resolve("tc")
	if !ok || name != "test-cmd" {
		t.Errorf("Resolve(tc) = %s, %v; want test-cmd, true", name, ok)
	}

	// Resolve case insensitive
	name, ok = Resolve("TC")
	if !ok || name != "test-cmd" {
		t.Errorf("Resolve(TC) = %s, %v; want test-cmd, true", name, ok)
	}
}

func TestGetAll(t *testing.T) {
	// Since tests run in parallel or sequentially, GetAll might return other registered commands.
	// We just check if our registered command is present.
	
	all := GetAll()
	found := false
	for _, s := range all {
		if s.Name == "test-cmd" {
			found = true
			break
		}
	}

	if !found {
		t.Error("GetAll() did not return registered command 'test-cmd'")
	}
}
