// Package manager implements the core process management logic.
package manager

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	spec2 "github.com/Jaro-c/Lynx/internal/spec"
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

// Restore loads existing specs from disk and starts them.
func (m *Manager) Restore() error {
	specs, err := spec2.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load specs: %w", err)
	}

	log.Printf("Found %d specs to restore", len(specs))

	for _, s := range specs {
		if s.Disabled {
			log.Printf("Skipping disabled process: %s", s.Name)
			continue
		}
		log.Printf("Restoring process: %s (%s)", s.Name, s.ID)
		if _, err := m.StartWithSpec(s); err != nil {
			log.Printf("Error restoring process %s: %v", s.ID, err)
			// Continue restoring others
		}
	}

	return nil
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

	if limitStr := os.Getenv("LYNX_MAX_PROCESSES"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return types.ProcessInfo{}, fmt.Errorf("ERR_LIMITS: invalid LYNX_MAX_PROCESSES: %w", err)
		}
		if limit <= 0 {
			return types.ProcessInfo{}, errors.New("ERR_LIMITS: LYNX_MAX_PROCESSES must be > 0")
		}
		if len(m.processes) >= limit {
			return types.ProcessInfo{}, errors.New("ERR_LIMITS: max processes reached")
		}
	}

	if spec.Namespace == "" {
		spec.Namespace = "default"
	}

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

	// Ensure Disabled is false (in case it was restarted manually)
	if spec.Disabled {
		spec.Disabled = false
		if _, err := spec2.SaveSpec(spec.ID, spec); err != nil {
			log.Printf("Warning: failed to update spec for %s: %v", spec.ID, err)
		}
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

	if err := proc.Stop(true); err != nil {
		return err
	}

	// Persist Disabled state
	s := proc.Spec()
	s.Disabled = true
	if _, err := spec2.SaveSpec(s.ID, s); err != nil {
		log.Printf("Warning: failed to save disabled state for %s: %v", id, err)
	}

	return nil
}

// Delete stops a process and removes it from the manager.
func (m *Manager) Delete(id string) error {
	// Best effort stop
	_ = m.Stop(id) //nolint:errcheck

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

// Reload reloads a process configuration from its spec file and restarts the process.
func (m *Manager) Reload(id string) error {
	s, err := spec2.LoadSpec(id)
	if err != nil {
		return fmt.Errorf("failed to load spec: %w", err)
	}

	if s.Namespace == "" {
		s.Namespace = "default"
	}

	s.Disabled = false
	if _, err := spec2.SaveSpec(s.ID, *s); err != nil {
		log.Printf("Warning: failed to save spec for %s: %v", s.ID, err)
	}

	_ = m.Stop(id) //nolint:errcheck

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.processes, id)

	proc, err := NewProcess(id, *s)
	if err != nil {
		return err
	}
	if err := proc.Start(); err != nil {
		return err
	}

	m.processes[id] = proc
	return nil
}

// ResolveID resolves an identifier (ID, prefix, or name) to a unique ID.
func (m *Manager) ResolveID(identifier string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if idx := strings.Index(identifier, ":"); idx != -1 {
		ns := identifier[:idx]
		name := identifier[idx+1:]
		var candidates []string
		for id, proc := range m.processes {
			if proc.info.Namespace == ns && proc.info.Name == name {
				candidates = append(candidates, id)
			}
		}
		if len(candidates) == 0 {
			return "", fmt.Errorf("process not found: %s", identifier)
		}
		if len(candidates) > 1 {
			return "", fmt.Errorf("ambiguous selector '%s': matches %v", identifier, candidates)
		}
		return candidates[0], nil
	}

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
		_ = m.Stop(id) //nolint:errcheck
	}
}
