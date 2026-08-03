package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/GuanceCloud/agent-telemetry/internal/install"
	"github.com/GuanceCloud/agent-telemetry/internal/manage"
	"github.com/GuanceCloud/agent-telemetry/internal/sharedconfig"
)

const defaultGitHubRepo = "GuanceCloud/agent-telemetry"

type Options struct {
	Home           string
	ReleaseVersion string
	GitHubRepo     string
	BaseURL        string
	HTTPClient     *http.Client
}

type Result struct {
	InstalledVersion string
	Runtime          string
	Targets          []string
	Warnings         []string
	ConfigSource     string
}

func Upgrade(options Options) (Result, error) {
	home := strings.TrimSpace(options.Home)
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Result{}, err
		}
	}
	releaseVersion := normalizeReleaseVersion(options.ReleaseVersion)
	repo := strings.TrimSpace(options.GitHubRepo)
	if repo == "" {
		repo = defaultGitHubRepo
	}

	runtimePath, err := install.RuntimePath(home)
	if err != nil {
		return Result{}, err
	}
	if runtime.GOOS == "windows" {
		currentPath, currentErr := os.Executable()
		if currentErr == nil {
			absCurrent, _ := filepath.Abs(currentPath)
			absRuntime, _ := filepath.Abs(runtimePath)
			if absCurrent == absRuntime {
				return Result{}, errors.New("self-upgrade from the installed Windows runtime is not supported; use install-release.ps1")
			}
		}
	}

	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	archiveName, binaryName, err := currentPlatformAsset(releaseVersion)
	if err != nil {
		return Result{}, err
	}
	baseURL := releaseBaseURL(options.BaseURL, repo, releaseVersion)
	statusesBefore, err := manage.Discover(home)
	if err != nil {
		return Result{}, err
	}

	workDir, err := os.MkdirTemp("", "agent-telemetry-upgrade-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(workDir)

	sumsPath := filepath.Join(workDir, "SHA256SUMS")
	archivePath := filepath.Join(workDir, archiveName)
	if err := downloadFile(client, baseURL+"/SHA256SUMS", sumsPath); err != nil {
		return Result{}, err
	}
	if err := downloadFile(client, baseURL+"/"+archiveName, archivePath); err != nil {
		return Result{}, err
	}
	if err := verifyChecksum(archiveName, sumsPath, archivePath); err != nil {
		return Result{}, err
	}
	extractedBinary, extractedVersion, err := extractArchive(archivePath, workDir, binaryName)
	if err != nil {
		return Result{}, err
	}

	installResult, err := manage.Install("", install.CodexOptions{
		Home:                  home,
		SourceExecutable:      extractedBinary,
		DestinationExecutable: runtimePath,
		NoConfig:              true,
		SkipTrust:             true,
	})
	if err != nil {
		return Result{}, err
	}
	warnings := append([]string{}, installResult.Warnings...)
	configSource := ""
	defaults, defaultsOK, defaultsSource, defaultsErr := resolveBootstrapConfig(home)
	if defaultsErr != nil {
		warnings = append(warnings, "Bootstrap config defaults were unavailable: "+defaultsErr.Error())
	}
	if defaultsOK {
		configSource = defaultsSource
		for _, status := range missingConfigStatuses(statusesBefore) {
			bootstrap := defaultsToInstallOptions(defaults)
			bootstrap.Home = home
			bootstrap.SourceExecutable = runtimePath
			bootstrap.DestinationExecutable = runtimePath
			bootstrap.SkipTrust = true
			targetResult, installErr := manage.Install(status.Name, bootstrap)
			if installErr != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to bootstrap %s config during upgrade: %v", status.Name, installErr))
				continue
			}
			warnings = append(warnings, targetResult.Warnings...)
		}
	} else {
		for _, status := range missingConfigStatuses(statusesBefore) {
			warnings = append(warnings,
				fmt.Sprintf("%s hook was upgraded but %s is still missing; run \"agent-telemetry install %s --endpoint <url> --x-token <token> --enable\" once to create it",
					status.Name, status.ConfigFile, status.Name))
		}
	}
	return Result{
		InstalledVersion: firstNonEmpty(extractedVersion, releaseVersion),
		Runtime:          installResult.Runtime,
		Targets:          installResult.Targets,
		Warnings:         uniqueStrings(warnings),
		ConfigSource:     configSource,
	}, nil
}

