package manager

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/Jaro-c/Lynx/internal/env"
)

const (
	defaultRotateMaxBytes int64 = 50 * 1024 * 1024 // 50 MiB
	defaultRotateKeep           = 3
)

type rotateConfig struct {
	maxBytes int64
	keep     int
}

func currentRotateConfig() rotateConfig {
	return rotateConfig{
		maxBytes: env.Int64("LYNX_LOG_MAX_BYTES", defaultRotateMaxBytes),
		keep:     env.Int("LYNX_LOG_KEEP", defaultRotateKeep),
	}
}

func rotateIfLarge(path string) {
	rotateIfLargeCfg(path, currentRotateConfig())
}

func rotateIfLargeCfg(path string, cfg rotateConfig) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < cfg.maxBytes {
		return
	}

	// Delete oldest backup.
	oldest := fmt.Sprintf("%s.%d.gz", path, cfg.keep)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		log.Printf("log-rotate: remove %s: %v", oldest, err)
	}

	// Shift: foo.log.(N-1).gz -> foo.log.N.gz
	for i := cfg.keep - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d.gz", path, i)
		dst := fmt.Sprintf("%s.%d.gz", path, i+1)
		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			log.Printf("log-rotate: rename %s → %s: %v", src, dst, err)
		}
	}

	// Current -> foo.log.1.gz (compress)
	if err := compressFile(path, path+".1.gz"); err != nil {
		log.Printf("log-rotate: compress %s: %v", path, err)
		return
	}

	// Truncate original so the open file handle keeps working.
	if err := os.Truncate(path, 0); err != nil {
		log.Printf("log-rotate: truncate %s: %v", path, err)
	}
}

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	gz, err := gzip.NewWriterLevel(out, gzip.BestSpeed)
	if err != nil {
		_ = os.Remove(dst)
		return err
	}
	if _, err := io.Copy(gz, in); err != nil {
		_ = gz.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := gz.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
