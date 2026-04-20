// Package main is a compiled Go worker that honours ctx-based graceful
// shutdown. Stdlib-only so it builds on any Go toolchain without
// go.mod gymnastics. Used by the Debian smoke to prove that `lynxpm`
// supervises statically-linked binaries identically to interpreted
// apps (shell/node/python).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("go-compiled pid=%d\n", os.Getpid())

	tick := 0
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("go-compiled received signal, exiting")
			return
		case <-ticker.C:
			fmt.Printf("go-compiled tick=%d\n", tick)
			tick++
		}
	}
}
