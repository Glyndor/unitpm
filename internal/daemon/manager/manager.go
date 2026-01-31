//go:build linux

// Package manager implements the core process management logic.
package manager

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/types"
)

// Manager handles the lifecycle of managed processes.
type Manager struct {
	mu        sync.RWMutex
	processes map[string]*Process
}

// NewManager creates a new process manager.
func NewManager() *Manager {
	return &Manager{
		processes: make(map[string]*Process),
	}
}

// Start creates and starts a new process.
//
// Deprecated: Use StartWithSpec instead.
func (m *Manager) Start(_, _ string) (string, error) {
	// This legacy method doesn't support IDs, so we'd have to gen one or error out.
	// For now, let's just error or not support it fully as it's deprecated.
	// Or mock a spec.
	return "", errors.New("deprecated: use StartWithSpec")
}

// StartWithSpec creates and starts a new process based on the spec.
func (m *Manager) StartWithSpec(spec protocol.AppSpec) (types.ProcessInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[spec.ID]; exists {
		return types.ProcessInfo{}, fmt.Errorf("process with ID %s already exists", spec.ID)
	}

	proc, err := NewProcess(spec.ID, spec)
	if err != nil {
		return types.ProcessInfo{}, err
	}

	if err := proc.Start(); err != nil {
		return types.ProcessInfo{}, err
	}

	m.processes[spec.ID] = proc
	return proc.Info(), nil
}

// Get returns a process by ID.
func (m *Manager) Get(id string) (*Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.processes[id]
	return p, ok
}

// Stop signals a process to stop.
func (m *Manager) Stop(id string) error {
	m.mu.RLock()
	proc, exists := m.processes[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process not found: %s", id)
	}

	return proc.Stop()
}

// Delete stops a process and removes it from the manager.
func (m *Manager) Delete(id string) error {
	// Best effort stop
	_ = m.Stop(id)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[id]; !exists {
		return fmt.Errorf("process not found: %s", id)
	}

	delete(m.processes, id)
	return nil
}

// Restart restarts a process.
func (m *Manager) Restart(id string) error {
	m.mu.RLock()
	proc, exists := m.processes[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process not found: %s", id)
	}

	// Manual restart resets backoff
	proc.ResetBackoff()

	return proc.Restart()
}

// ResolveID resolves an identifier (ID, prefix, or name) to a unique ID.
func (m *Manager) ResolveID(identifier string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. Exact ID match
	if _, exists := m.processes[identifier]; exists {
		return identifier, nil
	}

	var candidates []string

	// 2. Prefix Match
	for id := range m.processes {
		if strings.HasPrefix(id, identifier) {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) > 0 {
		goto CheckCandidates
	}

	// 3. Name Match
	for id, proc := range m.processes {
		if proc.info.Name == identifier {
			candidates = append(candidates, id)
		}
	}

CheckCandidates:
	if len(candidates) == 0 {
		return "", fmt.Errorf("process not found: %s", identifier)
	}
	if len(candidates) > 1 {
		return "", fmt.Errorf("ambiguous selector '%s': matches %v", identifier, candidates)
	}

	return candidates[0], nil
}

// List returns a snapshot of all managed processes.
func (m *Manager) List() []types.ProcessInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]types.ProcessInfo, 0, len(m.processes))
	for _, proc := range m.processes {
		list = append(list, proc.Info())
	}
	return list
}

// Shutdown stops all processes.
func (m *Manager) Shutdown() {
	m.mu.RLock()
	// Create a copy of IDs to avoid holding lock during Stop calls if Stop takes time
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	for _, id := range ids {
		_ = m.Stop(id)
	}
}
