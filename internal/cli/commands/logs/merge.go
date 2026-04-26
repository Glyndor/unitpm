package logs

import (
	"bufio"
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Jaro-c/Lynx/internal/term"
)

// tsLayout matches the prefix written by manager.timestampWriter:
// "2006-01-02 15:04:05 ". 19 chars + space.
const tsLayout = "2006-01-02 15:04:05"

const tsLen = 19

// entry is a single chronologically-anchored log record. body keeps the
// timestamp prefix stripped so callers can re-format on emit. Multi-line
// bodies (banners, stack traces) are folded under one anchor ts.
type entry struct {
	ts    time.Time
	label string
	body  string
	hasTS bool
	// seq breaks ties between entries with identical timestamps so the
	// merge stays stable per source.
	seq uint64
}

// filter is an optional post-parse predicate. since drops entries with
// ts before the cutoff (zero = no cutoff). grep, when non-nil, drops
// entries whose body does not match.
type filter struct {
	since time.Time
	grep  *regexp.Regexp
}

func (f filter) keep(e entry) bool {
	if !f.since.IsZero() && e.ts.Before(f.since) {
		return false
	}
	if f.grep != nil && !f.grep.MatchString(e.body) {
		return false
	}
	return true
}

// streamSource describes a file to be streamed during merge.
type streamSource struct {
	path    string
	label   string
	seqBase uint64
}

// parseLine extracts (ts, body, ok). ok=false means the line has no
// parseable timestamp — caller should fold it into the prior entry.
func parseLine(line string) (time.Time, string, bool) {
	if len(line) < tsLen+1 {
		return time.Time{}, line, false
	}
	t, err := time.ParseInLocation(tsLayout, line[:tsLen], time.Local)
	if err != nil {
		return time.Time{}, line, false
	}
	body := line[tsLen:]
	if len(body) > 0 && body[0] == ' ' {
		body = body[1:]
	}
	return t, body, true
}

// readEntries reads ALL entries from r in order. Continuation lines
// fold into the prior entry. Returns the next seq value so multiple
// sources can share a monotonic counter.
func readEntries(r io.Reader, label string, seq uint64) ([]entry, uint64) {
	out := make([]entry, 0, 64)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		ts, body, ok := parseLine(line)
		if !ok {
			if len(out) > 0 {
				out[len(out)-1].body += "\n" + line
				continue
			}
			out = append(out, entry{label: label, body: line, seq: seq})
			seq++
			continue
		}
		out = append(out, entry{ts: ts, label: label, body: body, hasTS: true, seq: seq})
		seq++
	}
	return out, seq
}

// readLastNEntries seeks near the end of f and reads at most n entries.
// The seek window grows if too few entries are recovered (e.g. very
// long lines), bounded so we never scan more than the whole file.
func readLastNEntries(f *os.File, label string, n int, seq uint64) ([]entry, uint64, error) {
	stat, err := f.Stat()
	if err != nil {
		return nil, seq, err
	}
	size := stat.Size()
	if size == 0 {
		return nil, seq, nil
	}

	guess := int64(n) * 200
	for attempt := 0; attempt < 4; attempt++ {
		if guess > size {
			guess = size
		}
		if _, err := f.Seek(size-guess, io.SeekStart); err != nil {
			return nil, seq, err
		}
		var r io.Reader = f
		if guess < size {
			br := bufio.NewReader(f)
			if _, err := br.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
				return nil, seq, err
			}
			r = br
		}
		entries, nextSeq := readEntries(r, label, seq)
		if len(entries) >= n || guess >= size {
			if len(entries) > n {
				entries = entries[len(entries)-n:]
			}
			return entries, nextSeq, nil
		}
		guess *= 4
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, seq, err
	}
	entries, nextSeq := readEntries(f, label, seq)
	if len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries, nextSeq, nil
}

// mergeByTS performs a stable k-way merge by (ts, seq). Each input
// slice must already be in source-order (which is also chronological:
// log files are append-only).
func mergeByTS(sources ...[]entry) []entry {
	total := 0
	for _, s := range sources {
		total += len(s)
	}
	out := make([]entry, 0, total)

	idx := make([]int, len(sources))
	for {
		bestSrc := -1
		for i, s := range sources {
			if idx[i] >= len(s) {
				continue
			}
			if bestSrc == -1 {
				bestSrc = i
				continue
			}
			a := s[idx[i]]
			b := sources[bestSrc][idx[bestSrc]]
			if a.ts.Before(b.ts) || (a.ts.Equal(b.ts) && a.seq < b.seq) {
				bestSrc = i
			}
		}
		if bestSrc == -1 {
			break
		}
		out = append(out, sources[bestSrc][idx[bestSrc]])
		idx[bestSrc]++
	}
	return out
}

