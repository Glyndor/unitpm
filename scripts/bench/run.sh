#!/usr/bin/env bash
# Run all supervisor scenarios, merge results into a single JSON document, and
# render a markdown table. Usage:
#   bash scripts/bench/run.sh                # run lynx, pm2, supervisor
#   bash scripts/bench/run.sh lynx pm2       # subset
#
# Requires: jq, python3, supervisor binaries on PATH.
# For Lynx: builds lynxd/lynxpm into bin/ if not already present.

set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
OUT="$ROOT/scripts/bench/out"
mkdir -p "$OUT"

# Build Lynx if needed.
if [[ ! -x "$ROOT/bin/lynxd" || ! -x "$ROOT/bin/lynxpm" ]]; then
	(cd "$ROOT" && go build -ldflags='-s -w' -o bin/lynxd ./cmd/lynxd)
	(cd "$ROOT" && go build -ldflags='-s -w' -o bin/lynxpm ./cmd/lynxpm)
fi

export LYNX_DAEMON="$ROOT/bin/lynxd"
export LYNX_CLI="$ROOT/bin/lynxpm"

scenarios=("$@")
if [[ ${#scenarios[@]} -eq 0 ]]; then
	scenarios=(lynx pm2 supervisor)
fi

results=()
for s in "${scenarios[@]}"; do
	echo "==> $s" >&2
	if ! json=$(bash "$HERE/scenarios/$s.sh"); then
		echo "    skipped ($s failed — see stderr)" >&2
		continue
	fi
	results+=("$json")
done

if [[ ${#results[@]} -eq 0 ]]; then
	echo "no scenarios produced results" >&2
	exit 1
fi

# Stitch the per-scenario JSON objects into one array.
{
	printf '['
	first=1
	for r in "${results[@]}"; do
		if [[ $first -eq 1 ]]; then first=0; else printf ','; fi
		printf '%s' "$r"
	done
	printf ']'
} | jq '{
	timestamp: now | strftime("%Y-%m-%dT%H:%M:%SZ"),
	host: env.HOSTNAME // "unknown",
	kernel: $kernel,
	results: .
}' --arg kernel "$(uname -r)" >"$OUT/results.json"

python3 "$HERE/render.py" "$OUT/results.json" >"$OUT/results.md"

echo
echo "JSON: $OUT/results.json"
echo "MD:   $OUT/results.md"
echo
cat "$OUT/results.md"
