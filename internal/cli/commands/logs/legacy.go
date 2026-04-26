package logs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Jaro-c/Lynx/internal/term"
)

// runLegacySplit reproduces the pre-merge behavior: each source is
// tailed in its own goroutine, lines emitted in arrival order with no
// cross-stream ordering. Kept as an escape hatch behind --no-merge for
// users who script against the old format.
func runLegacySplit(ctx context.Context, sources []streamSource, opts options) error {
	var wg sync.WaitGroup
	for _, s := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tailFileLegacy(ctx, s.path, s.label, opts.lines, opts.follow, time.Sleep)
		}()
	}
	wg.Wait()
	return nil
}

func tailFileLegacy(ctx context.Context, path, label string, n int, follow bool, sleep Sleeper) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			if follow {
				_, _ = term.Printf("%s File not found, waiting...\n", colorLabel(label))
				for {
					select {
					case <-ctx.Done():
						return
					default:
					}
					sleep(1 * time.Second)
					f, err = os.Open(path)
					if err == nil {
						break
					}
					if !os.IsNotExist(err) {
						_, _ = fmt.Fprintf(os.Stderr, "%s %s\n", colorLabel(label), term.RedString("Error: %v", err))
						return
					}
				}
			} else {
				_, _ = term.Printf("%s File not found\n", colorLabel(label))
				return
			}
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "%s %s\n", colorLabel(label), term.RedString("Error: %v", err))
			return
		}
	}
	defer func() { _ = f.Close() }()

	printLastNLinesLegacy(f, label, n)

	if !follow {
		return
	}
	_, _ = f.Seek(0, io.SeekEnd) //nolint:errcheck
	reader := bufio.NewReader(f)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				sleep(200 * time.Millisecond)
				continue
			}
			_, _ = fmt.Fprintf(os.Stderr, "%s %s\n", colorLabel(label), term.RedString("Error: %v", err))
			return
		}
		fmt.Printf("%s %s", colorLabel(label), line)
	}
}

func printLastNLinesLegacy(f *os.File, label string, n int) {
	stat, err := f.Stat()
	if err != nil {
		return
	}
	fileSize := stat.Size()
	offset := fileSize - int64(n*150)
	if offset < 0 {
		offset = 0
	}
	_, _ = f.Seek(offset, io.SeekStart) //nolint:errcheck

	scanner := bufio.NewScanner(f)
	if offset > 0 {
		scanner.Scan()
	}
	ring := make([]string, n)
	idx := 0
	for scanner.Scan() {
		ring[idx%n] = scanner.Text()
		idx++
	}
	total := idx
	if total > n {
		total = n
	}
	start := 0
	if idx > n {
		start = idx % n
	}
	for i := 0; i < total; i++ {
		fmt.Printf("%s %s\n", colorLabel(label), ring[(start+i)%n])
	}
}
