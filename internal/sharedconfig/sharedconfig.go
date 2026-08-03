package sharedconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirName  = ".agent-telemetry"
	fileName = "config.json"
)

type Config struct {
	Endpoint           string            `json:"endpoint,omitempty"`
	TracePath          string            `json:"trace_path,omitempty"`
	MetricsPath        string            `json:"metrics_path,omitempty"`
	InstallType        string            `json:"install_type,omitempty"`
	XToken             string            `json:"x_token,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	ResourceAttributes map[string]string `json:"resource_attributes,omitempty"`
	CaptureContent     string            `json:"capture_content,omitempty"`
	MaxChars           int               `json:"max_chars,omitempty"`
	Enabled            *bool             `json:"enabled,omitempty"`
}

func Path(home string) (string, error) {
	if strings.TrimSpace(home) == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(home, dirName, fileName), nil
}

func Load(home string) (Config, string, error) {
	path, err := Path(home)
	if err != nil {
		return Config{}, "", err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, path, nil
		}
		return Config{}, path, err
	}
	var cfg Config
	if len(strings.TrimSpace(string(body))) == 0 {
		return Config{}, path, nil
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return Config{}, path, err
	}
	return cfg, path, nil
}

func Save(home string, cfg Config) (string, error) {
	path, err := Path(home)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')
	temp := path + ".tmp"
	if err := os.WriteFile(temp, body, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(temp, path); err != nil {
		return "", err
	}
	return path, nil
}

func HasMeaningfulValues(cfg Config) bool {
	return strings.TrimSpace(cfg.Endpoint) != "" ||
		strings.TrimSpace(cfg.TracePath) != "" ||
		strings.TrimSpace(cfg.MetricsPath) != "" ||
		strings.TrimSpace(cfg.InstallType) != "" ||
		strings.TrimSpace(cfg.XToken) != "" ||
		len(cfg.Headers) > 0 ||
		len(cfg.ResourceAttributes) > 0 ||
		strings.TrimSpace(cfg.CaptureContent) != "" ||
		cfg.MaxChars > 0 ||
		cfg.Enabled != nil
}
