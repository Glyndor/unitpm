// Package registry provides a central command registry for the CLI.
package registry

import (
	"github.com/Jaro-c/Lynx/internal/cli/help"
)

var (
	specs []help.CommandSpec
)

// Register registers a command specification.
// This should be called during CLI startup.
func Register(spec help.CommandSpec) {
	specs = append(specs, spec)
}

// GetAll returns all registered command specifications.
func GetAll() []help.CommandSpec {
	// Return a copy to be safe
	result := make([]help.CommandSpec, len(specs))
	copy(result, specs)
	return result
}

// Resolve resolves a command name or alias to the primary command name.
// Returns the primary name and true if found, or empty string and false if not.
func Resolve(name string) (string, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec.Name, true
		}
		for _, alias := range spec.Aliases {
			if alias == name {
				return spec.Name, true
			}
		}
	}
	return "", false
}
