package install

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

type RemoveResult struct {
	Adapter      string
	HookFile     string
	ConfigFile   string
	HookRemoved  bool
	ConfigRemoved bool
	StatePurged  bool
}

func RemoveAdapter(adapter, home string, purge bool) (RemoveResult, error) {
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return RemoveResult{}, err
		}
	}
	switch adapter {
	case "claude":
		return removeClaude(home, purge)
	case "codex":
		return removeCodex(home, purge)
	default:
		return RemoveResult{}, errors.New("unsupported adapter " + adapter)
	}
}

func RemoveRuntime(home string) error {
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	names := []string{"agent-telemetry", "gtrace-agent"}
	if runtime.GOOS == "windows" {
		names = []string{"agent-telemetry.exe", "gtrace-agent.exe"}
	}
	for _, name := range names {
		path := filepath.Join(home, ".local", "bin", name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func removeClaude(home string, purge bool) (RemoveResult, error) {
	result := RemoveResult{
		Adapter:    "claude",
		HookFile:   filepath.Join(home, ".claude", "settings.json"),
		ConfigFile: filepath.Join(home, ".claude", "gtrace.json"),
	}
	settings, exists, err := readJSONObjectIfExists(result.HookFile)
	if err != nil {
		return result, err
	}
	if exists {
		hooks, _ := settings["hooks"].(map[string]any)
		for _, event := range []string{"Stop", "SessionEnd"} {
			groups, _ := hooks[event].([]any)
			next, changed := removeManagedGroups(groups, managedClaudeHook)
			if changed {
				hooks[event] = next
				result.HookRemoved = true
			}
		}
		if result.HookRemoved {
			if err := writeJSONAtomic(result.HookFile, settings); err != nil {
				return result, err
			}
		}
	}
	if err := removeFileIfExists(result.ConfigFile); err != nil {
		return result, err
	}
	result.ConfigRemoved = true
	if purge {
		for _, name := range []string{"agent-telemetry", "gtrace-agent"} {
			if err := os.RemoveAll(filepath.Join(home, ".claude", "state", name)); err != nil {
				return result, err
			}
		}
		result.StatePurged = true
	}
	return result, nil
}

func removeCodex(home string, purge bool) (RemoveResult, error) {
	result := RemoveResult{
		Adapter:    "codex",
		HookFile:   filepath.Join(home, ".codex", "hooks.json"),
		ConfigFile: filepath.Join(home, ".codex", "gtrace.json"),
	}
	settings, exists, err := readJSONObjectIfExists(result.HookFile)
	if err != nil {
		return result, err
	}
	if exists {
		hooks, _ := settings["hooks"].(map[string]any)
		groups, _ := hooks["Stop"].([]any)
		next, changed := removeManagedGroups(groups, managedCodexHook)
		if changed {
			hooks["Stop"] = next
			result.HookRemoved = true
			if err := writeJSONAtomic(result.HookFile, settings); err != nil {
				return result, err
			}
		}
	}
	if err := removeFileIfExists(result.ConfigFile); err != nil {
		return result, err
	}
	result.ConfigRemoved = true
	if purge {
		for _, name := range []string{"agent-telemetry", "gtrace-agent"} {
			if err := os.RemoveAll(filepath.Join(home, ".codex", "state", name)); err != nil {
				return result, err
			}
		}
		result.StatePurged = true
	}
	return result, nil
}

func removeManagedGroups(groups []any, managed func(any) bool) ([]any, bool) {
	next := make([]any, 0, len(groups))
	changed := false
	for _, group := range groups {
		if managed(group) {
			changed = true
			continue
		}
		next = append(next, group)
	}
	return next, changed
}

func removeFileIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
