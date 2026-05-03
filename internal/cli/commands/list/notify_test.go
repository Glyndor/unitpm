package list_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
	"github.com/Jaro-c/Lynx/internal/ipc/transport"
	"github.com/Jaro-c/Lynx/internal/types"
	"github.com/Jaro-c/Lynx/internal/updater"
)

func TestWaitUpdateAndNotify_NilRelease(t *testing.T) {
	ch := make(chan *updater.Release, 1)
	ch <- nil // nil release should be a no-op
	deadline := time.Now().Add(100 * time.Millisecond)
	// Should not panic.
	list.WaitUpdateAndNotify(ch, deadline)
}

func TestWaitUpdateAndNotify_WithRelease(t *testing.T) {
	ch := make(chan *updater.Release, 1)
	ch <- &updater.Release{TagName: "v1.2.3"}
	deadline := time.Now().Add(100 * time.Millisecond)
	// Should print banner to stderr without panic.
	list.WaitUpdateAndNotify(ch, deadline)
}

func TestWaitUpdateAndNotify_Timeout(t *testing.T) {
	ch := make(chan *updater.Release) // nothing sent
	deadline := time.Now().Add(-1 * time.Second) // already expired
	// Should return immediately (timer fires instantly).
	list.WaitUpdateAndNotify(ch, deadline)
}

func TestPrintUpdateBanner_NoPanel(t *testing.T) {
	rel := &updater.Release{TagName: "v9.9.9"}
	// Should not panic.
	list.PrintUpdateBanner(rel)
}

func TestFetchAndRender_CallsFails(t *testing.T) {
	// FetchAndRender should swallow errors silently.
	client := &mockListClient{err: errors.New("daemon offline")}
	list.FetchAndRender(client, nil)
}

func TestFetchAndRender_EmptyList(t *testing.T) {
	client := &mockListClient{processes: []types.ProcessInfo{}}
	list.FetchAndRender(client, nil)
}

type mockListClient struct {
	processes []types.ProcessInfo
	err       error
}

func (m *mockListClient) Call(_ string, _ any, result any) error {
	if m.err != nil {
		return m.err
	}
	b, _ := json.Marshal(m.processes)
	return json.Unmarshal(b, result)
}

func (m *mockListClient) Close() error { return nil }

// Compile-time check that mockListClient implements transport.IPCClient.
var _ transport.IPCClient = (*mockListClient)(nil)