// streamMerge reads entries from each source via streaming iterators
// and writes them ordered to w. RAM stays O(num sources): one peeked
// entry per stream.
func streamMerge(ctx context.Context, w io.Writer, fs filter, sources ...streamSource) error {
	iters := make([]*entryIterator, 0, len(sources))
	defer func() {
		for _, it := range iters {
			it.close()
		}
	}()
	for _, s := range sources {
		it, err := newEntryIterator(s.path, s.label, s.seqBase)
		if err != nil {
			if os.IsNotExist(err) {
				_, _ = term.Printf("%s File not found\n", colorLabel(s.label))
				continue
			}
			return err
		}
		iters = append(iters, it)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		bestIdx := -1
		var best entry
		for i, it := range iters {
			e, ok := it.peek()
			if !ok {
				continue
			}
			if bestIdx == -1 {
				bestIdx = i
				best = e
				continue
			}
			if e.ts.Before(best.ts) || (e.ts.Equal(best.ts) && e.seq < best.seq) {
				bestIdx = i
				best = e
			}
		}
		if bestIdx == -1 {
			return nil
		}
		iters[bestIdx].advance()
		if !fs.keep(best) {
			continue
		}
		if _, err := fmt.Fprintln(w, formatEntry(best)); err != nil {
			return err
		}
	}
}

// entryIterator walks a file one entry at a time without loading more
// than one entry plus one held-back header line into memory.
type entryIterator struct {
	f       *os.File
	br      *bufio.Reader
	label   string
	seq     uint64
	current entry
	hasCur  bool
	// nextHdr holds an already-parsed header line that belongs to the
	// next entry. We read one line ahead to know when continuation
	// lines for the current entry have ended.
	nextTS   time.Time
	nextBody string
	hasNext  bool
	eof      bool
	primed   bool
}

func newEntryIterator(path, label string, seqBase uint64) (*entryIterator, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	it := &entryIterator{
		f:     f,
		br:    bufio.NewReaderSize(f, 64*1024),
		label: label,
		seq:   seqBase,
	}
	it.advance()
	return it, nil
}

func (it *entryIterator) close() {
	if it.f != nil {
		_ = it.f.Close()
		it.f = nil
	}
}

func (it *entryIterator) peek() (entry, bool) {
	if !it.hasCur {
		return entry{}, false
	}
	return it.current, true
}

// advance produces the next complete entry by reading lines until it
// hits the following header (or EOF). The trailing header is buffered
// for the next call.
func (it *entryIterator) advance() {
	if !it.primed {
		it.primed = true
		it.readHeader()
	}
	if !it.hasNext && it.eof {
		it.hasCur = false
		return
	}
	cur := entry{ts: it.nextTS, label: it.label, body: it.nextBody, hasTS: true, seq: it.seq}
	it.seq++
	it.hasNext = false

	for !it.eof {
		line, err := it.br.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\n")
			ts, body, ok := parseLine(line)
			if ok {
				it.nextTS = ts
				it.nextBody = body
				it.hasNext = true
				if err != nil {
					it.eof = true
				}
				it.current = cur
				it.hasCur = true
				return
			}
			cur.body += "\n" + line
		}
		if err != nil {
			it.eof = true
			break
		}
	}
	it.current = cur
	it.hasCur = true
}

// readHeader scans until it finds the first ts-bearing line, dropping
// any header-less prefix. Sets hasNext / nextTS / nextBody on success.
func (it *entryIterator) readHeader() {
	for !it.eof {
		line, err := it.br.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\n")
			if ts, body, ok := parseLine(line); ok {
				it.nextTS = ts
				it.nextBody = body
				it.hasNext = true
				if err != nil {
					it.eof = true
				}
				return
			}
		}
		if err != nil {
			it.eof = true
			return
		}
	}
}

// formatEntry renders an entry for terminal output. Multi-line bodies
// are emitted as-is so stack traces stay readable.
func formatEntry(e entry) string {
	ts := e.ts.Format(tsLayout)
	if !e.hasTS {
		ts = strings.Repeat(" ", tsLen)
	}
	return fmt.Sprintf("%s %s %s", colorLabel(e.label), term.DimString("%s", ts), e.body)
}

