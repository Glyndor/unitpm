#!/usr/bin/env bash
# check-binary-naming.sh — fail if the old CLI name `lynx` slips back into the
# tree where it should be `lynxpm`. The binary was renamed from `lynx` to
# `lynxpm` in 0.7.x; the renames in PRs #26-#29 fixed every residual hit and
# this guard prevents regressions.
#
# Logic:
#   - Scan tracked source files (no built artifacts, vendor, lockfiles).
#   - Skip whole files that are legitimately allowed to mention `lynx`
#     (defined-once constants, test fixtures, bench scenarios).
#   - For remaining files, flag any line that matches the bare word `lynx`.
#   - Pardon a flagged line if it contains any allowlisted substring
#     (system user, socket file, config dir, polkit unit prefix, etc.).
#
# To allow a new context, add to ALLOW_SUBSTRINGS below with a comment
# explaining why.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Files where `lynx` is the canonical name and the check would just fight us.
FILE_EXCLUDES_RE='(^|/)('
FILE_EXCLUDES_RE+='.*_test\.go$'                    # test fixtures, arbitrary names
FILE_EXCLUDES_RE+='|scripts/bench/'                 # bench scenario scripts + Dockerfile
FILE_EXCLUDES_RE+='|site/dist/'                     # built site
FILE_EXCLUDES_RE+='|site/node_modules/'             # third-party
FILE_EXCLUDES_RE+='|node_modules/'                  # idem
FILE_EXCLUDES_RE+='|vendor/'                        # idem
FILE_EXCLUDES_RE+='|debian/changelog$'              # historical entries
FILE_EXCLUDES_RE+='|internal/paths/system_mode\.go$' # const SystemUser = "lynx"
FILE_EXCLUDES_RE+='|scripts/check-binary-naming\.sh$' # this script (defines the patterns)
FILE_EXCLUDES_RE+='|.*\.lock$'
FILE_EXCLUDES_RE+='|.*\.lock\.json$'
FILE_EXCLUDES_RE+='|package-lock\.json$'
FILE_EXCLUDES_RE+=')'

# Substrings that, when present on a flagged line, mark it as intentional.
ALLOW_SUBSTRINGS=(
  # system user lynx (Debian postinst-created, distinct from CLI)
  '`lynx`'
  'user `lynx`'
  '`lynx` user'
  'lynx system user'
  'system user `lynx`'
  'lynx user'             # "lynx user creation", "the lynx user", etc.
  'non-lynx'              # tests differentiating lynx vs non-lynx daemons
  'lynx-owned'            # idem
  'lynx daemon is left'   # idem
  '"lynx"'                # const SystemUser = "lynx", Username == "lynx"
  "'lynx'"                # alt quoting
  'User=lynx'
  'Group=lynx'
  'chown lynx'
  'lynx:lynx'
  'adduser lynx'
  'getent passwd lynx'
  '/var/lib/lynx-pm lynx'

  # real filesystem paths (XDG project dir is "lynx", distinct from CLI binary)
  'lynx.sock'
  'lynx-<uid>'
  'XDG_RUNTIME_DIR/lynx-'
  '.config/lynx'          # also matches ~/.config/lynx and absolute /home/.../.config/lynx
  'XDG_CONFIG_HOME/lynx'
  '"lynx/logs"'           # filepath.Join(..., "lynx/logs") in paths/logs.go
  '/lynx/logs'             # rendered absolute form of XDG_STATE_HOME/lynx/logs
  '.local/state/lynx'     # default XDG_STATE_HOME path
  '/run/lynxd'
  '/var/lib/lynx-pm'
  '/var/log/lynx-pm'
  '/tmp/lynx-'            # security comment about /tmp socket hijack class

  # polkit unit prefix (must match policy)
  'lynx-app-'
  'lynx-`'                # polkit policy doc text
  '"lynx-"'               # JS in polkit.rules
  'with the `lynx-`'
  'lynx-*'                # docs/comments referring to the prefix glob
  'scoped to lynx-'       # polkit comment phrasing

  # backward-compat upgrade fallbacks (legacy debian tests probe both)
  'lynx.polkit.rules'

  # docs example for systemd unit naming convention (lynx-{processname}.service)
  'lynx-api'

  # bench scenario tag (product name)
  'lynx-bench'

  # historical debhelper build dirs (not produced anymore)
  'debian/lynx/'
  'debian/lynx-pm/'

  # site assets + product comparison styling (Lynx the product)
  'lynx.svg'
  'compare__td-lynx'
  'compare__th-lynx'

  # docs render the system user value in table output
  '| lynx '
  '| lynx|'

  # package + product capitalized references
  'Lynx'
  'lynx-pm'
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
  echo "binary-naming check: ${#hits[@]} stale 'lynx' reference(s) found."
  echo "Either rename to 'lynxpm', or add a justified entry to"
  echo "  scripts/check-binary-naming.sh (FILE_EXCLUDES_RE or ALLOW_SUBSTRINGS)."
  echo
  printf '  %s\n' "${hits[@]}"
  exit 1
fi

echo "binary-naming check: clean (${#FILES[@]} files scanned)"
