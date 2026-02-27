//go:build linux

package startup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Jaro-c/Lynx/internal/term"
)

var (
	getEuid  = os.Geteuid
	stat     = os.Stat
	lookPath = exec.LookPath
)

func runPlatformStartup(runner Runner) error {
	// 1. Check root
	if getEuid() != 0 {
		fmt.Println("Admin privileges required. Run:")
		fmt.Println(term.BoldString("  sudo lynx startup"))
		return errors.New("admin privileges required")
	}

	// 2. Detect systemd availability
	// if /run/systemd/system does not exist OR systemctl is not available
	_, errStat := stat("/run/systemd/system")
	_, errLook := lookPath("systemctl")

	if os.IsNotExist(errStat) || errLook != nil {
		return errors.New("ERR_UNSUPPORTED: Lynx requires Linux with systemd")
	}

	// 3. Run systemctl commands

	// 1) systemctl daemon-reload
	if _, stderr, _, err := runner.Run("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload daemon: %w\n%s", err, stderr)
	}

	// 2) systemctl enable --now lynx.lynxd.service
	if _, stderr, _, err := runner.Run("systemctl", "enable", "--now", "lynx.lynxd.service"); err != nil {
		return fmt.Errorf("failed to enable lynxd: %w\n%s", err, stderr)
	}

	// 3) systemctl is-active lynx.lynxd.service
	stdout, stderr, _, err := runner.Run("systemctl", "is-active", "lynx.lynxd.service")
	if err != nil {
		// is-active returns exit code 3 if inactive, check output
		return fmt.Errorf("lynxd service check failed: %w\n%s", err, stderr)
	}

	if strings.TrimSpace(stdout) != "active" {
		return fmt.Errorf("lynxd service is not active: %s (stderr: %s)", stdout, stderr)
	}

	fmt.Println("Lynx system daemon started. Autostart enabled.")
	return nil
}
