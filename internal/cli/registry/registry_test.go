package registry_test

import (
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/cli/registry"
)

const testCmdName = "test-cmd"

func TestRegisterAndResolve(t *testing.T) {
	// Reset state (although global state is bad for tests, this is a simple registry)
	// We can't easily reset unexported variables without export_test.go
	// But we can just add new commands.

	spec := help.CommandSpec{
		Name:    testCmdName,
		Aliases: []string{"tc", "tcmd"},
	}

	registry.Register(spec)

	// Resolve canonical
	name, ok := registry.Resolve(testCmdName)
	if !ok || name != testCmdName {
		t.Errorf("Resolve(%s) = %s, %v; want %s, true", testCmdName, name, ok, testCmdName)
	}

	// Resolve alias
	name, ok = registry.Resolve("tc")
	if !ok || name != testCmdName {
		t.Errorf("Resolve(tc) = %s, %v; want %s, true", name, ok, testCmdName)
	}

	// Resolve case insensitive
	name, ok = registry.Resolve("TC")
	if !ok || name != testCmdName {
		t.Errorf("Resolve(TC) = %s, %v; want %s, true", name, ok, testCmdName)
	}
}

func TestGetAll(t *testing.T) {
	// Since tests run in parallel or sequentially, GetAll might return other registered commands.
	// We just check if our registered command is present.

	all := registry.GetAll()
	found := false
	for _, s := range all {
		if s.Name == testCmdName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("GetAll() did not return registered command '%s'", testCmdName)
	}
}
