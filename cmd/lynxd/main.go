//go:build linux

// Package main is the entry point for the lynx daemon.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/daemon"
	"github.com/Jaro-c/Lynx/internal/daemon/manager"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
)

func main() {
	log.Println("lynxd starting...")

	mgr := manager.NewManager()
	server := transport.NewServer()

	// Register all handlers
	daemon.RegisterHandlers(server, mgr, false)

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
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	mgr.Shutdown()
	_ = server.Close()
}
