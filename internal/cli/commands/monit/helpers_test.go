package monit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/metrics"
	"github.com/Jaro-c/Lynx/internal/types"
)

func TestFmtBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{int64(1.5 * 1024 * 1024), "1.5 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, c := range cases {
		got := fmtBytes(c.in)
		if got != c.want {
			t.Errorf("fmtBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFmtUptime(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, "0s"},
		{5000, "5s"},
		{65000, "1m 5s"},
		{3600000, "1h 0m"},
		{3661000, "1h 1m"},
	}
	for _, c := range cases {
		got := fmtUptime(c.ms)
		if got != c.want {
			t.Errorf("fmtUptime(%d) = %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestVisLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"hello", 5},
		{"", 0},
		{"\033[32mok\033[0m", 2},
		{"\033[1mBold\033[0m text", 9},
		{"abc", 3},
	}
	for _, c := range cases {
		got := visLen(c.in)
		if got != c.want {
			t.Errorf("visLen(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPadTo(t *testing.T) {
	got := padTo("hi", 6, 2)
	if got != "hi    " {
		t.Errorf("padTo(%q, 6, 2) = %q", "hi", got)
	}
	// vl >= innerWidth: no padding, no truncation
	got = padTo("hello", 3, 5)
	if got != "hello" {
		t.Errorf("padTo with vl>innerWidth = %q", got)
	}
}

func TestBorderTop(t *testing.T) {
	s := borderTop(20, " Title ")
	if !strings.HasPrefix(s, "╭") || !strings.HasSuffix(s, "╮") {
		t.Errorf("borderTop missing corners: %q", s)
	}
	if !strings.Contains(s, " Title ") {
		t.Errorf("borderTop missing title: %q", s)
	}
}

func TestBorderBot(t *testing.T) {
	s := borderBot(10)
	if !strings.HasPrefix(s, "╰") || !strings.HasSuffix(s, "╯") {
		t.Errorf("borderBot missing corners: %q", s)
	}
	if len([]rune(s)) != 10 {
		t.Errorf("borderBot width = %d, want 10", len([]rune(s)))
	}
}

func TestGraphRowStr_InRange(t *testing.T) {
	rows := []string{"abc", "def"}
	got := graphRowStr(rows, 0, 3)
	if got != "abc" {
		t.Errorf("graphRowStr in range = %q, want %q", got, "abc")
	}
}

func TestGraphRowStr_OutOfRange(t *testing.T) {
	rows := []string{"abc"}
	got := graphRowStr(rows, 5, 4)
	if got != "    " {
		t.Errorf("graphRowStr out of range = %q, want spaces", got)
	}
}

func TestBuildGraph_Empty(t *testing.T) {
	rows := buildGraph(nil, 100, 10, 3)
	if len(rows) != 3 {
		t.Fatalf("buildGraph height = %d, want 3", len(rows))
	}
	for _, r := range rows {
		if strings.TrimSpace(r) != "" {
			t.Errorf("expected all spaces for empty data, got %q", r)
		}
	}
}

func TestBuildGraph_FullBar(t *testing.T) {
	// All values at max → all rows should be '█'
	vals := make([]float64, 10)
	for i := range vals {
		vals[i] = 100
	}
	rows := buildGraph(vals, 100, 10, 4)
	for _, r := range rows {
		for _, ch := range r {
			if ch != '█' {
				t.Errorf("expected '█' for full bar, got %q", string(ch))
			}
		}
	}
}

func TestBuildGraph_Width(t *testing.T) {
	rows := buildGraph([]float64{50}, 100, 8, 2)
	for _, r := range rows {
		if len([]rune(r)) != 8 {
			t.Errorf("buildGraph row width = %d, want 8", len([]rune(r)))
		}
	}
}

func TestDetailRow(t *testing.T) {
	row := detailRow("key", "value", "k2", "v2")
	if !strings.Contains(row, "value") {
		t.Errorf("detailRow missing value: %q", row)
	}
	if !strings.Contains(row, "v2") {
		t.Errorf("detailRow missing v2: %q", row)
	}
}

func TestStateStr(t *testing.T) {
	states := []types.ProcessState{
		types.StateRunning, types.StateOnline,
		types.StateStopped, types.StateExited,
		types.StateFailed, types.StateRestarting,
		"unknown",
	}
	for _, s := range states {
		got := stateStr(s)
		if got == "" {
			t.Errorf("stateStr(%q) returned empty string", s)
		}
	}
}

func TestPrintJSON_NoError(t *testing.T) {
	s := &monitState{}
	err := printJSON(s)
	if err != nil {
		t.Errorf("printJSON returned error: %v", err)
	}
}

// dataClient populates the result pointer by JSON-encoding per-verb fixtures.
type dataClient struct {
	fixtures map[string]any
}

func (d *dataClient) Call(verb string, _ any, result any) error {
	fix, ok := d.fixtures[verb]
	if !ok || result == nil {
		return nil
	}
	b, err := json.Marshal(fix)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, result)
}

func (d *dataClient) Close() error { return nil }

func TestFetchState_PopulatesInfo(t *testing.T) {
	info := types.ProcessInfo{
		Name:   "testproc",
		PID:    12345,
		State:  types.StateRunning,
		CPU:    1.5,
		Memory: 1024 * 1024,
	}
	dc := &dataClient{fixtures: map[string]any{
		"show": showResponse{Info: info, Spec: protocol.AppSpec{}},
	}}
	s := &monitState{}
	if err := fetchState(dc, "testproc", s); err != nil {
		t.Fatalf("fetchState: %v", err)
	}
	if s.info.Name != "testproc" {
		t.Errorf("info.Name = %q, want %q", s.info.Name, "testproc")
	}
	if s.info.PID != 12345 {
		t.Errorf("info.PID = %d, want 12345", s.info.PID)
	}
	if len(s.cpuHist) != 1 || s.cpuHist[0] != 1.5 {
		t.Errorf("cpuHist = %v, want [1.5]", s.cpuHist)
	}
	if s.memMax != int64(1024*1024) {
		t.Errorf("memMax = %d, want %d", s.memMax, 1024*1024)
	}
}

func TestFetchState_HistoryTrimmed(t *testing.T) {
	dc := &dataClient{fixtures: map[string]any{
		"show": showResponse{Info: types.ProcessInfo{CPU: 50}, Spec: protocol.AppSpec{}},
	}}
	s := &monitState{}
	for i := 0; i < maxHistory+10; i++ {
		if err := fetchState(dc, "x", s); err != nil {
			t.Fatalf("fetchState iteration %d: %v", i, err)
		}
	}
	if len(s.cpuHist) != maxHistory {
		t.Errorf("cpuHist len = %d, want %d", len(s.cpuHist), maxHistory)
	}
	if len(s.memHist) != maxHistory {
		t.Errorf("memHist len = %d, want %d", len(s.memHist), maxHistory)
	}
}

func TestRunSingle_JSONMode(t *testing.T) {
	info := types.ProcessInfo{Name: "svc", PID: 999, State: types.StateRunning}
	dc := &dataClient{fixtures: map[string]any{
		"show": showResponse{Info: info, Spec: protocol.AppSpec{}},
	}}
	err := runSingle(dc, "svc", true)
	if err != nil {
		t.Errorf("runSingle JSON mode error: %v", err)
	}
}

func makeFullState() *monitState {
	return &monitState{
		info: types.ProcessInfo{
			Name:      "testsvc",
			Namespace: "default",
			PID:       42,
			State:     types.StateRunning,
			CPU:       12.5,
			Memory:    4 * 1024 * 1024,
			Uptime:    3725000,
			Restarts:  3,
			GitBranch: "main",
			GitCommit: "abc1234",
			Version:   "1.0",
			Mode:      "cluster",
			User:      "root",
		},
		spec: protocol.AppSpec{
			Exec: protocol.AppExec{
				Command: "/usr/bin/node",
				Args:    []string{"server.js"},
			},
		},
		cpuHist: []float64{0, 5, 10, 15, 20, 25, 12.5},
		memHist: []int64{0, 1024 * 1024, 2 * 1024 * 1024, 4 * 1024 * 1024},
		memMax:  4 * 1024 * 1024,
	}
}

func TestRender_NoPanic(t *testing.T) {
	// render writes to os.Stdout; verify it doesn't panic with a full state.
	render(makeFullState())
}

func TestRender_WithProcessTree(t *testing.T) {
	s := makeFullState()
	s.tree = []metrics.ChildStat{
		{PID: 42, Comm: "node", Depth: 0, MemoryBytes: 1024 * 1024},
		{PID: 43, Comm: "worker", Depth: 1, MemoryBytes: 512 * 1024},
	}
	render(s)
}

func TestRender_StoppedState(t *testing.T) {
	s := makeFullState()
	s.info.State = types.StateStopped
	render(s)
}

func TestRender_FailedState(t *testing.T) {
	s := makeFullState()
	s.info.State = types.StateFailed
	render(s)
}

func TestRender_EmptyHistory(t *testing.T) {
	// render with no history — graph should fill with spaces, no panic
	render(&monitState{info: types.ProcessInfo{Name: "empty", State: types.StateRunning}})
}

func TestRender_NoGit(t *testing.T) {
	s := makeFullState()
	s.info.GitBranch = ""
	s.info.GitCommit = ""
	render(s)
}
