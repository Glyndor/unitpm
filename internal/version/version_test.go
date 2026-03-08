package version

import (
	"testing"
)

func TestGet(t *testing.T) {
	info := Get()
	if info.Version != Version {
		t.Errorf("Expected version %s, got %s", Version, info.Version)
	}
	if info.Commit != Commit {
		t.Errorf("Expected commit %s, got %s", Commit, info.Commit)
	}
	if info.BuildDate != BuildDate {
		t.Errorf("Expected build date %s, got %s", BuildDate, info.BuildDate)
	}
	if info.ProtocolVersion != ProtocolVersion {
		t.Errorf("Expected protocol version %d, got %d", ProtocolVersion, info.ProtocolVersion)
	}
}
