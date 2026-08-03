package sharedconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	home := t.TempDir()
	enabled := true
	original := Config{
		Endpoint:           "https://llm-openway.guance.com",
		TracePath:          "v1/write/otel-llm",
		MetricsPath:        "v1/write/otel-metrics",
		InstallType:        "gtrace",
		XToken:             "token",
		Headers:            map[string]string{"X-Test": "1"},
		ResourceAttributes: map[string]string{"env": "prod"},
		CaptureContent:     "preview",
		MaxChars:           2048,
		Enabled:            &enabled,
	}
	path, err := Save(home, original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	loaded, loadedPath, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loadedPath != filepath.Join(home, ".agent-telemetry", "config.json") {
		t.Fatalf("unexpected path: %s", loadedPath)
	}
	if loaded.Endpoint != original.Endpoint || loaded.XToken != original.XToken ||
		loaded.Headers["X-Test"] != "1" || loaded.ResourceAttributes["env"] != "prod" ||
		loaded.Enabled == nil || !*loaded.Enabled {
		t.Fatalf("unexpected loaded config: %#v", loaded)
	}
}
