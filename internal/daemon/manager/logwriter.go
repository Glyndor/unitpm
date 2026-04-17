package manager

import (
	"bytes"
	"io"
	"sync"
	"time"
)

// timestampWriter wraps an io.Writer and prefixes each line with a timestamp.
type timestampWriter struct {
	mu  sync.Mutex
	w   io.Writer
	buf []byte // incomplete line buffer
}

func newTimestampWriter(w io.Writer) *timestampWriter {
	return &timestampWriter{w: w}
}

func (tw *timestampWriter) Write(p []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	total := len(p)
	data := append(tw.buf, p...)
	tw.buf = nil

	for {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			tw.buf = data
			break
		}

		line := data[:idx+1]
		ts := time.Now().Format("2006-01-02 15:04:05")
		if _, err := io.WriteString(tw.w, ts+" "); err != nil {
			return total, err
		}
		if _, err := tw.w.Write(line); err != nil {
			return total, err
		}
		data = data[idx+1:]
	}

	return total, nil
}
