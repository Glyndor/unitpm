#!/usr/bin/env bash
# Lynx scenario for the supervisor benchmark.
# Expects $LYNX_DAEMON and $LYNX_CLI env vars pointing at lynxd / lynxpm.
# Outputs one JSON result object on stdout.

set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
source "${HERE}/../lib.sh"

: "${LYNX_DAEMON:?lynxd path required}"
: "${LYNX_CLI:?lynxpm path required}"

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/state" "$WORK/sock"
chmod 755 "$WORK/sock"

export XDG_CONFIG_HOME="$WORK/state"
export LYNX_SOCKET="$WORK/sock/lynx.sock"

# Cold start: launch -> socket ready.
start_ns=$(date +%s%N)
"$LYNX_DAEMON" >"$WORK/lynxd.log" 2>&1 &
DAEMON_PID=$!
trap '
	kill_wait "$DAEMON_PID"
	rm -rf "$WORK"
' EXIT

cold_ns=$(time_until "$COLD_TIMEOUT_MS" test -S "$LYNX_SOCKET") || {
	echo "lynxd did not become ready" >&2
	exit 1
}

# Idle RSS sampled three times, take median.
idle_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
idle_kb=$(echo "$idle_samples" | median)

# Supervise N noop apps via repeated `lynxpm start`.
for i in $(seq 1 "$NOOP_N"); do
	"$LYNX_CLI" start "$NOOP_CMD" --name "noop-$i" --restart always >/dev/null 2>&1
done

# Settle.
sleep 2

with_n_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
with_n_kb=$(echo "$with_n_samples" | median)

version=$("$LYNX_CLI" version 2>&1 | awk '/^  Version/ {print $3; exit}')
emit_result "lynx" "${version:-unknown}" "$cold_ns" "$idle_kb" "$NOOP_N" "$with_n_kb"
