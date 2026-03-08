package term

import (
	"testing"
)

func TestColorize(t *testing.T) {
	s := &Styler{enabled: true}

	if s.Red("test") != Red+"test"+Reset {
		t.Error("Red output incorrect")
	}
	if s.Green("test") != Green+"test"+Reset {
		t.Error("Green output incorrect")
	}
	if s.Yellow("test") != Yellow+"test"+Reset {
		t.Error("Yellow output incorrect")
	}
	if s.Blue("test") != Blue+"test"+Reset {
		t.Error("Blue output incorrect")
	}
	if s.Magenta("test") != Magenta+"test"+Reset {
		t.Error("Magenta output incorrect")
	}
	if s.Cyan("test") != Cyan+"test"+Reset {
		t.Error("Cyan output incorrect")
	}
	if s.Bold("test") != Bold+"test"+Reset {
		t.Error("Bold output incorrect")
	}
	if s.Dim("test") != Dim+"test"+Reset {
		t.Error("Dim output incorrect")
	}
}

func TestColorizeDisabled(t *testing.T) {
	s := &Styler{enabled: false}

	if s.Red("test") != "test" {
		t.Error("Red output should be plain text when disabled")
	}
}

func TestGlobalHelpers(t *testing.T) {
	// Force enable for testing
	original := std
	std = &Styler{enabled: true}
	defer func() { std = original }()

	if RedString("test") != Red+"test"+Reset {
		t.Error("RedString output incorrect")
	}
	if GreenString("test") != Green+"test"+Reset {
		t.Error("GreenString output incorrect")
	}
}

func TestShouldUseColor(t *testing.T) {
	// Mock NO_COLOR
	t.Setenv("NO_COLOR", "1")
	if ShouldUseColor() {
		t.Error("ShouldUseColor should be false when NO_COLOR is set")
	}

	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if ShouldUseColor() {
		t.Error("ShouldUseColor should be false when TERM=dumb")
	}
}
