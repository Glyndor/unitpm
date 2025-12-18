// Package registry provides a central command registry for the CLI.
package registry

import (
	"sort"

	"github.com/Jaro-c/Lynx/internal/cli/help"
)

var (
	// specs is a map of canonical command name to command specification.
	specs = make(map[string]help.CommandSpec)
)

// Register registers a command specification.
// It overwrites any existing command with the same name.
// This should be called during CLI startup.
func Register(spec help.CommandSpec) {
	specs[spec.Name] = spec
}

// GetAll returns all registered command specifications, sorted by name.
func GetAll() []help.CommandSpec {
	result := make([]help.CommandSpec, 0, len(specs))
	for _, spec := range specs {
		result = append(result, spec)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// Resolve resolves a command name or alias to the primary command name.
// Returns the primary name and true if found, or empty string and false if not.
func Resolve(name string) (string, bool) {
	// Check canonical names first
	if spec, ok := specs[name]; ok {
		return spec.Name, true
	}

	// Check aliases
	for _, spec := range specs {
		for _, alias := range spec.Aliases {
			if alias == name {
				return spec.Name, true
			}
		}
	}

	return "", false
}
