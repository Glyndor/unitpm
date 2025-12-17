// Package main is the entry point for the lynx daemon.
package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jaro-c/Lynx/internal/daemon"
	"github.com/Jaro-c/Lynx/internal/ipc"
)

func main() {
	log.Println("lynxd starting...")

	mgr := daemon.NewManager()
	server := ipc.NewServer()

	// Register ping handler
	server.Register("ping", func(_ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"response": "pong"})
	})

	// Register start handler
	server.Register("start", func(params json.RawMessage) (json.RawMessage, error) {
		var args struct {
			Name    string `json:"name"`
			Command string `json:"command"`
		}
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		id, err := mgr.Start(args.Name, args.Command)
		if err != nil {
			return nil, err
		}

		return json.Marshal(map[string]int{"id": id})
	})

	// Register stop handler
	server.Register("stop", func(params json.RawMessage) (json.RawMessage, error) {
		var args struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(params, &args); err != nil {
			return nil, err
		}

		if err := mgr.Stop(args.ID); err != nil {
			return nil, err
		}

		return json.Marshal(map[string]string{"status": "stopped"})
	})

	// Register list handler (replacing status)
	// Returns a list of processes with their detailed status
	server.Register("list", func(_ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(mgr.List())
	})

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start IPC server: %v", err)
	}

	path, err := ipc.GetSocketPath()
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
