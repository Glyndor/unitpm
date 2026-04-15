// Package list implements the list command.
//
//nolint:gocognit,gocyclo,cyclop
package list

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
	xterm "golang.org/x/term"
)

// DefaultNamespace is the namespace used when an AppSpec has no explicit
// namespace set, both for storage and for `lynx list --namespace` filtering.
const DefaultNamespace = "default"

// Run executes the list command.
func Run(client transport.IPCClient, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	var showLong bool
	var namespaceFilter string
	var sortSpec string
	var jsonOutput bool
	fs.BoolVar(&showLong, "long", false, "Show full process IDs")
	fs.StringVar(&namespaceFilter, "namespace", "", "Filter by namespace")
	fs.StringVar(&sortSpec, "sort", "", "Sort order, e.g. 'namespace:asc,name:asc,createdAt:desc'")
	fs.BoolVar(&jsonOutput, "json", false, "Emit the process list as JSON on stdout")

	if err := fs.Parse(args); err != nil {
		if strings.HasPrefix(err.Error(), "flag provided but not defined: -") {
			flagName := strings.TrimPrefix(err.Error(), "flag provided but not defined: -")
			return &errs.UsageError{Message: "Unknown flag: -" + flagName}
		}
		return &errs.UsageError{Message: err.Error()}
	}

	if len(fs.Args()) > 0 {
		return &errs.UsageError{Message: fmt.Sprintf("Unexpected arguments: %v", fs.Args())}
	}

	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

	var processes []types.ProcessInfo
	if err := client.Call("list", nil, &processes); err != nil {
		return fmt.Errorf("list failed: %w", err)
	}

	processes = filterProcesses(processes, namespaceFilter)

	if err := sortProcesses(processes, sortSpec); err != nil {
		return err
	}

	if jsonOutput {
		if processes == nil {
			processes = []types.ProcessInfo{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(processes)
	}

	renderTable(processes, showLong)
	return nil
}

func filterProcesses(processes []types.ProcessInfo, filter string) []types.ProcessInfo {
	if filter == "" {
		return processes
	}
	filtered := processes[:0]
	for _, p := range processes {
		ns := p.Namespace
		if ns == "" {
			ns = DefaultNamespace
		}
		if ns == filter {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func sortProcesses(processes []types.ProcessInfo, spec string) error {
	fields, err := ParseSortSpec(spec)
	if err != nil {
		return err
	}

	if len(fields) == 0 {
		fields = []SortField{
			{Field: "namespace", Asc: true},
			{Field: "name", Asc: true},
			{Field: "createdAt", Asc: false},
			{Field: "id", Asc: true},
		}
	}

	sort.Slice(processes, func(i, j int) bool {
		for _, f := range fields {
			if res := compareProcess(processes[i], processes[j], f); res != 0 {
				return res < 0
			}
		}
		return false
	})
	return nil
}

func compareProcess(pi, pj types.ProcessInfo, f SortField) int {
	switch f.Field {
	case "namespace":
		ni := pi.Namespace
		if ni == "" {
			ni = DefaultNamespace
		}
		nj := pj.Namespace
		if nj == "" {
			nj = DefaultNamespace
		}
		if ni == nj {
			return 0
		}
		if f.Asc {
			if ni < nj {
				return -1
			}
			return 1
		}
		if ni > nj {
			return -1
		}
		return 1
	case "name":
		ni := strings.ToLower(pi.Name)
		nj := strings.ToLower(pj.Name)
		if ni == nj {
			return 0
		}
		if f.Asc {
			if ni < nj {
				return -1
			}
			return 1
		}
		if ni > nj {
			return -1
		}
		return 1
	case "createdAt":
		ci := pi.CreatedAt
		cj := pj.CreatedAt
		if ci == cj {
			return 0
		}
		if f.Asc {
			if ci < cj {
				return -1
			}
			return 1
		}
		if ci > cj {
			return -1
		}
		return 1
	case "id":
		if pi.ID == pj.ID {
			return 0
		}
		if f.Asc {
			if pi.ID < pj.ID {
				return -1
			}
			return 1
		}
		if pi.ID > pj.ID {
			return -1
		}
		return 1
	}
	return 0
}

// shortIDLen computes the minimum prefix length (>= 8) that uniquely identifies
// every process ID in the list. Prevents collision when multiple processes are
// created in rapid succession (UUID v7 timestamps overlap).
func shortIDLen(processes []types.ProcessInfo) int {
	const minLen = 8
	if len(processes) <= 1 {
		return minLen
	}
	for l := minLen; l <= 36; l++ {
		seen := make(map[string]bool, len(processes))
		collide := false
		for _, p := range processes {
			prefix := p.ID
			if len(prefix) > l {
				prefix = prefix[:l]
			}
			if seen[prefix] {
				collide = true
				break
			}
			seen[prefix] = true
		}
		if !collide {
			return l
		}
	}
	return 36 // full UUID as last resort
}

func renderTable(processes []types.ProcessInfo, showLong bool) {
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
		term.CyanString("%s", term.BoldString("git")),
		term.CyanString("%s", term.BoldString("watch")),
	}

	t := newTable(headers)
	idColWidth := shortIDLen(processes)
	if showLong {
		idColWidth = 36
	}
	t.maxColWidths = []int{
		idColWidth, // id — dynamic width to avoid short-ID collisions
		30,         // name
		20,         // namespace
		10,         // version
		10,         // mode
		8,          // pid
		10,         // uptime
		5,          // restarts
		15,         // status
		8,          // cpu
		10,         // mem
		15,         // user
		20,         // git
		10,         // watch
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

		var idStr string
		if showLong {
			idStr = p.ID
		} else {
			l := shortIDLen(processes)
			if len(p.ID) > l {
				idStr = p.ID[:l]
			} else {
				idStr = p.ID
			}
		}

		var gitStr string
		if p.GitBranch != "" {
			gitStr = fmt.Sprintf("%s@%s", p.GitBranch, p.GitCommit)
			if p.GitDirty {
				gitStr += "*"
				gitStr = term.YellowString("%s", gitStr)
			} else {
				gitStr = term.DimString("%s", gitStr)
			}
		} else {
			gitStr = term.DimString("-")
		}

		row := []string{
			idStr,
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
			gitStr,
			watchStr,
		}
		t.addRow(row)
	}

	t.render()
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "list",
		Aliases:     []string{"ls", "ps"},
		Usage:       term.BoldString("lynx list|ls|ps") + " [options]",
		Description: "List all managed processes.",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--long", Description: "Show full process IDs."},
			{Short: "", Long: "--namespace <name>", Description: "Filter by namespace"},
			{
				Short:       "",
				Long:        "--sort <fields>",
				Description: "Sort order, e.g. 'namespace:asc,name:asc,createdAt:desc'",
			},
			{Short: "", Long: "--json", Description: "Emit the process list as JSON on stdout"},
		},
		Examples: []string{
			`lynx list`,
			`lynx ls --namespace prod`,
			`lynx ls --sort name:asc`,
			`lynx ls --long`,
			`lynx ls --json | jq '.[] | {name, state, pid}'`,
		},
	}
}

// PrintHelp prints the help message for the list command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
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

	// Shrink the widest columns iteratively when the total exceeds the
	// terminal width. We never drop below minColWidth so narrow columns
	// (pid, ↺) still render readably; text in wider columns gets wrapped by
	// wrapText() at render time.
	const minColWidth = 3
	for {
		totalWidth := 1 // left border
		for _, w := range widths {
			totalWidth += w + 3 // cell padding + right border
		}
		if totalWidth <= termWidth {
			break
		}
		// Find the widest column above minColWidth.
		widestIdx := -1
		for i, w := range widths {
			if w <= minColWidth {
				continue
			}
			if widestIdx == -1 || w > widths[widestIdx] {
				widestIdx = i
			}
		}
		if widestIdx == -1 {
			break // nothing left to shrink
		}
		widths[widestIdx]--
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
