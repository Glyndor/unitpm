// Package monit implements the monit command: live btop-style view of a managed process.
package monit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	xterm "golang.org/x/term"

	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/metrics"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
)

const (
	graphHeight = 6
	maxHistory  = 120
	refreshRate = time.Second
)

var blockRunes = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

type showResponse struct {
	Info types.ProcessInfo `json:"info"`
	Spec protocol.AppSpec  `json:"spec"`
}

type monitState struct {
	info    types.ProcessInfo
	spec    protocol.AppSpec
	tree    []metrics.ChildStat
	cpuHist []float64
	memHist []int64
	memMax  int64
}

// Run executes the monit command. Client is created lazily if nil.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	// Pre-scan for --json/-json regardless of position so that both
	// `monit --json App-Web` and `monit App-Web --json` work correctly.
	// flag.FlagSet stops at the first non-flag argument, which would
	// silently drop flags that appear after a positional argument.
	var jsonOutput bool
	var positional []string
	for _, a := range args {
		if a == "--json" || a == "-json" {
			jsonOutput = true
		} else {
			positional = append(positional, a)
		}
	}

	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

	if len(positional) > 0 && !strings.HasPrefix(positional[0], "-") {
		return runSingle(client, positional[0], jsonOutput)
	}
	return runAll(client)
}

func runAll(client transport.IPCClient) error {
	interval := time.Second * 2
	for {
		var processes []types.ProcessInfo
		if err := client.Call("list", nil, &processes); err != nil {
			return fmt.Errorf("monit failed: %w", err)
		}
		fmt.Print("\033[H\033[2J")
		_, _ = term.Printf("Lynx monit\n")
		for _, p := range processes {
			_, _ = term.Printf(
				"%s/%s pid=%d state=%s cpu=%.1f%% mem=%d\n",
				p.Namespace, p.Name, p.PID, p.State, p.CPU, p.Memory,
			)
		}
		time.Sleep(interval)
	}
}

func runSingle(client transport.IPCClient, target string, jsonOut bool) error {
	s := &monitState{}
	if err := fetchState(client, target, s); err != nil {
		return err
	}

	// Non-interactive JSON mode: print one snapshot and exit.
	if jsonOut {
		return printJSON(s)
	}

	rawMode := xterm.IsTerminal(int(os.Stdin.Fd()))
	var oldState *xterm.State
	if rawMode {
		var err error
		oldState, err = xterm.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			rawMode = false
		}
	}

	fmt.Print("\033[?25l") // hide cursor
	defer func() {
		fmt.Print("\033[?25h\033[0m") // show cursor, reset colors
		if rawMode && oldState != nil {
			_ = xterm.Restore(int(os.Stdin.Fd()), oldState)
		}
	}()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	keyCh := make(chan byte, 8)
	if rawMode {
		go readKeys(keyCh)
	}

	ticker := time.NewTicker(refreshRate)
	defer ticker.Stop()

	render(s)

	for {
		select {
		case sig := <-sigCh:
			if sig == syscall.SIGWINCH {
				render(s)
			} else {
				return nil
			}
		case k := <-keyCh:
			if k == 'q' || k == 3 { // q or Ctrl+C
				return nil
			}
		case <-ticker.C:
			if err := fetchState(client, target, s); err != nil {
				return err
			}
			render(s)
		}
	}
}

func printJSON(s *monitState) error {
	out := map[string]any{
		"info": s.info,
		"tree": s.tree,
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

func readKeys(ch chan<- byte) {
	buf := make([]byte, 4)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return
		}
		ch <- buf[0]
	}
}

func fetchState(client transport.IPCClient, target string, s *monitState) error {
	var resp showResponse
	if err := client.Call("show", map[string]string{"id": target}, &resp); err != nil {
		return fmt.Errorf("monit: %w", err)
	}
	s.info = resp.Info
	s.spec = resp.Spec

	var tree []metrics.ChildStat
	_ = client.Call("proctree", map[string]string{"id": target}, &tree)
	s.tree = tree

	s.cpuHist = append(s.cpuHist, resp.Info.CPU)
	s.memHist = append(s.memHist, resp.Info.Memory)
	if resp.Info.Memory > s.memMax {
		s.memMax = resp.Info.Memory
	}
	if len(s.cpuHist) > maxHistory {
		s.cpuHist = s.cpuHist[len(s.cpuHist)-maxHistory:]
		s.memHist = s.memHist[len(s.memHist)-maxHistory:]
	}
	return nil
}

