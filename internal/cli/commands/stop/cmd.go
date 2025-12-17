// Package stop implements the stop command.
package stop

import (
	"errors"

	"github.com/Jaro-c/Lynx/internal/ipc"
)

// Run executes the stop command.
func Run(_ *ipc.Client) error {
	// TODO: Implement stop command logic here
	return errors.New("stop command not implemented yet")
}
