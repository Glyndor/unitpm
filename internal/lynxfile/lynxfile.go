package lynxfile

import (
	"fmt"
	"io"
	"strings"

	"github.com/Jaro-c/Lynx/internal/ipc/protocol"
	"gopkg.in/yaml.v3"
)

type File struct {
	Version   string      `yaml:"version"`
	Namespace string      `yaml:"namespace,omitempty"`
	Apps      []AppConfig `yaml:"apps"`
}

type AppConfig struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`

	Command string `yaml:"command,omitempty"`
	Entry   string `yaml:"entry,omitempty"`
	Runtime string `yaml:"runtime,omitempty"`

	Cwd       string            `yaml:"cwd,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	Instances int               `yaml:"instances,omitempty"`

	Restart RestartConfig `yaml:"restart,omitempty"`
	Logs    LogsConfig    `yaml:"logs,omitempty"`
}

type RestartConfig struct {
	Policy      string `yaml:"policy"`
	MaxRestarts int    `yaml:"max_restarts,omitempty"`
	DelayMs     int    `yaml:"delay_ms,omitempty"`
	Backoff     string `yaml:"backoff,omitempty"`
	StopOnExit  []int  `yaml:"stop_on_exit,omitempty"`
}

type LogsConfig struct {
	Dir       string `yaml:"dir,omitempty"`
	Stdout    string `yaml:"stdout,omitempty"`
	Stderr    string `yaml:"stderr,omitempty"`
	Format    string `yaml:"format,omitempty"`
	Timestamp string `yaml:"timestamp,omitempty"`
}

func Parse(r io.Reader) (*File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read Lynxfile: %w", err)
	}

	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to parse Lynxfile: %w", err)
	}

	if len(f.Apps) == 0 {
		return nil, fmt.Errorf("lynxfile has no apps")
	}

	for _, app := range f.Apps {
		if strings.TrimSpace(app.Name) == "" {
			return nil, fmt.Errorf("Lynxfile app has empty name")
		}
		if app.Command != "" && app.Entry != "" {
			return nil, fmt.Errorf("lynxfile app %s has both command and entry", app.Name)
		}
		if app.Command == "" && app.Entry == "" {
			return nil, fmt.Errorf("lynxfile app %s must specify command or entry", app.Name)
		}
	}

	return &f, nil
}

func (f *File) ToAppSpecs() ([]protocol.AppSpec, error) {
	var specs []protocol.AppSpec

	for _, app := range f.Apps {
		ns := app.Namespace
		if ns == "" {
			ns = f.Namespace
		}
		if ns == "" {
			ns = "default"
		}

		base := protocol.AppSpec{
			Version:   1,
			Name:      app.Name,
			Namespace: ns,
			Cwd:       app.Cwd,
			Env:       app.Env,
		}

		if app.Command != "" {
			cmdParts := tokenizeCommand(app.Command)
			if len(cmdParts) == 0 {
				return nil, fmt.Errorf("invalid command for app %s", app.Name)
			}
			base.Exec = protocol.AppExec{
				Type:    "command",
				Command: cmdParts[0],
				Args:    cmdParts[1:],
			}
		} else {
			base.Exec = protocol.AppExec{
				Type:    "entry",
				Entry:   app.Entry,
				Runtime: app.Runtime,
			}
		}

		if app.Logs.Dir != "" || app.Logs.Stdout != "" || app.Logs.Stderr != "" || app.Logs.Format != "" || app.Logs.Timestamp != "" {
			base.Logs = &protocol.AppLogs{
				Dir:       app.Logs.Dir,
				Stdout:    app.Logs.Stdout,
				Stderr:    app.Logs.Stderr,
				Format:    app.Logs.Format,
				Timestamp: app.Logs.Timestamp,
			}
		}

		if app.Restart.Policy != "" {
			base.Restart = &protocol.AppRestart{
				Policy:      app.Restart.Policy,
				MaxRetries:  app.Restart.MaxRestarts,
				BackoffMs:   app.Restart.DelayMs,
				BackoffType: app.Restart.Backoff,
				StopOnExit:  app.Restart.StopOnExit,
			}
		}

		instances := app.Instances
		if instances < 1 {
			instances = 1
		}

		for i := 0; i < instances; i++ {
			specs = append(specs, base)
		}
	}

	return specs, nil
}

func tokenizeCommand(cmd string) []string {
	fields := strings.Fields(cmd)
	return fields
}
