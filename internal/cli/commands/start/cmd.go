// Package start implements the start command: parses a command line into
// an AppSpec and tells the daemon to spawn it.
//
//nolint:gocognit,nestif,cyclop
package start

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/help"
	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/spec"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Run executes the start command. If client is nil, it is created lazily
// *after* argument validation so bad invocations fail without touching the
// daemon socket.
func Run(client transport.IPCClient, args []string) error {
	if help.IsHelp(args) {
		PrintHelp()
		return nil
	}

	dryRun := false
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--dry-run" || a == "-n" {
			dryRun = true
			continue
		}
		filtered = append(filtered, a)
	}
	args = filtered

	appSpec, scale, err := ParseAppSpec(args)
	if err != nil {
		return err
	}

	if scale < 1 {
		scale = 1
	}

	if dryRun {
		return printDryRun(appSpec, scale)
	}

	if client == nil {
		c, err := transport.NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		client = c
	}

	// Derive base name if empty
	autoNamed := false
	baseName := appSpec.Name
	if baseName == "" {
		autoNamed = true
		if appSpec.Exec.Type == "entry" {
			baseName = filepath.Base(appSpec.Exec.Entry)
		} else {
			baseName = filepath.Base(appSpec.Exec.Command)
		}
	}

	for i := 0; i < scale; i++ {
		thisSpec := appSpec

		// Generate ID first
		id, err := spec.GenerateID()
		if err != nil {
			return fmt.Errorf("failed to generate ID: %w", err)
		}
		thisSpec.ID = id
		thisSpec.CreatedAt = time.Now().Format(time.RFC3339)

		// Set Name
		if autoNamed {
			shortID := id
			if len(id) > 8 {
				shortID = id[:8]
			}
			if scale > 1 {
				thisSpec.Name = fmt.Sprintf("%s-%d-%s", baseName, i+1, shortID)
			} else {
				thisSpec.Name = fmt.Sprintf("%s-%s", baseName, shortID)
			}
		} else {
			if scale > 1 {
				thisSpec.Name = fmt.Sprintf("%s-%d", baseName, i+1)
			} else {
				thisSpec.Name = baseName
			}
		}

		// Inject Instance Index
		if thisSpec.Env == nil {
			thisSpec.Env = make(map[string]string)
		}
		thisSpec.Env["LYNX_INSTANCE"] = strconv.Itoa(i)

		// Save the spec to disk before calling daemon (daemon may restart mid-flight).
		_, err = spec.SaveSpec(thisSpec.ID, thisSpec)
		if err != nil {
			return fmt.Errorf("failed to save spec: %w", err)
		}

		// Send Request
		req := protocol.StartRequest{
			ProtocolVersion: 1,
			Type:            "start",
			RequestID:       id, // Use same ID for request correlation
			Spec:            thisSpec,
		}

		var startResp protocol.StartResponseData
		err = client.Call("start", req, &startResp)
		if err != nil {
			// Daemon rejected the spec — remove it from disk to prevent phantom
			// entries being restored on next daemon restart.
			_ = spec.DeleteSpec(thisSpec.ID)
			return fmt.Errorf("start failed for instance %d: %w", i+1, err)
		}

		printSuccessResponse(&startResp, thisSpec.Name)
	}

	return nil
}

// ParseAppSpec parses command-line arguments into an AppSpec.
func ParseAppSpec(args []string) (protocol.AppSpec, int, error) {
	return (&specParser{args: args}).parse()
}

type specParser struct {
	args []string
	pos  int

	name          string
	namespace     string
	cwd           string
	stdio         string
	runAs         string
	cmdParts      []string
	cron          string
	runtime       string
	envFile       string
	shell         bool
	restartPolicy string
	maxRestarts   int
	restartDelay  int
	backoff       string
	stopOnExit    []int
	logDir        string
	stdout        string
	stderr        string
	logFormat     string
	logTimestamp  string

	// Stop policy
	stopSignal    string
	stopTimeoutMs int

	// Resource limits
	memoryMax string
	cpuMaxPct int
	tasksMax  int

	// Watch
	watch       bool
	watchIgnore string

	parsingFlags bool
	scale        int
}

