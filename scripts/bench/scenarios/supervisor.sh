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
nodaemon=true
silent=false
loglevel=warn

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

# Cold start: COLD_RUNS launches, take median. Probe with `pid` — it returns
# 0 as soon as the RPC server is bound, while `status` exits 3 when no
# programs are running, which would never satisfy time_until.
cold_samples=""
for i in $(seq 1 "$COLD_RUNS"); do
	supervisord -c "$CONF" >"$WORK/supervisord-$i.stderr" 2>&1 &
	pid=$!
	if ! sample_ns=$(time_until "$COLD_TIMEOUT_MS" supervisorctl -c "$CONF" pid); then
		echo "supervisord did not become ready (run $i, stderr below):" >&2
		cat "$WORK/supervisord-$i.stderr" >&2 || true
		kill_wait "$pid"
		exit 1
	fi
	cold_samples="${cold_samples}${sample_ns}"$'\n'
	kill_wait "$pid"
done
cold_ns=$(printf '%s' "$cold_samples" | median)

# Final daemon for RSS measurements (nodaemon=true so it stays in fg).
supervisord -c "$CONF" >"$WORK/supervisord.stderr" 2>&1 &
DAEMON_PID=$!
time_until "$COLD_TIMEOUT_MS" supervisorctl -c "$CONF" pid >/dev/null || {
	echo "supervisord did not become ready (final run, stderr below):" >&2
	cat "$WORK/supervisord.stderr" >&2 || true
	exit 1
}

idle_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
idle_kb=$(echo "$idle_samples" | median)

# Start the N programs. supervisorctl takes a space-separated list, not a
# glob, when used non-interactively.
names=""
for i in $(seq 1 "$NOOP_N"); do
	names="$names noop-$i"
done
# shellcheck disable=SC2086
supervisorctl -c "$CONF" start $names >/dev/null 2>&1 || true
sleep 2

with_n_samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
with_n_kb=$(echo "$with_n_samples" | median)

version=$(supervisord --version 2>&1 | head -1)
emit_result "supervisor" "${version:-unknown}" "$cold_ns" "$idle_kb" "$NOOP_N" "$with_n_kb"
