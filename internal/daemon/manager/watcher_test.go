package manager

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(file, []byte("initial"), 0600); err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32
	w := newFileWatcher(dir, nil, func() { called.Add(1) })
	w.interval = 100 * time.Millisecond
	w.Start()
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(file, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}

	time.Sleep(250 * time.Millisecond)
	if called.Load() == 0 {
		t.Error("expected onChange to fire after file change")
	}
}

func TestFileWatcher_NoChangeNoFire(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stable.txt"), []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32
	w := newFileWatcher(dir, nil, func() { called.Add(1) })
	w.interval = 100 * time.Millisecond
	w.Start()
	defer w.Stop()

	time.Sleep(350 * time.Millisecond)
	if called.Load() != 0 {
		t.Error("expected no onChange without file change")
	}
}

func TestFileWatcher_IgnorePattern(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "ignored")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "data.txt"), []byte("init"), 0600); err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32
	w := newFileWatcher(dir, []string{"ignored"}, func() { called.Add(1) })
	w.interval = 100 * time.Millisecond
	w.Start()
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(sub, "data.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}

	time.Sleep(250 * time.Millisecond)
	if called.Load() != 0 {
		t.Error("expected no onChange for ignored directory")
	}
}

func TestFileWatcher_IgnoreGlob(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.log"), []byte("init"), 0600); err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32
	w := newFileWatcher(dir, []string{"*.log"}, func() { called.Add(1) })
	w.interval = 100 * time.Millisecond
	w.Start()
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "app.log"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}

	time.Sleep(250 * time.Millisecond)
	if called.Load() != 0 {
		t.Error("expected no onChange for ignored glob pattern")
	}
}

func TestFileWatcher_DoubleStartNoLeak(t *testing.T) {
	dir := t.TempDir()
	w := newFileWatcher(dir, nil, func() {})
	w.interval = 100 * time.Millisecond

	w.Start()
	w.Start() // should be no-op
	w.Stop()
}

func TestFileWatcher_StopBeforeStart(t *testing.T) {
	dir := t.TempDir()
	w := newFileWatcher(dir, nil, func() {})
	w.Stop() // should not panic
}

func TestMatchIgnore_PathTraversal(t *testing.T) {
	if matchIgnore("test", "test", "../secret") {
		t.Error("should reject path traversal pattern")
	}
	if matchIgnore("test", "test", "/etc/passwd") {
		t.Error("should reject absolute path pattern")
	}
}

func TestMatchIgnore_ExactAndGlob(t *testing.T) {
	if !matchIgnore("node_modules", "node_modules", "node_modules") {
		t.Error("should match exact name")
	}
	if !matchIgnore("app.log", "app.log", "*.log") {
		t.Error("should match glob pattern")
	}
	if matchIgnore("app.txt", "app.txt", "*.log") {
		t.Error("should not match different extension")
	}
}