func render(s *monitState) {
	w, _, err := xterm.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 40 {
		w = 80
	}

	var b strings.Builder
	b.WriteString("\033[H\033[2J")

	// ── Header ──────────────────────────────────────────────────────────────
	headerText := fmt.Sprintf("  %s  •  %s  •  pid %d  •  %s  •  restarts %d  ",
		term.BoldString("%s", s.info.Name),
		stateStr(s.info.State),
		s.info.PID,
		fmtUptime(s.info.Uptime),
		s.info.Restarts,
	)
	writeBorderTop(&b, w, " Lynx monit ")
	b.WriteString("│" + padTo(headerText, w-2, visLen(headerText)) + "│\n")
	writeBorderBot(&b, w)

	// ── Graphs ──────────────────────────────────────────────────────────────
	leftW := w / 2
	rightW := w - leftW
	cpuGW := leftW - 4
	memGW := rightW - 4

	cpuRows := buildGraph(s.cpuHist, 100.0, cpuGW, graphHeight)
	memF := make([]float64, len(s.memHist))
	memMaxF := float64(s.memMax)
	if memMaxF == 0 {
		memMaxF = 1
	}
	for i, v := range s.memHist {
		memF[i] = float64(v)
	}
	memRows := buildGraph(memF, memMaxF, memGW, graphHeight)

	b.WriteString(borderTop(leftW, " CPU ") + borderTop(rightW, " Memory ") + "\n")

	cpuVal := fmt.Sprintf("  %.1f%%", s.info.CPU)
	memVal := fmt.Sprintf("  %s / peak %s", fmtBytes(s.info.Memory), fmtBytes(s.memMax))
	b.WriteString(
		"│" + padTo(cpuVal, leftW-2, len(cpuVal)) + "│" +
			"│" + padTo(memVal, rightW-2, len(memVal)) + "│\n")

	for r := 0; r < graphHeight; r++ {
		cpuRow := graphRowStr(cpuRows, r, cpuGW)
		memRow := graphRowStr(memRows, r, memGW)
		b.WriteString(
			"│ " + term.GreenString("%s", cpuRow) + " │" +
				"│ " + term.CyanString("%s", memRow) + " │\n")
	}
	b.WriteString(borderBot(leftW) + borderBot(rightW) + "\n")

	// ── Details ─────────────────────────────────────────────────────────────
	git := s.info.GitBranch
	if git != "" && s.info.GitCommit != "" {
		git += "@" + s.info.GitCommit
	}
	if git == "" {
		git = "—"
	}
	cmd := s.spec.Exec.Command
	if len(s.spec.Exec.Args) > 0 {
		cmd += " " + strings.Join(s.spec.Exec.Args, " ")
	}

	writeBorderTop(&b, w, " Details ")
	for _, row := range []string{
		detailRow("namespace", s.info.Namespace, "version", s.info.Version),
		detailRow("mode", s.info.Mode, "git", git),
		detailRow("user", s.info.User, "cmd", cmd),
	} {
		b.WriteString("│" + padTo(row, w-2, visLen(row)) + "│\n")
	}
	writeBorderBot(&b, w)

	// ── Process Tree ─────────────────────────────────────────────────────────
	if len(s.tree) > 0 {
		writeBorderTop(&b, w, " Process Tree ")
		hdr := detailRow("PID", "Process", "Memory", "")
		b.WriteString("│" + padTo(term.DimString("%s", hdr), w-2, visLen(hdr)) + "│\n")
		for _, entry := range s.tree {
			indent := strings.Repeat("  ", entry.Depth)
			prefix := ""
			if entry.Depth > 0 {
				prefix = "└─ "
			}
			procName := indent + prefix + entry.Comm
			row := fmt.Sprintf("  %-8d  %-24s  %s", entry.PID, procName, fmtBytes(entry.MemoryBytes))
			b.WriteString("│" + padTo(row, w-2, len(row)) + "│\n")
		}
		writeBorderBot(&b, w)
	}

	// ── Footer ──────────────────────────────────────────────────────────────
	b.WriteString(term.DimString("  [q] quit") + "   refresh: 1s\n")

	fmt.Print(b.String())
}

