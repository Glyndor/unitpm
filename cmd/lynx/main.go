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
	defer func() {
		_ = client.Close()
	}()

	switch command {
	case "ping":
		var result map[string]string
		if err := client.Call("ping", nil, &result); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", term.RedString("Ping failed: %v", err))
			os.Exit(1)
		}
		fmt.Printf("%s %s\n", term.GreenString("Success"), term.BoldString("pong"))

	case "status", "list":
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
			statusStr = term.GreenString("%s", p.State)
		case types.StateStopped, types.StateFailed:
			statusStr = term.RedString("%s", p.State)
		case types.StateRestarting:
			statusStr = term.YellowString("%s", p.State)
		default:
			statusStr = string(p.State)
		}

		// Formatting helpers
		pidStr := fmt.Sprintf("%d", p.PID)
		if p.PID == 0 {
			pidStr = term.DimString("-")
		}

		uptimeStr := formatUptime(p.Uptime)
		memStr := formatBytes(p.Memory)

		cpuStr := fmt.Sprintf("%.1f%%", p.CPU)
		if p.CPU == 0 {
			cpuStr = "0%"
		}

		var watchStr string
		if p.Watch {
			watchStr = term.GreenString("enabled")
		} else {
			watchStr = term.DimString("disabled")
		}

		row := []string{
			fmt.Sprintf("%d", p.ID),
			term.BoldString("%s", p.Name),
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

// formatUptime formats milliseconds into a human-readable string (max 2 units)
func formatUptime(ms int64) string {
	if ms <= 0 {
		return term.DimString("-")
	}

	d := time.Duration(ms) * time.Millisecond
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		if seconds > 0 {
			return fmt.Sprintf("%dm %ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}

	return fmt.Sprintf("%ds", seconds)
}

// formatBytes formats bytes into human readable string (B, KB, MB, GB, TB)
func formatBytes(b int64) string {
	if b <= 0 {
		return term.DimString("-")
	}

	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}

	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	value := float64(b) / float64(div)
	suffix := "KMGT"[exp]

	// Format with 1 decimal place
	return fmt.Sprintf("%.1f %cB", value, suffix)
}
