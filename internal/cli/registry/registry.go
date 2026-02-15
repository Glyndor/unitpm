// Package registry provides a central command registry for the CLI.
//go:build linux

package registry

import (
	"sort"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/help"
)

var (
	// specs is a map of canonical command name to command specification.
	specs = make(map[string]help.CommandSpec)
	// aliases maps alias names to canonical command names.
	aliases = make(map[string]string)
)

// Register registers a command specification.
// It overwrites any existing command with the same name.
// This should be called during CLI startup.
func Register(spec help.CommandSpec) {
	normName := normalize(spec.Name)
	specs[normName] = spec

	for _, alias := range spec.Aliases {
		aliases[normalize(alias)] = normName
	}
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
	normName := normalize(name)

	// Check canonical names first
	if spec, ok := specs[normName]; ok {
		return spec.Name, true
	}

	// Check aliases
	if canonicalName, ok := aliases[normName]; ok {
		if spec, ok := specs[canonicalName]; ok {
			return spec.Name, true
		}
	}

	return "", false
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
