//go:build linux

package startup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/Jaro-c/Lynx/internal/term"
)

var (
	getEuid  = os.Geteuid
	stat     = os.Stat
	lookPath = exec.LookPath
)

// systemdUserUnit is the template for the user-level systemd service
const systemdUserUnit = `[Unit]
Description=Lynx Process Manager (User Daemon)
Documentation=https://github.com/Jaro-c/Lynx
After=network.target

[Service]
Type=simple
ExecStart=%s
Restart=always
RestartSec=3
Environment="LYNX_SOCKET=%s"

[Install]
WantedBy=default.target
`

func runPlatformStartup(runner Runner) error {
	// 1. Detect systemd availability
	// if /run/systemd/system does not exist OR systemctl is not available
	_, errStat := stat("/run/systemd/system")
	_, errLook := lookPath("systemctl")

	if os.IsNotExist(errStat) || errLook != nil {
		return errors.New("ERR_UNSUPPORTED: Lynx requires Linux with systemd")
	}

	// 2. Check if running as root (System Mode)
	if getEuid() == 0 {
		return runSystemStartup(runner)
	}

	// 3. Running as non-root (User Mode)
	return runUserStartup(runner)
}

func runSystemStartup(runner Runner) error {
	fmt.Println("Detected root user. Installing system-wide daemon...")

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

	fmt.Println(term.GreenString("✅ Lynx system daemon started. Autostart enabled."))
	return nil
}

func runUserStartup(runner Runner) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	fmt.Printf("Detected user mode (%s). Installing user daemon...\n", currentUser.Username)

	// 1. Create ~/.config/systemd/user directory
	configDir := filepath.Join(currentUser.HomeDir, ".config", "systemd", "user")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	// 2. Locate lynxd binary
	lynxdPath, err := exec.LookPath("lynxd")
	if err != nil {
		// Fallback to common locations if not in PATH
		if _, err := os.Stat("/usr/sbin/lynxd"); err == nil {
			lynxdPath = "/usr/sbin/lynxd"
		} else if _, err := os.Stat("/usr/local/bin/lynxd"); err == nil {
			lynxdPath = "/usr/local/bin/lynxd"
		} else {
			return errors.New("lynxd binary not found. Please install Lynx correctly")
		}
	}

	// Resolve absolute path
	lynxdPath, _ = filepath.Abs(lynxdPath)

	// 3. Generate Unit File
	// Default user socket path logic mirrors socket_unix.go
	// We don't strictly need to set LYNX_SOCKET env if we use defaults,
	// but it's safer to be explicit if needed. For now, let's rely on default behavior.
	// But we DO need to know where the binary is.
	unitContent := fmt.Sprintf(systemdUserUnit, lynxdPath, "")

	unitPath := filepath.Join(configDir, "lynx.service")
	if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("failed to write unit file: %w", err)
	}
	fmt.Printf("Created unit file at %s\n", unitPath)

	// 4. Enable Lingering (Persist after logout)
	// We need to use loginctl. This might require PolicyKit or being in the right group,
	// but usually users can enable lingering for themselves.
	fmt.Println("Enabling lingering to keep process running after logout...")
	if _, stderr, _, err := runner.Run("loginctl", "enable-linger", currentUser.Username); err != nil {
		fmt.Print(term.YellowString("Warning: Failed to enable lingering: %v\n%s\n", err, stderr))
		fmt.Println("You might need to run this manually: sudo loginctl enable-linger " + currentUser.Username)
	} else {
		fmt.Println("Lingering enabled.")
	}

	// 5. Systemd User Commands
	// systemctl --user daemon-reload
	if _, stderr, _, err := runner.Run("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload user daemon: %w\n%s", err, stderr)
	}

	// systemctl --user enable --now lynx
	if _, stderr, _, err := runner.Run("systemctl", "--user", "enable", "--now", "lynx"); err != nil {
		return fmt.Errorf("failed to enable user lynxd: %w\n%s", err, stderr)
	}

	fmt.Println(term.GreenString("✅ Lynx user daemon started and enabled for autostart."))
	fmt.Println("You can manage it with: systemctl --user status lynx")
	return nil
}
