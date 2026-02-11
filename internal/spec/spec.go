// Package spec implements application specification management.
package spec

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"github.com/Jaro-c/Lynx/internal/jsonx"
	"github.com/google/uuid"
)

// GenerateUUIDv4 generates a random UUID v4 string.
func GenerateUUIDv4() (string, error) {
	return uuid.NewString(), nil
}

// GetSpecDir returns the directory where specs are stored, following XDG standards.
// Creates the directory if it doesn't exist (0700).
func GetSpecDir() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home dir: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}

	specDir := filepath.Join(configHome, "lynx", "apps")
	if err := os.MkdirAll(specDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create spec dir: %w", err)
	}

	return specDir, nil
}

// SaveSpec writes the spec to the XDG config directory.
func SaveSpec(id string, data interface{}) (string, error) {
	dir, err := GetSpecDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, id+".json")

	bytes, err := jsonx.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal spec: %w", err)
	}

	// Write with 0600 permissions
	if err := os.WriteFile(path, bytes, 0600); err != nil {
		return "", fmt.Errorf("failed to write spec file: %w", err)
	}

	return path, nil
}

// DeleteSpec removes the spec file.
func DeleteSpec(id string) error {
	dir, err := GetSpecDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadAll loads all application specs from the config directory.
// It skips invalid files with a warning log.
func LoadAll() ([]protocol.AppSpec, error) {
	dir, err := GetSpecDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec dir: %w", err)
	}

	var specs []protocol.AppSpec
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		bytes, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: failed to read spec file %s: %v", path, err)
			continue
		}

		var spec protocol.AppSpec
		if err := jsonx.Unmarshal(bytes, &spec); err != nil {
			log.Printf("Warning: failed to parse spec file %s: %v", path, err)
			continue
		}

		specs = append(specs, spec)
	}

	return specs, nil
}
