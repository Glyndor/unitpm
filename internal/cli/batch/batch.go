// Package batch provides the shared result shape for CLI commands that
// operate on multiple targets (delete, stop, reload, reset, restart, flush,
// scale, apply). It normalizes three concerns:
//
//   - JSON output ({ results: [...], summary: {...} })
//   - Non-zero exit when any target failed
//   - Optional human-readable trailing summary when more than one target
//     is involved
//
// Commands still emit their own per-target human lines as results arrive —
// batch only owns the aggregate shape and the final reporting.
package batch

import (
	"fmt"
	"os"
	"strings"

	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/Jaro-c/Lynx/internal/term"
)

// SplitArgs partitions args into flag-like tokens (anything starting with
// "-") and positional tokens. Used by the batch commands so users can
// put --json / --purge / etc. either before or after the target IDs;
// Go's stdlib flag package stops at the first non-flag, which would
// otherwise force "flags first" and break common shell habits like
// `lynxpm stop api worker --json`.
//
// Safe ONLY for commands whose flags are all boolean (no
// --key value pairs). Value-taking flags would be misclassified as
// positionals. Use SplitArgsWithValues when the command accepts
// value-taking flags like `--namespace prod`.
func SplitArgs(args []string) ([]string, []string) {
	return SplitArgsWithValues(args, nil)
}

// SplitArgsWithValues is SplitArgs but aware of value-taking flag names.
// Pass the long flag names (without leading dashes) that consume the next
// token as their value, e.g. {"namespace": true}. The function recognises
// both `--namespace prod` (two tokens) and `--namespace=prod` (one token).
// Unknown long flags fall back to the boolean-style classification used
// by SplitArgs.
func SplitArgsWithValues(args []string, valueFlags map[string]bool) ([]string, []string) {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// `--key=value` keeps the value glued to the flag.
			if strings.Contains(a, "=") {
				continue
			}
			name := strings.TrimLeft(a, "-")
			if valueFlags[name] && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return flags, positional
}

// Status classifies a single target's outcome.
type Status string

const (
	// StatusOK means the operation completed with effect.
	StatusOK Status = "ok"
	// StatusFailed means the daemon returned an error for this target.
	StatusFailed Status = "failed"
	// StatusNoop means the operation was a no-op (e.g. already stopped).
	StatusNoop Status = "noop"
)

// Result is one target's outcome.
type Result struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
	// Error is the daemon error message when Status == failed.
	Error string `json:"error,omitempty"`
	// Extra carries command-specific payload (e.g. bytes freed, was_running)
	// that the caller wants surfaced in --json output.
	Extra map[string]any `json:"extra,omitempty"`
}

// Summary counts results per status.
type Summary struct {
	Total  int `json:"total"`
	Ok     int `json:"ok"`
	Failed int `json:"failed"`
	Noop   int `json:"noop,omitempty"`
}

// Report is the batch-wide aggregate returned by --json and used to decide
// the process exit code.
type Report struct {
	Op      string   `json:"op"`
	Results []Result `json:"results"`
	Summary Summary  `json:"summary"`
}

// New creates a report for the given op name (used as the "op" field in
// --json output and in the trailing human summary).
func New(op string) *Report {
	return &Report{Op: op, Results: []Result{}}
}

// Add appends a result and updates counters.
func (r *Report) Add(res Result) {
	r.Results = append(r.Results, res)
	r.Summary.Total++
	switch res.Status {
	case StatusOK:
		r.Summary.Ok++
	case StatusFailed:
		r.Summary.Failed++
	case StatusNoop:
		r.Summary.Noop++
	}
}

// OK records a successful target.
func (r *Report) OK(id string, extra map[string]any) {
	r.Add(Result{ID: id, Status: StatusOK, Extra: extra})
}

// Noop records a target that was already in the desired state.
func (r *Report) Noop(id string, extra map[string]any) {
	r.Add(Result{ID: id, Status: StatusNoop, Extra: extra})
}

// Fail records a target that errored. Pass the raw daemon error.
func (r *Report) Fail(id string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	r.Add(Result{ID: id, Status: StatusFailed, Error: msg})
}

// Err returns a non-nil error when any target failed. Designed to be the
// command's return value so the caller gets a non-zero exit code — but
// since per-target lines were already printed, the error keeps the
// operator-facing message short.
func (r *Report) Err() error {
	if r.Summary.Failed == 0 {
		return nil
	}
	if r.Summary.Failed == 1 && r.Summary.Total == 1 {
		return fmt.Errorf("%s failed", r.Op)
	}
	return fmt.Errorf("%s: %d of %d targets failed", r.Op, r.Summary.Failed, r.Summary.Total)
}

// PrintSummary emits a single trailing line when more than one target was
// processed. Silent for single-target invocations so the common path stays
// terse. Uses term.Printf so --quiet suppresses it.
func (r *Report) PrintSummary() {
	if r.Summary.Total <= 1 {
		return
	}
	parts := []string{fmt.Sprintf("%d ok", r.Summary.Ok)}
	if r.Summary.Noop > 0 {
		parts = append(parts, term.YellowString("%d noop", r.Summary.Noop))
	}
	if r.Summary.Failed > 0 {
		parts = append(parts, term.RedString("%d failed", r.Summary.Failed))
	}
	_, _ = term.Printf("\n%s %s: %s\n",
		statusMark(r.Summary.Failed == 0),
		term.BoldString("%s", r.Op),
		joinParts(parts),
	)
}

// EmitJSON marshals the report to stdout. Returns any marshal/write error.
func (r *Report) EmitJSON() error {
	b, err := jsonx.Marshal(r)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(b))
	return err
}

func statusMark(ok bool) string {
	if ok {
		return term.GreenString("✓")
	}
	return term.RedString("✗")
}

func joinParts(parts []string) string {
	return strings.Join(parts, ", ")
}
