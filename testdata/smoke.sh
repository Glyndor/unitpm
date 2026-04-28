#!/usr/bin/env bash
# End-to-end smoke for lynxpm / lynxd. Runs against an already-installed
# CLI + daemon (system path or PATH override). The daemon is expected to
# be up and listening — the caller starts lynxd beforehand.
#
# Intended callers:
#   - .github/workflows/debian-tests.yml (install-matrix job)
#   - local dev: `bash testdata/smoke.sh`
#
# Each scenario is a standalone block so a failure prints a focused
# "FAIL: <what>" line plus the relevant daemon log before exiting non-zero.

set -eu

die() {
    echo "FAIL: $*"
    [ -f /tmp/lynxd.log ] && { echo "--- lynxd.log tail ---"; tail -40 /tmp/lynxd.log; }
    exit 1
}

# run_worker_scenario <name> <start-cmd> <log-marker>
# Start a worker, wait up to 2s for its startup line to appear in the
# log, stop + delete. Used by scenarios that only need to prove a given
# runtime's lifecycle works end-to-end against the installed .deb.
run_worker_scenario() {
    lynxpm start "$2" --name "$1" --restart never
    for i in $(seq 1 20); do
        lynxpm logs "$1" --stdout --lines 10 2>/dev/null | grep -q "$3 pid=" && break
        sleep 0.1
    done
    lynxpm logs "$1" --stdout --lines 10 2>/dev/null | grep -q "$3 pid=" || \
        die "$3 never printed its startup line"
    lynxpm stop   "$1"
    lynxpm delete "$1" --purge
}

# wait_count <expected> <selector-ns>
# Poll lynxpm list until the number of procs in the given namespace
# equals expected, or fail after ~2s. Covers async scale/delete paths
# without the flaky `sleep N; assert` pattern.
wait_count() {
    for i in $(seq 1 20); do
        local c
        c=$(lynxpm list --namespace "$2" --json | grep -o "\"namespace\":\"$2\"" | wc -l)
        [ "$c" -eq "$1" ] && return 0
        sleep 0.1
    done
    die "expected $1 procs in namespace $2, got $c"
}

# Poll until the daemon socket is responsive (bootstrap from the caller
# happens in parallel; the CLI gets EAGAIN until the server loop runs).
for i in $(seq 1 50); do
    lynxpm list --json >/dev/null 2>&1 && break
    sleep 0.1
done
lynxpm list --json >/dev/null || die "daemon socket never became responsive"

APPS_DIR="$(cd "$(dirname "$0")/apps" && pwd)"

# Sanity: empty list after a fresh daemon.
[ "$(lynxpm list --json)" = "[]" ] || die "fresh daemon should have an empty process list"

# Scenario 1: vanilla lifecycle with sleep (no children to kill).
# Exercises start / list --json / show / stop / delete as a baseline.
echo "=== scenario: vanilla lifecycle ==="
lynxpm start "/bin/sleep 300" --name smoke-vanilla --restart never
lynxpm list --json | grep -q smoke-vanilla || die "smoke-vanilla not in list"
lynxpm show smoke-vanilla >/dev/null
lynxpm stop   smoke-vanilla
lynxpm delete smoke-vanilla
[ "$(lynxpm list --json)" = "[]" ] || die "list not empty after delete"

# Scenario 2: shell forkstorm — regresses gracefulKill's /proc descendant
# walk. The bash wrapper spawns 10 long-running sleep children; stop
# must kill every one of them, not just the wrapper.
echo "=== scenario: shell forkstorm ==="
lynxpm start "bash $APPS_DIR/shell-forkstorm/run.sh" --name fs --restart never
sleep 1
BEFORE=$(pgrep -f "sleep 3600" 2>/dev/null | wc -l)
[ "$BEFORE" -ge 10 ] || die "forkstorm only spawned $BEFORE/10 sleep workers"
lynxpm stop fs
sleep 2
ALIVE=0
for p in $(pgrep -f "sleep 3600" 2>/dev/null || true); do
    # Zombies don't hold fds — count them as dead, matching the
    # supervisor's promise to the operator (no EADDRINUSE, no port leak).
    if ! grep -q '^State:.*Z' "/proc/$p/status" 2>/dev/null; then
        ALIVE=$((ALIVE + 1))
    fi
done
[ "$ALIVE" -eq 0 ] || die "forkstorm left $ALIVE live sleep children after stop"
lynxpm delete fs

# Scenario 3: restart / reset / flush against a python worker. Exercises
# the three lifecycle ops that the previous smoke revision never touched
# plus the JSON batch report shape.
echo "=== scenario: restart + reset + flush ==="
command -v python3 >/dev/null || die "python3 missing — install python3 before running smoke"
lynxpm start "python3 $APPS_DIR/python-worker/worker.py" --name pyw --restart on-failure
sleep 1
lynxpm restart pyw --json | grep -q '"op":"restart"' || die "restart --json missing op field"
lynxpm reset   pyw --json | grep -q '"op":"reset"'   || die "reset --json missing op field"
lynxpm flush   pyw --json | grep -q '"op":"flush"'   || die "flush --json missing op field"
lynxpm stop    pyw
lynxpm delete  pyw --purge

# Scenario 4: max-restarts cap enforced. python-crashloop exits 1 after
# 1s; with --max-restarts 2 the supervisor must stop restarting after
# the cap and leave State: failed.
echo "=== scenario: max-restarts cap ==="
lynxpm start "python3 $APPS_DIR/python-crashloop/crash.py" \
    --name crashloop --restart on-failure --max-restarts 2 --restart-delay 100
