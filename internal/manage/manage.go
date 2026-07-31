package manage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/GuanceCloud/agent-telemetry/internal/install"
)

var adapterNames = []string{"claude", "codex"}

type AdapterStatus struct {
	Name       string
	Detected   bool
	Installed  bool
	Enabled    string
	HookFile   string
	ConfigFile string
}

type InstallResult struct {
	Runtime  string
	Targets  []string
	Warnings []string
}

func Names() []string {
	return append([]string(nil), adapterNames...)
}

func Install(target string, options install.CodexOptions) (InstallResult, error) {
	targets, err := resolveTargets(target, options.Home, true)
	if err != nil {
		return InstallResult{}, err
	}
	runtimePath, err := install.InstallRuntime(options.Home, options.SourceExecutable, options.DestinationExecutable)
	if err != nil {
		return InstallResult{}, err
	}
	options.SourceExecutable = runtimePath
	options.DestinationExecutable = runtimePath

	result := InstallResult{Runtime: runtimePath, Targets: targets}
	for _, name := range targets {
		switch name {
		case "claude":
			_, err = install.InstallClaude(install.ClaudeOptions{
				Home:                  options.Home,
				SourceExecutable:      runtimePath,
				DestinationExecutable: runtimePath,
				Endpoint:              options.Endpoint,
				TracePath:             options.TracePath,
				MetricsPath:           options.MetricsPath,
				InstallType:           options.InstallType,
				XToken:                options.XToken,
				Headers:               options.Headers,
				ResourceAttributes:    options.ResourceAttributes,
				CaptureContent:        options.CaptureContent,
				MaxChars:              options.MaxChars,
				Enabled:               options.Enabled,
				NoConfig:              options.NoConfig,
			})
			if err == nil {
				result.Warnings = append(result.Warnings, "Restart Claude Code to load the reconciled Hook.")
			}
		case "codex":
			var codexResult install.CodexResult
			codexResult, err = install.InstallCodex(options)
			if err == nil && codexResult.TrustSkipped {
				result.Warnings = append(result.Warnings,
					"Codex Hook trust was skipped; restart Codex and trust the Hook when prompted.")
			}
		}
		if err != nil {
			return result,
				fmt.Errorf("install %s adapter: %w", name, err)
		}
	}
	return result, nil
}

func Discover(home string) ([]AdapterStatus, error) {
	statuses := make([]AdapterStatus, 0, len(adapterNames))
	for _, name := range adapterNames {
		status, err := Status(name, home)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func Status(name, home string) (AdapterStatus, error) {
	name = normalizeAdapter(name)
	if !supported(name) {
		return AdapterStatus{}, unsupportedAdapter(name)
	}
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return AdapterStatus{}, err
		}
	}
	status := AdapterStatus{Name: name}
	switch name {
	case "claude":
		status.HookFile = filepath.Join(home, ".claude", "settings.json")
		status.ConfigFile = filepath.Join(home, ".claude", "gtrace.json")
		status.Detected = commandOrDirectoryExists("claude", filepath.Join(home, ".claude"))
	case "codex":
		status.HookFile = filepath.Join(home, ".codex", "hooks.json")
		status.ConfigFile = filepath.Join(home, ".codex", "gtrace.json")
		status.Detected = commandOrDirectoryExists("codex", filepath.Join(home, ".codex"))
	}
	status.Installed = adapterHookInstalled(status.HookFile, name)
	status.Enabled = enabledValue(status.ConfigFile)
	return status, nil
}

func SetEnabled(name, home string, enabled bool) (string, error) {
	status, err := Status(name, home)
	if err != nil {
		return "", err
	}
	current, exists, err := readJSONObject(status.ConfigFile)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("%s adapter config does not exist: %s", name, status.ConfigFile)
	}
	current["enabled"] = enabled
	if err := writeJSONAtomic(status.ConfigFile, current); err != nil {
		return "", err
	}
	return status.ConfigFile, nil
}

func Remove(target, home string, purge bool) ([]install.RemoveResult, error) {
	targets, err := resolveTargets(target, home, false)
	if err != nil {
		return nil, err
	}
	results := make([]install.RemoveResult, 0, len(targets))
	for _, name := range targets {
		result, err := install.RemoveAdapter(name, home, purge)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	if target == "" || strings.EqualFold(strings.TrimSpace(target), "all") {
		if err := install.RemoveRuntime(home); err != nil {
			return results, err
		}
	}
	return results, nil
}

func resolveTargets(target, home string, detectedOnly bool) ([]string, error) {
	target = normalizeAdapter(target)
	if target != "" && target != "all" {
		if !supported(target) {
			return nil, unsupportedAdapter(target)
		}
		return []string{target}, nil
	}
	if target == "all" || !detectedOnly {
		return Names(), nil
	}
	statuses, err := Discover(home)
	if err != nil {
		return nil, err
	}
	targets := make([]string, 0, len(statuses))
	for _, status := range statuses {
		if status.Detected {
			targets = append(targets, status.Name)
		}
	}
	sort.Strings(targets)
	return targets, nil
}

func normalizeAdapter(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func supported(name string) bool {
	for _, item := range adapterNames {
		if item == name {
			return true
		}
	}
	return false
}

func unsupportedAdapter(name string) error {
	return fmt.Errorf("unsupported adapter %q; available adapters: %s", name, strings.Join(adapterNames, ", "))
}

func commandOrDirectoryExists(command, directory string) bool {
	if _, err := exec.LookPath(command); err == nil {
		return true
	}
	info, err := os.Stat(directory)
	return err == nil && info.IsDir()
}

func adapterHookInstalled(path, adapter string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	return containsAdapterHook(value, adapter)
}

func containsAdapterHook(value any, adapter string) bool {
	switch current := value.(type) {
	case []any:
		for _, item := range current {
			if containsAdapterHook(item, adapter) {
				return true
			}
		}
	case map[string]any:
		command := strings.ToLower(strings.TrimSpace(fmt.Sprint(current["command"])))
		if managedRuntimeCommand(command) && strings.Contains(command, "hook "+adapter) {
			return true
		}
		args, _ := current["args"].([]any)
		if managedRuntimeCommand(command) && len(args) >= 2 &&
			fmt.Sprint(args[0]) == "hook" && fmt.Sprint(args[1]) == adapter {
			return true
		}
		for _, item := range current {
			if containsAdapterHook(item, adapter) {
				return true
			}
		}
	}
	return false
}

func managedRuntimeCommand(command string) bool {
	command = strings.ReplaceAll(command, `\`, "/")
	return strings.Contains(command, "agent-telemetry") || strings.Contains(command, "gtrace-agent")
}

func enabledValue(path string) string {
	value, exists, err := readJSONObject(path)
	if err != nil {
		return "invalid"
	}
	if !exists {
		return "-"
	}
	enabled, ok := value["enabled"].(bool)
	if !ok {
		return "missing"
	}
	if enabled {
		return "true"
	}
	return "false"
}

func readJSONObject(path string) (map[string]any, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return map[string]any{}, true, nil
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		return nil, true, err
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, true, nil
}

func writeJSONAtomic(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func RuntimePath(home string) string {
	path, err := install.RuntimePath(home)
	if err != nil {
		return ""
	}
	return path
}

func LegacyRuntimePath(home string) string {
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	name := "gtrace-agent"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(home, ".local", "bin", name)
}
