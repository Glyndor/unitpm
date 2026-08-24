#!/usr/bin/env bash
# unitpm scenario for the supervisor benchmark.
# Expects $LYNX_DAEMON and $LYNX_CLI env vars pointing at unitpmd / unitpm.
# Outputs one JSON result object on stdout.

set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
source "${HERE}/../lib.sh"

: "${LYNX_DAEMON:?unitpmd path required}"
: "${LYNX_CLI:?unitpm path required}"

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/state" "$WORK/sock"
chmod 755 "$WORK/sock"

export XDG_CONFIG_HOME="$WORK/state"
export LYNX_SOCKET="$WORK/sock/unitpm.sock"

# Cold start: COLD_RUNS launches, take median. Each run uses a fresh socket
# path so a stale file can never short-circuit the readiness probe.
cold_samples=""
for i in $(seq 1 "$COLD_RUNS"); do
	export LYNX_SOCKET="$WORK/sock/unitpm-$i.sock"
	"$LYNX_DAEMON" >"$WORK/unitpmd-$i.log" 2>&1 &
	pid=$!
	if ! sample_ns=$(time_until "$COLD_TIMEOUT_MS" test -S "$LYNX_SOCKET"); then
		echo "unitpmd did not become ready (run $i)" >&2
		kill_wait "$pid"
		exit 1
	fi
	cold_samples="${cold_samples}${sample_ns}"$'\n'
	kill_wait "$pid"
done
cold_ns=$(printf '%s' "$cold_samples" | median)

# Final daemon for RSS measurements.
export LYNX_SOCKET="$WORK/sock/unitpm.sock"
"$LYNX_DAEMON" >"$WORK/unitpmd.log" 2>&1 &
DAEMON_PID=$!
trap '
	kill_wait "$DAEMON_PID"
	rm -rf "$WORK"
' EXIT
time_until "$COLD_TIMEOUT_MS" test -S "$LYNX_SOCKET" >/dev/null || {
	echo "unitpmd did not become ready (final run)" >&2
	exit 1
}

# Idle RSS sampled three times, take median.
idle_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
idle_kb=$(echo "$idle_samples" | median)

# Cumulative tier RSS measurements: start the delta between tiers, settle,
# sample. The same daemon supervises the growing fleet across tiers.
tier_args=()
prev=0
for n in "${TIERS[@]}"; do
	for i in $(seq $((prev+1)) "$n"); do
		"$LYNX_CLI" start "$NOOP_CMD" --name "noop-$i" --restart always --log-timestamp none >/dev/null 2>&1
	done
	prev=$n
	sleep 2
	samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
	kb=$(echo "$samples" | median)
	tier_args+=("$n" "$kb")
done
rss_json=$(tiers_json "${tier_args[@]}")

version=$("$LYNX_CLI" version 2>&1 | awk '/^  Version/ {print $3; exit}')
emit_result "unitpm" "${version:-unknown}" "$cold_ns" "$idle_kb" "$rss_json"
