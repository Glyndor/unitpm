package manager

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"
)

// timestampWriter wraps an io.Writer and prefixes each line with a timestamp.
type timestampWriter struct {
	mu  sync.Mutex
	w   interface{ Write([]byte) (int, error) }
	buf []byte
	out bytes.Buffer

	// Rotation state. path == "" disables in-writer rotation entirely
	// (used by unit tests that wrap a bytes.Buffer). When set, every
	// writeRotateBytesEvery bytes that flow through the writer trigger a
	// best-effort size check via maybeRotate. lastRotateAt anchors the
	// age-based trigger; logrotate-style "weekly" semantics need a
	// per-stream baseline because file mtime gets refreshed by every
	// write and would never cross the age threshold for an active log.
	rotateMu        sync.Mutex
	path            string
	bytesSinceCheck int64
	lastRotateAt    time.Time
	// rotateCfg is captured once at construction so each Write/tick does
	// not re-read four env vars and rebuild the struct. Live env-var
	// changes won't take effect until the writer is recreated (e.g. on
	// process restart) — acceptable for daemon-lifetime config.
	rotateCfg rotateConfig
}

// writeRotateBytesEvery bounds how often the writer pays for a stat() to
// decide whether the file has crossed the rotation threshold. 4 MiB keeps
// the per-write overhead negligible while ensuring we react to a 50 MiB
// breach within at most one extra check window.
const writeRotateBytesEvery int64 = 4 * 1024 * 1024

func newTimestampWriter(w interface{ Write([]byte) (int, error) }) *timestampWriter {
	return &timestampWriter{w: w}
}

// newRotatingTimestampWriter wraps w with a path so the writer can rotate
// the underlying file on its own. Only used by setupLogs; tests use the
// non-rotating constructor. lastRotateAt is seeded to time.Now so the
// age trigger only fires after maxAge elapsed since the writer opened
// (i.e. since the daemon started writing this stream).
func newRotatingTimestampWriter(w interface{ Write([]byte) (int, error) }, path string) *timestampWriter {
	return &timestampWriter{
		w:            w,
		path:         path,
		lastRotateAt: time.Now(),
		rotateCfg:    currentRotateConfig(),
	}
}

const maxLogBuf = 1 << 20 // 1 MB

// writeLocked is the original Write body. Caller must hold tw.mu.
func (tw *timestampWriter) writeLocked(p []byte) (int, error) {
	total := len(p)
	tw.buf = append(tw.buf, p...)

	ts := time.Now().Format("2006-01-02 15:04:05 ")
	tw.out.Reset()

	for {
		idx := bytes.IndexByte(tw.buf, '\n')
		if idx < 0 {
			if len(tw.buf) > maxLogBuf {
				tw.out.WriteString(ts)
				tw.out.Write(tw.buf)
				tw.out.WriteByte('\n')
				tw.buf = tw.buf[:0]
			}
			break
		}
		tw.out.WriteString(ts)
		tw.out.Write(tw.buf[:idx+1])
		tw.buf = tw.buf[idx+1:]
	}

	if tw.out.Len() > 0 {
		if _, err := tw.w.Write(tw.out.Bytes()); err != nil {
			return total, err
		}
	}

	return total, nil
}

func (tw *timestampWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	n, err := tw.writeLocked(p)

	shouldRotate := false
	if err == nil && tw.path != "" {
		tw.bytesSinceCheck += int64(n)
		if tw.bytesSinceCheck >= writeRotateBytesEvery {
			tw.bytesSinceCheck = 0
			shouldRotate = true
		}
	}
	tw.mu.Unlock()

	// Drop tw.mu before rotating so a 50 MiB compress doesn't stall further
	// writes for the duration of the rotation. rotateMu serializes against
	// the periodic ticker (and any other rotation triggered on this path).
	if shouldRotate {
		tw.maybeRotate()
	}
	return n, err
}

// maybeRotate runs rotation under TryLock so a rotation already in
// flight (from the periodic ticker or another goroutine) is left alone
// rather than queued — duplicate work would just produce a no-op stat.
// On a successful rotation we advance lastRotateAt so the age trigger
// resets cleanly.
func (tw *timestampWriter) maybeRotate() {
	if tw == nil || tw.path == "" {
		return
	}
	if !tw.rotateMu.TryLock() {
		return
	}
	defer tw.rotateMu.Unlock()
	if rotateNowCfg(tw.path, tw.rotateCfg, tw.lastRotateAt) {
		tw.lastRotateAt = time.Now()
	}
}

// bannerWidth is the fixed column width of the lifecycle banner block.
const bannerWidth = 80

// writeBanner writes a 3-line lifecycle marker (===/middle/===) to w.
// The middle line carries `event` on the left and the current timestamp on
// the right, padded with `=` to bannerWidth. Bypasses timestampWriter so
// the banner is not double-prefixed when the underlying file is wrapped.
func writeBanner(w io.Writer, event, detail string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	sep := strings.Repeat("=", bannerWidth)

	left := "==  " + event
	if detail != "" {
		left += "  " + detail
	}
	left += "  "
	right := "  " + ts + "  =="

	fillN := bannerWidth - len(left) - len(right)
	if fillN < 4 {
		fillN = 4
	}
	mid := left + strings.Repeat("=", fillN) + right

	var b bytes.Buffer
	b.Grow(len(sep)*2 + len(mid) + 3)
	b.WriteString(sep)
	b.WriteByte('\n')
	b.WriteString(mid)
	b.WriteByte('\n')
	b.WriteString(sep)
	b.WriteByte('\n')

	_, _ = w.Write(b.Bytes())
}