// formatEntries emits a captured slice through w, applying fs.
func formatEntries(w io.Writer, entries []entry, fs filter) error {
	for _, e := range entries {
		if !fs.keep(e) {
			continue
		}
		if _, err := fmt.Fprintln(w, formatEntry(e)); err != nil {
			return err
		}
	}
	return nil
}

// boundedTail reads the last n entries from each path, merges them by
// timestamp, and trims the merged result back to n.
func boundedTail(w io.Writer, sources []streamSource, n int, fs filter) error {
	all := make([][]entry, 0, len(sources))
	var seq uint64
	for _, s := range sources {
		f, err := os.Open(s.path)
		if err != nil {
			if os.IsNotExist(err) {
				_, _ = term.Printf("%s File not found\n", colorLabel(s.label))
				continue
			}
			return err
		}
		entries, nextSeq, err := readLastNEntries(f, s.label, n, seq)
		_ = f.Close()
		if err != nil {
			return err
		}
		seq = nextSeq
		all = append(all, entries)
	}
	merged := mergeByTS(all...)
	if len(merged) > n {
		merged = merged[len(merged)-n:]
	}
	return formatEntries(w, merged, fs)
}

// followMessage carries either an entry or an error from a per-source
// follow goroutine to the merger.
type followMessage struct {
	e   entry
	err error
}

// followMerge tails every source in parallel, feeds new entries into a
// small-window heap, and flushes entries older than `now - flushDelay`
// so out-of-order arrivals get re-sorted by their write-time ts.
func followMerge(ctx context.Context, w io.Writer, sources []streamSource, fs filter, sleep Sleeper) error {
	const flushDelay = 200 * time.Millisecond

	ch := make(chan followMessage, 64)
	for _, s := range sources {
		go tailFollow(ctx, s, ch, sleep)
	}

	pq := &entryHeap{}
	heap.Init(pq)

	ticker := time.NewTicker(flushDelay / 2)
	defer ticker.Stop()

	flush := func(force bool) error {
		cutoff := time.Now().Add(-flushDelay)
		for pq.Len() > 0 {
			top := (*pq)[0]
			if !force && top.ts.After(cutoff) {
				return nil
			}
			heap.Pop(pq)
			if !fs.keep(top) {
				continue
			}
			if _, err := fmt.Fprintln(w, formatEntry(top)); err != nil {
				return err
			}
		}
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return flush(true)
		case msg := <-ch:
			if msg.err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "%s\n", term.RedString("follow error: %v", msg.err))
				continue
			}
			heap.Push(pq, msg.e)
		case <-ticker.C:
			if err := flush(false); err != nil {
				return err
			}
		}
	}
}

// tailFollow opens path, seeks to end, and pushes each new entry to ch.
// Continuation lines fold into the prior entry held briefly per stream.
func tailFollow(ctx context.Context, s streamSource, ch chan<- followMessage, sleep Sleeper) {
	f, err := waitOpen(ctx, s.path, sleep)
	if err != nil {
		ch <- followMessage{err: err}
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		ch <- followMessage{err: err}
		return
	}
	br := bufio.NewReader(f)
	var pending entry
	var hasPending bool
	flush := func() {
		if hasPending {
			ch <- followMessage{e: pending}
			pending = entry{}
			hasPending = false
		}
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		default:
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\n")
			ts, body, ok := parseLine(line)
			if ok {
				flush()
				pending = entry{ts: ts, label: s.label, body: body, hasTS: true}
				hasPending = true
			} else if hasPending {
				pending.body += "\n" + line
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				flush()
				sleep(150 * time.Millisecond)
				continue
			}
			ch <- followMessage{err: err}
			return
		}
	}
}

// waitOpen blocks until path exists (or ctx cancels). Used by follow
// mode where the producer may not yet have created the log file.
func waitOpen(ctx context.Context, path string, sleep Sleeper) (*os.File, error) {
	for {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		sleep(500 * time.Millisecond)
	}
}

// entryHeap is a min-heap on (ts, seq).
type entryHeap []entry

func (h *entryHeap) Len() int { return len(*h) }
func (h *entryHeap) Less(i, j int) bool {
	s := *h
	if s[i].ts.Equal(s[j].ts) {
		return s[i].seq < s[j].seq
	}
	return s[i].ts.Before(s[j].ts)
}
func (h *entryHeap) Swap(i, j int) { s := *h; s[i], s[j] = s[j], s[i] }
func (h *entryHeap) Push(x any) {
	e, ok := x.(entry)
	if !ok {
		return
	}
	*h = append(*h, e)
}
func (h *entryHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}