func (p *specParser) parse() (protocol.AppSpec, int, error) {
	p.parsingFlags = true
	p.scale = 1
	p.stdio = "file"
	p.runAs = "self"
	p.restartPolicy = "on-failure"
	p.maxRestarts = 10
	p.restartDelay = 2000
	p.backoff = "expo"
	p.stopOnExit = []int{0}
	p.logFormat = "plain"
	p.logTimestamp = "rfc3339"

	for p.pos = 0; p.pos < len(p.args); p.pos++ {
		arg := p.args[p.pos]

		if !p.parsingFlags {
			p.cmdParts = append(p.cmdParts, arg)
			continue
		}

		if arg == "--" {
			p.pos++ // Skip "--"
			// Append remaining args to cmdParts
			for ; p.pos < len(p.args); p.pos++ {
				p.cmdParts = append(p.cmdParts, p.args[p.pos])
			}
			break
		}

		if strings.HasPrefix(arg, "-") {
			// Check if it's a known flag
			err := p.handleFlag(arg)
			if err == nil {
				continue
			}

			// If unknown flag:
			// If we already have a command, treat it as an argument to that command.
			if len(p.cmdParts) > 0 {
				p.cmdParts = append(p.cmdParts, arg)
				continue
			}

			// If no command yet, it's an invalid flag for lynx
			return protocol.AppSpec{}, 0, err
		}

		p.cmdParts = append(p.cmdParts, arg)
	}

	spec, err := p.finalize()
	if err != nil {
		return protocol.AppSpec{}, 0, err
	}
	return spec, p.scale, nil
}

func (p *specParser) handleFlag(arg string) error {
	switch arg {
	case "--name":
		return p.readStringValue(&p.name)
	case "--namespace":
		return p.readStringValue(&p.namespace)
	case "--cwd":
		return p.readStringValue(&p.cwd)
	case "--cron", "--schedule":
		return p.readStringValue(&p.cron)
	case "--runtime":
		return p.readStringValue(&p.runtime)
	case "--isolation":
		return p.readStringValue(&p.runAs)
	case "--shell":
		p.shell = true
		return nil
	case "--env-file":
		return p.readStringValue(&p.envFile)
	case "--restart":
		return p.readStringValue(&p.restartPolicy)
	case "--max-restarts":
		return p.readIntValue(&p.maxRestarts)
	case "--restart-delay":
		return p.readIntValue(&p.restartDelay)
	case "--backoff":
		return p.readStringValue(&p.backoff)
	case "--stop-on-exit":
		return p.readIntList(&p.stopOnExit)
	case "--log-dir":
		return p.readStringValue(&p.logDir)
	case "--stdout":
		return p.readStringValue(&p.stdout)
	case "--stderr":
		return p.readStringValue(&p.stderr)
	case "--log-format":
		return p.readStringValue(&p.logFormat)
	case "--log-timestamp":
		return p.readStringValue(&p.logTimestamp)
	case "--scale", "--instances":
		return p.readIntValue(&p.scale)
	case "--stop-signal":
		return p.readStringValue(&p.stopSignal)
	case "--stop-timeout":
		return p.readIntValue(&p.stopTimeoutMs)
	case "--watch":
		p.watch = true
		return nil
	case "--watch-ignore":
		return p.readStringValue(&p.watchIgnore)
	case "--memory-max":
		return p.readStringValue(&p.memoryMax)
	case "--cpu-max":
		return p.readIntValue(&p.cpuMaxPct)
	case "--tasks-max":
		return p.readIntValue(&p.tasksMax)
	default:
		return fmt.Errorf("unknown flag: %s", arg)
	}
}

func (p *specParser) readStringValue(target *string) error {
	p.pos++
	if p.pos >= len(p.args) {
		return errors.New("missing value for flag")
	}
	*target = p.args[p.pos]
	return nil
}

