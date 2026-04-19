//go:build linux

package lynxfile_test

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/lynxfile"
)

// FuzzParse feeds arbitrary byte sequences to the YAML parser and the
// subsequent ToAppSpecs validation to surface panics, infinite loops, or
// memory exhaustion in the config-file input path. Parse errors are
// expected and fine; what this catches is the parser or the converter
// crashing outright.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		"version: \"1\"\napps: []",
		"version: \"1\"\napps:\n  - name: a\n    command: echo",
		"version: \"1\"\nnamespace: prod\napps:\n  - name: api\n    command: node server.js\n    env_file: .env\n    restart: always",
		"apps:\n  - name: a\n    cron: \"@every 30s\"\n    command: echo",
		"---\n---\n",
		"!!binary |\n  SGVsbG8=",
		"&anchor value\n*anchor",
		strings.Repeat("a: b\n", 100),
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := lynxfile.Parse(strings.NewReader(string(data)))
		if err != nil || file == nil {
			return
		}
		// Conversion to AppSpecs may legitimately return an error for
		// invalid but syntactically-valid configurations; we only care
		// that it does not panic.
		_, _ = file.ToAppSpecs()
	})
}
