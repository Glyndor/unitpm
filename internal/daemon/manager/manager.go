// Package manager implements the core process management logic.
package manager

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
			return types.ProcessInfo{}, fmt.Errorf(
				"ERR_LIMITS: invalid LYNX_MAX_PROCESSES: %w",
				err,
			)
		}
		if limit <= 0 {
			return types.ProcessInfo{}, errors.New("ERR_LIMITS: LYNX_MAX_PROCESSES must be > 0")
		}
		if len(m.processes) >= limit {
			return types.ProcessInfo{}, errors.New("ERR_LIMITS: max processes reached")
		}
	}

	if spec.Namespace == "" {
		spec.Namespace = DefaultNamespace
	}

	if _, exists := m.processes[spec.ID]; exists {
		return types.ProcessInfo{}, fmt.Errorf("process with ID %s already exists", spec.ID)
	}

	// Enforce (namespace, name) uniqueness so `namespace:name` resolution
	// stays unambiguous.
	for _, existing := range m.processes {
		if existing.info.Namespace == spec.Namespace && existing.info.Name == spec.Name {
			return types.ProcessInfo{}, fmt.Errorf(
				"ERR_CONFLICT: a process named %q already exists in namespace %q",
				spec.Name, spec.Namespace,
			)
		}
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

// Reset zeroes the Restarts counter and internal backoff state for a process
// without restarting it. Useful after resolving a crash loop.
func (m *Manager) Reset(id string) error {
	m.mu.RLock()
	proc, exists := m.processes[id]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("process not found: %s", id)
	}
	proc.resetMetrics()
	return nil
}

// Scale brings the number of running processes whose name matches
// "<base>" or "<base>-N" (within the given namespace) to target. It uses
// the spec of an existing instance as the template for new instances.
// Returns an error if no instance exists to use as template.
func (m *Manager) Scale(namespace, base string, target int) (*protocol.ScaleResponse, error) {
	if target < 0 {
		return nil, fmt.Errorf("ERR_BAD_REQUEST: target count must be >= 0")
	}
	if target > 1024 {
		return nil, fmt.Errorf("ERR_LIMITS: target count must be <= 1024")
	}
	if namespace == "" {
		namespace = DefaultNamespace
	}

	// Snapshot atomically: names, IDs, and a cloned template spec. This
	// avoids a race where a concurrent Delete drops members[0] between
	// scaleMembers() and the template read below.
	snap := m.scaleSnapshot(namespace, base)
	res := &protocol.ScaleResponse{BaseName: base, Namespace: namespace, Before: len(snap.names)}

	switch {
	case target == len(snap.names):
		res.After = target
		return res, nil
	case target < len(snap.names):
		// Stop+delete the highest-indexed members first so the lower
		// indices stay stable for the caller's mental model.
		for i := len(snap.names) - 1; i >= target; i-- {
			name := snap.names[i]
			id := snap.ids[i]
			if err := m.Delete(id); err != nil {
				return res, fmt.Errorf("scale down: delete %s: %w", name, err)
			}
			res.Deleted = append(res.Deleted, name)
		}
	case target > len(snap.names):
		if len(snap.names) == 0 {
			return nil, fmt.Errorf(
				"ERR_NOT_FOUND: no existing instance of %q in namespace %q to use as template",
				base, namespace,
			)
		}
		template := snap.template
		taken := make(map[string]bool, len(snap.names))
		for _, n := range snap.names {
			taken[n] = true
		}
		next := 1
		for added := 0; added < target-len(snap.names); added++ {
			name := fmt.Sprintf("%s-%d", base, next)
			for taken[name] {
				next++
				name = fmt.Sprintf("%s-%d", base, next)
			}
			taken[name] = true
			newSpec := template
			newSpec.Name = name
			newSpec.Namespace = namespace
			id, err := spec2.GenerateID()
			if err != nil {
				return res, fmt.Errorf("scale up: generate id: %w", err)
			}
			newSpec.ID = id
			newSpec.CreatedAt = time.Now().Format(time.RFC3339)
			if newSpec.Env == nil {
				newSpec.Env = map[string]string{}
			}
			newSpec.Env["LYNX_INSTANCE"] = strconv.Itoa(added + len(snap.names))

			if _, err := spec2.SaveSpec(newSpec.ID, newSpec); err != nil {
				return res, fmt.Errorf("scale up: save spec: %w", err)
			}
			if _, err := m.StartWithSpec(newSpec); err != nil {
				_ = spec2.DeleteSpec(newSpec.ID)
				return res, fmt.Errorf("scale up: start %s: %w", name, err)
			}
			res.Created = append(res.Created, name)
			next++
		}
	}

	res.After = target
	return res, nil
}

