// Package show implements the show command: prints detailed runtime + spec
// information for a single process as a set of box-drawing tables.
package show

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/format"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/cli/table"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/term"
	"github.com/Jaro-c/Lynx/internal/types"
)

type showResponse struct {
	Info types.ProcessInfo `json:"info"`
	Spec protocol.AppSpec  `json:"spec"`
}

// Run executes the show command.
func Run(client transport.IPCClient, args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	var jsonOutput bool
	fs.BoolVar(&jsonOutput, "json", false, "Emit the raw daemon response as JSON on stdout")

	if err := fs.Parse(args); err != nil {
		if strings.HasPrefix(err.Error(), "flag provided but not defined: -") {
			flagName := strings.TrimPrefix(err.Error(), "flag provided but not defined: -")
			return &errs.UsageError{Message: "Unknown flag: -" + flagName}
		}
		return &errs.UsageError{Message: err.Error()}
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("missing process ID or name")
	}
	id := rest[0]

	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

	var resp showResponse
	if err := client.Call("show", map[string]string{"id": id}, &resp); err != nil {
		return fmt.Errorf("show failed: %w", err)
	}

	if jsonOutput {
		b, err := jsonx.Marshal(resp)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, string(b))
		return err
	}

	render(resp)
	return nil
}

func render(resp showResponse) {
	info := resp.Info
	spec := resp.Spec

	_, _ = term.Printf("%s %s %s\n\n",
		term.BoldString("Process"),
		term.CyanString("%s", nonEmpty(info.Name, spec.Name)),
		term.DimString("(%s)", nonEmpty(info.ID, spec.ID)),
	)

	renderProcess(info, spec)
	renderExec(spec)
	renderEnv(spec)
	renderLogs(spec)
	renderRestart(spec)
	renderStop(spec)
	renderResources(spec)
	renderIsolation(spec)
	renderSchedule(spec)
	renderWatch(spec)
}

func renderProcess(info types.ProcessInfo, spec protocol.AppSpec) {
	ns := info.Namespace
	if ns == "" {
		ns = spec.Namespace
	}
	table.KV("Process", []table.KVRow{
		{"state", colorState(string(info.State))},
		{"pid", pidStr(info.PID)},
		{"namespace", ns},
		{"version", info.Version},
		{"mode", info.Mode},
		{"uptime", format.UptimeExact(info.Uptime)},
		{"restarts", strconv.Itoa(info.Restarts)},
		{"cpu", format.Percent(info.CPU)},
		{"memory", format.BytesExact(info.Memory)},
		{"user", info.User},
		{"created at", format.Timestamp(info.CreatedAt)},
		{"git", gitStr(info)},
		{"watch", watchStr(info.Watch)},
		{"disabled", boolDimmed(spec.Disabled)},
	})
	fmt.Println()
}

func renderExec(spec protocol.AppSpec) {
	cmd := spec.Exec.Command
	if spec.Exec.Type == "entry" {
		cmd = spec.Exec.Entry
	}
	table.KV("Exec", []table.KVRow{
		{"type", spec.Exec.Type},
		{"runtime", spec.Exec.Runtime},
		{"command", cmd},
		{"args", joinArgs(spec.Exec.Args)},
		{"shell", boolDimmed(spec.Exec.Shell)},
		{"cwd", spec.Cwd},
	})
	fmt.Println()
}

func renderEnv(spec protocol.AppSpec) {
	if spec.EnvFile == "" && len(spec.Env) == 0 {
		return
	}
	rows := []table.KVRow{}
	if spec.EnvFile != "" {
		rows = append(rows, table.KVRow{"env-file", spec.EnvFile})
	}
	keys := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rows = append(rows, table.KVRow{k, maskSecret(k, spec.Env[k])})
	}
	table.KV("Environment", rows)
	fmt.Println()
}

func renderLogs(spec protocol.AppSpec) {
	if spec.Logs == nil {
		return
	}
	l := spec.Logs
	dir := l.Dir
	table.KV("Logs", []table.KVRow{
		{"mode", l.Mode},
		{"dir", dir},
		{"stdout", joinLogPath(dir, l.Stdout)},
		{"stderr", joinLogPath(dir, l.Stderr)},
		{"format", l.Format},
		{"timestamp", l.Timestamp},
	})
	fmt.Println()
}

func renderRestart(spec protocol.AppSpec) {
	if spec.Restart == nil {
		return
	}
	r := spec.Restart
	backoff := ""
	if r.BackoffType != "" || r.BackoffMs > 0 {
		backoff = fmt.Sprintf("%s (%s)", strDefault(r.BackoffType, "expo"), format.Uptime(int64(r.BackoffMs)))
	}
	stopOn := ""
	if len(r.StopOnExit) > 0 {
		parts := make([]string, len(r.StopOnExit))
		for i, c := range r.StopOnExit {
			parts[i] = strconv.Itoa(c)
		}
		stopOn = strings.Join(parts, ", ")
	}
	table.KV("Restart", []table.KVRow{
		{"policy", r.Policy},
		{"maxRetries", intOrDash(r.MaxRetries)},
		{"backoff", backoff},
		{"stopOnExit", stopOn},
	})
	fmt.Println()
}

