package manager

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var versionPatterns = []struct {
	file    string
	extract func([]byte) string
}{
	{"package.json", extractJSON("version")},
	{"Cargo.toml", extractTOML("version")},
	{"pyproject.toml", extractTOML("version")},
	{"setup.cfg", extractINI("version")},
	{"mix.exs", extractRegex(`version:\s*"([^"]+)"`)},
}

func detectProjectVersion(cwd string) string {
	if cwd == "" {
		return ""
	}
	for _, p := range versionPatterns {
		data, err := os.ReadFile(filepath.Join(cwd, p.file))
		if err != nil {
			continue
		}
		if v := p.extract(data); v != "" {
			return v
		}
	}
	return ""
}

func extractJSON(key string) func([]byte) string {
	// Minimal JSON extraction without importing encoding/json.
	re := regexp.MustCompile(`"` + key + `"\s*:\s*"([^"]+)"`)
	return func(data []byte) string {
		m := re.FindSubmatch(data)
		if m != nil {
			return string(m[1])
		}
		return ""
	}
}

func extractTOML(key string) func([]byte) string {
	re := regexp.MustCompile(`(?m)^` + key + `\s*=\s*"([^"]+)"`)
	return func(data []byte) string {
		m := re.FindSubmatch(data)
		if m != nil {
			return string(m[1])
		}
		return ""
	}
}

func extractINI(key string) func([]byte) string {
	re := regexp.MustCompile(`(?m)^` + key + `\s*=\s*(.+)$`)
	return func(data []byte) string {
		m := re.FindSubmatch(data)
		if m != nil {
			return strings.TrimSpace(string(m[1]))
		}
		return ""
	}
}

func extractRegex(pattern string) func([]byte) string {
	re := regexp.MustCompile(pattern)
	return func(data []byte) string {
		m := re.FindSubmatch(data)
		if m != nil {
			return string(m[1])
		}
		return ""
	}
}
