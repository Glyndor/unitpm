package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		// Default to list if no args provided? PM2 does this.
		// Prompt says "lynx list", let's require "list" for now or show usage.
		// "The command: lynx list Must display..."
		fmt.Println("Usage: lynx <command>")
		os.Exit(1)
	}

	command := os.Args[1]

	// Common client setup
	client, err := ipc.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Failed to connect to daemon: %v", err))
		os.Exit(1)
	}
	defer client.Close()

	switch command {
	case "ping":
		var result map[string]string
		if err := client.Call("ping", nil, &result); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Ping failed: %v", err))
			os.Exit(1)
		}
		fmt.Printf("%s %s\n", term.GreenString("Success"), term.BoldString("pong"))

	case "status", "list": // Alias status to list for backward compatibility/convenience
		var processes []types.ProcessInfo
		if err := client.Call("list", nil, &processes); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", term.RedString("List failed: %v", err))
			os.Exit(1)
		}
		renderTable(processes)

	default:
		fmt.Printf("%s\n", term.YellowString("Unknown command: %s", command))
		os.Exit(1)
	}
}

func renderTable(processes []types.ProcessInfo) {
	if len(processes) == 0 {
		fmt.Println("No processes managed.")
		return
	}

	// id | name | namespace | version | mode | pid | uptime | ↺ | status | cpu | mem | user | watch
	headers := []string{
		"id", "name", "namespace", "version", "mode", "pid", "uptime", "↺", "status", "cpu", "mem", "user", "watch",
	}

	t := NewTable(headers)

	for _, p := range processes {
		// Colors based on state
		var statusStr string
		switch p.State {
		case types.StateRunning, types.StateOnline:
			statusStr = term.GreenString(string(p.State))
		case types.StateStopped, types.StateFailed:
			statusStr = term.RedString(string(p.State))
		case types.StateRestarting:
			statusStr = term.YellowString(string(p.State))
		default:
			statusStr = string(p.State)
		}

		// Formatting helpers
		pidStr := fmt.Sprintf("%d", p.PID)
		if p.PID == 0 {
			pidStr = term.DimString("-")
		}

		uptimeStr := formatDuration(time.Duration(p.Uptime) * time.Millisecond)
		if p.Uptime == 0 {
			uptimeStr = term.DimString("-")
		}

		memStr := formatBytes(p.Memory)

		cpuStr := fmt.Sprintf("%.1f%%", p.CPU)
		if p.CPU == 0 {
			cpuStr = "0%"
		}

		watchStr := "disabled"
		if p.Watch {
			watchStr = term.GreenString("enabled")
		} else {
			watchStr = term.DimString("disabled")
		}

		row := []string{
			fmt.Sprintf("%d", p.ID),
			term.BoldString(p.Name),
			p.Namespace,
			p.Version,
			p.Mode,
			pidStr,
			uptimeStr,
			fmt.Sprintf("%d", p.Restarts),
			statusStr,
			cpuStr,
			memStr,
			p.User,
			watchStr,
		}
		t.AddRow(row)
	}

	t.Render()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	// PM2 usually shows mb
	return fmt.Sprintf("%.1fmb", float64(b)/float64(1024*1024))
}
