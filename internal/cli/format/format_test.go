package format_test

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/format"
)

func TestBytes(t *testing.T) {
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
		if got := format.Bytes(tt.b); got != tt.want {
			t.Errorf("Bytes(%d) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

func TestBytesExact(t *testing.T) {
	if got := format.BytesExact(512); got != "512 B" {
		t.Errorf("BytesExact(512) = %q, want %q", got, "512 B")
	}
	got := format.BytesExact(1024 * 1024)
	if !strings.Contains(got, "1.0 MB") || !strings.Contains(got, "1048576 bytes") {
		t.Errorf("BytesExact(1 MiB) = %q, missing MB or raw bytes", got)
	}
}

func TestUptime(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{1000, "1s"},
		{61000, "1m 1s"},
		{3600000, "1h"},
		{3660000, "1h 1m"},
		{86400000, "1d"},
	}
	for _, tt := range tests {
		if got := format.Uptime(tt.ms); got != tt.want {
			t.Errorf("Uptime(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

func TestPercent(t *testing.T) {
	if got := format.Percent(0); got != "0%" {
		t.Errorf("Percent(0) = %q", got)
	}
	if got := format.Percent(1.5); got != "1.5%" {
		t.Errorf("Percent(1.5) = %q", got)
	}
}

func TestStripAnsi(t *testing.T) {
	if got := format.StripAnsi("\033[31mhello\033[0m"); got != "hello" {
		t.Errorf("StripAnsi = %q", got)
	}
}

func TestTimestampEmpty(t *testing.T) {
	// "-" dimmed — just check non-empty.
	if got := format.Timestamp(""); got == "" {
		t.Errorf("Timestamp(empty) should be dimmed dash, got empty")
	}
}

func TestTimestampParses(t *testing.T) {
	got := format.Timestamp("2024-01-01T12:00:00Z")
	if !strings.Contains(got, "2024-01-01") {
		t.Errorf("Timestamp = %q, missing abs date", got)
	}
	if !strings.Contains(got, "ago") {
		t.Errorf("Timestamp = %q, missing relative form", got)
	}
}
