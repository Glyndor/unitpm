#!/usr/bin/env bash
# PM2 scenario.
# Requires `pm2` on PATH. Pinned in the Dockerfile/CI workflow.
# Outputs one JSON result object on stdout.

set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=scripts/bench/lib.sh
# shellcheck disable=SC1091  # the reusable does not run shellcheck -x, so it
# cannot follow the path even with the directive above.
source "${HERE}/../lib.sh"

WORK=$(mktemp -d)
export PM2_HOME="$WORK/.pm2"
mkdir -p "$PM2_HOME"

cleanup() {
	pm2 kill >/dev/null 2>&1 || true
	rm -rf "$WORK"
}
trap cleanup EXIT

# PM2's daemon is launched lazily by the first command. Cold start = launch
# -> daemon ready (`pm2 ping` returns "pong"). Use `pm2 ping` itself as the
# trigger. COLD_RUNS samples, take median; `pm2 kill` between runs ensures a
# fresh God Daemon each time.
cold_samples=""
for i in $(seq 1 "$COLD_RUNS"); do
	pm2 kill >/dev/null 2>&1 || true
	start_ns=$(date +%s%N)
	pm2 ping >/dev/null 2>&1
	end_ns=$(date +%s%N)
	cold_samples="${cold_samples}$((end_ns - start_ns))"$'\n'
done
cold_ns=$(printf '%s' "$cold_samples" | median)

# Find the daemon PID. PM2's daemon process is renamed at runtime to a string
# like "PM2 v6.0.14: God Daemon (/home/.../.pm2)". Match it loosely.
DAEMON_PID=$(pgrep -f 'PM2.*God Daemon' | head -1 || true)
if [[ -z "$DAEMON_PID" ]]; then
	echo "could not locate PM2 God Daemon pid" >&2
	pm2 list 2>&1 | head -3 >&2
	exit 1
fi

idle_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
idle_kb=$(echo "$idle_samples" | median)

# PM2 needs a script path, not an inline shell, so write a noop.sh once and
# start cumulative tiers via `pm2 start ... --name noop-i`.
NOOP="$WORK/noop.sh"
cat >"$NOOP" <<'EOF'
#!/bin/sh
trap 'exit 0' TERM INT HUP
while true; do sleep 30; done
EOF
chmod +x "$NOOP"

tier_args=()
prev=0
for n in "${TIERS[@]}"; do
	for i in $(seq $((prev+1)) "$n"); do
		pm2 start "$NOOP" --name "noop-$i" >/dev/null 2>&1
	done
	prev=$n
	sleep 2
	samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
	kb=$(echo "$samples" | median)
	tier_args+=("$n" "$kb")
done
rss_json=$(tiers_json "${tier_args[@]}")

version=$(pm2 --version 2>/dev/null | head -1)
emit_result "pm2" "${version:-unknown}" "$cold_ns" "$idle_kb" "$rss_json"
