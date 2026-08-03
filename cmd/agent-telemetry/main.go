package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	claudehook "github.com/GuanceCloud/agent-telemetry/internal/adapters/claude/hook"
	codexhook "github.com/GuanceCloud/agent-telemetry/internal/adapters/codex/hook"
	"github.com/GuanceCloud/agent-telemetry/internal/install"
	"github.com/GuanceCloud/agent-telemetry/internal/manage"
)

var version = "0.3.0-rc.2"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help") {
		printUsage(os.Stdout)
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
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
			printRuntimeStatus()
			if target == "" || target == "all" {
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
			printStatuses([]manage.AdapterStatus{status})
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
		}
	}
	printUsage(os.Stderr)
	os.Exit(2)
}

func printUsage(output *os.File) {
	fmt.Fprintln(output, `agent-telemetry

Usage:
  agent-telemetry install [claude|codex|all] [options]
  agent-telemetry discover
  agent-telemetry status [claude|codex]
  agent-telemetry enable <claude|codex>
  agent-telemetry disable <claude|codex>
  agent-telemetry uninstall [claude|codex|all] [--purge]
  agent-telemetry hook <claude|codex>
  agent-telemetry version

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
	purge := fs.Bool("purge", false, "remove adapter configuration and state")
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
		fmt.Printf("Removed %s adapter hook; config preserved: %t\n", result.Adapter, !result.ConfigPurged)
	}
	if target == "" || target == "all" {
		fmt.Println("Removed the shared agent-telemetry runtime.")
	}
	return nil
}

func exitError(code int, operation string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", operation, err)
	os.Exit(code)
}