# 2 attempts × (1s run + 0.1s delay) ≈ 3s budget; give it 8s to settle.
for i in $(seq 1 40); do
    STATE=$(lynxpm list --json | awk -F'"state":"' '/crashloop/{print $2}' | cut -d'"' -f1 || true)
    [ "$STATE" = "failed" ] && break
    sleep 0.2
done
[ "$STATE" = "failed" ] || die "crashloop state=$STATE (want failed after cap)"
lynxpm delete crashloop --purge

# Scenario 5: namespace bulk selectors across stop / delete. Spawns two
# apps in a shared namespace, stops them both with `--namespace`, then
# deletes with the `ns:*` glob form.
echo "=== scenario: namespace bulk ops ==="
lynxpm start "/bin/sleep 300" --name api    --namespace probe --restart never
lynxpm start "/bin/sleep 300" --name worker --namespace probe --restart never
wait_count 2 probe
lynxpm stop --namespace probe >/dev/null
# Both should now be stopped — list still shows them (stopped), delete
# with ns:* glob cleans them up in one shot.
lynxpm delete 'probe:*' --purge >/dev/null
[ "$(lynxpm list --namespace probe --json)" = "[]" ] || \
    die "namespace probe not empty after bulk delete"

# Scenario 6: node HTTP with graceful SIGTERM. Verifies the full
# start/stop cycle for a listener. Only runs when node is available
# on the smoke host — skipped silently otherwise so this script
# works on minimal CI images.
if command -v node >/dev/null; then
    echo "=== scenario: node HTTP graceful stop ==="
    run_worker_scenario nh "node $APPS_DIR/node-http/server.js" node-http
else
    echo "=== scenario: node HTTP (skipped — node not installed) ==="
fi

# Scenarios 7-9: interpreted + compiled workers. Same shape, one line
# per runtime — the run_worker_scenario helper covers start/wait-for-
# log/stop/delete so any runtime-specific regression is a single
# failure, not a 10-line copy-paste.
if command -v php >/dev/null; then
    echo "=== scenario: PHP worker ==="
    run_worker_scenario phpw "php $APPS_DIR/php-worker/worker.php" php-worker
else
    echo "=== scenario: PHP worker (skipped — php not installed) ==="
fi

if command -v ruby >/dev/null; then
    echo "=== scenario: Ruby worker ==="
    run_worker_scenario rbw "ruby $APPS_DIR/ruby-worker/worker.rb" ruby-worker
else
    echo "=== scenario: Ruby worker (skipped — ruby not installed) ==="
fi

GO_BIN="$APPS_DIR/go-compiled/go-compiled"
if [ -x "$GO_BIN" ]; then
    echo "=== scenario: compiled Go binary ==="
    run_worker_scenario gob "$GO_BIN" go-compiled
else
    echo "=== scenario: Go binary (skipped — $GO_BIN not built) ==="
fi

# Scenario 10: SIGKILL fallback. node-ignores-term masks SIGTERM, so
# the supervisor has to escalate to SIGKILL after --stop-timeout
# expires. With --stop-timeout 2000 the whole stop must complete in
# the 2-4s window (2s grace + signal delivery latency); anything
# beyond that means the SIGKILL path did not fire.
if command -v node >/dev/null; then
    echo "=== scenario: SIGKILL fallback ==="
    lynxpm start "node $APPS_DIR/node-ignores-term/server.js" \
        --name stubborn --restart never \
        --stop-signal SIGTERM --stop-timeout 2000
    sleep 1
    START=$(date +%s)
    lynxpm stop stubborn
    END=$(date +%s)
    ELAPSED=$((END - START))
    [ "$ELAPSED" -le 4 ] || die "stop took ${ELAPSED}s — SIGKILL fallback did not fire"
    [ "$ELAPSED" -ge 2 ] || die "stop returned in ${ELAPSED}s (<2s) — SIGTERM handler did NOT get ignored as expected"
    lynxpm delete stubborn --purge
else
    echo "=== scenario: SIGKILL fallback (skipped — node not installed) ==="
fi

# Scenario 11: scale. Starts 3 instances in one invocation, then
# scales down to 1 and up to 2. wait_count polls so slow container
# runners don't race the daemon's spawn/reap.
echo "=== scenario: scale up + down ==="
lynxpm start "/bin/sleep 300" --name scaleapp --namespace scalens \
    --restart never --scale 3
wait_count 3 scalens
lynxpm scale scalens:scaleapp 1
wait_count 1 scalens
lynxpm scale scalens:scaleapp 2
wait_count 2 scalens
lynxpm delete 'scalens:*' --purge >/dev/null

# Scenario 12: process tree. Starts a bash wrapper that spawns sleep children
# and verifies that lynxpm monit --json reports the root entry and at least
# one child with depth > 0.
echo "=== scenario: process tree (monit --json) ==="
lynxpm start "bash -c 'sleep 60 & sleep 60 & wait'" --name tree-smoke --restart never
TREE_JSON=""
for i in $(seq 1 20); do
    TREE_JSON=$(lynxpm monit tree-smoke --json 2>/dev/null)
    echo "$TREE_JSON" | grep -q '"depth":1' && break
    sleep 0.5
done
echo "$TREE_JSON" | grep -q '"pid"' || die "monit --json missing pid field"
echo "$TREE_JSON" | grep -q '"depth":0' || die "monit --json missing root entry (depth 0)"
echo "$TREE_JSON" | grep -q '"depth":1' || die "monit --json missing child entry (depth 1)"
lynxpm stop   tree-smoke
lynxpm delete tree-smoke --purge

echo "=== all smoke scenarios passed ==="
