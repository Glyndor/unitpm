// Package format provides shared human-readable formatters for CLI commands.
package format

import (
	"fmt"
	"strings"
	"time"

	"github.com/Jaro-c/Lynx/internal/term"
)

// Bytes formats a byte count into a human-readable string using binary
// (1024) units: "512 B", "1.5 MB", "3.2 GB".
func Bytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// BytesExact formats a byte count as both human-readable and raw bytes,
// e.g. "232.6 MB (243867648 bytes)". For values below 1 KiB the raw form
// would be redundant, so only the short form is returned.
func BytesExact(b int64) string {
	if b < 1024 {
		return Bytes(b)
	}
	return fmt.Sprintf("%s (%d bytes)", Bytes(b), b)
}

// Uptime formats milliseconds into a compact duration string with at most
// two units: "22m 9s", "2d 3h". Non-positive input renders as a dimmed "-".
func Uptime(ms int64) string {
	if ms <= 0 {
		return term.DimString("-")
	}

	d := time.Duration(ms) * time.Millisecond
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	case minutes > 0 && seconds > 0:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

// UptimeExact renders milliseconds as both human form and raw ms,
// e.g. "22m 9s (1329123 ms)". Useful when the precise value matters
// (benchmarks, exact restart timing).
func UptimeExact(ms int64) string {
	if ms <= 0 {
		return term.DimString("-")
	}
	return fmt.Sprintf("%s (%d ms)", Uptime(ms), ms)
}

// Timestamp normalizes an RFC3339 (or similar) timestamp into
// "<abs> (<relative>)", e.g. "2026-04-19 14:03:22 (2h ago)". Falls back to
// the raw string on parse failure, or a dimmed "-" when empty.
func Timestamp(ts string) string {
	if ts == "" {
		return term.DimString("-")
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		if t, err = time.Parse(time.RFC3339Nano, ts); err != nil {
			return ts
		}
	}
	abs := t.Local().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("%s (%s)", abs, relativeAge(time.Since(t)))
}

func relativeAge(d time.Duration) string {
	switch {
	case d < 0:
		return "in the future"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

// Percent renders a CPU/memory-like percentage value. Zero renders as "0%"
// (no decimal), everything else as "%.1f%%".
func Percent(v float64) string {
	if v == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", v)
}

// StripAnsi removes ANSI escape sequences for width calculations and
// non-TTY output.
func StripAnsi(s string) string {
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
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
