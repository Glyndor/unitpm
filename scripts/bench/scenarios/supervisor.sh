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

# Generate a config with MAX_TIER noop programs preconfigured. supervisord
# doesn't support adding programs at runtime via supervisorctl in the same
# way pm2/lynx do — so we configure all of them upfront and start the tiers
# cumulatively via `supervisorctl start`. That bakes the config-parse cost
# of the largest tier into supervisord's cold-start metric, which is how it
# is actually deployed in practice.
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
	for i in $(seq 1 "$MAX_TIER"); do
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

# Cumulative tier RSS measurements via `supervisorctl start` on a growing
# space-separated list. (supervisorctl doesn't accept a glob non-interactively.)
tier_args=()
prev=0
for n in "${TIERS[@]}"; do
	names=""
	for i in $(seq $((prev+1)) "$n"); do
		names="$names noop-$i"
	done
	prev=$n
	# shellcheck disable=SC2086
	supervisorctl -c "$CONF" start $names >/dev/null 2>&1 || true
	sleep 2
	samples=$(for _ in 1 2 3; do sleep 0.2; rss_kb "$DAEMON_PID"; done)
	kb=$(echo "$samples" | median)
	tier_args+=("$n" "$kb")
done
rss_json=$(tiers_json "${tier_args[@]}")

version=$(supervisord --version 2>&1 | head -1)
emit_result "supervisor" "${version:-unknown}" "$cold_ns" "$idle_kb" "$rss_json"
