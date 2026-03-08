//go:build !windows

package paths

import "os"

func getEuid() int {
	return os.Geteuid()
}
