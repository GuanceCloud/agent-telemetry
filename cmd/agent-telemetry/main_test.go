package main

import (
	"strings"
	"testing"

	"github.com/GuanceCloud/agent-telemetry/internal/manage"
)

func TestFormatStatusDetailMatchesConnectorStyle(t *testing.T) {
	status := manage.AdapterStatus{
		Name:       "codex",
		Detected:   true,
		Installed:  true,
		Enabled:    "true",
		ConfigFile: "/home/liurui/.codex/gtrace.json",
	}

	output := formatStatusDetail(
		status,
		"0.3.0-rc.3",
		"/home/liurui/.local/bin/agent-telemetry",
		"/home/liurui",
	)

	expectedLines := []string{
		"Agent    : codex",
		"Command  : codex",
		"Supported: yes",
		"Installed: yes",
		"Version  : 0.3.0-rc.3",
		"Config   : ~/.codex/gtrace.json",
		"Path     : ~/.local/bin/agent-telemetry",
		"Enabled  : true",
	}
	for _, line := range expectedLines {
		if !strings.Contains(output, line) {
			t.Fatalf("missing line %q in output:\n%s", line, output)
		}
	}
}