func (p *specParser) readIntValue(target *int) error {
	p.pos++
	if p.pos >= len(p.args) {
		return errors.New("missing value for flag")
	}
	val, err := strconv.Atoi(p.args[p.pos])
	if err != nil {
		return fmt.Errorf("invalid integer value: %s", p.args[p.pos])
	}
	*target = val
	return nil
}

func (p *specParser) readIntList(target *[]int) error {
	p.pos++
	if p.pos >= len(p.args) {
		return errors.New("missing value for flag")
	}
	parts := strings.Split(p.args[p.pos], ",")
	list := make([]int, 0, len(parts))
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		val, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid integer in list: %s", s)
		}
		list = append(list, val)
	}
	*target = list
	return nil
}

func (p *specParser) finalize() (protocol.AppSpec, error) {
	if len(p.cmdParts) == 0 {
		return protocol.AppSpec{}, errs.NewUsageError("missing command or entry file")
	}

	// Resolve CWD
	cwd := p.cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return protocol.AppSpec{}, fmt.Errorf("failed to get current directory: %w", err)
		}
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return protocol.AppSpec{}, fmt.Errorf("failed to resolve absolute path for cwd: %w", err)
	}

	ns := p.namespace
	if ns == "" {
		ns = "default"
	}

	spec := protocol.AppSpec{
		Version:   1,
		Name:      p.name,
		Namespace: ns,
		Cwd:       cwd,
		Cron:      p.cron,
		Logs: &protocol.AppLogs{
			Mode:      p.stdio,
			Stdout:    p.stdout,
			Stderr:    p.stderr,
			Dir:       p.logDir,
			Format:    p.logFormat,
			Timestamp: p.logTimestamp,
		},
		Restart: &protocol.AppRestart{
			Policy:      p.restartPolicy,
			MaxRetries:  p.maxRestarts,
			BackoffMs:   p.restartDelay,
			BackoffType: p.backoff,
			StopOnExit:  p.stopOnExit,
		},
		RunAs: &protocol.RunAsPolicy{
			Mode: p.runAs,
		},
		Env:     make(map[string]string),
		EnvFile: p.envFile,
	}

	if p.stopSignal != "" || p.stopTimeoutMs != 0 {
		spec.Stop = &protocol.AppStop{
			Signal:    p.stopSignal,
			TimeoutMs: p.stopTimeoutMs,
		}
	}

	if p.watch {
		var ignore []string
		if p.watchIgnore != "" {
			for _, pat := range strings.Split(p.watchIgnore, ",") {
				pat = strings.TrimSpace(pat)
				if pat == "" {
					continue
				}
				if strings.Contains(pat, "..") || filepath.IsAbs(pat) {
					return protocol.AppSpec{}, fmt.Errorf("invalid ignore pattern %q: must be relative, no '..'", pat)
				}
				ignore = append(ignore, pat)
			}
			if len(ignore) > 100 {
				return protocol.AppSpec{}, fmt.Errorf("too many ignore patterns (max 100)")
			}
		}
		spec.Watch = &protocol.AppWatch{
			Enabled: true,
			Ignore:  ignore,
		}
	}

	if p.memoryMax != "" || p.cpuMaxPct != 0 || p.tasksMax != 0 {
		memBytes, err := parseMemorySize(p.memoryMax)
		if err != nil {
			return protocol.AppSpec{}, err
		}
		spec.Resources = &protocol.AppResources{
			MemoryMaxBytes: memBytes,
			CPUMaxPercent:  p.cpuMaxPct,
			TasksMax:       p.tasksMax,
		}
	}

	p.resolveExec(&spec)

	return spec, nil
}

// parseMemorySize accepts values like "512M", "2G", "1024k", "10485760".
// Returns 0 on empty string. K, M, G are base 1024. Case-insensitive.
func parseMemorySize(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'k', 'K':
		mult = 1024
		s = s[:len(s)-1]
	case 'm', 'M':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'g', 'G':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid memory size %q (expected e.g. 512M or 2G)", s)
	}
	return n * mult, nil
}

