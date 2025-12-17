// Package daemon implements the core process management logic.
package daemon

import (
	"fmt"
	"sync"

	"github.com/Jaro-c/Lynx/internal/types"
)

// Manager handles the lifecycle of managed processes.
type Manager struct {
	mu        sync.RWMutex
	processes map[int]*Process
	nextID    int
}

// NewManager creates a new process manager.
func NewManager() *Manager {
	return &Manager{
		processes: make(map[int]*Process),
		nextID:    0,
	}
}

// Start creates and starts a new process.
func (m *Manager) Start(name, command string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID
	m.nextID++

	proc, err := NewProcess(id, name, command)
	if err != nil {
		return 0, err
	}

	if err := proc.Start(); err != nil {
		return 0, err
	}

	m.processes[id] = proc
	return id, nil
}

// Stop signals a process to stop.
func (m *Manager) Stop(id int) error {
	m.mu.RLock()
	proc, exists := m.processes[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process not found: %d", id)
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
	defer m.mu.RUnlock()
	for _, proc := range m.processes {
		_ = proc.Stop() //nolint:errcheck // Best effort shutdown
	}
}
