//go:build !linux

package metrics

import "errors"

// GetProcessTree is not supported on non-Linux platforms.
func GetProcessTree(_ int) ([]ChildStat, error) {
	return nil, errors.New("process tree not supported on this platform")
}
