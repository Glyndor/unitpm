package transport

import (
	"sync"
	"time"

	"github.com/Jaro-c/Lynx/internal/env"
)

// Rate-limit defaults: tight enough to stop a flood, generous enough that
// interactive CLI use (rapid-fire 'lynx start' in scripts) still works.
// Overridable via env vars on daemon startup.
const (
	defaultRateCapacity = 200 // burst
	defaultRateRefill   = 100 // tokens per second
	minRefillInterval   = 1 * time.Millisecond
)

// rateLimiter is a per-UID token bucket. It is safe for concurrent use.
// Zero value is not valid; use newRateLimiter.
type rateLimiter struct {
	mu           sync.Mutex
	buckets      map[uint32]*bucket
	capacity     int
	refillRate   float64 // tokens per second
	sweepCounter int
}

type bucket struct {
	tokens   float64
	lastFill time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		buckets:    make(map[uint32]*bucket),
		capacity:   env.Int("LYNX_IPC_RATE_BURST", defaultRateCapacity),
		refillRate: float64(env.Int("LYNX_IPC_RATE_PER_SEC", defaultRateRefill)),
	}
}

// idleEvictionWindow is how long a UID's bucket can sit idle before it
// is garbage-collected. Anything larger than this is equivalent to a
// fresh bucket on the caller's next request anyway.
const idleEvictionWindow = 5 * time.Minute

// allow reports whether a request from the given uid may proceed. It
// deducts one token on success. On first sight of a uid the bucket is
// initialized full. Periodically sweeps buckets idle for more than
// idleEvictionWindow to bound memory for long-lived daemons on hosts
// with many transient UIDs (systemd DynamicUser, container runtimes).
func (r *rateLimiter) allow(uid uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.maybeSweep(now)

	b, ok := r.buckets[uid]
	if !ok {
		b = &bucket{tokens: float64(r.capacity), lastFill: now}
		r.buckets[uid] = b
	} else {
		elapsed := now.Sub(b.lastFill)
		if elapsed > minRefillInterval {
			b.tokens += elapsed.Seconds() * r.refillRate
			if b.tokens > float64(r.capacity) {
				b.tokens = float64(r.capacity)
			}
			b.lastFill = now
		}
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// maybeSweep drops buckets idle for more than idleEvictionWindow. Called
// opportunistically from allow() while holding the lock, so no extra
// goroutine / ticker. sweepEveryN throttles the sweep so cost is amortized.
const sweepEveryN = 1024

func (r *rateLimiter) maybeSweep(now time.Time) {
	r.sweepCounter++
	if r.sweepCounter < sweepEveryN {
		return
	}
	r.sweepCounter = 0
	cutoff := now.Add(-idleEvictionWindow)
	for uid, b := range r.buckets {
		if b.lastFill.Before(cutoff) {
			delete(r.buckets, uid)
		}
	}
}
