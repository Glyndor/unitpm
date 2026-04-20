package expand_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
	"github.com/Jaro-c/Lynx/internal/cli/expand"
	"github.com/Jaro-c/Lynx/internal/types"
)

type listMock struct {
	procs []types.ProcessInfo
	err   error
	calls int
}

func (m *listMock) Call(cmd string, _ any, result any) error {
	m.calls++
	if m.err != nil {
		return m.err
	}
	if cmd != "list" {
		return errors.New("unexpected call: " + cmd)
	}
	b, _ := json.Marshal(m.procs)
	_ = json.Unmarshal(b, result)
	return nil
}

func (m *listMock) Close() error { return nil }

func sample() []types.ProcessInfo {
	return []types.ProcessInfo{
		{ID: "id-prod-api", Name: "api", Namespace: "prod"},
		{ID: "id-prod-worker", Name: "worker", Namespace: "prod"},
		{ID: "id-dev-api", Name: "api", Namespace: "dev"},
		{ID: "id-default-cron", Name: "cron", Namespace: ""}, // empty → default
	}
}

func TestParseSelector_Literals(t *testing.T) {
	for _, tok := range []string{"api", "id-abc", "prod:api"} {
		s := expand.ParseSelector(tok)
		if s.AllInNS || s.AllProcs {
			t.Errorf("literal %q misclassified as wildcard: %+v", tok, s)
		}
	}
}

func TestParseSelector_Wildcards(t *testing.T) {
	cases := []struct {
		tok     string
		ns      string
		allInNS bool
		all     bool
	}{
		{"*", "", false, true},
		{"*:*", "", false, true},
		{"prod:*", "prod", true, false},
	}
	for _, c := range cases {
		s := expand.ParseSelector(c.tok)
		if s.Namespace != c.ns || s.AllInNS != c.allInNS || s.AllProcs != c.all {
			t.Errorf("%q → %+v, want ns=%q allInNS=%v all=%v",
				c.tok, s, c.ns, c.allInNS, c.all)
		}
	}
}

func TestTargets_LiteralPassthrough_NoIPC(t *testing.T) {
	mc := &listMock{procs: sample()}
	out, err := expand.Targets(mc, []string{"api", "prod:worker"}, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if mc.calls != 0 {
		t.Errorf("expected 0 IPC calls for literal-only, got %d", mc.calls)
	}
	if strings.Join(out, ",") != "api,prod:worker" {
		t.Errorf("out = %v", out)
	}
}

func TestTargets_NamespaceWildcard(t *testing.T) {
	mc := &listMock{procs: sample()}
	out, err := expand.Targets(mc, []string{"prod:*"}, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	got := strings.Join(out, ",")
	if got != "id-prod-api,id-prod-worker" {
		t.Errorf("out = %s", got)
	}
}

func TestTargets_AllProcsWildcard(t *testing.T) {
	mc := &listMock{procs: sample()}
	out, err := expand.Targets(mc, []string{"*"}, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 4 {
		t.Errorf("expected all 4 procs, got %d: %v", len(out), out)
	}
}

func TestTargets_AllProcsWildcard_EmptyClusterErrors(t *testing.T) {
	mc := &listMock{procs: nil}
	if _, err := expand.Targets(mc, []string{"*"}, ""); err == nil {
		t.Fatal("expected error for '*' on empty cluster")
	}
}

func TestTargets_DefaultNamespace_EmptyOnSpecMatched(t *testing.T) {
	mc := &listMock{procs: sample()}
	out, err := expand.Targets(mc, []string{"default:*"}, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.Join(out, ",") != "id-default-cron" {
		t.Errorf("out = %v", out)
	}
}

func TestTargets_NamespaceFlag(t *testing.T) {
	mc := &listMock{procs: sample()}
	out, err := expand.Targets(mc, nil, "prod")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.Join(out, ",") != "id-prod-api,id-prod-worker" {
		t.Errorf("out = %v", out)
	}
}

func TestTargets_NamespaceFlag_RejectsMixWithPositional(t *testing.T) {
	mc := &listMock{procs: sample()}
	_, err := expand.Targets(mc, []string{"api"}, "prod")
	if err == nil {
		t.Fatal("expected usage error when --namespace mixed with positional")
	}
	var ue *errs.UsageError
	if !errors.As(err, &ue) {
		t.Errorf("err = %T (%v), want *errs.UsageError", err, err)
	}
}

func TestTargets_EmptyNamespace_Errors(t *testing.T) {
	mc := &listMock{procs: sample()}
	_, err := expand.Targets(mc, []string{"ghost:*"}, "")
	if err == nil || !strings.Contains(err.Error(), `"ghost"`) {
		t.Errorf("expected empty-namespace error, got %v", err)
	}
}

func TestTargets_DedupesAcrossSelectors(t *testing.T) {
	mc := &listMock{procs: sample()}
	out, err := expand.Targets(mc, []string{"prod:*", "id-prod-api"}, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// id-prod-api should appear exactly once.
	count := 0
	for _, id := range out {
		if id == "id-prod-api" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("id-prod-api appears %d times in %v, want 1", count, out)
	}
}

func TestTargets_ListErrorPropagates(t *testing.T) {
	mc := &listMock{err: errors.New("connection refused")}
	_, err := expand.Targets(mc, []string{"prod:*"}, "")
	if err == nil || !strings.Contains(err.Error(), "list failed") {
		t.Errorf("err = %v", err)
	}
}

func TestTargets_NilClient_RejectedWhenWildcard(t *testing.T) {
	_, err := expand.Targets(nil, []string{"prod:*"}, "")
	if err == nil {
		t.Fatal("expected error for nil client + wildcard")
	}
}

func TestTargets_NilClient_OkForLiteral(t *testing.T) {
	out, err := expand.Targets(nil, []string{"api"}, "")
	if err != nil {
		t.Fatalf("literal-only must not need a client, got %v", err)
	}
	if len(out) != 1 || out[0] != "api" {
		t.Errorf("out = %v", out)
	}
}