func (p *specParser) resolveExec(spec *protocol.AppSpec) {
	if len(p.cmdParts) == 1 {
		// Single token: might be "bun dev" (quoted) or "main.js"
		token := p.cmdParts[0]

		// If explicit runtime is provided, treat as Entry
		if p.runtime != "" {
			spec.Exec = protocol.AppExec{
				Type:    "entry",
				Entry:   token,
				Runtime: p.runtime,
				Shell:   p.shell,
			}
			return
		}

		// Try to tokenize to see if it's a command string
		parts, err := Tokenize(token)
		if err == nil && len(parts) > 1 {
			// It was a quoted string with multiple parts -> Command
			spec.Exec = protocol.AppExec{
				Type:    "command",
				Command: parts[0],
				Args:    parts[1:],
				Shell:   p.shell,
			}
			return
		}

		// Single part. Check extension for inference.
		ext := filepath.Ext(token)
		switch ext {
		case ".js", ".mjs", ".cjs":
			spec.Exec = protocol.AppExec{
				Type:    "entry",
				Entry:   token,
				Runtime: "node",
				Shell:   p.shell,
			}
		case ".go":
			spec.Exec = protocol.AppExec{
				Type:    "entry",
				Entry:   token,
				Runtime: "go run",
				Shell:   p.shell,
			}
		default:
			// Treat as simple command
			spec.Exec = protocol.AppExec{
				Type:    "command",
				Command: token,
				Shell:   p.shell,
			}
		}
	} else {
		// Multiple tokens: "node index.js" -> Command
		spec.Exec = protocol.AppExec{
			Type:    "command",
			Command: p.cmdParts[0],
			Args:    p.cmdParts[1:],
			Shell:   p.shell,
		}
	}
}

// PrintHelp prints the help message for the start command.
func PrintHelp() {
	help.RenderCommandHelp(os.Stdout, GetSpec())
}

// printDryRun renders the resolved spec without talking to the daemon.
func printDryRun(spec protocol.AppSpec, scale int) error {
	_, _ = term.Printf("%s Dry run — would start %d instance(s):\n", term.CyanString("i"), scale)
	_, _ = term.Printf("  command:   %s\n", spec.Exec.Command)
	if len(spec.Exec.Args) > 0 {
		_, _ = term.Printf("  args:      %v\n", spec.Exec.Args)
	}
	if spec.Exec.Entry != "" {
		_, _ = term.Printf("  entry:     %s (%s)\n", spec.Exec.Entry, spec.Exec.Runtime)
	}
	_, _ = term.Printf("  cwd:       %s\n", spec.Cwd)
	_, _ = term.Printf("  namespace: %s\n", spec.Namespace)
	if spec.Name != "" {
		_, _ = term.Printf("  name:      %s\n", spec.Name)
	}
	if spec.RunAs != nil && spec.RunAs.Mode != "self" {
		_, _ = term.Printf("  isolation: %s\n", spec.RunAs.Mode)
	}
	if spec.Cron != "" {
		_, _ = term.Printf("  schedule:  %s\n", spec.Cron)
	}
	if spec.Restart != nil {
		_, _ = term.Printf("  restart:   policy=%s max=%d backoff=%s\n",
			spec.Restart.Policy, spec.Restart.MaxRetries, spec.Restart.BackoffType)
	}
	if spec.EnvFile != "" {
		_, _ = term.Printf("  env-file:  %s\n", spec.EnvFile)
	}
	return nil
}

func printSuccessResponse(data *protocol.StartResponseData, name string) {
	_, _ = term.Printf("%s Started %s\n", term.GreenString("✓"), name)
	if len(data.ProcID) > 8 {
		_, _ = term.Printf("  ID:     %s (short: %s)\n", data.ProcID, data.ProcID[:8])
	} else {
		_, _ = term.Printf("  ID:     %s\n", data.ProcID)
	}
	_, _ = term.Printf("  PID:    %d\n", data.PID)
	_, _ = term.Printf("  Status: %s\n", data.Status)
}

