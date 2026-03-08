package lynxfile

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	yamlData := `
version: "1.0"
namespace: "test-ns"
apps:
  - name: "app1"
    command: "python app.py"
    cwd: "/app"
    instances: 2
    logs:
      dir: "/var/log/app1"
    restart:
      policy: "always"
      max_restarts: 5
      delay_ms: 1000
`

	file, err := Parse(strings.NewReader(yamlData))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if file.Namespace != "test-ns" {
		t.Errorf("Expected namespace test-ns, got %s", file.Namespace)
	}
	if len(file.Apps) != 1 {
		t.Fatalf("Expected 1 app, got %d", len(file.Apps))
	}

	app := file.Apps[0]
	if app.Name != "app1" {
		t.Errorf("Expected app name app1, got %s", app.Name)
	}
	if app.Instances != 2 {
		t.Errorf("Expected 2 instances, got %d", app.Instances)
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		errSubstr string
	}{
		{
			name: "empty apps",
			yaml: `
version: "1.0"
apps: []
`,
			errSubstr: "lynxfile has no apps",
		},
		{
			name: "missing app name",
			yaml: `
version: "1.0"
apps:
  - command: "echo hello"
`,
			errSubstr: "lynxfile app has empty name",
		},
		{
			name: "both command and entry",
			yaml: `
version: "1.0"
apps:
  - name: "bad"
    command: "echo"
    entry: "main.js"
`,
			errSubstr: "has both command and entry",
		},
		{
			name: "neither command nor entry",
			yaml: `
version: "1.0"
apps:
  - name: "bad"
`,
			errSubstr: "must specify command or entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.yaml))
			if err == nil {
				t.Fatalf("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("Expected error containing %q, got %q", tt.errSubstr, err.Error())
			}
		})
	}
}

func TestToAppSpecs(t *testing.T) {
	yamlData := `
version: "1.0"
apps:
  - name: "app1"
    command: "echo hello"
    instances: 2
`
	file, err := Parse(strings.NewReader(yamlData))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	specs, err := file.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs failed: %v", err)
	}

	if len(specs) != 2 {
		t.Errorf("Expected 2 specs (instances), got %d", len(specs))
	}

	if specs[0].Exec.Command != "echo" {
		t.Errorf("Expected command echo, got %s", specs[0].Exec.Command)
	}
}
