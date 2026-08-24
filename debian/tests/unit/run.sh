#!/bin/sh
# Portable unit-test runner for debian/postinst and debian/prerm.
#
# These tests do NOT install the package. They run the maintainer scripts
# against a sandbox of mocked system binaries (getent, adduser, pgrep, ps,
# kill, mkdir, chown, chmod) and verify the expected calls.
#
# Run with: sh debian/tests/unit/run.sh
set -eu

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
POSTINST="$REPO_ROOT/debian/postinst"
PRERM="$REPO_ROOT/debian/prerm"

PASS=0
FAIL=0
FAILED_TESTS=""

# ---- helpers ---------------------------------------------------------------

# mkmock <name> [exit_code]
# Creates an executable in $MOCKS named <name> that records its invocation
# (one $name<tab>$* line per call) into $CALLS_LOG and exits with exit_code (default 0).
# Uses underscored variable names to avoid clobbering callers' $name.
mkmock() (
    _m_name=$1
    _m_code=${2:-0}
    cat >"$MOCKS/$_m_name" <<EOF
#!/bin/sh
printf '%s\t%s\n' "$_m_name" "\$*" >> "\$CALLS_LOG"
exit $_m_code
EOF
    /bin/chmod +x "$MOCKS/$_m_name"
)

# Intercepts destructive system calls (mkdir/chown/chmod) by routing them to
# safe paths under $TEST_ROOT.
mkmock_mkdir() (
    cat >"$MOCKS/mkdir" <<'EOF'
#!/bin/sh
printf 'mkdir\t%s\n' "$*" >> "$CALLS_LOG"
# Strip absolute paths and re-anchor under TEST_ROOT.
args=""
for a in "$@"; do
    case "$a" in
        /*) args="$args $TEST_ROOT$a" ;;
        *)  args="$args $a" ;;
    esac
done
# shellcheck disable=SC2086
exec /bin/mkdir $args
EOF
    /bin/chmod +x "$MOCKS/mkdir"
)

reset_env() {
    rm -rf "$TEST_ROOT" "$MOCKS"
    mkdir -p "$TEST_ROOT" "$MOCKS"
    CALLS_LOG="$TEST_ROOT/calls.log"
    : >"$CALLS_LOG"
    export CALLS_LOG TEST_ROOT
    PATH="$MOCKS:/usr/bin:/bin"
    export PATH
}

assert_called() (
    _ac_needle=$1
    _ac_msg=$2
    if tr '\t' ' ' < "$CALLS_LOG" | grep -qF "$_ac_needle"; then
        exit 0
    fi
    echo "    FAIL: expected call not seen: $_ac_needle ($_ac_msg)"
    echo "    --- calls log ---"
    sed 's/^/    /' "$CALLS_LOG"
    exit 1
)

assert_not_called() (
    _anc_needle=$1
    _anc_msg=$2
    if tr '\t' ' ' < "$CALLS_LOG" | grep -qF "$_anc_needle"; then
        echo "    FAIL: unexpected call: $_anc_needle ($_anc_msg)"
        exit 1
    fi
)

run_test() {
    _rt_name=$1
    _rt_fn=$2
    reset_env
    if "$_rt_fn"; then
        PASS=$((PASS + 1))
        echo "  ok  $_rt_name"
    else
        FAIL=$((FAIL + 1))
        FAILED_TESTS="$FAILED_TESTS $_rt_name"
        echo "  not ok $_rt_name"
    fi
}

TEST_ROOT_BASE=$(mktemp -d)
TEST_ROOT="$TEST_ROOT_BASE/sandbox"
MOCKS="$TEST_ROOT_BASE/mocks"

cleanup() { rm -rf "$TEST_ROOT_BASE"; }
trap cleanup EXIT

# ---- test cases ------------------------------------------------------------

test_configure_creates_user_when_missing() {
    mkmock getent 1   # group/user lookup → not found
    mkmock addgroup
    mkmock adduser
    mkmock_mkdir
    mkmock chown
    mkmock chmod
    mkmock pgrep 1    # no running daemon
    mkmock ps
    mkmock kill

    sh "$POSTINST" configure || return 1
    assert_called "addgroup --system unitpm" "unitpm group creation" || return 1
    assert_called "adduser --system" "glyndor-unitpm user creation" || return 1
    assert_called "adduser glyndor-unitpm unitpm" "membership" || return 1
    assert_called "chown glyndor-unitpm:glyndor-unitpm" "ownership" || return 1
    assert_called "chmod 0700" "0700 perms" || return 1
}

test_configure_skips_creation_when_present() {
    mkmock getent 0   # exists
    mkmock addgroup
    mkmock adduser
    mkmock_mkdir
    mkmock chown
    mkmock chmod
    mkmock pgrep 1
    mkmock ps
    mkmock kill

    sh "$POSTINST" configure || return 1
    assert_not_called "addgroup --system unitpm" "skip group create" || return 1
    assert_not_called "adduser --system" "skip user create" || return 1
    assert_called "adduser glyndor-unitpm unitpm" "membership ensured" || return 1
}

test_configure_signals_user_daemons() {
    # Skip if bash is unavailable: dash's `kill` is a builtin we cannot shadow
    # via PATH, so the assertion would always fail under plain /bin/sh.
    if ! command -v bash >/dev/null 2>&1; then
        echo "    skipped (bash required to disable kill builtin)"
        return 0
    fi
    mkmock getent 0
    mkmock addgroup
    mkmock adduser
    mkmock_mkdir
    mkmock chown
    mkmock chmod
    # pgrep returns 2 fake pids
    cat >"$MOCKS/pgrep" <<'EOF'
#!/bin/sh
printf 'pgrep\t%s\n' "$*" >> "$CALLS_LOG"
echo 4242
echo 4243
EOF
    /bin/chmod +x "$MOCKS/pgrep"
    # ps says first pid runs as bob (non-glyndor-unitpm), second as glyndor-unitpm (system daemon).
    cat >"$MOCKS/ps" <<'EOF'
#!/bin/sh
printf 'ps\t%s\n' "$*" >> "$CALLS_LOG"
case "$*" in
    *4242*) echo "bob" ;;
    *4243*) echo "glyndor-unitpm" ;;
esac
EOF
    /bin/chmod +x "$MOCKS/ps"
    mkmock kill

    # bash treats `kill` as a builtin; disabling it via BASH_ENV in a
    # non-interactive subshell forces PATH lookup so our mock is used.
    bash_env=$TEST_ROOT/disable-kill.sh
    echo "enable -n kill" >"$bash_env"
    BASH_ENV=$bash_env bash "$POSTINST" configure || return 1
    # Only the non-glyndor-unitpm user pid should be HUP'd; glyndor-unitpm-owned daemon is left
    # for systemd's restart hook.
    assert_called "kill -HUP 4242" "HUP non-glyndor-unitpm daemon" || return 1
    assert_not_called "kill -HUP 4243" "skip glyndor-unitpm-owned daemon" || return 1
}

test_postinst_noop_for_other_actions() {
    mkmock getent
    mkmock addgroup
    mkmock adduser
    mkmock_mkdir
    mkmock chown
    mkmock chmod
    mkmock pgrep 1
    mkmock ps
    mkmock kill

    sh "$POSTINST" abort-upgrade 1.0.0 || return 1
    assert_not_called "addgroup" "no group ops on non-configure" || return 1
    assert_not_called "adduser" "no user ops on non-configure" || return 1
    assert_not_called "chmod" "no chmod on non-configure" || return 1
}

test_prerm_runs_clean() {
    sh "$PRERM" remove >/dev/null 2>&1 || return 1
}

# ---- runner ---------------------------------------------------------------

echo "TAP version 13"
run_test "configure: creates user/group when missing" test_configure_creates_user_when_missing
run_test "configure: skips creation when present"     test_configure_skips_creation_when_present
run_test "configure: HUPs only non-glyndor-unitpm user daemons" test_configure_signals_user_daemons
run_test "postinst: no-op on non-configure actions"   test_postinst_noop_for_other_actions
run_test "prerm: runs to completion"                  test_prerm_runs_clean

echo
echo "1..$((PASS + FAIL))"
echo "passed: $PASS, failed: $FAIL"
if [ "$FAIL" -gt 0 ]; then
    echo "failures:$FAILED_TESTS"
    exit 1
fi
