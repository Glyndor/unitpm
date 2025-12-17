// Package list implements the list command.
package list

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
)

// Run executes the list command.
func Run(client *ipc.Client) error {
	var processes []types.ProcessInfo
	if err := client.Call("list", nil, &processes); err != nil {
		return fmt.Errorf("list failed: %w", err)
	}
	renderTable(processes)
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

	t := newTable(headers)

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
		t.addRow(row)
	}

	t.render()
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

// table represents a printable table.
type table struct {
	headers []string
	rows    [][]string
}

// newTable creates a new Table with the given headers.
func newTable(headers []string) *table {
	return &table{
		headers: headers,
		rows:    [][]string{},
	}
}

// addRow adds a row of data to the table.
func (t *table) addRow(row []string) {
	t.rows = append(t.rows, row)
}

// render prints the table to stdout.
func (t *table) render() {
	// Calculate column widths
	widths := t.calculateWidths()

	// Print Top Border
	t.printBorder("┌", "┬", "┐", widths)

	// Print Headers
	t.printRow(t.headers, widths)

	// Print Header Separator
	t.printBorder("├", "┼", "┤", widths)

	// Print Rows
	for _, row := range t.rows {
		t.printRow(row, widths)
	}

	// Print Bottom Border
	t.printBorder("└", "┴", "┘", widths)
}

func (t *table) calculateWidths() []int {
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = utf8.RuneCountInString(h)
	}

	for _, row := range t.rows {
		for i, cell := range row {
			// Strip ANSI codes for length calculation
			l := utf8.RuneCountInString(stripAnsi(cell))
			if l > widths[i] {
				widths[i] = l
			}
		}
	}
	return widths
}

func (t *table) printBorder(left, mid, right string, widths []int) {
	fmt.Print(left)
	for i, w := range widths {
		fmt.Print(strings.Repeat("─", w+2)) // +2 for padding
		if i < len(widths)-1 {
			fmt.Print(mid)
		}
	}
	fmt.Println(right)
}

func (t *table) printRow(row []string, widths []int) {
	fmt.Print("│")
	for i, cell := range row {
		// We need to pad manually because Printf padding counts ANSI codes as characters
		// So we calculate the visible length
		visibleLen := utf8.RuneCountInString(stripAnsi(cell))
		padding := widths[i] - visibleLen
		fmt.Printf(" %s%s │", cell, strings.Repeat(" ", padding))
	}
	fmt.Println()
}

// Simple ANSI stripper for length calculation.
func stripAnsi(str string) string {
	var ret strings.Builder
	inSeq := false
	for _, r := range str {
		if r == '\033' {
			inSeq = true
			continue
		}
		if inSeq {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inSeq = false
			}
		} else {
			ret.WriteRune(r)
		}
	}
	return ret.String()
}
