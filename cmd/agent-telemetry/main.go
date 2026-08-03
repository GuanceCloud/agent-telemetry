package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	claudehook "github.com/GuanceCloud/agent-telemetry/internal/adapters/claude/hook"
	codexhook "github.com/GuanceCloud/agent-telemetry/internal/adapters/codex/hook"
	"github.com/GuanceCloud/agent-telemetry/internal/install"
	"github.com/GuanceCloud/agent-telemetry/internal/manage"
	"github.com/GuanceCloud/agent-telemetry/internal/sharedconfig"
	"github.com/GuanceCloud/agent-telemetry/internal/selfupdate"
)

var version = "0.3.0-rc.4"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help") {
		printUsage(os.Stdout)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) >= 3 && os.Args[1] == "hook" {
		switch os.Args[2] {
		case "codex":
			os.Exit(codexhook.RunCLI())
		case "claude":
			os.Exit(claudehook.RunCLI())
		}
	}
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "install":
			if containsHelp(os.Args[2:]) {
				fmt.Print(install.InstallUsage())
				return
			}
			target, args := splitTarget(os.Args[2:])
			options, err := install.ParseInstallArgs(args)
			if err != nil {
				exitError(2, "parse install options", err)
			}
			result, err := manage.Install(target, options)
			if err != nil {
				exitError(1, "install agent-telemetry", err)
			}
			if err := saveSharedConfigDefaults(options); err != nil {
				exitError(1, "persist shared install defaults", err)
			}
			fmt.Printf("Installed agent-telemetry: %s\n", result.Runtime)
			if len(result.Targets) == 0 {
				fmt.Println("No supported Agent was detected; run \"agent-telemetry install <adapter>\" after installing one.")
			} else {
				fmt.Printf("Installed adapters: %s\n", strings.Join(result.Targets, ", "))
			}
			for _, warning := range result.Warnings {
				fmt.Printf("Note: %s\n", warning)
			}
			return
		case "discover", "list":
			statuses, err := manage.Discover("")
			if err != nil {
				exitError(1, "discover adapters", err)
			}
			printStatuses(statuses)
			return
		case "status":
			target := ""
			if len(os.Args) > 2 {
				target = os.Args[2]
			}
			if target == "" || target == "all" {
				printRuntimeStatus()
				statuses, err := manage.Discover("")
				if err != nil {
					exitError(1, "read adapter status", err)
				}
				printStatuses(statuses)
				return
			}
			status, err := manage.Status(target, "")
			if err != nil {
				exitError(1, "read adapter status", err)
			}
			printStatusDetail(status)
			return
		case "enable", "disable":
			if len(os.Args) != 3 {
				exitError(2, os.Args[1], fmt.Errorf("adapter name is required"))
			}
			enabled := os.Args[1] == "enable"
			path, err := manage.SetEnabled(os.Args[2], "", enabled)
			if err != nil {
				exitError(1, os.Args[1]+" adapter", err)
			}
			fmt.Printf("%s adapter %s in %s\n", titleState(enabled), os.Args[2], path)
			return
		case "uninstall", "remove":
			if err := runUninstall(os.Args[2:]); err != nil {
				exitError(1, "uninstall agent-telemetry", err)
			}
			return
		case "version":
			if err := runVersion(os.Args[2:]); err != nil {
				exitError(1, "version", err)
			}
			return
		}
	}
	printUsage(os.Stderr)
	os.Exit(2)
}

func printUsage(output *os.File) {
	fmt.Fprintln(output, `agent-telemetry

Usage:
  agent-telemetry install [claude|codex|all] [options]   install the shared runtime and register adapter hooks
  agent-telemetry discover                               detect supported Agents on this machine without changing config
  agent-telemetry status [claude|codex]                  show runtime install state and adapter hook/config status
  agent-telemetry enable <claude|codex>                  set enabled=true in the adapter gtrace.json
  agent-telemetry disable <claude|codex>                 set enabled=false in the adapter gtrace.json
  agent-telemetry uninstall [claude|codex|all] [--purge] remove managed hooks and delete adapter gtrace.json
  agent-telemetry hook <claude|codex>                    internal hook entrypoint invoked by Claude/Codex
  agent-telemetry version [-u]                           print the version or upgrade the shared runtime

Notes:
  --purge on uninstall also removes adapter state directories.
  Re-running install upgrades the shared runtime; there is no per-adapter update command.
  Use "agent-telemetry version -u" to upgrade from GitHub Release assets.
  Run "agent-telemetry install --help" for installation options.`)
}

func splitTarget(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return strings.ToLower(strings.TrimSpace(args[0])), args[1:]
	}
	return "", args
}

func containsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func printStatuses(statuses []manage.AdapterStatus) {
	fmt.Println("ADAPTER  DETECTED  INSTALLED  ENABLED  CONFIG")
	for _, status := range statuses {
		fmt.Printf("%-8s %-9s %-10s %-8s %s\n",
			status.Name,
			yesNo(status.Detected),
			yesNo(status.Installed),
			status.Enabled,
			status.ConfigFile,
		)
	}
}

