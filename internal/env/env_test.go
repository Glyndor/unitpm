//go:build linux

package env_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jaro-c/Lynx/internal/env"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "lynx-env-*.env")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	return f.Name()
}

func TestParseFile_BasicKeyValue(t *testing.T) {
	path := writeEnvFile(t, "FOO=bar\nBAZ=qux\n")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", got["FOO"], "bar")
	}
	if got["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want %q", got["BAZ"], "qux")
	}
}

func TestParseFile_DoubleQuoted(t *testing.T) {
	path := writeEnvFile(t, `KEY="hello world"`)
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["KEY"] != "hello world" {
		t.Errorf("KEY = %q, want %q", got["KEY"], "hello world")
	}
}

func TestParseFile_DoubleQuotedEscapedQuote(t *testing.T) {
	path := writeEnvFile(t, `KEY="say \"hello\""`)
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["KEY"] != `say "hello"` {
		t.Errorf("KEY = %q, want %q", got["KEY"], `say "hello"`)
	}
}

func TestParseFile_SingleQuoted(t *testing.T) {
	path := writeEnvFile(t, "KEY='hello world'")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["KEY"] != "hello world" {
		t.Errorf("KEY = %q, want %q", got["KEY"], "hello world")
	}
}

func TestParseFile_UnquotedWithInlineComment(t *testing.T) {
	path := writeEnvFile(t, "KEY=value # this is a comment")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["KEY"] != "value" {
		t.Errorf("KEY = %q, want %q", got["KEY"], "value")
	}
}

func TestParseFile_UnquotedHashInValue(t *testing.T) {
	// Hash without preceding space is NOT a comment
	path := writeEnvFile(t, "KEY=val#ue")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["KEY"] != "val#ue" {
		t.Errorf("KEY = %q, want %q", got["KEY"], "val#ue")
	}
}

func TestParseFile_EmptyValue(t *testing.T) {
	path := writeEnvFile(t, "KEY=")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["KEY"] != "" {
		t.Errorf("KEY = %q, want empty string", got["KEY"])
	}
}

func TestParseFile_CommentsSkipped(t *testing.T) {
	path := writeEnvFile(t, "# comment\nKEY=val\n# another comment\n")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(got), got)
	}
	if got["KEY"] != "val" {
		t.Errorf("KEY = %q, want %q", got["KEY"], "val")
	}
}

func TestParseFile_BlankLinesSkipped(t *testing.T) {
	path := writeEnvFile(t, "\n\nKEY=val\n\n")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(got), got)
	}
}

func TestParseFile_MissingFile(t *testing.T) {
	_, err := env.ParseFile(filepath.Join(t.TempDir(), "nonexistent.env"))
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestParseFile_NoEqualsSkipped(t *testing.T) {
	path := writeEnvFile(t, "NOEQUALS\nKEY=val\n")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["NOEQUALS"]; ok {
		t.Error("NOEQUALS should have been skipped")
	}
	if got["KEY"] != "val" {
		t.Errorf("KEY = %q, want %q", got["KEY"], "val")
	}
}

func TestParseFile_MultipleEquals(t *testing.T) {
	// Only split on first =
	path := writeEnvFile(t, "KEY=a=b=c")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["KEY"] != "a=b=c" {
		t.Errorf("KEY = %q, want %q", got["KEY"], "a=b=c")
	}
}

func TestParseFile_WhitespaceAroundKey(t *testing.T) {
	path := writeEnvFile(t, "  KEY  =value\n")
	got, err := env.ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["KEY"] != "value" {
		t.Errorf("KEY = %q, want %q", got["KEY"], "value")
	}
}