func resolveBootstrapConfig(home string) (sharedconfig.Config, bool, string, error) {
	cfg, path, err := sharedconfig.Load(home)
	if err != nil {
		return sharedconfig.Config{}, false, "", err
	}
	if sharedconfig.HasMeaningfulValues(cfg) {
		return cfg, true, path, nil
	}
	cfg, path, err = inferConfigFromInstalledAgents(home)
	if err != nil {
		return sharedconfig.Config{}, false, "", err
	}
	if sharedconfig.HasMeaningfulValues(cfg) {
		return cfg, true, path, nil
	}
	return sharedconfig.Config{}, false, "", nil
}

func inferConfigFromInstalledAgents(home string) (sharedconfig.Config, string, error) {
	statuses, err := manage.Discover(home)
	if err != nil {
		return sharedconfig.Config{}, "", err
	}
	for _, status := range statuses {
		cfg, ok, err := configFromJSONFile(status.ConfigFile)
		if err != nil {
			return sharedconfig.Config{}, "", err
		}
		if ok {
			return cfg, status.ConfigFile, nil
		}
	}
	return sharedconfig.Config{}, "", nil
}

func configFromJSONFile(path string) (sharedconfig.Config, bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sharedconfig.Config{}, false, nil
		}
		return sharedconfig.Config{}, false, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return sharedconfig.Config{}, false, nil
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		return sharedconfig.Config{}, false, nil
	}
	cfg := sharedconfig.Config{
		Endpoint:           trimJSONString(value["endpoint"]),
		TracePath:          trimJSONString(value["tracePath"]),
		MetricsPath:        trimJSONString(value["metricsPath"]),
		CaptureContent:     trimJSONString(firstPresent(value, "captureContent", "capture_content")),
		MaxChars:           intJSON(value["max_chars"]),
		Headers:            stringMap(value["headers"]),
		ResourceAttributes: stringMap(value["resourceAttributes"]),
	}
	if enabled, ok := boolJSON(value["enabled"]); ok {
		cfg.Enabled = &enabled
	}
	if cfg.TracePath == "v1/traces" || cfg.MetricsPath == "v1/metrics" {
		cfg.InstallType = "otlp"
	} else {
		cfg.InstallType = "gtrace"
	}
	if token := cfg.Headers["X-Token"]; strings.TrimSpace(token) != "" {
		cfg.XToken = token
		delete(cfg.Headers, "X-Token")
	}
	if !sharedconfig.HasMeaningfulValues(cfg) {
		return sharedconfig.Config{}, false, nil
	}
	return cfg, true, nil
}

func defaultsToInstallOptions(cfg sharedconfig.Config) install.CodexOptions {
	options := install.CodexOptions{
		Endpoint:           cfg.Endpoint,
		TracePath:          cfg.TracePath,
		MetricsPath:        cfg.MetricsPath,
		InstallType:        firstNonEmpty(cfg.InstallType, "gtrace"),
		XToken:             cfg.XToken,
		CaptureContent:     cfg.CaptureContent,
		MaxChars:           cfg.MaxChars,
		Headers:            stringMapEntries(cfg.Headers),
		ResourceAttributes: stringMapEntries(cfg.ResourceAttributes),
	}
	if cfg.Enabled != nil {
		value := *cfg.Enabled
		options.Enabled = &value
	}
	return options
}

func missingConfigStatuses(statuses []manage.AdapterStatus) []manage.AdapterStatus {
	result := make([]manage.AdapterStatus, 0, len(statuses))
	for _, status := range statuses {
		if !status.Detected {
			continue
		}
		if strings.TrimSpace(status.Enabled) == "-" {
			result = append(result, status)
		}
	}
	return result
}

func stringMapEntries(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	entries := make([]string, 0, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		entries = append(entries, key+"="+value)
	}
	sort.Strings(entries)
	return entries
}

