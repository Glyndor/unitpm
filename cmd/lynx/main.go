// Package main provides the CLI for interacting with the Lynx daemon.
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", term.RedString("%v", err))
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		fmt.Println("Usage: lynx <command>")
		return nil
	}

	command := os.Args[1]

	// Common client setup
	client, err := ipc.NewClient()
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer func() {
		_ = client.Close()
	}()

	switch command {
	case "ping":
		var result map[string]string
		if err := client.Call("ping", nil, &result); err != nil {
			return fmt.Errorf("ping failed: %w", err)
		}
		fmt.Printf("%s %s\n", term.GreenString("Success"), term.BoldString("pong"))

	case "status", "list":
		var processes []types.ProcessInfo
		if err := client.Call("list", nil, &processes); err != nil {
			return fmt.Errorf("list failed: %w", err)
		}
		renderTable(processes)

	default:
		return fmt.Errorf("unknown command: %s", command)
	}

	return nil
}

func renderTable(processes []types.ProcessInfo) {
	// id | name | namespace | version | mode | pid | uptime | ↺ | status | cpu | mem | user | watch
	headers := []string{
		term.MagentaString("id"),
		term.MagentaString("name"),
		term.MagentaString("namespace"),
		term.MagentaString("version"),
		term.MagentaString("mode"),
		term.MagentaString("pid"),
		term.MagentaString("uptime"),
		term.MagentaString("↺"),
		term.MagentaString("status"),
		term.MagentaString("cpu"),
		term.MagentaString("mem"),
		term.MagentaString("user"),
		term.MagentaString("watch"),
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
		pidStr := strconv.Itoa(p.PID)
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
			strconv.Itoa(p.ID),
			term.BoldString("%s", p.Name),
			p.Namespace,
			p.Version,
			p.Mode,
			pidStr,
			uptimeStr,
			strconv.Itoa(p.Restarts),
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

// formatUptime formats milliseconds into a human-readable string (max 2 units).
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

// formatBytes formats bytes into human readable string (B, KB, MB, GB, TB).
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
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
