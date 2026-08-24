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
FILE_EXCLUDES_RE+='.*_test\.go$'                    # test fixtures, arbitrary names
FILE_EXCLUDES_RE+='|scripts/bench/'                 # bench scenario scripts + Dockerfile
FILE_EXCLUDES_RE+='|site/dist/'                     # built site
FILE_EXCLUDES_RE+='|site/node_modules/'             # third-party
FILE_EXCLUDES_RE+='|node_modules/'                  # idem
FILE_EXCLUDES_RE+='|vendor/'                        # idem
FILE_EXCLUDES_RE+='|debian/changelog$'              # historical entries
FILE_EXCLUDES_RE+='|internal/paths/system_mode\.go$' # const SystemUser = "unitpm"
FILE_EXCLUDES_RE+='|scripts/check-binary-naming\.sh$' # this script (defines the patterns)
FILE_EXCLUDES_RE+='|.*\.lock$'
FILE_EXCLUDES_RE+='|.*\.lock\.json$'
FILE_EXCLUDES_RE+='|package-lock\.json$'
FILE_EXCLUDES_RE+=')'

# Substrings that, when present on a flagged line, mark it as intentional.
ALLOW_SUBSTRINGS=(
  # system user unitpm (Debian postinst-created, distinct from CLI)
  '`unitpm`'
  'user `unitpm`'
  '`unitpm` user'
  'unitpm system user'
  'system user `unitpm`'
  'unitpm user'             # "unitpm user creation", "the unitpm user", etc.
  'non-unitpm'              # tests differentiating unitpm vs non-unitpm daemons
  'unitpm-owned'            # idem
  'unitpm daemon is left'   # idem
  '"unitpm"'                # const SystemUser = "unitpm", Username == "unitpm"
  "'unitpm'"                # alt quoting
  'User=unitpm'
  'Group=unitpm'
  'chown unitpm'
  'unitpm:unitpm'
  'adduser unitpm'
  'getent passwd unitpm'
  '/var/lib/glyndor-unitpm unitpm'

  # real filesystem paths (XDG project dir is "unitpm", distinct from CLI binary)
  'unitpm.sock'
  'unitpm-<uid>'
  'XDG_RUNTIME_DIR/unitpm-'
  '.config/unitpm'          # also matches ~/.config/unitpm and absolute /home/.../.config/unitpm
  'XDG_CONFIG_HOME/unitpm'
  '"unitpm/logs"'           # filepath.Join(..., "unitpm/logs") in paths/logs.go
  '/unitpm/logs'             # rendered absolute form of XDG_STATE_HOME/unitpm/logs
  '.local/state/unitpm'     # default XDG_STATE_HOME path
  '/run/unitpmd'
  '/var/lib/glyndor-unitpm'
  '/var/log/glyndor-unitpm'
  '/tmp/unitpm-'            # security comment about /tmp socket hijack class

  # polkit unit prefix (must match policy)
  'unitpm-app-'
  'unitpm-`'                # polkit policy doc text
  '"unitpm-"'               # JS in polkit.rules
  'with the `unitpm-`'
  'unitpm-*'                # docs/comments referring to the prefix glob
  'scoped to unitpm-'       # polkit comment phrasing

  # backward-compat upgrade fallbacks (legacy debian tests probe both)
  'unitpm.polkit.rules'

  # docs example for systemd unit naming convention (unitpm-{processname}.service)
  'unitpm-api'

  # bench scenario tag (product name)
  'unitpm-bench'

  # historical debhelper build dirs (not produced anymore)
  'debian/unitpm/'
  'debian/glyndor-unitpm/'

  # site assets + product comparison styling (unitpm the product)
  'unitpm.svg'
  'compare__td-unitpm'
  'compare__th-unitpm'

  # docs render the system user value in table output
  '| unitpm '
  '| unitpm|'

  # package + product capitalized references
  'unitpm'
  'glyndor-unitpm'
  'LYNX_'
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
  done < <(grep -nE '\blynx\b' "$f" 2>/dev/null || true)
done

if (( ${#hits[@]} > 0 )); then
  echo "binary-naming check: ${#hits[@]} stale 'unitpm' reference(s) found."
  echo "Either rename to 'unitpm', or add a justified entry to"
  echo "  scripts/check-binary-naming.sh (FILE_EXCLUDES_RE or ALLOW_SUBSTRINGS)."
  echo
  printf '  %s\n' "${hits[@]}"
  exit 1
fi

echo "binary-naming check: clean (${#FILES[@]} files scanned)"
