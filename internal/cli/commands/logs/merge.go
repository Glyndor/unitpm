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

// isBannerRule reports whether a line is the top/bottom rule of a
// lifecycle banner — a non-empty run of '=' chars. The daemon uses an
// 80-char run; we accept any length ≥8 to stay robust against future
// width changes.
func isBannerRule(line string) bool {
	if len(line) < 8 {
		return false
	}
	for i := 0; i < len(line); i++ {
		if line[i] != '=' {
			return false
		}
	}
	return true
}

// parseBannerMiddle decodes the middle line of a banner, written as
// `==  EVENT  ==...==  YYYY-MM-DD HH:MM:SS  ==`. The trailing 4 chars
// are always "  ==" and the 19 chars before that are the timestamp.
// Returns ts and ok=true on match.
func parseBannerMiddle(line string) (time.Time, bool) {
	const tail = "  =="
	if !strings.HasSuffix(line, tail) {
		return time.Time{}, false
	}
	if len(line) < len(tail)+tsLen {
		return time.Time{}, false
	}
	inner := line[:len(line)-len(tail)]
	tsStr := inner[len(inner)-tsLen:]
	t, err := time.ParseInLocation(tsLayout, tsStr, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// tryConsumeBanner inspects three consecutive lines and, if they form
// a lifecycle banner, returns the synthesized entry. ok=false leaves
// the caller to fall through to the regular ts-line path.
func tryConsumeBanner(rule1, mid, rule2, label string, seq uint64) (entry, bool) {
	if !isBannerRule(rule1) || !isBannerRule(rule2) {
		return entry{}, false
	}
	ts, ok := parseBannerMiddle(mid)
	if !ok {
		return entry{}, false
	}
	body := rule1 + "\n" + mid + "\n" + rule2
	return entry{ts: ts, label: label, body: body, hasTS: true, seq: seq}, true
}

// readEntries reads ALL entries from r in order. Continuation lines
// fold into the prior entry. Lifecycle banners (3-line === / middle /
// === blocks written by writeBanner) are recognized as standalone
// entries with the timestamp embedded in the middle line. Returns the
// next seq value so multiple sources can share a monotonic counter.
func readEntries(r io.Reader, label string, seq uint64) ([]entry, uint64) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lines := make([]string, 0, 128)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}

	out := make([]entry, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		// Banner block: 3 consecutive lines. Look ahead before treating
		// the rule as continuation, because banners are written by the
		// daemon as a single logical event and need their own ts anchor.
		if i+2 < len(lines) && isBannerRule(lines[i]) {
			if e, ok := tryConsumeBanner(lines[i], lines[i+1], lines[i+2], label, seq); ok {
				out = append(out, e)
				seq++
				i += 2 // loop will i++ once more
				continue
			}
		}
		ts, body, ok := parseLine(lines[i])
		if !ok {
			if len(out) > 0 {
				out[len(out)-1].body += "\n" + lines[i]
				continue
			}
			out = append(out, entry{label: label, body: lines[i], seq: seq})
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
// than three raw lines into memory. The three-slot look-ahead lets us
// detect lifecycle banners (rule / middle / rule) before deciding how
// to fold continuation lines.
type entryIterator struct {
	f       *os.File
	br      *bufio.Reader
	label   string
	seq     uint64
	current entry
	hasCur  bool
	// look holds up to 3 unprocessed lines pulled from br. advance()
	// peeks here before deciding whether to emit a banner block, a ts
	// entry with folded continuations, or to drop a stray header-less
	// line at the file head.
	look []string
	eof  bool
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

// refill pulls lines from br until len(look) >= want or EOF. Trailing
// '\n' is stripped so callers compare against full-line content.
func (it *entryIterator) refill(want int) {
	for !it.eof && len(it.look) < want {
		line, err := it.br.ReadString('\n')
		if len(line) > 0 {
			it.look = append(it.look, strings.TrimRight(line, "\n"))
		}
		if err != nil {
			it.eof = true
			return
		}
	}
}

// looksLikeBannerStart returns true when the next 3 lookahead lines
// form a complete lifecycle banner. Caller must have already filled
// it.look to at least 3 entries (or hit EOF).
func (it *entryIterator) looksLikeBannerStart() bool {
	if len(it.look) < 3 {
		return false
	}
	if !isBannerRule(it.look[0]) || !isBannerRule(it.look[2]) {
		return false
	}
	_, ok := parseBannerMiddle(it.look[1])
	return ok
}

// advance produces the next complete entry. Algorithm:
//  1. Refill 1 line; if none, mark done.
//  2. If line[0] is a rule, refill to 3 and try banner. Hit → emit
//     banner entry; miss → fall through (treat as continuation/stray).
//  3. If line[0] parses as a ts header, consume it plus all following
//     non-header non-banner-start lines as continuation body, then emit.
//  4. Otherwise drop the stray line and loop.
func (it *entryIterator) advance() {
	for {
		it.refill(1)
		if len(it.look) == 0 {
			it.hasCur = false
			return
		}
		if isBannerRule(it.look[0]) {
			it.refill(3)
			if it.looksLikeBannerStart() {
				e, _ := tryConsumeBanner(it.look[0], it.look[1], it.look[2], it.label, it.seq)
				it.seq++
				it.look = it.look[3:]
				it.current = e
				it.hasCur = true
				return
			}
		}
		ts, body, ok := parseLine(it.look[0])
		if !ok {
			// Stray header-less line at file head (or after a malformed
			// banner). Drop it: there is no prior entry to fold into,
			// and surfacing it as an epoch-zero entry would corrupt
			// merge ordering across streams.
			it.look = it.look[1:]
			continue
		}
		cur := entry{ts: ts, label: it.label, body: body, hasTS: true, seq: it.seq}
		it.seq++
		it.look = it.look[1:]
		// Fold continuations until the next ts header or banner start.
		for {
			it.refill(1)
			if len(it.look) == 0 {
				break
			}
			if isBannerRule(it.look[0]) {
				it.refill(3)
				if it.looksLikeBannerStart() {
					break
				}
			}
			if _, _, okts := parseLine(it.look[0]); okts {
				break
			}
			cur.body += "\n" + it.look[0]
			it.look = it.look[1:]
		}
		it.current = cur
		it.hasCur = true
		return
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
// Continuation lines fold into the prior entry; lifecycle banner blocks
// (3 consecutive lines: rule / middle / rule) are emitted as a single
// entry with the timestamp embedded in the middle line.
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
	// bannerBuf holds an in-progress banner block (the leading rule and
	// the middle line) until the closing rule arrives. Because each
	// banner.Write hits the file as one write but tail loops poll
	// post-EOF, the three lines almost always arrive in the same poll
	// cycle. We still defend against partial reads by keeping the
	// pending state across iterations.
	var bannerBuf []string

	flush := func() {
		if hasPending {
			ch <- followMessage{e: pending}
			pending = entry{}
			hasPending = false
		}
	}
	flushBannerAsContinuation := func() {
		// A banner that never completed its closing rule. Treat the
		// captured lines as continuation of the prior entry so they
		// at least appear in the output rather than vanish.
		if len(bannerBuf) == 0 {
			return
		}
		if hasPending {
			pending.body += "\n" + strings.Join(bannerBuf, "\n")
		}
		bannerBuf = nil
	}
	for {
		select {
		case <-ctx.Done():
			flushBannerAsContinuation()
			flush()
			return
		default:
		}
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\n")
			switch {
			case len(bannerBuf) == 1:
				// Expect the middle line.
				if _, ok := parseBannerMiddle(line); ok {
					bannerBuf = append(bannerBuf, line)
				} else {
					flushBannerAsContinuation()
					handleFollowLine(line, s.label, &pending, &hasPending, &bannerBuf, flush)
				}
			case len(bannerBuf) == 2:
				// Expect the closing rule.
				if isBannerRule(line) {
					bannerBuf = append(bannerBuf, line)
					if mid, ok := parseBannerMiddle(bannerBuf[1]); ok {
						flush()
						body := bannerBuf[0] + "\n" + bannerBuf[1] + "\n" + bannerBuf[2]
						ch <- followMessage{e: entry{ts: mid, label: s.label, body: body, hasTS: true}}
					}
					bannerBuf = nil
				} else {
					flushBannerAsContinuation()
					handleFollowLine(line, s.label, &pending, &hasPending, &bannerBuf, flush)
				}
			default:
				handleFollowLine(line, s.label, &pending, &hasPending, &bannerBuf, flush)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Don't flush incomplete banner here — the closing rule
				// is likely en route in the next poll iteration.
				flush()
				sleep(150 * time.Millisecond)
				continue
			}
			ch <- followMessage{err: err}
			return
		}
	}
}

// handleFollowLine routes a non-banner line for the follow path:
// either start a new pending entry (ts header), open a banner block
// (rule), or fold continuation into the current pending entry.
func handleFollowLine(line, label string, pending *entry, hasPending *bool, bannerBuf *[]string, flush func()) {
	if isBannerRule(line) {
		*bannerBuf = append(*bannerBuf, line)
		return
	}
	if ts, body, ok := parseLine(line); ok {
		flush()
		*pending = entry{ts: ts, label: label, body: body, hasTS: true}
		*hasPending = true
		return
	}
	if *hasPending {
		pending.body += "\n" + line
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