func stringMap(value any) map[string]string {
	current, ok := value.(map[string]any)
	if !ok || len(current) == 0 {
		return nil
	}
	result := make(map[string]string, len(current))
	for key, raw := range current {
		text := trimJSONString(raw)
		if text != "" {
			result[key] = text
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func trimJSONString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func intJSON(value any) int {
	switch current := value.(type) {
	case float64:
		return int(current)
	case int:
		return current
	default:
		return 0
	}
}

func boolJSON(value any) (bool, bool) {
	current, ok := value.(bool)
	return current, ok
}

func firstPresent(value map[string]any, keys ...string) any {
	for _, key := range keys {
		if current, ok := value[key]; ok {
			return current
		}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeReleaseVersion(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "latest"
	}
	if strings.EqualFold(normalized, "latest") {
		return "latest"
	}
	return strings.TrimPrefix(normalized, "v")
}

func releaseBaseURL(override, repo, releaseVersion string) string {
	if trimmed := strings.TrimRight(strings.TrimSpace(override), "/"); trimmed != "" {
		return trimmed
	}
	if releaseVersion == "latest" {
		return "https://github.com/" + repo + "/releases/latest/download"
	}
	return "https://github.com/" + repo + "/releases/download/v" + releaseVersion
}

func currentPlatformAsset(releaseVersion string) (string, string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", "", fmt.Errorf("unsupported operating system: %s", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
	platform := goos + "-" + goarch
	binaryName := "agent-telemetry"
	if goos == "windows" {
		binaryName += ".exe"
	}
	if releaseVersion == "latest" {
		if goos == "windows" {
			return "agent-telemetry-" + platform + ".zip", binaryName, nil
		}
		return "agent-telemetry-" + platform + ".tar.gz", binaryName, nil
	}
	if goos == "windows" {
		return "agent-telemetry-v" + releaseVersion + "-" + platform + ".zip", binaryName, nil
	}
	return "agent-telemetry-v" + releaseVersion + "-" + platform + ".tar.gz", binaryName, nil
}

func downloadFile(client *http.Client, url, path string) error {
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, response.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(file, response.Body); err != nil {
		return err
	}
	return nil
}

func verifyChecksum(assetName, sumsPath, archivePath string) error {
	expected, err := expectedChecksum(assetName, sumsPath)
	if err != nil {
		return err
	}
	actual, err := fileChecksum(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}

func expectedChecksum(assetName, sumsPath string) (string, error) {
	file, err := os.Open(sumsPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == assetName {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("missing checksum entry for %s", assetName)
}

func fileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func extractArchive(archivePath, workDir, binaryName string) (string, string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZipArchive(archivePath, workDir, binaryName)
	}
	return extractTarArchive(archivePath, workDir, binaryName)
}

func extractTarArchive(archivePath, workDir, binaryName string) (string, string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", "", err
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	var extractedBinary string
	var extractedVersion string
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", err
		}
		name := normalizeArchivePath(header.Name)
		switch name {
		case "bin/" + binaryName:
			extractedBinary = filepath.Join(workDir, binaryName)
			if err := writeExtractedFile(extractedBinary, reader, 0o755); err != nil {
				return "", "", err
			}
		case "VERSION":
			body, readErr := io.ReadAll(reader)
			if readErr != nil {
				return "", "", readErr
			}
			extractedVersion = strings.TrimSpace(string(body))
		}
	}
	if extractedBinary == "" {
		return "", "", fmt.Errorf("release archive %s does not contain %s", archivePath, binaryName)
	}
	return extractedBinary, extractedVersion, nil
}

func extractZipArchive(archivePath, workDir, binaryName string) (string, string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", "", err
	}
	defer reader.Close()
	var extractedBinary string
	var extractedVersion string
	for _, file := range reader.File {
		name := normalizeArchivePath(file.Name)
		switch name {
		case "bin/" + binaryName:
			handle, openErr := file.Open()
			if openErr != nil {
				return "", "", openErr
			}
			extractedBinary = filepath.Join(workDir, binaryName)
			writeErr := writeExtractedFile(extractedBinary, handle, 0o755)
			closeErr := handle.Close()
			if writeErr != nil {
				return "", "", writeErr
			}
			if closeErr != nil {
				return "", "", closeErr
			}
		case "VERSION":
			handle, openErr := file.Open()
			if openErr != nil {
				return "", "", openErr
			}
			body, readErr := io.ReadAll(handle)
			closeErr := handle.Close()
			if readErr != nil {
				return "", "", readErr
			}
			if closeErr != nil {
				return "", "", closeErr
			}
			extractedVersion = strings.TrimSpace(string(body))
		}
	}
	if extractedBinary == "" {
		return "", "", fmt.Errorf("release archive %s does not contain %s", archivePath, binaryName)
	}
	return extractedBinary, extractedVersion, nil
}

func writeExtractedFile(path string, input io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, input)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return closeErr
	}
	return os.Chmod(path, mode)
}

func normalizeArchivePath(path string) string {
	path = strings.ReplaceAll(path, `\`, "/")
	path = strings.TrimPrefix(path, "./")
	return strings.TrimPrefix(path, "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