// scaleSnapshot takes an atomic read-lock snapshot of all processes in
// namespace whose name is exactly `base` or matches `base-<N>`. Names/ids
// are ordered so the bare name (if present) comes first, then numeric
// suffix ascending. The first member's spec is cloned as the scale-up
// template while still holding the lock, which prevents a TOCTOU race
// with a concurrent Delete.
type scaleSnap struct {
	names    []string
	ids      []string
	template protocol.AppSpec
}

func (m *Manager) scaleSnapshot(namespace, base string) scaleSnap {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var bare *Process
	type idx struct {
		p *Process
		n int
	}
	withIdx := []idx{}
	prefix := base + "-"
	for _, p := range m.processes {
		if p.info.Namespace != namespace {
			continue
		}
		if p.info.Name == base {
			bare = p
			continue
		}
		if strings.HasPrefix(p.info.Name, prefix) {
			n, err := strconv.Atoi(p.info.Name[len(prefix):])
			if err != nil {
				continue
			}
			withIdx = append(withIdx, idx{p, n})
		}
	}
	sort.Slice(withIdx, func(i, j int) bool { return withIdx[i].n < withIdx[j].n })

	ordered := make([]*Process, 0, len(withIdx)+1)
	if bare != nil {
		ordered = append(ordered, bare)
	}
	for _, w := range withIdx {
		ordered = append(ordered, w.p)
	}

	snap := scaleSnap{
		names: make([]string, len(ordered)),
		ids:   make([]string, len(ordered)),
	}
	for i, p := range ordered {
		snap.names[i] = p.info.Name
		snap.ids[i] = p.info.ID
	}
	if len(ordered) > 0 {
		// Deep-copy via Spec() so Scale doesn't alias the original's Env map
		// or pointer fields (Logs, Restart, RunAs, Stop, Resources).
		snap.template = ordered[0].Spec()
	}
	return snap
}

// Reload reloads a process configuration from its spec file and restarts the process.
func (m *Manager) Reload(id string) error {
	s, err := spec2.LoadSpec(id)
	if err != nil {
		return fmt.Errorf("failed to load spec: %w", err)
	}

	if s.Namespace == "" {
		s.Namespace = DefaultNamespace
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
		return resolveFromCandidates(identifier, candidates)
	}

	// 1. Exact ID match
	if _, exists := m.processes[identifier]; exists {
		return identifier, nil
	}

	// 2. Prefix Match
	var candidates []string
	for id := range m.processes {
		if strings.HasPrefix(id, identifier) {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) > 0 {
		return resolveFromCandidates(identifier, candidates)
	}

	// 3. Name Match
	for id, proc := range m.processes {
		if proc.info.Name == identifier {
			candidates = append(candidates, id)
		}
	}

	return resolveFromCandidates(identifier, candidates)
}

// resolveFromCandidates returns the single match or an appropriate error.
func resolveFromCandidates(identifier string, candidates []string) (string, error) {
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("process not found: %s (run 'lynx list' to see all processes)", identifier)
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("ambiguous selector '%s': matches %d processes %v", identifier, len(candidates), candidates)
	}
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

// Shutdown gracefully stops all processes without marking them as disabled,
// so they are restored on daemon restart (reboot, re-exec, crash recovery).
func (m *Manager) Shutdown() {
	m.mu.RLock()
	procs := make([]*Process, 0, len(m.processes))
	for _, p := range m.processes {
		procs = append(procs, p)
	}
	m.mu.RUnlock()

	for _, p := range procs {
		_ = p.Stop(false)
	}
}
