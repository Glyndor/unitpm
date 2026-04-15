//go:build linux

package version_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/version"
)

func TestGet_ReturnsInfo(t *testing.T) {
	info := version.Get()

	if info.Version == "" {
		t.Error("version should not be empty")
	}
	if info.ProtocolVersion <= 0 {
		t.Errorf("protocol version = %d, want > 0", info.ProtocolVersion)
	}
}

func TestGet_DefaultValues(t *testing.T) {
	info := version.Get()

	// Default values when not set via ldflags
	if !strings.Contains(info.Version, ".") {
		t.Errorf("version %q should contain a dot (semver)", info.Version)
	}
}

func TestGet_JSONSerializable(t *testing.T) {
	info := version.Get()
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got version.Info
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Version != info.Version {
		t.Errorf("version = %q, want %q", got.Version, info.Version)
	}
	if got.ProtocolVersion != info.ProtocolVersion {
		t.Errorf("protocol_version = %d, want %d", got.ProtocolVersion, info.ProtocolVersion)
	}
}

func TestGet_InfoFields(t *testing.T) {
	info := version.Get()

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, field := range []string{"version", "commit", "build_date", "protocol_version"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("JSON missing field %q", field)
		}
	}
}

func TestGet_ProtocolVersionMatchesProtocolPackage(t *testing.T) {
	info := version.Get()
	// ProtocolVersion in version package should match the protocol constant
	if info.ProtocolVersion != 1 {
		t.Errorf("protocol version = %d, want 1", info.ProtocolVersion)
	}
}
