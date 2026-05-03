package start

import (
	"testing"
)

func TestParseMemorySize_Empty(t *testing.T) {
	n, err := parseMemorySize("")
	if err != nil || n != 0 {
		t.Errorf("parseMemorySize('') = %d, %v; want 0, nil", n, err)
	}
}

func TestParseMemorySize_Whitespace(t *testing.T) {
	n, err := parseMemorySize("   ")
	if err != nil || n != 0 {
		t.Errorf("parseMemorySize('   ') = %d, %v; want 0, nil", n, err)
	}
}

func TestParseMemorySize_Kilobytes(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"512k", 512 * 1024},
		{"512K", 512 * 1024},
		{"1K", 1024},
	}
	for _, tt := range cases {
		got, err := parseMemorySize(tt.input)
		if err != nil || got != tt.want {
			t.Errorf("parseMemorySize(%q) = %d, %v; want %d, nil", tt.input, got, err, tt.want)
		}
	}
}

func TestParseMemorySize_Megabytes(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"512m", 512 * 1024 * 1024},
		{"512M", 512 * 1024 * 1024},
		{"1M", 1024 * 1024},
	}
	for _, tt := range cases {
		got, err := parseMemorySize(tt.input)
		if err != nil || got != tt.want {
			t.Errorf("parseMemorySize(%q) = %d, %v; want %d, nil", tt.input, got, err, tt.want)
		}
	}
}

func TestParseMemorySize_Gigabytes(t *testing.T) {
	got, err := parseMemorySize("2G")
	want := int64(2 * 1024 * 1024 * 1024)
	if err != nil || got != want {
		t.Errorf("parseMemorySize('2G') = %d, %v; want %d, nil", got, err, want)
	}
}

func TestParseMemorySize_RawBytes(t *testing.T) {
	got, err := parseMemorySize("10485760")
	if err != nil || got != 10485760 {
		t.Errorf("parseMemorySize('10485760') = %d, %v; want 10485760, nil", got, err)
	}
}

func TestParseMemorySize_Invalid(t *testing.T) {
	cases := []string{"abc", "0M", "-1M", "0"}
	for _, input := range cases {
		_, err := parseMemorySize(input)
		if err == nil {
			t.Errorf("parseMemorySize(%q) expected error, got nil", input)
		}
	}
}

func TestReadIntList_Basic(t *testing.T) {
	p := &specParser{args: []string{"--cpus", "0,1,2"}, pos: 0}
	var result []int
	if err := p.readIntList(&result); err != nil {
		t.Fatalf("readIntList: %v", err)
	}
	if len(result) != 3 || result[0] != 0 || result[1] != 1 || result[2] != 2 {
		t.Errorf("result = %v, want [0 1 2]", result)
	}
}

func TestReadIntList_Single(t *testing.T) {
	p := &specParser{args: []string{"--cpus", "7"}, pos: 0}
	var result []int
	if err := p.readIntList(&result); err != nil {
		t.Fatalf("readIntList: %v", err)
	}
	if len(result) != 1 || result[0] != 7 {
		t.Errorf("result = %v, want [7]", result)
	}
}

func TestReadIntList_WithSpaces(t *testing.T) {
	p := &specParser{args: []string{"--cpus", "0, 1, 2"}, pos: 0}
	var result []int
	if err := p.readIntList(&result); err != nil {
		t.Fatalf("readIntList: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("result = %v, want 3 elements", result)
	}
}

func TestReadIntList_MissingValue(t *testing.T) {
	p := &specParser{args: []string{"--cpus"}, pos: 0}
	var result []int
	if err := p.readIntList(&result); err == nil {
		t.Error("expected error for missing value, got nil")
	}
}

func TestReadIntList_InvalidInt(t *testing.T) {
	p := &specParser{args: []string{"--cpus", "0,abc,2"}, pos: 0}
	var result []int
	if err := p.readIntList(&result); err == nil {
		t.Error("expected error for invalid integer, got nil")
	}
}
