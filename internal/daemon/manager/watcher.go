package manager

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultWatchInterval = 2 * time.Second
	maxWatchFiles        = 50000
)

// fileWatcher polls the filesystem for changes and triggers a callback.
type fileWatcher struct {
	root     string
	ignore   []string
	interval time.Duration
	onChange func()

	mu       sync.Mutex
	cancel   context.CancelFunc
	running  bool
	snapshot map[string]fileEntry
}

type fileEntry struct {
	modTime time.Time
	size    int64
}

func newFileWatcher(root string, ignore []string, onChange func()) *fileWatcher {
	return &fileWatcher{
		root:     root,
		ignore:   ignore,
		interval: defaultWatchInterval,
		onChange: onChange,
	}
}

func (w *fileWatcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.snapshot = w.scan()
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.mu.Unlock()

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := w.scan()
				w.mu.Lock()
				changed := w.diff(w.snapshot, current)
				if changed {
					w.snapshot = current
				}
				w.mu.Unlock()
				if changed {
					log.Printf("watch: change detected in %s, restarting", w.root)
					w.onChange()
				}
			}
		}
	}()
}

func (w *fileWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.running = false
	w.snapshot = nil
}

func (w *fileWatcher) scan() map[string]fileEntry {
	entries := make(map[string]fileEntry)
	count := 0
	_ = filepath.WalkDir(w.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if count >= maxWatchFiles {
			return filepath.SkipAll
		}

		rel, _ := filepath.Rel(w.root, path)
		name := d.Name()

		if d.IsDir() {
			if d.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			for _, pattern := range w.ignore {
				if matchIgnore(name, rel, pattern) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Skip symlink files
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		for _, pattern := range w.ignore {
			if matchIgnore(name, rel, pattern) {
				return nil
			}
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		entries[rel] = fileEntry{modTime: info.ModTime(), size: info.Size()}
		count++
		return nil
	})
	return entries
}

func (w *fileWatcher) diff(old, cur map[string]fileEntry) bool {
	if len(old) != len(cur) {
		return true
	}
	for path, ce := range cur {
		oe, ok := old[path]
		if !ok || oe.modTime != ce.modTime || oe.size != ce.size {
			return true
		}
	}
	return false
}

func matchIgnore(name, rel, pattern string) bool {
	if strings.Contains(pattern, "..") || filepath.IsAbs(pattern) {
		return false
	}
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(name, pattern[1:])
	}
	if name == pattern {
		return true
	}
	matched, _ := filepath.Match(pattern, rel)
	return matched
}
