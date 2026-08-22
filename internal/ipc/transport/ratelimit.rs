//! Per-UID token bucket rate limiter.
//!
//! Each UID gets its own bucket. A peer is allowed when its bucket has at
//! least one token; the call deducts one token on success. Idle buckets are
//! reaped every [`sweep_every_n`] calls so long-lived daemons with many
//! transient UIDs (systemd DynamicUser, container runtimes) do not leak
//! memory.

use std::collections::HashMap;
use std::sync::Mutex;
use std::time::Instant;

use crate::env;

/// Default burst size — generous enough that interactive CLI use (rapid-fire
/// `start` in scripts) is not throttled, tight enough to stop a flood.
pub const DEFAULT_RATE_CAPACITY: i64 = 200;
/// Default sustained rate, in tokens per second.
pub const DEFAULT_RATE_REFILL: i64 = 100;
/// Refill maths is skipped for sub-millisecond gaps; the rounding error is
/// well below a token at the default capacity.
const MIN_REFILL_INTERVAL: std::time::Duration = std::time::Duration::from_millis(1);

/// Idle eviction window. A UID's bucket is dropped after this much idleness.
const IDLE_EVICTION_WINDOW: std::time::Duration = std::time::Duration::from_secs(5 * 60);

/// Sweep every Nth call. Bounds sweep cost regardless of how many UIDs are
/// tracked.
const SWEEP_EVERY_N: u64 = 1024;

#[derive(Debug, Clone, Copy)]
struct Bucket {
	tokens: f64,
	last_fill: Instant,
}

/// Per-UID token bucket. `Zero` is not valid; use [`new_rate_limiter`].
#[derive(Debug)]
pub struct RateLimiter {
	inner: Mutex<Inner>,
}

#[derive(Debug)]
struct Inner {
	buckets: HashMap<u32, Bucket>,
	capacity: f64,
	refill_rate: f64,
	sweep_counter: u64,
}

/// Build a rate limiter with capacity and refill rate taken from
/// `UNITPM_IPC_RATE_BURST` / `UNITPM_IPC_RATE_PER_SEC`.
#[must_use]
pub fn new_rate_limiter() -> RateLimiter {
	let capacity = env::int("UNITPM_IPC_RATE_BURST", DEFAULT_RATE_CAPACITY) as f64;
	let refill_rate = env::int("UNITPM_IPC_RATE_PER_SEC", DEFAULT_RATE_REFILL) as f64;
	RateLimiter {
		inner: Mutex::new(Inner {
			buckets: HashMap::new(),
			capacity,
			refill_rate,
			sweep_counter: 0,
		}),
	}
}

impl RateLimiter {
	/// Direct constructor for tests that need to pin the capacity and refill
	/// rate rather than reading them from the environment.
	#[must_use]
	pub fn with_capacity_and_refill(capacity: f64, refill_rate: f64) -> Self {
		RateLimiter {
			inner: Mutex::new(Inner {
				buckets: HashMap::new(),
				capacity,
				refill_rate,
				sweep_counter: 0,
			}),
		}
	}

	/// `true` if a request from `uid` may proceed; deducts one token on
	/// success. The first call for a UID starts the bucket full.
	pub fn allow(&self, uid: u32) -> bool {
		let now = Instant::now();
		let mut inner = self.inner.lock().unwrap_or_else(|e| e.into_inner());
		inner.maybe_sweep(now);

		// Capture the capacity up front; the borrow on `inner.buckets` would
		// otherwise extend through the `entry().or_insert` call.
		let capacity = inner.capacity;
		let refill_rate = inner.refill_rate;
		let needs_init = !inner.buckets.contains_key(&uid);
		if needs_init {
			inner.buckets.insert(
				uid,
				Bucket {
					tokens: capacity,
					last_fill: now,
				},
			);
		}
		let entry = inner.buckets.get_mut(&uid).expect("just inserted");
		let elapsed = now.saturating_duration_since(entry.last_fill);
		if elapsed > MIN_REFILL_INTERVAL {
			entry.tokens += elapsed.as_secs_f64() * refill_rate;
			if entry.tokens > capacity {
				entry.tokens = capacity;
			}
			entry.last_fill = now;
		}

		if entry.tokens < 1.0 {
			return false;
		}
		entry.tokens -= 1.0;
		true
	}

	/// Current token count for `uid`. Test-only helper.
	#[cfg(test)]
	pub fn tokens_for(&self, uid: u32) -> Option<f64> {
		self.inner
			.lock()
			.unwrap_or_else(|e| e.into_inner())
			.buckets
			.get(&uid)
			.map(|b| b.tokens)
	}
}

impl Inner {
	fn maybe_sweep(&mut self, now: Instant) {
		self.sweep_counter += 1;
		if self.sweep_counter < SWEEP_EVERY_N {
			return;
		}
		self.sweep_counter = 0;
		let cutoff = now.checked_sub(IDLE_EVICTION_WINDOW).unwrap_or(now);
		self.buckets.retain(|_, b| b.last_fill >= cutoff);
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn burst_then_deny() {
		let r = RateLimiter::with_capacity_and_refill(5.0, 0.0);
		for i in 0..5 {
			assert!(
				r.allow(1000),
				"request {} inside burst window denied",
				i + 1
			);
		}
		assert!(
			!r.allow(1000),
			"6th request should be denied (burst=5, refill=0)"
		);
	}

	#[test]
	fn refill_over_time() {
		let r = RateLimiter::with_capacity_and_refill(2.0, 100.0);
		assert!(r.allow(1));
		assert!(r.allow(1));
		assert!(!r.allow(1), "expected denial after burst");
		std::thread::sleep(std::time::Duration::from_millis(50));
		assert!(r.allow(1), "expected refill to allow the next request");
	}

	#[test]
	fn per_uid_isolation() {
		let r = RateLimiter::with_capacity_and_refill(1.0, 0.0);
		assert!(r.allow(1000), "first uid=1000 should pass");
		assert!(!r.allow(1000), "second uid=1000 should be denied");
		assert!(r.allow(1001), "uid=1001 must not share bucket with 1000");
	}

	#[test]
	fn new_rate_limiter_env_overrides() {
		// Process-global mutation. The env::int helper reads from std::env
		// which is itself process-global, so this test is not parallel-safe;
		// phase 1 paid the same tax. cargo runs tests on multiple threads;
		// the mutex taken in env_lock() does not extend to env::int, so we
		// use unique env keys and rely on each test removing its key on the
		// way out.
		std::env::set_var("UNITPM_IPC_RATE_BURST", "7");
		std::env::set_var("UNITPM_IPC_RATE_PER_SEC", "3");
		let r = new_rate_limiter();
		let (cap, refill) = {
			let inner = r.inner.lock().unwrap();
			(inner.capacity, inner.refill_rate)
		};
		std::env::remove_var("UNITPM_IPC_RATE_BURST");
		std::env::remove_var("UNITPM_IPC_RATE_PER_SEC");
		assert_eq!(cap, 7.0, "capacity should pick up env override");
		assert_eq!(refill, 3.0, "refill rate should pick up env override");
	}
}