// GetSpec returns the command specification.
func GetSpec() help.CommandSpec {
	return help.CommandSpec{
		Name:        "start",
		Usage:       term.BoldString("lynx start <command|file> [flags]"),
		Description: "Start a new process.",
		Options: []help.Option{
			{Short: "", Long: "--name <name>", Description: "Assign a name to the process"},
			{
				Short:       "",
				Long:        "--namespace <name>",
				Description: "Assign a namespace to the process",
			},
			{Short: "", Long: "--cwd <dir>", Description: "Working directory"},
			{Short: "", Long: "--shell", Description: "Execute command in shell"},
			{
				Short:       "",
				Long:        "--schedule <cron>",
				Description: "Cron schedule for restart (alias --cron)",
			},
			{
				Short:       "",
				Long:        "--restart <policy>",
				Description: "Restart policy (never, on-failure, always)",
			},
			{Short: "", Long: "--max-restarts <N>", Description: "Max restarts (default 10)"},
			{
				Short:       "",
				Long:        "--restart-delay <ms>",
				Description: "Restart delay in ms (default 2000)",
			},
			{
				Short:       "",
				Long:        "--backoff <type>",
				Description: "Backoff strategy (none, linear, expo)",
			},
			{
				Short:       "",
				Long:        "--stop-on-exit <codes>",
				Description: "Exit codes to stop on (comma-separated)",
			},
			{Short: "", Long: "--log-dir <path>", Description: "Directory for logs"},
			{Short: "", Long: "--stdout <file>", Description: "Stdout file (relative to log-dir)"},
			{Short: "", Long: "--stderr <file>", Description: "Stderr file (relative to log-dir)"},
			{Short: "", Long: "--log-format <fmt>", Description: "Log format (plain, json)"},
			{
				Short:       "",
				Long:        "--log-timestamp <fmt>",
				Description: "Log timestamp (rfc3339, unix, none)",
			},
			{
				Short:       "",
				Long:        "--runtime <rt>",
				Description: "Runtime for entry file (e.g., node, python)",
			},
			{Short: "", Long: "--env-file <file>", Description: "Path to environment file"},
			{Short: "", Long: "--isolation <mode>", Description: "Isolation mode (self, dynamic, sandbox)"},
			{Short: "", Long: "--scale <N>", Description: "Number of instances to start"},
			{Short: "", Long: "--stop-signal <name>", Description: "Signal sent on stop (SIGTERM, SIGINT, SIGHUP, SIGQUIT, SIGUSR1, SIGUSR2)"},
			{Short: "", Long: "--stop-timeout <ms>", Description: "Grace period before SIGKILL (default 10000, range 1000-300000)"},
			{Short: "", Long: "--memory-max <size>", Description: "Hard memory ceiling: 512M, 2G, or bytes"},
			{Short: "", Long: "--cpu-max <percent>", Description: "CPU cap as percent of one core (100 = 1 core, 200 = 2 cores)"},
			{Short: "", Long: "--tasks-max <N>", Description: "Maximum number of tasks (threads + subprocesses)"},
			{Short: "", Long: "--watch", Description: "Restart on file changes in cwd"},
			{Short: "", Long: "--watch-ignore <globs>", Description: "Extra ignore patterns (comma-separated)"},
			{Short: "-n", Long: "--dry-run", Description: "Print the resolved spec without starting anything"},
			{Short: "-q", Long: "--quiet", Description: "Suppress success messages (errors still printed)"},
		},
		Examples: []string{
			`lynx start "node server.js" --name api`,
			`lynx start app.py --runtime python3 --restart on-failure`,
			`lynx start "uv run main.py" --name worker --cwd /srv/app`,
			`lynx start "bun run dev" --name web --env-file .env`,
			`lynx start ./target/release/api --name api --restart always`,
			`lynx start worker.js --name w --scale 3`,
			`lynx start server.js --isolation sandbox --cwd /srv/app`,
			`# Runtime recipes:  docs/RUNTIMES.md`,
		},
	}
}
