#!/usr/bin/env bash
# supervisord scenario.
# Requires `supervisord` and `supervisorctl` on PATH (pip install supervisor).
# Outputs one JSON result object on stdout.

set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
source "${HERE}/../lib.sh"

WORK=$(mktemp -d)
trap 'cleanup' EXIT

cleanup() {
	if [[ -n "${DAEMON_PID:-}" ]]; then
		kill_wait "$DAEMON_PID"
	fi
	rm -rf "$WORK"
}

# Generate a config with N noop programs preconfigured. supervisord doesn't
# support adding programs at runtime via supervisorctl in the same way pm2/lynx
# do — so we configure all N upfront. That gives supervisord a slight edge on
# the supervise-N RSS metric, which we accept; it reflects how it's actually
# used.
NOOP="$WORK/noop.sh"
cat >"$NOOP" <<'EOF'
#!/bin/sh
trap 'exit 0' TERM INT HUP
while true; do sleep 30; done
EOF
chmod +x "$NOOP"

CONF="$WORK/supervisord.conf"
{
	cat <<EOF
[supervisord]
logfile=$WORK/supervisord.log
pidfile=$WORK/supervisord.pid
nodaemon=false
silent=false

[unix_http_server]
file=$WORK/supervisor.sock

[supervisorctl]
serverurl=unix://$WORK/supervisor.sock

[rpcinterface:supervisor]
supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface

EOF
	for i in $(seq 1 "$NOOP_N"); do
		cat <<EOF
[program:noop-$i]
command=$NOOP
autostart=false
autorestart=true
startsecs=0
stopwaitsecs=2
EOF
	done
} >"$CONF"

# Cold start: launch supervisord with autostart=false (no programs yet) -> the
# control socket is ready. We measure with no programs running so it's a fair
# comparison to lynxd / pm2's "daemon-only" startup.
start_ns=$(date +%s%N)
supervisord -c "$CONF" >/dev/null 2>&1 &
DAEMON_PID=$!

cold_ns=$(time_until "$COLD_TIMEOUT_MS" supervisorctl -c "$CONF" status) || {
	echo "supervisord did not become ready" >&2
	exit 1
}

# Re-bind to the actual daemon PID (the launched process forks).
DAEMON_PID=$(cat "$WORK/supervisord.pid")

idle_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
idle_kb=$(echo "$idle_samples" | median)

# Start the N programs.
supervisorctl -c "$CONF" start "noop-*" >/dev/null 2>&1 || true
sleep 2

with_n_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
with_n_kb=$(echo "$with_n_samples" | median)

version=$(supervisord --version 2>&1 | head -1)
emit_result "supervisor" "${version:-unknown}" "$cold_ns" "$idle_kb" "$NOOP_N" "$with_n_kb"
