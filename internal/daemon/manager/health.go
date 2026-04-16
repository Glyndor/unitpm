package manager

import (
	"context"
	"net/http"
	"os/exec"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

// Default probe tuning, used when the spec leaves a field zero.
const (
	defaultHealthInterval = 10 * time.Second
	defaultHealthTimeout  = 3 * time.Second
	defaultHealthFails    = 3
)

// startHealthProbe launches a background goroutine that probes the process
// every IntervalMs. On FailThreshold consecutive failures it triggers a
// Restart() — the normal restart policy (max retries, backoff, stop-on-exit)
// still applies downstream, so a misbehaving probe can't create an infinite
// restart loop unless the user configured --restart always.
//
// It is a no-op when spec.Health is nil. The returned cancel func is stored
// on Process and called from monitor() when the process exits naturally or
// from Stop().
func (p *Process) startHealthProbe() {
	h := p.spec.Health
	if h == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.healthCancel = cancel

	interval := time.Duration(h.IntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = defaultHealthInterval
	}
	timeout := time.Duration(h.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultHealthTimeout
	}
	threshold := h.FailThreshold
	if threshold <= 0 {
		threshold = defaultHealthFails
	}

	go p.healthLoop(ctx, h, interval, timeout, threshold)
}

// stopHealthProbe cancels the active probe goroutine, if any.
func (p *Process) stopHealthProbe() {
	if p.healthCancel != nil {
		p.healthCancel()
		p.healthCancel = nil
	}
}

func (p *Process) healthLoop(
	ctx context.Context,
	h *protocol.AppHealth,
	interval, timeout time.Duration,
	threshold int,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fails := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		ok := probeOnce(ctx, h, timeout, p.spec.Cwd)
		if ok {
			fails = 0
			continue
		}
		fails++
		if fails < threshold {
			continue
		}

		// Threshold hit: trigger restart and reset counter. The restart
		// machinery will log the event via info.Restarts. This goroutine
		// dies either way — Restart() -> Stop() -> stopHealthProbe via
		// the monitor exit path, or a new goroutine is started on the
		// next Start().
		_ = p.Restart() //nolint:errcheck
		return
	}
}

// probeOnce executes a single health probe and returns true on success.
func probeOnce(ctx context.Context, h *protocol.AppHealth, timeout time.Duration, cwd string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch h.Type {
	case "http":
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, h.URL, nil)
		if err != nil {
			return false
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode >= 200 && resp.StatusCode < 300
	case "exec":
		cmd := exec.CommandContext(probeCtx, "/bin/sh", "-c", h.Exec)
		if cwd != "" {
			cmd.Dir = cwd
		}
		return cmd.Run() == nil
	}
	return false
}
