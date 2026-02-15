//go:build linux

package startup //nolint:testpackage

import (
	"strings"
)

// MockRunner is a mock implementation of Runner.
type MockRunner struct {
	Calls []string // Log of called commands "name arg1 arg2..."
	// Responses maps a command prefix (e.g. "systemctl is-active") to a response.
	// We check if the actual command starts with the key.
	Responses map[string]MockResult
}

type MockResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func (m *MockRunner) Run(name string, args ...string) (string, string, int, error) {
	cmdStr := name
	if len(args) > 0 {
		cmdStr += " " + strings.Join(args, " ")
	}
	m.Calls = append(m.Calls, cmdStr)

	// Find best match
	for k, v := range m.Responses {
		if strings.HasPrefix(cmdStr, k) {
			return v.Stdout, v.Stderr, v.ExitCode, v.Err
		}
	}

	return "", "", 0, nil
}

func (m *MockRunner) Reset() {
	m.Calls = []string{}
	m.Responses = make(map[string]MockResult)
}
