package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveClaudeDeletesConfigAndPurgesStateOnRequest(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	configPath := filepath.Join(home, ".claude", "gtrace.json")
	statePath := filepath.Join(home, ".claude", "state", "agent-telemetry")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, settingsPath, map[string]any{
		"theme": "dark",
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{"hooks": []any{map[string]any{
					"type": "command", "command": "/tmp/agent-telemetry", "args": []any{"hook", "claude"},
				}}},
				map[string]any{"hooks": []any{map[string]any{
					"type": "command", "command": "echo keep",
				}}},
			},
			"SessionEnd": []any{
				map[string]any{"hooks": []any{map[string]any{
					"type": "command", "command": "/tmp/gtrace-agent", "args": []any{"hook", "claude"},
				}}},
			},
		},
	})
	writeTestJSON(t, configPath, map[string]any{"enabled": true})

	result, err := RemoveAdapter("claude", home, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HookRemoved || !result.ConfigRemoved || result.StatePurged {
		t.Fatalf("unexpected result: %#v", result)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config should be removed: %v", err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state should be preserved without purge: %v", err)
	}

	var settings map[string]any
	readTestJSON(t, settingsPath, &settings)
	stop := settings["hooks"].(map[string]any)["Stop"].([]any)
	sessionEnd := settings["hooks"].(map[string]any)["SessionEnd"].([]any)
	if len(stop) != 1 || len(sessionEnd) != 0 || settings["theme"] != "dark" {
		t.Fatalf("unrelated settings were not preserved: %#v", settings)
	}

	result, err = RemoveAdapter("claude", home, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ConfigRemoved || !result.StatePurged {
		t.Fatalf("purge was not reported: %#v", result)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config was not purged: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state was not purged: %v", err)
	}
}
