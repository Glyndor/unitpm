#!/usr/bin/env bash
# Run all supervisor scenarios, merge results into a single JSON document, and
# render a markdown table. Usage:
#   bash scripts/bench/run.sh                # run unitpm, pm2, supervisor
#   bash scripts/bench/run.sh unitpm pm2       # subset
#
# Requires: jq, python3, supervisor binaries on PATH.
# For unitpm: builds unitpmd/unitpm into bin/ if not already present.

set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
OUT="$ROOT/scripts/bench/out"
mkdir -p "$OUT"

# Build unitpm if needed.
if [[ ! -x "$ROOT/bin/unitpmd" || ! -x "$ROOT/bin/unitpm" ]]; then
	(cd "$ROOT" && go build -ldflags='-s -w' -o bin/unitpmd ./cmd/unitpmd)
	(cd "$ROOT" && go build -ldflags='-s -w' -o bin/unitpm ./cmd/unitpm)
fi

export LYNX_DAEMON="$ROOT/bin/unitpmd"
export LYNX_CLI="$ROOT/bin/unitpm"

scenarios=("$@")
if [[ ${#scenarios[@]} -eq 0 ]]; then
	scenarios=(unitpm pm2 supervisor)
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
} | jq --arg kernel "$(uname -r)" --arg host "${HOSTNAME:-unknown}" '{
	timestamp: now | strftime("%Y-%m-%dT%H:%M:%SZ"),
	host: $host,
	kernel: $kernel,
	results: .
}' >"$OUT/results.json"

python3 "$HERE/render.py" "$OUT/results.json" >"$OUT/results.md"

echo
echo "JSON: $OUT/results.json"
echo "MD:   $OUT/results.md"
echo
cat "$OUT/results.md"
