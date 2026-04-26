package logs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Jaro-c/Lynx/internal/term"
)

// Size guard rails for "read whole file" paths (--all, very large -n).
// Bounded tail with seek-from-end is unaffected — it never scans more
// than ~n*200 bytes per source.
const (
	warnSizeBytes  int64 = 10 * 1024 * 1024  // 10 MiB
	blockSizeBytes int64 = 100 * 1024 * 1024 // 100 MiB
)

// totalSize returns the summed size of every existing source file.
// Missing files contribute zero (caller already prints "File not
// found" notices when it opens them).
func totalSize(sources []streamSource) int64 {
	var total int64
	for _, s := range sources {
		st, err := os.Stat(s.path)
		if err != nil {
			continue
		}
		total += st.Size()
	}
	return total
}

// formatBytes renders a human-readable size for guard messages.
func formatBytes(n int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case n >= gib:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(kib))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// guardLargeRead applies the 10/100 MiB policy. yes skips the prompt.
// in is the reader used for the y/N answer (os.Stdin in production,
// substitutable in tests). Returns nil when the read may proceed.
func guardLargeRead(sources []streamSource, yes bool, in io.Reader) error {
	total := totalSize(sources)
	if total < warnSizeBytes {
		return nil
	}
	size := formatBytes(total)
	suggestions := strings.Join([]string{
		"  --tail N        last N lines",
		"  --since 1h      time window",
		"  --grep pattern  regex filter",
	}, "\n")

	if total >= blockSizeBytes {
		if !yes {
			return fmt.Errorf("log size %s exceeds %s; pass --yes to override or narrow with:\n%s",
				size, formatBytes(blockSizeBytes), suggestions)
		}
		_, _ = fmt.Fprintf(os.Stderr, "%s reading %s of logs (--yes set)\n",
			term.YellowString("warning:"), size)
		return nil
	}

	// 10–100 MiB: warn + confirm if interactive, proceed otherwise.
	if yes {
		return nil
	}
	if !term.IsTTY() {
		_, _ = fmt.Fprintf(os.Stderr, "%s reading %s of logs (non-tty, proceeding)\n",
			term.YellowString("warning:"), size)
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "log size %s. options:\n%s\nproceed anyway? [y/N] ", size, suggestions)
	r := bufio.NewReader(in)
	answer, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read confirmation: %w", err)
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return errors.New("aborted by user")
	}
	return nil
}
