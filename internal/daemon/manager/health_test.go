package manager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
)

func TestProbeOnce_HTTP_2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ok := probeOnce(context.Background(), &protocol.AppHealth{
		Type: "http",
		URL:  srv.URL,
	}, 2*time.Second, "")
	if !ok {
		t.Error("expected 200 to be healthy")
	}
}

func TestProbeOnce_HTTP_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ok := probeOnce(context.Background(), &protocol.AppHealth{
		Type: "http",
		URL:  srv.URL,
	}, 2*time.Second, "")
	if ok {
		t.Error("500 must be unhealthy")
	}
}

func TestProbeOnce_HTTP_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	ok := probeOnce(context.Background(), &protocol.AppHealth{
		Type: "http",
		URL:  srv.URL,
	}, 50*time.Millisecond, "")
	if ok {
		t.Error("slow server with short timeout should fail")
	}
}

func TestProbeOnce_Exec_Success(t *testing.T) {
	ok := probeOnce(context.Background(), &protocol.AppHealth{
		Type: "exec",
		Exec: "true",
	}, time.Second, "")
	if !ok {
		t.Error("'true' should succeed")
	}
}

func TestProbeOnce_Exec_Failure(t *testing.T) {
	ok := probeOnce(context.Background(), &protocol.AppHealth{
		Type: "exec",
		Exec: "false",
	}, time.Second, "")
	if ok {
		t.Error("'false' should fail")
	}
}

func TestProbeOnce_UnknownType(t *testing.T) {
	ok := probeOnce(context.Background(), &protocol.AppHealth{Type: "tcp"}, time.Second, "")
	if ok {
		t.Error("unknown probe type must be unhealthy")
	}
}
