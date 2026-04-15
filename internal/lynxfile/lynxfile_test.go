//go:build linux

package lynxfile_test

import (
	"strings"
	"testing"

	"github.com/Jaro-c/Lynx/internal/lynxfile"
)

func parse(t *testing.T, yaml string) *lynxfile.File {
	t.Helper()
	f, err := lynxfile.Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	return f
}

func parseErr(t *testing.T, yaml string) error {
	t.Helper()
	_, err := lynxfile.Parse(strings.NewReader(yaml))
	return err
}

const minimalYAML = `
version: "1"
apps:
  - name: myapp
    command: node server.js
`

func TestParse_Minimal(t *testing.T) {
	f := parse(t, minimalYAML)
	if len(f.Apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(f.Apps))
	}
	if f.Apps[0].Name != "myapp" {
		t.Errorf("name = %q, want %q", f.Apps[0].Name, "myapp")
	}
}

func TestParse_EntryApp(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: server
    entry: main.go
    runtime: go run
`
	f := parse(t, yaml)
	if f.Apps[0].Entry != "main.go" {
		t.Errorf("entry = %q, want %q", f.Apps[0].Entry, "main.go")
	}
	if f.Apps[0].Runtime != "go run" {
		t.Errorf("runtime = %q, want %q", f.Apps[0].Runtime, "go run")
	}
}

func TestParse_NamespaceInherited(t *testing.T) {
	yaml := `
version: "1"
namespace: production
apps:
  - name: api
    command: ./api
`
	f := parse(t, yaml)
	if f.Namespace != "production" {
		t.Errorf("namespace = %q, want %q", f.Namespace, "production")
	}
}

func TestParse_MultipleApps(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: app1
    command: cmd1
  - name: app2
    command: cmd2
  - name: app3
    entry: main.py
    runtime: python3
`
	f := parse(t, yaml)
	if len(f.Apps) != 3 {
		t.Errorf("expected 3 apps, got %d", len(f.Apps))
	}
}

func TestParse_RestartConfig(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: worker
    command: ./worker
    restart:
      policy: on-failure
      max_restarts: 5
      delay_ms: 1000
      backoff: expo
      stop_on_exit: [0, 2]
`
	f := parse(t, yaml)
	r := f.Apps[0].Restart
	if r.Policy != "on-failure" {
		t.Errorf("policy = %q, want on-failure", r.Policy)
	}
	if r.MaxRestarts != 5 {
		t.Errorf("max_restarts = %d, want 5", r.MaxRestarts)
	}
	if r.DelayMs != 1000 {
		t.Errorf("delay_ms = %d, want 1000", r.DelayMs)
	}
	if r.Backoff != "expo" {
		t.Errorf("backoff = %q, want expo", r.Backoff)
	}
	if len(r.StopOnExit) != 2 || r.StopOnExit[0] != 0 || r.StopOnExit[1] != 2 {
		t.Errorf("stop_on_exit = %v, want [0 2]", r.StopOnExit)
	}
}

func TestParse_LogsConfig(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: svc
    command: ./svc
    logs:
      dir: /var/log/myapp
      stdout: out.log
      stderr: err.log
      format: json
      timestamp: rfc3339
`
	f := parse(t, yaml)
	l := f.Apps[0].Logs
	if l.Dir != "/var/log/myapp" {
		t.Errorf("dir = %q, want /var/log/myapp", l.Dir)
	}
	if l.Format != "json" {
		t.Errorf("format = %q, want json", l.Format)
	}
	if l.Timestamp != "rfc3339" {
		t.Errorf("timestamp = %q, want rfc3339", l.Timestamp)
	}
}

func TestParse_EnvVars(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: api
    command: ./api
    env:
      PORT: "8080"
      DEBUG: "true"
`
	f := parse(t, yaml)
	if f.Apps[0].Env["PORT"] != "8080" {
		t.Errorf("PORT = %q, want 8080", f.Apps[0].Env["PORT"])
	}
	if f.Apps[0].Env["DEBUG"] != "true" {
		t.Errorf("DEBUG = %q, want true", f.Apps[0].Env["DEBUG"])
	}
}

func TestParse_Instances(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: worker
    command: ./worker
    instances: 3
`
	f := parse(t, yaml)
	if f.Apps[0].Instances != 3 {
		t.Errorf("instances = %d, want 3", f.Apps[0].Instances)
	}
}

// Error cases

func TestParse_NoApps(t *testing.T) {
	yaml := `version: "1"
apps: []
`
	if err := parseErr(t, yaml); err == nil {
		t.Error("expected error for empty apps")
	}
}

