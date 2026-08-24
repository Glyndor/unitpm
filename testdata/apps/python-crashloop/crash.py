#!/usr/bin/env python3
"""Exits 1 after 1 second. Used to regression-test the
`--restart on-failure --max-restarts N` cap: unitpm should stop
restarting after N attempts and leave the process in failed state."""

import os
import sys
import time

print(f"python-crashloop pid={os.getpid()} — will exit 1 in 1s", flush=True)
time.sleep(1)
sys.exit(1)
