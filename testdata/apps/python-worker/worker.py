#!/usr/bin/env python3
"""Long-running worker. Emits a heartbeat line each second and exits 0
on SIGTERM, so the supervisor sees a clean graceful stop."""

import os
import signal
import sys
import time

running = True


def _shutdown(sig, _frame):
    global running
    print(f"python-worker received signal {sig}, exiting", flush=True)
    running = False


signal.signal(signal.SIGTERM, _shutdown)
signal.signal(signal.SIGINT, _shutdown)

print(f"python-worker pid={os.getpid()}", flush=True)
tick = 0
while running:
    print(f"python-worker tick={tick}", flush=True)
    tick += 1
    time.sleep(1)

sys.exit(0)
