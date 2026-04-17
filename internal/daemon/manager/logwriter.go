package manager

import (
	"bytes"
	"sync"
	"time"
)

// timestampWriter wraps an io.Writer and prefixes each line with a timestamp.
type timestampWriter struct {
	mu  sync.Mutex
	w   interface{ Write([]byte) (int, error) }
	buf []byte
	out bytes.Buffer
}

func newTimestampWriter(w interface{ Write([]byte) (int, error) }) *timestampWriter {
	return &timestampWriter{w: w}
}

const maxLogBuf = 1 << 20 // 1 MB

func (tw *timestampWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

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
