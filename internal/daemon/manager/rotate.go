package manager

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/Jaro-c/Lynx/internal/env"
)

const (
	defaultRotateMaxBytes int64         = 50 * 1024 * 1024 // 50 MiB
	defaultRotateKeep                   = 12               // matches debian/lynxpm.logrotate `rotate 12`
	defaultRotateMaxAge   time.Duration = 7 * 24 * time.Hour
	defaultDelayCompress                = true
	defaultNotifEmpty                   = true
)

type rotateConfig struct {
	maxBytes      int64
	keep          int
	maxAge        time.Duration
	delayCompress bool
	notifEmpty    bool
}

func currentRotateConfig() rotateConfig {
	hours := env.Int("LYNX_LOG_MAX_AGE_HOURS", int(defaultRotateMaxAge/time.Hour))
	return rotateConfig{
		maxBytes:      env.Int64("LYNX_LOG_MAX_BYTES", defaultRotateMaxBytes),
		keep:          env.Int("LYNX_LOG_KEEP", defaultRotateKeep),
		maxAge:        time.Duration(hours) * time.Hour,
		delayCompress: defaultDelayCompress,
		notifEmpty:    defaultNotifEmpty,
	}
}

// rotateIfLarge is the size-only entry point used by setupLogs at Start
// time. The age trigger requires a per-writer baseline that does not
// exist before the writer is constructed, so we pass the zero time and
// rely on rotateNowCfg to skip the age check.
func rotateIfLarge(path string) {
	rotateNowCfg(path, currentRotateConfig(), time.Time{})
}

// rotateIfLargeCfg keeps the original signature for unit tests that want
// to pin a specific rotateConfig (small thresholds, custom keep counts).
// Returns whether rotation actually happened.
func rotateIfLargeCfg(path string, cfg rotateConfig) bool {
	return rotateNowCfg(path, cfg, time.Time{})
}

// rotateNowCfg is the canonical rotation entry point. Both size and age
// triggers are evaluated; either one is sufficient. lastRotateAt is the
// caller's own anchor for age — pass time.Time{} to disable the age
// check entirely (e.g. at process start when no anchor exists yet).
func rotateNowCfg(path string, cfg rotateConfig, lastRotateAt time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if cfg.notifEmpty && info.Size() == 0 {
		return false
	}

	bySize := cfg.maxBytes > 0 && info.Size() >= cfg.maxBytes
	byAge := cfg.maxAge > 0 && !lastRotateAt.IsZero() && time.Since(lastRotateAt) >= cfg.maxAge
	if !bySize && !byAge {
		return false
	}

	if cfg.delayCompress {
		rotateWithDelayCompressCfg(path, cfg)
	} else {
		rotateImmediateCfg(path, cfg)
	}
	return true
}

// rotateImmediateCfg is the immediate-compress scheme: current log →
// .1.gz, .1.gz → .2.gz, etc. Kept for unit tests and as the fallback
// path when delayCompress is off. copytruncate-safe.
func rotateImmediateCfg(path string, cfg rotateConfig) {
	rotateChain(path, cfg, false)
}

// rotateWithDelayCompressCfg matches logrotate's `delaycompress`: the
// most recent rotation is left uncompressed at .1, and only on the next
// rotation is it compressed into the .gz chain. Useful when readers
// want a plain-text view of the last cycle without zcat.
func rotateWithDelayCompressCfg(path string, cfg rotateConfig) {
	rotateChain(path, cfg, true)
}

// rotateChain implements both rotation schemes. With delayCompress=false
// the .gz chain starts at index 1 and the live log is compressed on
// every rotation; with delayCompress=true the chain starts at index 2
// and a plain .1 holds the most recent rotated copy until the next
// cycle. Both branches end with a copytruncate of the live file so the
// daemon's open fd keeps writing to the same inode.
func rotateChain(path string, cfg rotateConfig, delayCompress bool) {
	keep := cfg.keep
	if keep < 1 {
		keep = 1
	}

	// Drop the oldest compressed archive.
	oldest := fmt.Sprintf("%s.%d.gz", path, keep)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		log.Printf("log-rotate: remove %s: %v", oldest, err)
	}

	// Shift the compressed chain up by one. Immediate mode shifts down to
	// .1.gz; delayCompress stops at .2.gz because .1 is plain.
	startIdx := 1
	if delayCompress {
		startIdx = 2
	}
	for i := keep - 1; i >= startIdx; i-- {
		src := fmt.Sprintf("%s.%d.gz", path, i)
		dst := fmt.Sprintf("%s.%d.gz", path, i+1)
		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			log.Printf("log-rotate: rename %s → %s: %v", src, dst, err)
		}
	}

	if delayCompress {
		// The previous-cycle plain .1 becomes the new .2.gz. compressFile
		// reads the source then writes a fresh .gz; remove the plain copy
		// only after compression succeeds so a failure leaves .1 intact.
		plain1 := path + ".1"
		if _, err := os.Stat(plain1); err == nil {
			if err := compressFile(plain1, path+".2.gz"); err != nil {
				log.Printf("log-rotate: compress %s: %v", plain1, err)
				return
			}
			if err := os.Remove(plain1); err != nil && !os.IsNotExist(err) {
				log.Printf("log-rotate: remove %s: %v", plain1, err)
			}
		}

		// Copy current → .1 (plain), then truncate current.
		if err := copyFile(path, plain1); err != nil {
			log.Printf("log-rotate: copy %s → %s: %v", path, plain1, err)
			return
		}
	} else {
		// Immediate compress: current → .1.gz.
		if err := compressFile(path, path+".1.gz"); err != nil {
			log.Printf("log-rotate: compress %s: %v", path, err)
			return
		}
	}

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

func copyFile(src, dst string) error {
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

	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