func TestParse_EmptyName(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: ""
    command: ./cmd
`
	if err := parseErr(t, yaml); err == nil {
		t.Error("expected error for empty app name")
	}
}

func TestParse_BothCommandAndEntry(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: app
    command: ./cmd
    entry: main.go
`
	if err := parseErr(t, yaml); err == nil {
		t.Error("expected error when both command and entry specified")
	}
}

func TestParse_NeitherCommandNorEntry(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: app
    cwd: /tmp
`
	if err := parseErr(t, yaml); err == nil {
		t.Error("expected error when neither command nor entry specified")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	if err := parseErr(t, "{ invalid yaml:"); err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ToAppSpecs tests

func TestToAppSpecs_SingleApp(t *testing.T) {
	f := parse(t, minimalYAML)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if len(specs) != 1 {
		t.Errorf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Name != "myapp" {
		t.Errorf("name = %q, want myapp", specs[0].Name)
	}
	if specs[0].Exec.Type != "command" {
		t.Errorf("exec type = %q, want command", specs[0].Exec.Type)
	}
	if specs[0].Exec.Command != "node" {
		t.Errorf("exec command = %q, want node", specs[0].Exec.Command)
	}
}

func TestToAppSpecs_MultipleInstances(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: worker
    command: ./worker
    instances: 3
`
	f := parse(t, yaml)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if len(specs) != 3 {
		t.Errorf("expected 3 specs for instances=3, got %d", len(specs))
	}
}

func TestToAppSpecs_DefaultNamespace(t *testing.T) {
	f := parse(t, minimalYAML)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if specs[0].Namespace != "default" {
		t.Errorf("namespace = %q, want default", specs[0].Namespace)
	}
}

func TestToAppSpecs_InheritedNamespace(t *testing.T) {
	yaml := `
version: "1"
namespace: staging
apps:
  - name: api
    command: ./api
`
	f := parse(t, yaml)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if specs[0].Namespace != "staging" {
		t.Errorf("namespace = %q, want staging", specs[0].Namespace)
	}
}

func TestToAppSpecs_AppOverridesNamespace(t *testing.T) {
	yaml := `
version: "1"
namespace: global
apps:
  - name: api
    namespace: local
    command: ./api
`
	f := parse(t, yaml)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if specs[0].Namespace != "local" {
		t.Errorf("namespace = %q, want local (app override)", specs[0].Namespace)
	}
}

func TestToAppSpecs_EntryType(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: pyapp
    entry: app.py
    runtime: python3
`
	f := parse(t, yaml)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if specs[0].Exec.Type != "entry" {
		t.Errorf("exec type = %q, want entry", specs[0].Exec.Type)
	}
	if specs[0].Exec.Entry != "app.py" {
		t.Errorf("entry = %q, want app.py", specs[0].Exec.Entry)
	}
}

func TestToAppSpecs_CommandArgs(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: server
    command: node server.js --port 8080
`
	f := parse(t, yaml)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if specs[0].Exec.Command != "node" {
		t.Errorf("command = %q, want node", specs[0].Exec.Command)
	}
	// "node server.js --port 8080" → command="node", args=["server.js", "--port", "8080"]
	if len(specs[0].Exec.Args) != 3 {
		t.Errorf("args count = %d, want 3: %v", len(specs[0].Exec.Args), specs[0].Exec.Args)
	}
}

func TestToAppSpecs_RestartBuilt(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: svc
    command: ./svc
    restart:
      policy: always
`
	f := parse(t, yaml)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if specs[0].Restart == nil {
		t.Fatal("restart should not be nil")
	}
	if specs[0].Restart.Policy != "always" {
		t.Errorf("policy = %q, want always", specs[0].Restart.Policy)
	}
}

func TestToAppSpecs_NoRestartConfig(t *testing.T) {
	f := parse(t, minimalYAML)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if specs[0].Restart != nil {
		t.Error("restart should be nil when no restart config provided")
	}
}

func TestToAppSpecs_LogsBuilt(t *testing.T) {
	yaml := `
version: "1"
apps:
  - name: svc
    command: ./svc
    logs:
      dir: /tmp/logs
`
	f := parse(t, yaml)
	specs, err := f.ToAppSpecs()
	if err != nil {
		t.Fatalf("ToAppSpecs error: %v", err)
	}
	if specs[0].Logs == nil {
		t.Fatal("logs should not be nil")
	}
	if specs[0].Logs.Dir != "/tmp/logs" {
		t.Errorf("logs.dir = %q, want /tmp/logs", specs[0].Logs.Dir)
	}
}
