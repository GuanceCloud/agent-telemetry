package manage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GuanceCloud/agent-telemetry/internal/install"
)

func TestInstallUsesOneRuntimeForDetectedAdapters(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "agent-telemetry")
	if err := os.WriteFile(source, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{".claude", ".codex"} {
		if err := os.MkdirAll(filepath.Join(home, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	enabled := true
	result, err := Install("", install.CodexOptions{
		Home:             home,
		SourceExecutable: source,
		Endpoint:         "http://127.0.0.1:4318",
		InstallType:      "otlp",
		Enabled:          &enabled,
		SkipTrust:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 2 || result.Runtime != filepath.Join(home, ".local", "bin", "agent-telemetry") {
		t.Fatalf("unexpected install result: %#v", result)
	}
	body, err := os.ReadFile(result.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "binary" {
		t.Fatalf("runtime body = %q", body)
	}
	for _, name := range []string{"claude", "codex"} {
		status, err := Status(name, home)
		if err != nil {
			t.Fatal(err)
		}
		if !status.Detected || !status.Installed || status.Enabled != "true" {
			t.Fatalf("unexpected %s status: %#v", name, status)
		}
	}
}

func TestEnableDisablePreservesConfigAndRemovePreservesByDefault(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	configPath := filepath.Join(home, ".codex", "gtrace.json")
	hooksPath := filepath.Join(home, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, configPath, map[string]any{"enabled": true, "unknown": "keep"})
	writeTestJSON(t, hooksPath, map[string]any{
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{"hooks": []any{
					map[string]any{"command": "/tmp/agent-telemetry hook codex", "type": "command"},
				}},
				map[string]any{"hooks": []any{
					map[string]any{"command": "echo keep", "type": "command"},
				}},
			},
		},
	})

	if _, err := SetEnabled("codex", home, false); err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	readTestJSON(t, configPath, &config)
	if config["enabled"] != false || config["unknown"] != "keep" {
		t.Fatalf("config was not preserved: %#v", config)
	}

	results, err := Remove("codex", home, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].HookRemoved || results[0].ConfigPurged {
		t.Fatalf("unexpected remove result: %#v", results)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config should be preserved: %v", err)
	}
	status, err := Status("codex", home)
	if err != nil {
		t.Fatal(err)
	}
	if status.Installed {
		t.Fatalf("managed hook was not removed: %#v", status)
	}
	var hooks map[string]any
	readTestJSON(t, hooksPath, &hooks)
	encoded, _ := json.Marshal(hooks)
	if string(encoded) == "" || !containsText(encoded, "echo keep") {
		t.Fatalf("unrelated hook was lost: %s", encoded)
	}
}

func TestInstallAlwaysInstallsSingleRuntime(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "agent-telemetry")
	if err := os.WriteFile(source, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := Install("", install.CodexOptions{
		Home:             home,
		SourceExecutable: source,
		SkipTrust:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(result.Runtime); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveAllPurgesConfigsAndSharedRuntime(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := filepath.Join(root, "agent-telemetry")
	if err := os.WriteFile(source, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	enabled := true
	result, err := Install("all", install.CodexOptions{
		Home:             home,
		SourceExecutable: source,
		Endpoint:         "http://127.0.0.1:4318",
		InstallType:      "otlp",
		Enabled:          &enabled,
		SkipTrust:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Remove("", home, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		result.Runtime,
		filepath.Join(home, ".claude", "gtrace.json"),
		filepath.Join(home, ".codex", "gtrace.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, got %v", path, err)
		}
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestJSON(t *testing.T, path string, target any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatal(err)
	}
}

func containsText(value []byte, text string) bool {
	for index := 0; index+len(text) <= len(value); index++ {
		if string(value[index:index+len(text)]) == text {
			return true
		}
	}
	return false
}
