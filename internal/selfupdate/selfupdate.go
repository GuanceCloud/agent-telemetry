package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/GuanceCloud/agent-telemetry/internal/install"
	"github.com/GuanceCloud/agent-telemetry/internal/manage"
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
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		InstalledVersion: firstNonEmpty(extractedVersion, releaseVersion),
		Runtime:          installResult.Runtime,
		Targets:          installResult.Targets,
		Warnings:         installResult.Warnings,
	}, nil
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
