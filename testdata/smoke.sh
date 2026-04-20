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
# grep -c counts lines but --json is single-line; count substrings instead.
COUNT=$(lynxpm list --namespace probe --json | grep -o '"namespace":"probe"' | wc -l)
[ "$COUNT" -eq 2 ] || die "expected 2 procs in namespace probe, got $COUNT"
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
    lynxpm start "node $APPS_DIR/node-http/server.js" --name nh --restart never
    # Wait up to 2s for the listener to report its chosen port.
    for i in $(seq 1 20); do
        lynxpm logs nh --stdout --lines 10 2>/dev/null | grep -q 'node-http pid=' && break
        sleep 0.1
    done
    lynxpm logs nh --stdout --lines 10 2>/dev/null | grep -q 'node-http pid=' || \
        die "node-http never printed its startup line"
    lynxpm stop   nh
    lynxpm delete nh --purge
else
    echo "=== scenario: node HTTP (skipped — node not installed) ==="
fi

echo "=== all smoke scenarios passed ==="
