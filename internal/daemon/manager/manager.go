//go:build linux

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
//
// Deprecated: Use StartWithSpec instead.
func (m *Manager) Start(name, command string) (int, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return 0, os.ErrInvalid
	}

	spec := protocol.StartSpec{
		Name:  name,
		Cmd:   parts[0],
		Args:  parts[1:],
		RunAs: protocol.RunAsPolicy{Mode: "self"},
	}

	info, err := m.StartWithSpec(spec)
	if err != nil {
		return 0, err
	}
	return info.ID, nil
}

// StartWithSpec creates and starts a new process based on the spec.
func (m *Manager) StartWithSpec(spec protocol.StartSpec) (types.ProcessInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID
	m.nextID++

	proc, err := NewProcess(id, spec)
	if err != nil {
		return types.ProcessInfo{}, err
	}

	if err := proc.Start(); err != nil {
		return types.ProcessInfo{}, err
	}

	m.processes[id] = proc
	return proc.Info(), nil
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
