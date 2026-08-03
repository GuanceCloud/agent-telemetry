package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnifiedManifestIsGoOnlyAndVersionedConsistently(t *testing.T) {
	root := repositoryRoot(t)
	versionBytes, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionBytes))

	manifestBytes, err := os.ReadFile(filepath.Join(root, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Version  string `json:"version"`
		Runtime  string `json:"runtime"`
		Binary   string `json:"binary"`
		Adapters []struct {
			ID         string `json:"id"`
			Entrypoint string `json:"entrypoint"`
		} `json:"adapters"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "agent-telemetry" || manifest.Name != "agent-telemetry" ||
		manifest.Version != version || manifest.Runtime != "go" || manifest.Binary != "agent-telemetry" {
		t.Fatalf("unexpected unified manifest: %#v", manifest)
	}
	if len(manifest.Adapters) != 2 ||
		manifest.Adapters[0].ID != "claude" ||
		manifest.Adapters[1].ID != "codex" {
		t.Fatalf("unexpected adapters: %#v", manifest.Adapters)
	}
	for _, adapter := range manifest.Adapters {
		if adapter.Entrypoint != "agent-telemetry hook "+adapter.ID {
			t.Fatalf("unexpected %s entrypoint: %q", adapter.ID, adapter.Entrypoint)
		}
	}

	for _, name := range []string{"install.sh", "install.ps1", "install-release.sh", "install-release.ps1"} {
		body, err := os.ReadFile(filepath.Join(root, "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ToLower(string(body))
		for _, forbidden := range []string{"git clone", "python", "pip", "node", "venv", "gtrace-agent"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains obsolete runtime dependency %q", name, forbidden)
			}
		}
		if !strings.Contains(text, "agent-telemetry") || !strings.Contains(text, "install") {
			t.Fatalf("%s does not invoke the unified installer", name)
		}
	}
}

func TestLegacyPerAgentPluginManifestsAreGone(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{
		filepath.Join(root, "plugins", "claude", "plugin.json"),
		filepath.Join(root, "plugins", "codex", "plugin.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy per-Agent manifest still exists: %s", path)
		}
	}
}

func TestReleaseBuildPackagesOnlyUnifiedRuntime(t *testing.T) {
	root := repositoryRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "scripts", "build-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		`binary_name="agent-telemetry"`,
		`./cmd/agent-telemetry`,
		`"${REPO_ROOT}/plugin.json"`,
		`"${REPO_ROOT}/scripts/install.sh"`,
		`"${REPO_ROOT}/scripts/install-release.sh"`,
		`agent-telemetry-${platform}.tar.gz`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("release build is missing %q", required)
		}
	}
	for _, obsolete := range []string{"plugins/claude", "plugins/codex", "gtrace-agent"} {
		if strings.Contains(text, obsolete) {
			t.Fatalf("release build still packages obsolete artifact %q", obsolete)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}
