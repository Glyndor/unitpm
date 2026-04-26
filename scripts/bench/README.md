# Supervisor benchmark

Compares **Lynx**, **PM2**, and **supervisord** on supervisor-level metrics. The
managed workload is identical for all three (a noop `/bin/sh` script that traps
SIGTERM and sleeps), so the deltas come from the supervisor itself, not the
apps it runs.

## Metrics

| Metric | Definition |
| :--- | :--- |
| **Cold start** | Wall time from launching the daemon to the control socket / RPC being responsive. Median of 3 fresh-launch samples per supervisor. |
| **Idle RSS** | Resident memory of the daemon process with **zero** programs managed. Median of 3 samples taken 200 ms apart. |
| **RSS @ N** | Same daemon RSS after `N` noop programs are running. Sampled at three tiers — `N=10` (light), `N=50` (medium), `N=100` (heavy) — against the same daemon, cumulatively (start the delta, settle 2 s, sample). Override `TIERS` to widen the matrix manually. |

What this **does not** measure: throughput, log rotation, hot-reload, restart
latency on crash. The last one is intentional: Lynx delegates restart-on-crash
to systemd, while PM2/supervisord poll from user-space — measuring them all
together would mix architectures, not products. A separate systemd-managed
bench is in scope but not yet wired up.

## Reproducing

The numbers are only meaningful with pinned versions on a known kernel. Use the
Docker image:

```bash
docker build -f scripts/bench/Dockerfile -t lynx-bench .
docker run --rm lynx-bench > out.md
```

Bare-metal run (assumes `lynxd`, `lynxpm`, `pm2`, `supervisord` already on
PATH):

```bash
bash scripts/bench/run.sh
```

Subset run:

```bash
bash scripts/bench/run.sh lynx          # lynx only
bash scripts/bench/run.sh lynx pm2      # skip supervisord
```

Output:

- `scripts/bench/out/results.json` — machine-readable
- `scripts/bench/out/results.md`   — table for the README/site

## Pinned versions

Bumped as a single PR when refreshing the bench. See
[`Dockerfile`](./Dockerfile) build args:

- Go (used to build Lynx)
- Node + PM2
- supervisord (Python)

## CI

[`.github/workflows/bench.yml`](../../.github/workflows/bench.yml) runs the
Docker image weekly and uploads the JSON + Markdown as artifacts. Numbers
quoted in the README and on the marketing site come from that run, not from
hand-typed estimates.

## Caveats

- **Tiers are still modest.** `N=10/50/100` covers the range most users hit
  in practice; it is not a stress test. RSS rarely scales linearly because
  much of the daemon footprint is one-time runtime cost. Set `TIERS="10 100
  500"` (or similar) to push harder — `pm2 start` is ~1 s per call, so the
  heavy tail is gated by PM2, not by Lynx.
- **PM2's God Daemon is shared per user.** Stopping PM2 between scenarios
  (`pm2 kill`) ensures we measure a fresh daemon, but the JIT warm-up of V8
  may still affect cold start vs a steady-state daemon.
- **supervisord configures programs ahead of time**, while Lynx and PM2 add
  them at runtime. The bench keeps them all in `autostart=false` until the
  measurement step so cold start is comparable.
- **Idle RSS for Go binaries underestimates the real virtual footprint.** Go's
  scheduler reserves a large virtual address space (`VmPeak` ~ 1.5 GB) that
  is *never* committed. The bench reports `VmRSS` (committed pages) which is
  what `top`, `ps`, and your container limit actually see.
