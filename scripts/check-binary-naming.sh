#!/usr/bin/env bash
# check-binary-naming.sh — fail if the old CLI name `unitpm` slips back into the
# tree where it should be `unitpm`. The binary was renamed from `unitpm` to
# `unitpm` in 0.7.x; the renames in PRs #26-#29 fixed every residual hit and
# this guard prevents regressions.
#
# Logic:
#   - Scan tracked source files (no built artifacts, vendor, lockfiles).
#   - Skip whole files that are legitimately allowed to mention `unitpm`
#     (defined-once constants, test fixtures, bench scenarios).
#   - For remaining files, flag any line that matches the bare word `unitpm`.
#   - Pardon a flagged line if it contains any allowlisted substring
#     (system user, socket file, config dir, polkit unit prefix, etc.).
#
# To allow a new context, add to ALLOW_SUBSTRINGS below with a comment
# explaining why.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Files where `unitpm` is the canonical name and the check would just fight us.
FILE_EXCLUDES_RE='(^|/)('
FILE_EXCLUDES_RE+='.*_test\.rs$'                    # test fixtures, arbitrary names
FILE_EXCLUDES_RE+='|scripts/bench/'                 # bench scenario scripts + Dockerfile
FILE_EXCLUDES_RE+='|site/dist/'                     # built site
FILE_EXCLUDES_RE+='|site/node_modules/'             # third-party
FILE_EXCLUDES_RE+='|node_modules/'                  # idem
FILE_EXCLUDES_RE+='|vendor/'                        # idem
FILE_EXCLUDES_RE+='|debian/changelog$'              # historical entries
FILE_EXCLUDES_RE+='|site/astro\.config\.mjs$'       # holds the redirect from the old indexed URL
FILE_EXCLUDES_RE+='|scripts/check-binary-naming\.sh$' # this script (defines the patterns)
FILE_EXCLUDES_RE+='|.*\.lock$'
FILE_EXCLUDES_RE+='|.*\.lock\.json$'
FILE_EXCLUDES_RE+='|package-lock\.json$'
FILE_EXCLUDES_RE+=')'

# Substrings that, when present on a flagged line, mark it as intentional.
ALLOW_SUBSTRINGS=(
  # The start command's env test asserts the OLD name is absent from the child's
  # environment. It has to name it to check for it, so these two lines are the
  # one place in the Rust tree where the old name is the point, not a leftover.
  'pre-org `LYNX_INSTANCE` through'
  '!env.contains_key("LYNX_INSTANCE")'
)

mapfile -t FILES < <(git ls-files | grep -vE "$FILE_EXCLUDES_RE")

hits=()
for f in "${FILES[@]}"; do
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    skip=0
    for a in "${ALLOW_SUBSTRINGS[@]}"; do
      if [[ "$line" == *"$a"* ]]; then
        skip=1
        break
      fi
    done
    if [[ $skip -eq 0 ]]; then
      hits+=("$f:$line")
    fi
  done < <(grep -niE 'lynx' "$f" 2>/dev/null || true)
done

if (( ${#hits[@]} > 0 )); then
  echo "binary-naming check: ${#hits[@]} stale 'lynx' reference(s) found."
  echo "Either rename to 'unitpm', or add a justified entry to"
  echo "  scripts/check-binary-naming.sh (FILE_EXCLUDES_RE or ALLOW_SUBSTRINGS)."
  echo
  printf '  %s\n' "${hits[@]}"
  exit 1
fi

echo "binary-naming check: clean (${#FILES[@]} files scanned)"
