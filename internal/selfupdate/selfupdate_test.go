package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpgradeInstallsLatestRuntimeAndPreservesConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping archive test in short mode")
	}

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, ".codex", "gtrace.json")
	configBody := "{\n  \"enabled\": false,\n  \"endpoint\": \"https://keep.example\",\n  \"unknown\": \"keep\"\n}\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	assetName, binaryName, err := currentPlatformAsset("latest")
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), assetName)
	if err := writeTarGzArchive(archivePath, binaryName, "9.9.9", "new-binary"); err != nil {
		t.Fatal(err)
	}
	archiveBody, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archiveBody)
	sumsBody := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), assetName)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			_, _ = w.Write([]byte(sumsBody))
		case "/" + assetName:
			_, _ = w.Write(archiveBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := Upgrade(Options{
		Home:    home,
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.InstalledVersion != "9.9.9" {
		t.Fatalf("unexpected installed version: %#v", result)
	}
	if !containsText(result.Targets, "codex") {
		t.Fatalf("expected codex target, got %#v", result.Targets)
	}
	runtimeBody, err := os.ReadFile(result.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeBody) != "new-binary" {
		t.Fatalf("unexpected runtime body: %q", runtimeBody)
	}
	currentConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentConfig) != configBody {
		t.Fatalf("config changed during upgrade:\n%s", currentConfig)
	}
	hooksBody, err := os.ReadFile(filepath.Join(home, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hooksBody), "agent-telemetry hook codex") {
		t.Fatalf("missing managed codex hook: %s", hooksBody)
	}
}

func containsText(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func writeTarGzArchive(path, binaryName, releaseVersion, binaryBody string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	files := []struct {
		Name string
		Body string
		Mode int64
	}{
		{Name: "./bin/" + binaryName, Body: binaryBody, Mode: 0o755},
		{Name: "./VERSION", Body: releaseVersion + "\n", Mode: 0o644},
	}
	for _, item := range files {
		header := &tar.Header{
			Name: item.Name,
			Mode: item.Mode,
			Size: int64(len(item.Body)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarWriter.Write([]byte(item.Body)); err != nil {
			return err
		}
	}
	return nil
}