func renderStop(spec protocol.AppSpec) {
	if spec.Stop == nil {
		return
	}
	s := spec.Stop
	table.KV("Stop", []table.KVRow{
		{"signal", s.Signal},
		{"timeout", format.UptimeExact(int64(s.TimeoutMs))},
	})
	fmt.Println()
}

func renderResources(spec protocol.AppSpec) {
	if spec.Resources == nil {
		return
	}
	r := spec.Resources
	if r.MemoryMaxBytes == 0 && r.CPUMaxPercent == 0 && r.TasksMax == 0 {
		return
	}
	table.KV("Resources", []table.KVRow{
		{"memory max", memOrUnlimited(r.MemoryMaxBytes)},
		{"cpu max", cpuOrUnlimited(r.CPUMaxPercent)},
		{"tasks max", intOrUnlimited(r.TasksMax)},
	})
	fmt.Println()
}

func renderIsolation(spec protocol.AppSpec) {
	if spec.RunAs == nil || spec.RunAs.Mode == "" {
		return
	}
	table.KV("Isolation", []table.KVRow{
		{"mode", spec.RunAs.Mode},
	})
	fmt.Println()
}

func renderSchedule(spec protocol.AppSpec) {
	if spec.Cron == "" {
		return
	}
	table.KV("Schedule", []table.KVRow{
		{"cron", spec.Cron},
	})
	fmt.Println()
}

func renderWatch(spec protocol.AppSpec) {
	if spec.Watch == nil {
		return
	}
	rows := []table.KVRow{
		{"enabled", boolDimmed(spec.Watch.Enabled)},
	}
	if len(spec.Watch.Ignore) > 0 {
		rows = append(rows, table.KVRow{"ignore", strings.Join(spec.Watch.Ignore, ", ")})
	}
	table.KV("Watch", rows)
	fmt.Println()
}

// --- helpers ---

func colorState(s string) string {
	switch s {
	case "running", "online":
		return term.GreenString("%s", s)
	case "stopped", "failed":
		return term.RedString("%s", s)
	case "restarting":
		return term.YellowString("%s", s)
	case "":
		return term.DimString("-")
	default:
		return s
	}
}

func pidStr(pid int) string {
	if pid == 0 {
		return term.DimString("-")
	}
	return strconv.Itoa(pid)
}

func gitStr(info types.ProcessInfo) string {
	if info.GitBranch == "" {
		return term.DimString("-")
	}
	s := fmt.Sprintf("%s@%s", info.GitBranch, info.GitCommit)
	if info.GitDirty {
		return term.YellowString("%s*", s)
	}
	return s
}

func watchStr(on bool) string {
	if on {
		return term.GreenString("enabled")
	}
	return term.DimString("disabled")
}

func boolDimmed(v bool) string {
	if v {
		return term.GreenString("true")
	}
	return term.DimString("false")
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"'") {
			quoted[i] = strconv.Quote(a)
		} else {
			quoted[i] = a
		}
	}
	return strings.Join(quoted, " ")
}

func joinLogPath(dir, file string) string {
	if file == "" {
		return ""
	}
	if filepath.IsAbs(file) || dir == "" {
		return file
	}
	return filepath.Join(dir, file)
}

func intOrDash(v int) string {
	if v == 0 {
		return term.DimString("-")
	}
	return strconv.Itoa(v)
}

func intOrUnlimited(v int) string {
	if v == 0 {
		return term.DimString("unlimited")
	}
	return strconv.Itoa(v)
}

func memOrUnlimited(b int64) string {
	if b == 0 {
		return term.DimString("unlimited")
	}
	return format.BytesExact(b)
}

func cpuOrUnlimited(pct int) string {
	if pct == 0 {
		return term.DimString("unlimited")
	}
	return fmt.Sprintf("%d%% (%.2f cores)", pct, float64(pct)/100)
}

func strDefault(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// maskSecret hides values for keys that look sensitive. Heuristic only —
// daemon already keeps --env-file values off disk; this is cosmetic for
// accidental shoulder-surfing.
func maskSecret(key, val string) string {
	if val == "" {
		return ""
	}
	upper := strings.ToUpper(key)
	for _, needle := range []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "KEY", "CREDENTIAL", "PRIVATE"} {
		if strings.Contains(upper, needle) {
			return term.DimString("********")
		}
	}
	return val
}

// GetSpec returns the command specification for the show command.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "show",
		Aliases:     []string{"info", "describe"},
		Usage:       term.BoldString("lynxpm show|info|describe") + " <id|name|namespace:name> [--json]",
		Description: "Show detailed information about a process",
		Options: []help.Option{
			{Short: "-h", Long: "--help", Description: "Show this help message."},
			{Short: "", Long: "--json", Description: "Emit the raw daemon response as JSON on stdout."},
		},
		Examples: []string{
			`lynxpm show my-api`,
			`lynxpm info prod:my-api`,
			`lynxpm describe 019d9a04`,
			`lynxpm show my-api --json | jq '.spec.env'`,
		},
	}
}

// PrintHelp prints the help information for the show command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}
