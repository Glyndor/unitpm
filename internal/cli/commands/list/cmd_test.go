package list_test

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
	"github.com/Jaro-c/Lynx/internal/types"
)

// mockClient satisfies transport.IPCClient.
type mockClient struct {
	response any
	err      error
	calls    []string
}

func (m *mockClient) Call(cmd string, _ any, result any) error {
	m.calls = append(m.calls, cmd)
	if m.err != nil {
		return m.err
	}
	if m.response != nil {
		b, _ := json.Marshal(m.response)
		_ = json.Unmarshal(b, result)
	}
	return nil
}

func (m *mockClient) Close() error { return nil }

// --- ParseSortSpec ---

func TestParseSortSpec(t *testing.T) {
	tests := []struct {
		input   string
		want    []list.SortField
		wantErr bool
	}{
		{input: "", want: nil},
		{
			input: "name",
			want:  []list.SortField{{Field: "name", Asc: true}},
		},
		{
			input: "name:desc",
			want:  []list.SortField{{Field: "name", Asc: false}},
		},
		{
			input: "namespace:asc, name:desc",
			want: []list.SortField{
				{Field: "namespace", Asc: true},
				{Field: "name", Asc: false},
			},
		},
		{input: "invalid", wantErr: true},
		{input: "name:invalid", wantErr: true},
		{
			input: "id:asc",
			want:  []list.SortField{{Field: "id", Asc: true}},
		},
		{
			input: "createdAt:desc",
			want:  []list.SortField{{Field: "createdAt", Asc: false}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := list.ParseSortSpec(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSortSpec(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSortSpec(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- FormatUptime ---

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{0, "-"},
		{-1, "-"},
		{500, "0s"},
		{1000, "1s"},
		{61000, "1m 1s"},
		{3600000, "1h"},
		{3660000, "1h 1m"},
		{86400000, "1d"},
		{86400000 + 3600000, "1d 1h"},
	}

	for _, tt := range tests {
		got := list.FormatUptime(tt.ms)
		// Strip ANSI codes for comparison
		got = stripAnsi(got)
		if got != tt.want {
			t.Errorf("FormatUptime(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

// --- FormatBytes ---

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		b    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}

	for _, tt := range tests {
		got := list.FormatBytes(tt.b)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

// --- ShortIDLen ---

func TestShortIDLen(t *testing.T) {
	// Empty or single: always 8
	if l := list.ShortIDLen(nil); l != 8 {
		t.Errorf("ShortIDLen(nil) = %d, want 8", l)
	}
	if l := list.ShortIDLen([]types.ProcessInfo{{ID: "abc12345"}}); l != 8 {
		t.Errorf("ShortIDLen(single) = %d, want 8", l)
	}

	// Distinct at 8 chars
	procs := []types.ProcessInfo{
		{ID: "aaaaaaaa-0000-0000-0000-000000000000"},
		{ID: "bbbbbbbb-0000-0000-0000-000000000000"},
	}
	if l := list.ShortIDLen(procs); l != 8 {
		t.Errorf("expected 8 for distinct 8-char prefixes, got %d", l)
	}

	// Collision at 8, distinct at 9
	procs2 := []types.ProcessInfo{
		{ID: "abcdefgh-aaa0-0000-0000-000000000000"},
		{ID: "abcdefgh-bbb0-0000-0000-000000000000"},
	}
	l := list.ShortIDLen(procs2)
	if l < 9 {
		t.Errorf("expected >= 9 for 8-char collision, got %d", l)
	}
}

// --- FilterProcesses ---

func TestFilterProcesses(t *testing.T) {
	procs := []types.ProcessInfo{
		{ID: "1", Namespace: "prod"},
		{ID: "2", Namespace: "staging"},
		{ID: "3", Namespace: ""}, // treated as "default"
	}

	// No filter: all returned
	got := list.FilterProcesses(procs, "")
	if len(got) != 3 {
		t.Errorf("expected 3, got %d", len(got))
	}

	// Filter by "prod"
	got = list.FilterProcesses(procs, "prod")
	if len(got) != 1 || got[0].ID != "1" {
		t.Errorf("expected only proc 1, got %v", got)
	}

	// Filter by "default" matches empty namespace
	got = list.FilterProcesses(procs, "default")
	if len(got) != 1 || got[0].ID != "3" {
		t.Errorf("expected only proc 3, got %v", got)
	}

	// Filter with no matches
	got = list.FilterProcesses(procs, "nonexistent")
	if len(got) != 0 {
		t.Errorf("expected 0 results, got %d", len(got))
	}
}

// --- Run (via mock) ---

func TestRun_Help(t *testing.T) {
	err := list.Run(nil, []string{"--help"})
	if err != nil {
		t.Errorf("expected no error for --help, got %v", err)
	}
}

func TestRun_UnknownFlag(t *testing.T) {
	err := list.Run(nil, []string{"--badFlag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestRun_UnexpectedArgs(t *testing.T) {
	mc := &mockClient{response: []types.ProcessInfo{}}
	err := list.Run(mc, []string{"unexpected"})
	if err == nil {
		t.Fatal("expected error for unexpected positional args")
	}
}

func TestRun_IPCError(t *testing.T) {
	mc := &mockClient{err: errors.New("connection refused")}
	err := list.Run(mc, []string{})
	if err == nil {
		t.Fatal("expected error from IPC failure")
	}
	if !strings.Contains(err.Error(), "list failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_Success_Empty(t *testing.T) {
	mc := &mockClient{response: []types.ProcessInfo{}}
	err := list.Run(mc, []string{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.calls) != 1 || mc.calls[0] != "list" {
		t.Errorf("expected one 'list' call, got %v", mc.calls)
	}
}

func TestRun_Success_WithProcesses(t *testing.T) {
	procs := []types.ProcessInfo{
		{
			ID:        "aaaaaaaa-0000-0000-0000-000000000000",
			Name:      "api",
			Namespace: "prod",
			State:     types.StateRunning,
			PID:       1234,
			CPU:       1.5,
			Memory:    7 * 1024 * 1024,
		},
		{
			ID:        "bbbbbbbb-0000-0000-0000-000000000000",
			Name:      "worker",
			Namespace: "prod",
			State:     types.StateStopped,
		},
	}
	mc := &mockClient{response: procs}
	err := list.Run(mc, []string{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRun_NamespaceFilter(t *testing.T) {
	procs := []types.ProcessInfo{
		{ID: "aaa", Namespace: "prod"},
		{ID: "bbb", Namespace: "staging"},
	}
	mc := &mockClient{response: procs}
	err := list.Run(mc, []string{"--namespace", "prod"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRun_LongFlag(t *testing.T) {
	procs := []types.ProcessInfo{
		{ID: "aaaaaaaa-0000-0000-0000-000000000000", Name: "api", Namespace: "default"},
	}
	mc := &mockClient{response: procs}
	err := list.Run(mc, []string{"--long"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRun_SortFlag(t *testing.T) {
	procs := []types.ProcessInfo{
		{ID: "aaa", Name: "z-app", Namespace: "default"},
		{ID: "bbb", Name: "a-app", Namespace: "default"},
	}
	mc := &mockClient{response: procs}
	err := list.Run(mc, []string{"--sort", "name:asc"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRun_InvalidSort(t *testing.T) {
	mc := &mockClient{response: []types.ProcessInfo{}}
	err := list.Run(mc, []string{"--sort", "badfield"})
	if err == nil {
		t.Fatal("expected error for invalid sort field")
	}
}

// captureStdout runs fn with os.Stdout rerouted and returns what was
// written. Panic-safe.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	done := make(chan struct{})
	var buf strings.Builder
	go func() {
		b := make([]byte, 4096)
		for {
			n, rerr := r.Read(b)
			if n > 0 {
				buf.Write(b[:n])
			}
			if rerr != nil {
				break
			}
		}
		close(done)
	}()

	func() {
		defer func() { _ = w.Close() }()
		fn()
	}()
	<-done
	return buf.String()
}

func TestRun_JSON(t *testing.T) {
	procs := []types.ProcessInfo{
		{ID: "id1", Name: "api", Namespace: "prod", State: types.StateRunning, PID: 1},
		{ID: "id2", Name: "worker", Namespace: "prod", State: types.StateStopped},
	}
	mc := &mockClient{response: procs}
	out := captureStdout(t, func() {
		if err := list.Run(mc, []string{"--json"}); err != nil {
			t.Fatalf("run --json: %v", err)
		}
	})

	var got []types.ProcessInfo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Errorf("want 2 procs, got %d", len(got))
	}
	if got[0].Name != "api" || got[1].Name != "worker" {
		t.Errorf("unexpected names: %v", got)
	}
}

func TestRun_JSON_Empty(t *testing.T) {
	mc := &mockClient{response: []types.ProcessInfo{}}
	out := captureStdout(t, func() {
		if err := list.Run(mc, []string{"--json"}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	var got []types.ProcessInfo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("want '[]', got %q (err=%v)", out, err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// stripAnsi removes ANSI escape codes for comparison.
func stripAnsi(s string) string {
	var b strings.Builder
	inSeq := false
	for _, r := range s {
		if r == '\033' {
			inSeq = true
			continue
		}
		if inSeq {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inSeq = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
