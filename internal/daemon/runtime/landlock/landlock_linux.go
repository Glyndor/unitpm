//go:build linux

// Package landlock provides a thin wrapper over the Linux Landlock syscalls
// for unprivileged filesystem sandboxing (kernel >= 5.13).
//
// The child process calls Apply(rules) before execve to restrict its own
// filesystem access. The ruleset is inherited across execve, is non-revocable,
// and applies to the current thread plus all descendants.
package landlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Raw syscall numbers for Landlock. x/sys/unix v0.42 exposes the constants
// and structs but no higher-level wrappers.
const (
	sysLandlockCreateRuleset = unix.SYS_LANDLOCK_CREATE_RULESET
	sysLandlockAddRule       = unix.SYS_LANDLOCK_ADD_RULE
	sysLandlockRestrictSelf  = unix.SYS_LANDLOCK_RESTRICT_SELF

	// Flag for LandlockCreateRuleset to query the ABI version.
	landlockCreateRulesetVersion = 1 << 0
)

// PathAccess describes access requested on a path prefix.
type PathAccess struct {
	// Path is the absolute directory (or file) that gates access.
	Path string
	// Read grants read-related filesystem rights under Path.
	Read bool
	// Write grants write-related filesystem rights under Path.
	Write bool
	// Execute grants execute rights on files under Path.
	Execute bool
}

// Ruleset is the full sandbox specification to apply.
type Ruleset struct {
	Allow []PathAccess
}

// Supported reports whether the running kernel supports Landlock ABI >= 1.
func Supported() bool {
	v, err := getABIVersion()
	return err == nil && v >= 1
}

func getABIVersion() (int, error) {
	r1, _, errno := unix.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 {
		return 0, errno
	}
	return int(r1), nil
}

// Apply activates the ruleset on the calling thread. Call in the child
// process before execve. Returns nil on kernels that do not support
// Landlock so callers can treat it as best-effort hardening.
func Apply(rs Ruleset) error {
	abi, err := getABIVersion()
	if err != nil || abi < 1 {
		return nil
	}

	handledFs := landlockFSMask(abi)

	attr := unix.LandlockRulesetAttr{
		Access_fs: handledFs,
	}

	r1, _, errno := unix.Syscall(
		sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	rulesetFD := int(r1)
	defer func() { _ = syscall.Close(rulesetFD) }()

	for _, a := range rs.Allow {
		if err := addPathRule(rulesetFD, a, handledFs); err != nil {
			return fmt.Errorf("landlock add rule for %q: %w", a.Path, err)
		}
	}

	// PR_SET_NO_NEW_PRIVS is required before LANDLOCK_RESTRICT_SELF.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}

	_, _, errno = unix.Syscall(sysLandlockRestrictSelf, uintptr(rulesetFD), 0, 0)
	if errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}
	return nil
}

func addPathRule(rulesetFD int, a PathAccess, handledMask uint64) error {
	if !filepath.IsAbs(a.Path) {
		return errors.New("path must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(a.Path)
	if err != nil {
		resolved = a.Path
	}

	fd, err := unix.Open(resolved, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		// Path does not exist or is inaccessible — skip silently so a missing
		// /lib64 on a pure-glibc system doesn't break the sandbox.
		return nil
	}
	defer func() { _ = syscall.Close(fd) }()

	allowed := accessMask(a, handledMask)
	if allowed == 0 {
		return nil
	}

	rule := unix.LandlockPathBeneathAttr{
		Allowed_access: allowed,
		Parent_fd:      int32(fd),
	}
	_, _, errno := unix.Syscall6(
		sysLandlockAddRule,
		uintptr(rulesetFD),
		unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&rule)),
		0, 0, 0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock_add_rule: %w", errno)
	}
	return nil
}

// accessMask builds the allowed_access bitmap from the high-level PathAccess.
func accessMask(a PathAccess, handledMask uint64) uint64 {
	var m uint64
	if a.Read {
		m |= unix.LANDLOCK_ACCESS_FS_READ_FILE
		m |= unix.LANDLOCK_ACCESS_FS_READ_DIR
	}
	if a.Write {
		m |= unix.LANDLOCK_ACCESS_FS_WRITE_FILE
		m |= unix.LANDLOCK_ACCESS_FS_REMOVE_DIR
		m |= unix.LANDLOCK_ACCESS_FS_REMOVE_FILE
		m |= unix.LANDLOCK_ACCESS_FS_MAKE_CHAR
		m |= unix.LANDLOCK_ACCESS_FS_MAKE_DIR
		m |= unix.LANDLOCK_ACCESS_FS_MAKE_REG
		m |= unix.LANDLOCK_ACCESS_FS_MAKE_SOCK
		m |= unix.LANDLOCK_ACCESS_FS_MAKE_FIFO
		m |= unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK
		m |= unix.LANDLOCK_ACCESS_FS_MAKE_SYM
	}
	if a.Execute {
		m |= unix.LANDLOCK_ACCESS_FS_EXECUTE
	}
	return m & handledMask
}

// landlockFSMask returns the union of filesystem rights supported at the
// given Landlock ABI version.
func landlockFSMask(abi int) uint64 {
	mask := uint64(
		unix.LANDLOCK_ACCESS_FS_EXECUTE |
			unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
			unix.LANDLOCK_ACCESS_FS_READ_FILE |
			unix.LANDLOCK_ACCESS_FS_READ_DIR |
			unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
			unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
			unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
			unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
			unix.LANDLOCK_ACCESS_FS_MAKE_REG |
			unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
			unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
			unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
			unix.LANDLOCK_ACCESS_FS_MAKE_SYM,
	)
	if abi >= 2 {
		mask |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		mask |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	return mask
}

// SensibleDefaults returns a ruleset that permits reading most of the
// filesystem (for runtime/loader/libs) but restricts writes to the provided
// workspace directory.
func SensibleDefaults(cwd, logDir string) Ruleset {
	rs := Ruleset{
		Allow: []PathAccess{
			{Path: "/usr", Read: true, Execute: true},
			{Path: "/bin", Read: true, Execute: true},
			{Path: "/sbin", Read: true, Execute: true},
			{Path: "/lib", Read: true, Execute: true},
			{Path: "/lib64", Read: true, Execute: true},
			{Path: "/proc", Read: true},
			{Path: "/sys", Read: true},
			{Path: "/dev", Read: true, Write: true},
			{Path: "/etc", Read: true},
			{Path: "/tmp", Read: true, Write: true, Execute: true},
			{Path: runtimeDir(), Read: true, Write: true, Execute: true},
		},
	}
	if cwd != "" {
		rs.Allow = append(rs.Allow, PathAccess{
			Path: cwd, Read: true, Write: true, Execute: true,
		})
	}
	if logDir != "" && logDir != cwd {
		rs.Allow = append(rs.Allow, PathAccess{
			Path: logDir, Read: true, Write: true,
		})
	}
	return rs
}

func runtimeDir() string {
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return v
	}
	return "/run"
}
