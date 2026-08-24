# shellcheck shell=bash
#
# Sourced by the bench scenarios; it has no shebang because it is never
# executed directly, so shellcheck needs telling which shell to assume.
# Shared helpers for the supervisor benchmarks.
# Sourced by scenarios/*.sh and run.sh.

set -euo pipefail

# Resident memory (KB) of a PID. Empty if the process is gone.
rss_kb() {
	local pid=$1
	awk '/^VmRSS:/ {print $2}' "/proc/${pid}/status" 2>/dev/null || true
}

# Sum RSS (KB) of a process tree rooted at PID.
rss_tree_kb() {
	local root=$1
	local total=0 pid kb
	for pid in $(pgrep -P "$root" -f . 2>/dev/null) "$root"; do
		kb=$(rss_kb "$pid")
		[[ -n "$kb" ]] && total=$((total + kb))
	done
	echo "$total"
}

# Wait until a predicate returns 0. Print elapsed nanoseconds, or empty on
# timeout. Predicate is the rest of the args.
time_until() {
	local timeout_ms=$1; shift
	local start_ns end_ns now_ns deadline_ns
	start_ns=$(date +%s%N)
	deadline_ns=$((start_ns + timeout_ms * 1000000))
	while true; do
		if "$@" >/dev/null 2>&1; then
			end_ns=$(date +%s%N)
			echo $((end_ns - start_ns))
			return 0
		fi
		now_ns=$(date +%s%N)
		(( now_ns >= deadline_ns )) && return 1
		sleep 0.005
	done
}

# Kill a process and wait until it's gone.
kill_wait() {
	local pid=$1
	[[ -z "$pid" ]] && return 0
	kill "$pid" 2>/dev/null || true
	for _ in $(seq 1 200); do
		kill -0 "$pid" 2>/dev/null || return 0
		sleep 0.05
	done
	kill -9 "$pid" 2>/dev/null || true
}

# Median of newline-separated numbers on stdin.
median() {
	sort -n | awk '
		{ a[NR] = $1 }
		END {
			n = NR
			if (n == 0) { print 0; exit }
			if (n % 2) { print a[(n + 1) / 2] } else { print (a[n/2] + a[n/2 + 1]) / 2 }
		}
	'
}

# Round nanoseconds to milliseconds. LC_ALL=C so awk emits "1.23", not "1,23".
ns_to_ms() {
	LC_ALL=C awk -v ns="$1" 'BEGIN { printf "%.2f", ns / 1000000 }'
}

# Emit one JSON object for a scenario result. rss_json is the JSON object
# produced by tiers_json — RSS samples keyed by tier size.
emit_result() {
	local name=$1 version=$2 cold_ns=$3 idle_kb=$4 rss_json=$5
	cat <<EOF
{
  "supervisor": "${name}",
  "version": "${version}",
  "cold_start_ms": $(ns_to_ms "$cold_ns"),
  "idle_rss_kb": ${idle_kb},
  "rss_by_n": ${rss_json}
}
EOF
}

# Path to the noop app: traps SIGTERM, sleeps forever. Each supervisor runs N
# copies of the same script so RSS deltas come from the supervisor, not the
# managed workload.
NOOP_CMD='/bin/sh -c '\''trap "exit 0" TERM INT HUP; while true; do sleep 30; done'\'''
COLD_TIMEOUT_MS=15000
# Cold-start is sampled COLD_RUNS times per scenario and the median reported,
# to dampen launch jitter (V8 JIT, page-cache warmth, scheduler noise). The
# RSS measurements still come from a single steady-state daemon.
COLD_RUNS=3

# Procs supervised per tier. Each tier is measured cumulatively against the
# same daemon (so the scenario only starts the *delta* between tiers — e.g.
# 10, then +40, then +50). Override TIERS to widen the matrix manually.
TIERS=(${TIERS:-10 50 100})

# Largest tier value, used by supervisord which has to preconfigure programs
# before the daemon launches.
MAX_TIER=0
for n in "${TIERS[@]}"; do (( n > MAX_TIER )) && MAX_TIER=$n; done

# Build a JSON object like {"10":kb1,"50":kb2,...} from alternating N kb args.
tiers_json() {
	local out="{" first=1 n kb
	while [[ $# -ge 2 ]]; do
		n=$1; kb=$2; shift 2
		if [[ $first -eq 1 ]]; then first=0; else out+=","; fi
		out+="\"$n\":$kb"
	done
	out+="}"
	echo "$out"
}
