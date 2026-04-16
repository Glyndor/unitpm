package transport

import (
	"testing"
	"time"
)

func TestRateLimiter_BurstThenDeny(t *testing.T) {
	r := &rateLimiter{
		buckets:    make(map[uint32]*bucket),
		capacity:   5,
		refillRate: 0, // no refill so we can test the burst ceiling
	}

	for i := 0; i < 5; i++ {
		if !r.allow(1000) {
			t.Fatalf("request %d denied inside burst window", i+1)
		}
	}
	if r.allow(1000) {
		t.Error("6th request should have been denied (burst=5, refill=0)")
	}
}

func TestRateLimiter_RefillOverTime(t *testing.T) {
	r := &rateLimiter{
		buckets:    make(map[uint32]*bucket),
		capacity:   2,
		refillRate: 100, // 100 tokens/s
	}

	// Drain
	r.allow(1)
	r.allow(1)
	if r.allow(1) {
		t.Fatal("expected denial after burst")
	}
	// Wait long enough for >=1 token to refill (100/s => 1 every 10ms; wait 50ms)
	time.Sleep(50 * time.Millisecond)
	if !r.allow(1) {
		t.Error("expected refill to allow the next request")
	}
}

func TestRateLimiter_PerUIDIsolation(t *testing.T) {
	r := &rateLimiter{
		buckets:    make(map[uint32]*bucket),
		capacity:   1,
		refillRate: 0,
	}
	if !r.allow(1000) {
		t.Fatal("first uid=1000 should pass")
	}
	if r.allow(1000) {
		t.Error("second uid=1000 should be denied")
	}
	if !r.allow(1001) {
		t.Error("uid=1001 must not share bucket with 1000")
	}
}

func TestNewRateLimiter_EnvOverrides(t *testing.T) {
	t.Setenv("LYNX_IPC_RATE_BURST", "7")
	t.Setenv("LYNX_IPC_RATE_PER_SEC", "3")
	r := newRateLimiter()
	if r.capacity != 7 {
		t.Errorf("capacity: got %d want 7", r.capacity)
	}
	if r.refillRate != 3 {
		t.Errorf("refillRate: got %v want 3", r.refillRate)
	}
}
