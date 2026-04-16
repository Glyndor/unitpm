package transport

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// Rate-limit defaults: tight enough to stop a flood, generous enough that
// interactive CLI use (rapid-fire 'lynx start' in scripts) still works.
// Overridable via env vars on daemon startup.
const (
	defaultRateCapacity = 200          // burst
	defaultRateRefill   = 100          // tokens per second
	minRefillInterval   = 1 * time.Millisecond
)

// rateLimiter is a per-UID token bucket. It is safe for concurrent use.
// Zero value is not valid; use newRateLimiter.
type rateLimiter struct {
	mu         sync.Mutex
	buckets    map[uint32]*bucket
	capacity   int
	refillRate float64 // tokens per second
}

type bucket struct {
	tokens   float64
	lastFill time.Time
}

func newRateLimiter() *rateLimiter {
	cap := envIntPositive("LYNX_IPC_RATE_BURST", defaultRateCapacity)
	refill := envIntPositive("LYNX_IPC_RATE_PER_SEC", defaultRateRefill)
	return &rateLimiter{
		buckets:    make(map[uint32]*bucket),
		capacity:   cap,
		refillRate: float64(refill),
	}
}

// allow reports whether a request from the given uid may proceed. It
// deducts one token on success. On first sight of a uid the bucket is
// initialized full.
func (r *rateLimiter) allow(uid uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
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

func envIntPositive(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
