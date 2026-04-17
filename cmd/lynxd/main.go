//go:build linux

// Package main is the entry point for the lynx daemon.
package main

import (
	"log"
	"os"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	"github.com/Jaro-c/Lynx/internal/daemon"
	"github.com/Jaro-c/Lynx/internal/daemon/audit"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

// auditPath returns the destination for the JSON-lines audit log. Empty
// string means audit is disabled (user mode, where the daemon is scoped
// to one user already).
func auditPath(systemDaemon bool) string {
	if !systemDaemon {
		return ""
	}
	return "/var/log/lynx-pm/audit.log"
}

// isSystemDaemon reports whether lynxd is the system-mode daemon, with
// the polkit grants to call systemd-run with DynamicUser. Covers both
// running as root and running as the `lynx` system user (the default
// deployment from the Debian package).
func isSystemDaemon() bool {
	if os.Geteuid() == 0 {
		return true
	}
	if u, err := user.Current(); err == nil && u.Username == "lynx" {
		return true
	}
	return false
}

func main() {
	log.Println("lynxd starting...")

	mgr := manager.NewManager()
	server := transport.NewServer()

	// Register all handlers
	privileged := isSystemDaemon()
	auditor := audit.Open(auditPath(privileged))
	daemon.RegisterHandlers(server, mgr, privileged, auditor)

	// Restore state
	log.Println("Restoring processes...")
	if err := mgr.Restore(); err != nil {
		log.Printf("Warning: Failed to restore state: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start IPC server: %v", err)
	}

	path, err := transport.GetSocketPath()
	if err != nil {
		log.Fatalf("Failed to get socket path: %v", err)
	}
	log.Printf("IPC server listening on %s", path)

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigCh {
		if sig == syscall.SIGHUP {
			log.Println("SIGHUP received — stopping processes and re-executing...")
			exe, err := os.Executable()
			if err != nil {
				log.Printf("re-exec: cannot resolve executable: %v", err)
				continue
			}
			// Graceful stop without marking Disabled so Restore() relaunches them.
			mgr.Shutdown()
			// Wait for child processes to fully release ports/resources.
			time.Sleep(500 * time.Millisecond)
			_ = server.Close()
			_ = auditor.Close()
			if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
				log.Fatalf("re-exec failed: %v — daemon exiting", err)
			}
		}
		break
	}

	log.Println("Shutting down...")
	mgr.Shutdown()
	_ = server.Close()
	_ = auditor.Close()
}
