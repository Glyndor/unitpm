// Package list implements the list command.
package list

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/format"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/cli/table"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
	"github.com/Jaro-c/Lynx/internal/updater"
	"github.com/Jaro-c/Lynx/internal/version"
)

// updateCheckTTL controls how long a cached update-check result stays valid.
const updateCheckTTL = 6 * time.Hour

// updateCheckBudget caps how long list will wait on the update check.
const updateCheckBudget = 1500 * time.Millisecond

// checkForUpdate is overridable for tests.
var checkForUpdate = func(ctx context.Context) *updater.Release {
	rel, _ := updater.CheckCached(ctx, updateCheckTTL)
	return rel
}

// DefaultNamespace is the namespace used when an AppSpec has no explicit
// namespace set, both for storage and for `lynxpm list --namespace` filtering.
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

	// Kick off the update check concurrently so it overlaps with the IPC
	// round-trip and table render. Suppressed for --json (machine-readable
	// output) and under `go test` (avoids unintended network calls).
	var (
		updateCh       chan *updater.Release
		updateDeadline time.Time
		updateCancel   context.CancelFunc
	)
	if !jsonOutput && !testing.Testing() {
		updateDeadline = time.Now().Add(updateCheckBudget)
		ctx, cancel := context.WithDeadline(context.Background(), updateDeadline)
		updateCancel = cancel
		updateCh = make(chan *updater.Release, 1)
		go func() {
			updateCh <- checkForUpdate(ctx)
		}()
	}

	var processes []types.ProcessInfo
	if err := client.Call("list", nil, &processes); err != nil {
		if updateCancel != nil {
			updateCancel()
		}
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
		b, err := jsonx.Marshal(processes)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, string(b))
		return err
	}

	Render(processes, RenderOptions{ShowLong: showLong})

	if updateCh != nil {
		waitUpdateAndNotify(updateCh, updateDeadline)
		updateCancel()
	}
	return nil
}

func waitUpdateAndNotify(ch <-chan *updater.Release, deadline time.Time) {
	remaining := time.Until(deadline)
	if remaining < 0 {
		remaining = 0
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case rel := <-ch:
		if rel != nil {
			printUpdateBanner(rel)
		}
	case <-timer.C:
		// Check too slow for this run; cache will populate for next time.
	}
}

func printUpdateBanner(rel *updater.Release) {
	_, _ = fmt.Fprintf(
		os.Stderr,
		"\n%s New version available: %s (current %s)\n",
		term.YellowString("!"),
		term.BoldString("%s", rel.TagName),
		version.Version,
	)
	_, _ = fmt.Fprintln(os.Stderr, "  Run 'lynxpm update --apply' to install.")
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

// RenderOptions controls how the process table is rendered.
type RenderOptions struct {
	// ShowLong expands the id column to the full 36-char UUID.
	ShowLong bool
	// Highlight is a set of process IDs or names that should be visually
	// marked in the rendered table (used to emphasize the targets of a
	// preceding start/stop/restart action, pm2-style).
	Highlight map[string]bool
}

// Render prints the process list as a box-drawing table. Exported so other
// commands (start/stop/restart) can reuse the same rendering after an action.
func Render(processes []types.ProcessInfo, opts RenderOptions) {
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
	showLong := opts.ShowLong

	t := table.New(headers)
	idColWidth := shortIDLen(processes)
	if showLong {
		idColWidth = 36
	}
	// +2 to accommodate the highlight marker / alignment padding added below.
	idColWidth += 2
	t.SetMaxColWidths([]int{
		idColWidth, // id — dynamic width to avoid short-ID collisions
		40,         // name — 128-char max upstream; 40 covers most labels
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
	})

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

		uptimeStr := format.Uptime(p.Uptime)
		memStr := format.Bytes(p.Memory)

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

		highlighted := opts.Highlight[p.ID] || opts.Highlight[p.Name]
		if highlighted {
			idStr = term.GreenString("▸ ") + term.BoldString("%s", idStr)
		} else {
			idStr = "  " + idStr
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
		t.AddRow(row)
	}

	t.Render()
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "list",
		Aliases:     []string{"ls", "ps"},
		Usage:       term.BoldString("lynxpm list|ls|ps") + " [options]",
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
			`lynxpm list`,
			`lynxpm ls --namespace prod`,
			`lynxpm ls --sort name:asc`,
			`lynxpm ls --long`,
			`lynxpm ls --json | jq '.[] | {name, state, pid}'`,
		},
	}
}

// PrintHelp prints the help message for the list command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
