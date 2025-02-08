package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var (
	LynxHome     string
	Lynx_LogsDir string
	Lynx_PidsDir string
)

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		Error_Fatal(fmt.Errorf("unable to detect user home directory"))
	}

	// Set LynxHome based on the operating system
	if runtime.GOOS == "windows" {
		LynxHome = filepath.Join(homeDir, "AppData", "Local", "lynx")
	} else {
		LynxHome = filepath.Join(homeDir, ".lynx")
	}

	// Subdirectories for Lynx
	Lynx_LogsDir = filepath.Join(LynxHome, "logs")
	Lynx_PidsDir = filepath.Join(LynxHome, "pids")
}

func Ensure_LynxHome() error {
	dirs := []string{LynxHome, Lynx_LogsDir, Lynx_PidsDir}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}

	return nil
}
