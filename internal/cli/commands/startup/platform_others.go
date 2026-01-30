//go:build !linux

package startup

import (
	"errors"
)

func runPlatformStartup(runner Runner) error {
	return errors.New("startup command is only supported on Linux")
}
