// Package manager implements the core process management logic.
package manager

import (
	"fmt"
	"os"
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
func (m *Manager) Start(name, command string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", os.ErrInvalid
	}

	// This legacy method doesn't support IDs, so we'd have to gen one or error out.
	// For now, let's just error or not support it fully as it's deprecated.
	// Or mock a spec.
	return "", fmt.Errorf("deprecated: use StartWithSpec")
}

// StartWithSpec creates and starts a new process based on the spec.
func (m *Manager) StartWithSpec(spec protocol.AppSpec) (types.ProcessInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[spec.Id]; exists {
		return types.ProcessInfo{}, fmt.Errorf("process with ID %s already exists", spec.Id)
	}

	proc, err := NewProcess(spec.Id, spec)
	if err != nil {
		return types.ProcessInfo{}, err
	}

	if err := proc.Start(); err != nil {
		return types.ProcessInfo{}, err
	}

	m.processes[spec.Id] = proc
	return proc.Info(), nil
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
