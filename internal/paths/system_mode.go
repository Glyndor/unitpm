//go:build linux

package paths

import "os/user"

// SystemUser is the dedicated unprivileged uid the Debian package
// provisions for lynxd. Mirrors debian/postinst.
const SystemUser = "lynx"

var currentUsername = func() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}

// IsSystemMode reports whether lynxd is the system-mode daemon — running
// as root, or as the dedicated `lynx` system user installed by the Debian
// package. Both cases share the same trust posture: requests come from
// lynxadm-group callers via /run/lynxd/lynx.sock and writes target the
// system layout under /var/{lib,log}/lynx-pm.
func IsSystemMode() bool {
	if IsRoot() {
		return true
	}
	return currentUsername() == SystemUser
}
