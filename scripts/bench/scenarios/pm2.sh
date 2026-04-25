#!/usr/bin/env bash
# PM2 scenario.
# Requires `pm2` on PATH. Pinned in the Dockerfile/CI workflow.
# Outputs one JSON result object on stdout.

set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
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
# trigger for a clean measurement.
start_ns=$(date +%s%N)
pm2 ping >/dev/null 2>&1
end_ns=$(date +%s%N)
cold_ns=$((end_ns - start_ns))

# Find the daemon PID. PM2's daemon process is renamed at runtime to a string
# like "PM2 v5.4.3: God Daemon (/home/.../.pm2)". Match it loosely.
DAEMON_PID=$(pgrep -f 'PM2.*God Daemon' | head -1 || true)
if [[ -z "$DAEMON_PID" ]]; then
	echo "could not locate PM2 God Daemon pid" >&2
	pm2 list 2>&1 | head -3 >&2
	exit 1
fi

idle_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
idle_kb=$(echo "$idle_samples" | median)

# Supervise N noop apps. PM2 needs a script path, not an inline shell, so
# write a noop.sh once and start N copies with --name noop-i.
NOOP="$WORK/noop.sh"
cat >"$NOOP" <<'EOF'
#!/bin/sh
trap 'exit 0' TERM INT HUP
while true; do sleep 30; done
EOF
chmod +x "$NOOP"

for i in $(seq 1 "$NOOP_N"); do
	pm2 start "$NOOP" --name "noop-$i" >/dev/null 2>&1
done

sleep 2

with_n_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
with_n_kb=$(echo "$with_n_samples" | median)

version=$(pm2 --version 2>/dev/null | head -1)
emit_result "pm2" "${version:-unknown}" "$cold_ns" "$idle_kb" "$NOOP_N" "$with_n_kb"
