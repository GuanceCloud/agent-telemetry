package buildinfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVersionMatchesPackageMetadata(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	packageFile := filepath.Clean(filepath.Join(
		filepath.Dir(currentFile),
		"..", "..", "..", "..",
		"plugin.json",
	))
	body, err := os.ReadFile(packageFile)
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		t.Fatal(err)
	}
	if Version != metadata.Version {
		t.Fatalf("build version %q does not match package metadata %q", Version, metadata.Version)
	}
}
