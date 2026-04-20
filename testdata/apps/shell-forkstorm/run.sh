#!/usr/bin/env bash
# Forks 10 long-running `sleep` workers in its own process group and
# waits for them, so the supervised PID's /proc-ppid tree has depth >= 2
# and width 10. Regression guard for gracefulKill's descendant walk:
# every worker must be reaped by `lynxpm stop`, not just the wrapper.
set -e

echo "forkstorm pid=$$"
for i in $(seq 1 10); do
    sleep 3600 &
    echo "forkstorm worker[$i] pid=$!"
done

# Wait keeps the wrapper alive and blocks its own SIGTERM handling so
# the supervisor has to kill the whole tree instead of relying on the
# wrapper to propagate signals itself.
wait
