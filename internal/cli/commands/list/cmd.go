// Package list implements the list command.
package list

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jaro-c/Lynx/internal/ipc"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
	xterm "golang.org/x/term"
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
		term.CyanString("%s", term.BoldString("id")),
		term.CyanString("%s", term.BoldString("name")),
		term.CyanString("%s", term.BoldString("namespace")),
		term.CyanString("%s", term.BoldString("version")),
		term.CyanString("%s", term.BoldString("mode")),
		term.CyanString("%s", term.BoldString("pid")),
		term.CyanString("%s", term.BoldString("uptime")),
		term.CyanString("%s", term.BoldString("↺")),
		term.CyanString("%s", term.BoldString("status")),
		term.CyanString("%s", term.BoldString("cpu")),
		term.CyanString("%s", term.BoldString("mem")),
		term.CyanString("%s", term.BoldString("user")),
		term.CyanString("%s", term.BoldString("watch")),
	}

	t := newTable(headers)
	t.maxColWidths = []int{
		5,  // id
		30, // name
		20, // namespace
		10, // version
		10, // mode
		8,  // pid
		10, // uptime
		5,  // restarts
		15, // status
		8,  // cpu
		10, // mem
		15, // user
		10, // watch
	}

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

// PrintHelp prints the help message for the list command.
func PrintHelp() {
	fmt.Println()
	fmt.Printf("%s\n", term.CyanString("Usage:"))
	fmt.Printf("  %s [options]\n", term.BoldString("lynx list|ls|ps"))
	fmt.Println()
	fmt.Printf("%s\n", term.CyanString("Description:"))
	fmt.Println("  List all managed processes.")
	fmt.Println()
	fmt.Printf("%s\n", term.CyanString("Options:"))
	fmt.Printf(
		"  %s, %s    Show this help message.\n",
		term.BoldString("-h"),
		term.BoldString("--help"),
	)
	fmt.Println()
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
	headers      []string
	rows         [][]string
	maxColWidths []int
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
	// Get terminal width
	width, _, err := xterm.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 120 // Fallback
	}

	// Calculate column widths
	widths := t.calculateWidths(width)

	// Print table
	t.printTable(widths)
}

func (t *table) printTable(widths []int) {
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

func (t *table) calculateWidths(termWidth int) []int {
	widths := make([]int, len(t.headers))

	// Initial width based on headers
	for i, h := range t.headers {
		widths[i] = utf8.RuneCountInString(stripAnsi(h))
	}

	// Max content width
	for _, row := range t.rows {
		for i, cell := range row {
			l := utf8.RuneCountInString(stripAnsi(cell))
			if l > widths[i] {
				widths[i] = l
			}
		}
	}

	// Apply configured maximums
	if len(t.maxColWidths) == len(widths) {
		for i, maxW := range t.maxColWidths {
			if widths[i] > maxW {
				widths[i] = maxW
			}
		}
	}

	// Check if we exceed terminal width
	totalWidth := 1 // Initial border
	for _, w := range widths {
		totalWidth += w + 3 // Padding + border
	}

	// If we exceed terminal width, we need to shrink columns
	// Simple strategy: shrink largest columns first until we fit
	if totalWidth > termWidth {
		// TODO: Implement more sophisticated column shrinking if needed
		// For now, we rely on wrapping to handle the overflow visually if we force widths down
		_ = 0
	}

	return widths
}

func (t *table) printBorder(left, mid, right string, widths []int) {
	fmt.Print(term.DimString("%s", left))
	for i, w := range widths {
		fmt.Print(term.DimString("%s", strings.Repeat("─", w+2))) // +2 for padding
		if i < len(widths)-1 {
			fmt.Print(term.DimString("%s", mid))
		}
	}
	fmt.Println(term.DimString("%s", right))
}

func (t *table) printRow(row []string, widths []int) {
	// Pre-calculate wrapped lines for each cell
	cellLines := make([][]string, len(row))
	maxLines := 1

	for i, cell := range row {
		width := widths[i]
		lines := wrapText(cell, width)
		cellLines[i] = lines
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}

	// Print each line of the row
	for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
		fmt.Print(term.DimString("│"))
		for i := range row {
			var cellContent string
			if lineIdx < len(cellLines[i]) {
				cellContent = cellLines[i][lineIdx]
			} else {
				cellContent = ""
			}

			// We need to pad manually because Printf padding counts ANSI codes as characters
			// So we calculate the visible length
			visibleLen := utf8.RuneCountInString(stripAnsi(cellContent))
			padding := widths[i] - visibleLen
			if padding < 0 {
				padding = 0
			}
			fmt.Printf(" %s%s ", cellContent, strings.Repeat(" ", padding))
			fmt.Print(term.DimString("│"))
		}
		fmt.Println()
	}
}

// wrapText wraps a string to the given width, preserving ANSI codes if possible (simplified).
// Note: This simplified wrapper assumes ANSI codes are not split across breaks.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	visibleLen := utf8.RuneCountInString(stripAnsi(text))
	if visibleLen <= width {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var lines []string
	currentLine := ""
	currentLen := 0

	for _, word := range words {
		wordLen := utf8.RuneCountInString(stripAnsi(word))

		if wordLen > width {
			if currentLen > 0 {
				lines = append(lines, currentLine)
				currentLine = ""
				currentLen = 0
			}
			lines = append(lines, splitLongWord(word, width)...)
			continue
		}

		if currentLen+wordLen+1 > width && currentLen > 0 {
			lines = append(lines, currentLine)
			currentLine = word
			currentLen = wordLen
		} else {
			if currentLen > 0 {
				currentLine += " " + word
				currentLen += 1 + wordLen
			} else {
				currentLine = word
				currentLen = wordLen
			}
		}
	}
	if currentLen > 0 {
		lines = append(lines, currentLine)
	}

	return lines
}

func splitLongWord(word string, width int) []string {
	var parts []string
	for len(word) > 0 {
		take := width
		if utf8.RuneCountInString(word) < take {
			take = utf8.RuneCountInString(word)
		}

		byteIdx := 0
		count := 0
		for i := range word {
			if count == take {
				byteIdx = i
				break
			}
			count++
		}
		if byteIdx == 0 {
			byteIdx = len(word)
		}

		parts = append(parts, word[:byteIdx])
		word = word[byteIdx:]
	}
	return parts
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