// buildGraph returns graphHeight rows of block chars, each width runes wide.
func buildGraph(values []float64, maxVal float64, width, height int) []string {
	rows := make([]string, height)
	for r := 0; r < height; r++ {
		var sb strings.Builder
		for c := 0; c < width; c++ {
			idx := len(values) - width + c
			var v float64
			if idx >= 0 && idx < len(values) {
				v = values[idx]
			}
			norm := v / maxVal
			rowTop := float64(height-r) / float64(height)
			rowBot := float64(height-r-1) / float64(height)
			switch {
			case norm >= rowTop:
				sb.WriteRune('█')
			case norm > rowBot:
				frac := (norm - rowBot) / (rowTop - rowBot)
				bi := int(frac * float64(len(blockRunes)-1))
				if bi < 0 {
					bi = 0
				}
				if bi >= len(blockRunes) {
					bi = len(blockRunes) - 1
				}
				sb.WriteRune(blockRunes[bi])
			default:
				sb.WriteRune(' ')
			}
		}
		rows[r] = sb.String()
	}
	return rows
}

func graphRowStr(rows []string, r, width int) string {
	if r < len(rows) {
		return rows[r]
	}
	return strings.Repeat(" ", width)
}

// ── Box-drawing helpers ──────────────────────────────────────────────────────

func writeBorderTop(b *strings.Builder, width int, title string) {
	b.WriteString(borderTop(width, title) + "\n")
}

func writeBorderBot(b *strings.Builder, width int) {
	b.WriteString(borderBot(width) + "\n")
}

func borderTop(width int, title string) string {
	inner := width - 2
	titlePart := "─" + title + "─"
	rem := inner - len(titlePart)
	if rem < 0 {
		rem = 0
	}
	return "╭" + titlePart + strings.Repeat("─", rem) + "╮"
}

func borderBot(width int) string {
	return "╰" + strings.Repeat("─", width-2) + "╯"
}

// padTo pads s (with visual length vl) to fill innerWidth characters.
func padTo(s string, innerWidth, vl int) string {
	pad := innerWidth - vl
	if pad < 0 {
		pad = 0
	}
	return s + strings.Repeat(" ", pad)
}

// visLen returns the visual display length of s, ignoring ANSI escape codes.
func visLen(s string) int {
	n := 0
	inEsc := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if inEsc {
			if b == 'm' {
				inEsc = false
			}
			continue
		}
		if b == 0x1b {
			inEsc = true
			continue
		}
		// Count UTF-8 lead bytes only (skips continuation bytes 0x80–0xBF).
		if b < 0x80 || b >= 0xC0 {
			n++
		}
	}
	return n
}

// detailRow builds a row of label/value pairs with fixed column widths.
func detailRow(pairs ...string) string {
	const labelW, valW = 12, 20
	var sb strings.Builder
	sb.WriteString("  ")
	for i := 0; i+1 < len(pairs); i += 2 {
		label := pairs[i]
		val := pairs[i+1]
		sb.WriteString(term.DimString("%s", label))
		sb.WriteString(strings.Repeat(" ", labelW-len(label)))
		sb.WriteString(val)
		if i+2 < len(pairs) {
			pad := valW - len(val)
			if pad < 1 {
				pad = 1
			}
			sb.WriteString(strings.Repeat(" ", pad))
		}
	}
	return sb.String()
}

// ── Format helpers ───────────────────────────────────────────────────────────

func stateStr(state types.ProcessState) string {
	switch state {
	case types.StateRunning, types.StateOnline:
		return term.GreenString("%s", string(state))
	case types.StateStopped, types.StateExited:
		return term.YellowString("%s", string(state))
	case types.StateFailed:
		return term.RedString("%s", string(state))
	case types.StateRestarting:
		return term.CyanString("%s", string(state))
	default:
		return string(state)
	}
}

func fmtUptime(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func fmtBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/gb)
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/mb)
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/kb)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// GetSpec returns the command specification for the monit command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "monit",
		Aliases:     []string{"top", "monitor"},
		Usage:       "lynxpm monit|top|monitor [process] [--json]",
		Description: "Live process statistics. Pass a name/ID for a single-process view with CPU/memory graphs and process tree. --json prints one snapshot and exits.",
	}
}

// PrintHelp prints the help information for the monit command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
