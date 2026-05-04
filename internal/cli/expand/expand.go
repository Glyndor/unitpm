// Package expand resolves CLI target selectors used by lifecycle commands
// (stop, restart, reload, reset, delete, flush) into concrete process IDs.
//
// Two selector forms are supported on top of the existing "<id|name>" /
// "<namespace>:<name>" forms:
//
//   - "<namespace>:*" — every process in that namespace
//   - "*" or "*:*"   — every managed process
//
// Plus a flag-style selector: --namespace <name>, which expands to the
// same set as "<name>:*" but cannot be combined with positional targets
// (mixing the two is rejected as a usage error to avoid surprise).
package expand

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/types"
)

// Public flag/selector tokens shared by the lifecycle commands so a rename
// only happens in one place.
const (
	NamespaceFlag      = "namespace"
	WildcardAll        = "*"
	WildcardAllPair    = "*:*"
	NamespaceSeparator = ":"
)

// Selector classifies a single positional target token.
type Selector struct {
	Namespace string // namespace part for "ns:*"; empty for "*" / "*:*"
	AllInNS   bool   // true when token is "<ns>:*"
	AllProcs  bool   // true when token is "*" or "*:*"
}

// ParseSelector parses a single positional token. Tokens that aren't
// wildcards are returned with both wildcard flags false; the caller passes
// them straight through to the daemon's ResolveID.
func ParseSelector(tok string) Selector {
	switch tok {
	case WildcardAll, WildcardAllPair:
		return Selector{AllProcs: true}
	}
	if idx := strings.Index(tok, NamespaceSeparator); idx != -1 {
		ns, name := tok[:idx], tok[idx+1:]
		if name == WildcardAll && ns != "" && ns != WildcardAll {
			return Selector{Namespace: ns, AllInNS: true}
		}
	}
	return Selector{}
}

// Targets resolves the positional `ids` and the optional `--namespace`
// value into a deduplicated slice of process IDs. Literal targets (no
// wildcard, no `--namespace` flag) are passed through unchanged so the
// daemon can resolve names/prefixes the same way as before.
//
// Wildcard expansion calls `list` over the provided client. The client
// is required as soon as any wildcard or `--namespace` flag is present;
// callers that pass only literals can pass a nil client and Targets will
// return early without an IPC round-trip.
func Targets(client transport.IPCClient, ids []string, namespace string) ([]string, error) {
	if namespace != "" {
		if len(ids) > 0 {
			return nil, &errs.UsageError{
				Message: "cannot combine --namespace with positional targets — use one or the other",
			}
		}
		return expandNamespace(client, namespace)
	}

	sels := make([]Selector, len(ids))
	hasWildcard := false
	for i, tok := range ids {
		sels[i] = ParseSelector(tok)
		if sels[i].AllInNS || sels[i].AllProcs {
			hasWildcard = true
		}
	}

	if !hasWildcard {
		return ids, nil
	}

	procs, err := fetchList(client)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(procs))
	out := make([]string, 0, len(procs))
	add := func(id string) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	for i, tok := range ids {
		sel := sels[i]
		switch {
		case sel.AllProcs:
			if len(procs) == 0 {
				return nil, errors.New("no managed processes")
			}
			for _, p := range procs {
				add(p.ID)
			}
		case sel.AllInNS:
			matched := false
			for _, p := range procs {
				if processNS(p) == sel.Namespace {
					add(p.ID)
					matched = true
				}
			}
			if !matched {
				return nil, fmt.Errorf("no processes in namespace %q", sel.Namespace)
			}
		default:
			add(tok)
		}
	}
	return out, nil
}

func expandNamespace(client transport.IPCClient, ns string) ([]string, error) {
	procs, err := fetchList(client)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(procs))
	for _, p := range procs {
		if processNS(p) == ns {
			out = append(out, p.ID)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no processes in namespace %q", ns)
	}
	return out, nil
}

func fetchList(client transport.IPCClient) ([]types.ProcessInfo, error) {
	if client == nil {
		return nil, errors.New("internal error: expand requires an IPC client")
	}
	var procs []types.ProcessInfo
	if err := client.Call("list", nil, &procs); err != nil {
		return nil, fmt.Errorf("list failed: %w", err)
	}
	return procs, nil
}

func processNS(p types.ProcessInfo) string {
	if p.Namespace == "" {
		return types.DefaultNamespace
	}
	return p.Namespace
}