func printStatusDetail(status manage.AdapterStatus) {
	home, _ := os.UserHomeDir()
	runtimePath := manage.RuntimePath("")
	runtimeVersion := "-"
	if runtimePath != "" {
		if _, err := os.Stat(runtimePath); err == nil {
			runtimeVersion = version
		}
	}
	fmt.Print(formatStatusDetail(status, runtimeVersion, runtimePath, home))
}

func formatStatusDetail(status manage.AdapterStatus, runtimeVersion, runtimePath, home string) string {
	var builder strings.Builder
	writeStatusLine(&builder, "Agent", status.Name)
	writeStatusLine(&builder, "Command", status.Name)
	writeStatusLine(&builder, "Supported", "yes")
	writeStatusLine(&builder, "Installed", yesNo(status.Installed))
	writeStatusLine(&builder, "Version", firstNonEmpty(runtimeVersion, "-"))
	writeStatusLine(&builder, "Config", shortenPath(status.ConfigFile, home))
	writeStatusLine(&builder, "Path", shortenPath(runtimePath, home))
	writeStatusLine(&builder, "Enabled", status.Enabled)
	return builder.String()
}

func writeStatusLine(builder *strings.Builder, key, value string) {
	builder.WriteString(fmt.Sprintf("%-9s: %s\n", key, value))
}

func printRuntimeStatus() {
	path := manage.RuntimePath("")
	installed := false
	if path != "" {
		_, err := os.Stat(path)
		installed = err == nil
	}
	fmt.Printf("PLUGIN           VERSION       INSTALLED  PATH\n")
	fmt.Printf("%-16s %-13s %-10s %s\n\n", "agent-telemetry", version, yesNo(installed), path)
}

func shortenPath(path, home string) string {
	if path == "" {
		return "-"
	}
	if home != "" && path == home {
		return "~"
	}
	if home != "" && strings.HasPrefix(path, home+"/") {
		return "~/" + strings.TrimPrefix(path, home+"/")
	}
	return path
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func titleState(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

func runUninstall(args []string) error {
	target, flagArgs := splitTarget(args)
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	purge := fs.Bool("purge", false, "remove adapter state directories after deleting config")
	home := fs.String("home", "", "override user home")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	results, err := manage.Remove(target, *home, *purge)
	if err != nil {
		return err
	}
	for _, result := range results {
		fmt.Printf("Removed %s adapter hook; config removed: %t; state purged: %t\n",
			result.Adapter, result.ConfigRemoved, result.StatePurged)
	}
	if target == "" || target == "all" {
		fmt.Println("Removed the shared agent-telemetry runtime.")
	}
	return nil
}

func runVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	upgrade := fs.Bool("u", false, "upgrade the shared runtime from GitHub Release")
	fs.BoolVar(upgrade, "upgrade", false, "upgrade the shared runtime from GitHub Release")
	home := fs.String("home", "", "override user home")
	releaseVersion := fs.String("release-version", os.Getenv("AGENT_TELEMETRY_RELEASE_VERSION"), "release version to install, or latest")
	githubRepo := fs.String("github-repo", os.Getenv("AGENT_TELEMETRY_GITHUB_REPO"), "GitHub repository in owner/repo form")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !*upgrade {
		fmt.Println(version)
		return nil
	}
	result, err := selfupdate.Upgrade(selfupdate.Options{
		Home:           *home,
		ReleaseVersion: *releaseVersion,
		GitHubRepo:     *githubRepo,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Upgraded agent-telemetry to %s\n", result.InstalledVersion)
	fmt.Printf("Runtime: %s\n", result.Runtime)
	if len(result.Targets) == 0 {
		fmt.Println("Reconciled adapters: none detected")
	} else {
		fmt.Printf("Reconciled adapters: %s\n", strings.Join(result.Targets, ", "))
	}
	if strings.TrimSpace(result.ConfigSource) != "" {
		fmt.Printf("Bootstrap config: %s\n", shortenPath(result.ConfigSource, *home))
	}
	for _, warning := range result.Warnings {
		fmt.Printf("Note: %s\n", warning)
	}
	return nil
}

func saveSharedConfigDefaults(options install.CodexOptions) error {
	cfg := sharedconfig.Config{
		Endpoint:           strings.TrimSpace(options.Endpoint),
		TracePath:          strings.Trim(strings.TrimSpace(options.TracePath), "/"),
		MetricsPath:        strings.Trim(strings.TrimSpace(options.MetricsPath), "/"),
		InstallType:        strings.TrimSpace(options.InstallType),
		XToken:             strings.TrimSpace(options.XToken),
		Headers:            toStringMap(options.Headers),
		ResourceAttributes: toStringMap(options.ResourceAttributes),
		CaptureContent:     strings.TrimSpace(options.CaptureContent),
		MaxChars:           options.MaxChars,
	}
	if options.Enabled != nil {
		value := *options.Enabled
		cfg.Enabled = &value
	}
	if !sharedconfig.HasMeaningfulValues(cfg) {
		return nil
	}
	_, err := sharedconfig.Save(options.Home, cfg)
	return err
}

func toStringMap(entries []string) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	result := map[string]string{}
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func exitError(code int, operation string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", operation, err)
	os.Exit(code)
}
