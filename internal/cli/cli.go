// Package cli implements the command-line interface for xpm.
//
// This package handles argument parsing, command dispatch, and user interaction.
// It provides commands for installing packages, checking package availability,
// and diagnosing the local environment. The full command list, with aliases,
// is commandTable in man.go.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/doctor"
	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
	"github.com/manifoldco/promptui"
)

// Version is the current version of xpm.
// This can be overridden at build time using:
//
//	go build -ldflags "-X github.com/crenspire/xpm/internal/cli.Version=1.0.0"
var Version = "0.0.1"

// cfg holds the loaded configuration.
var cfg config.Config

// ANSI color codes
const (
	colorReset      = "\033[0m"
	colorBold       = "\033[1m"
	colorCyan       = "\033[36m"
	colorYellow     = "\033[33m"
	colorGreen      = "\033[32m"
	colorBlue       = "\033[34m"
	colorMagenta    = "\033[35m"
	colorBrightCyan = "\033[96m"
)

// printBanner displays a styled banner with the xpm name in ASCII art style.
func printBanner() {
	fmt.Println()

	// XPM in clear ASCII art - simple and readable
	fmt.Printf("%s%s", colorYellow, colorBold)
	fmt.Println("  ██╗  ██╗ ██████╗  ███╗   ███╗")
	fmt.Println("  ╚██╗██╔╝ ██╔══██╗ ████╗ ████║")
	fmt.Println("   ╚███╔╝  ██████╔╝ ██╔████╔██║")
	fmt.Println("   ██╔██╗  ██╔═══╝  ██║╚██╔╝██║")
	fmt.Println("  ██╔╝ ██╗ ██║      ██║ ╚═╝ ██║")
	fmt.Println("  ╚═╝  ╚═╝ ╚═╝      ╚═╝     ╚═╝")
	fmt.Println()

	// Version and tagline
	fmt.Printf("%s%s", colorCyan, colorBold)
	fmt.Printf("     Cross-ecosystem package manager")
	fmt.Printf("%s\n", colorReset)
	fmt.Printf("%s%s", colorGreen, colorBold)
	fmt.Printf("              v%s\n", Version)
	fmt.Print(colorReset)
	fmt.Println()
}

// usage prints the help message to stdout.
func usage() {
	fmt.Printf("%s%sUsage:%s %sxpm%s <command> [options] [arguments]\n\n", colorBold, colorCyan, colorReset, colorBold, colorReset)

	fmt.Printf("%s%sCommands:%s\n", colorBold, colorYellow, colorReset)

	printCommandTable()
	fmt.Println()
	fmt.Println(experimentalNote)
	fmt.Printf("\n%sRun%s '%sxpm man <command>%s' for detailed help on a specific command.\n", colorBold, colorReset, colorCyan, colorReset)
}

// splitGlobalFlags consumes xpm's own flags (-v, --verbose, --version, -V)
// only while they appear BEFORE the subcommand. Everything from the
// subcommand onward is returned untouched, so `xpm run test -- --version`
// passes --version to the script instead of printing xpm's version.
// A lone `-v` means version; `-v` followed by a command means verbose.
func splitGlobalFlags(raw []string) (args []string, verbose, showVersion bool) {
	for i, a := range raw {
		switch a {
		case "--version", "-V":
			showVersion = true
		case "-v":
			if len(raw) == 1 {
				showVersion = true
			} else {
				verbose = true
			}
		case "--verbose":
			verbose = true
		default:
			return raw[i:], verbose, showVersion
		}
	}
	return nil, verbose, showVersion
}

// Run is the main entry point for the CLI.
// It parses arguments, loads configuration, and dispatches to the appropriate command.
// Returns an exit code (0 for success, non-zero for errors).
func Run() int {
	rawArgs := os.Args[1:]

	args, verbose, showVersion := splitGlobalFlags(rawArgs)
	logx.Verbose = verbose

	cfg = config.Load()
	cfg.Interactive = effectiveInteractive(cfg.Interactive, isInteractiveTerminal())
	logx.Info("config loaded: %+v", cfg)

	// Show banner and version if no args or version flag
	if len(rawArgs) == 0 {
		printBanner()
		usage()
		return 1
	}

	if showVersion {
		printBanner()
		return 0
	}

	if len(args) == 0 {
		printBanner()
		usage()
		return 1
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "i", "install":
		return cmdInstall(rest)
	case "ci":
		return cmdCleanInstall(rest)
	case "r", "run":
		return cmdRun(rest)
	case "w", "which":
		return cmdWhich(rest)
	case "l", "list":
		return cmdList(rest)
	case "u", "update":
		return cmdUpdate(rest)
	case "rm", "remove":
		return cmdRemove(rest)
	case "info":
		return cmdInfo(rest)
	case "lock":
		return cmdLock(rest)
	case "g", "graph":
		return cmdGraph(rest)
	case "s", "search":
		return cmdSearch(rest)
	case "workspaces":
		return cmdWorkspaces(rest)
	case "env":
		return cmdEnv(rest)
	case "d", "doctor":
		return cmdDoctor(rest)
	case "config":
		return cmdConfig(rest)
	case "version", "--version", "-V":
		printBanner()
		return 0
	case "help", "-h", "--help":
		printBanner()
		usage()
		return 0
	case "man":
		return cmdMan(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printBanner()
		usage()
		return 1
	}
}

// parsePackageVersion parses a package string like "axios@1.0.0" into name and version.
// If no version is specified, version will be empty.
// Validates input to handle edge cases safely.
//
// Edge cases handled:
//   - Empty string: returns ("", "")
//   - Just "@": returns ("", "")
//   - "@" at start (scoped packages): correctly parses @scope/package@version
//   - "@" at end: returns (package, "")
//   - Multiple "@": uses last "@" as separator for regular packages
//
// Examples:
//   - "axios@1.0.0" -> ("axios", "1.0.0")
//   - "@scope/pkg@1.0.0" -> ("@scope/pkg", "1.0.0")
//   - "axios@" -> ("axios", "")
//   - "@" -> ("", "")
func parsePackageVersion(pkgStr string) (name string, version string) {
	// Validate input length
	if len(pkgStr) == 0 {
		return "", ""
	}

	// Handle scoped packages like @scope/package@version
	if strings.HasPrefix(pkgStr, "@") {
		// Edge case: just "@"
		if len(pkgStr) == 1 {
			return "", ""
		}
		// Find the second @ which would be the version separator
		rest := pkgStr[1:]
		idx := strings.Index(rest, "@")
		if idx == -1 {
			return pkgStr, ""
		}
		// Validate we have content after the @
		if idx+1 >= len(pkgStr) {
			return pkgStr, ""
		}
		return pkgStr[:idx+1], rest[idx+1:]
	}

	// Regular package@version
	idx := strings.LastIndex(pkgStr, "@")
	if idx == -1 {
		return pkgStr, ""
	}
	// Validate we have content before and after the @
	if idx == 0 {
		// Package name starts with @ but not scoped format
		return "", ""
	}
	if idx+1 >= len(pkgStr) {
		// @ at the end, no version
		return pkgStr[:idx], ""
	}
	return pkgStr[:idx], pkgStr[idx+1:]
}

func cmdWhich(args []string) int {
	pkg, ok := exactlyOneArg("which", args)
	if !ok {
		return 1
	}
	fmt.Printf("Searching for %q...\n\n", pkg)

	searchOpts := search.OptionsFromConfig(cfg)
	rep, err := lookupReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	st := classify(rep, searchOpts)
	if len(rep.Results) == 0 {
		fmt.Printf("No matches found for %s.\n", pkg)
		fmt.Print(formatAvailability(st))
		return 1
	}

	fmt.Println("Found in:")
	for _, r := range rep.Results {
		line := fmt.Sprintf("- %s: %s", r.Manager, r.Name)
		if v := r.Extra["version"]; v != "" {
			line += " @" + v
		}
		if r.Info != "" {
			line += " - " + r.Info
		}
		fmt.Println(line)

		switch r.Manager {
		case pm.Npm:
			fmt.Println("  → Also available via: yarn, pnpm, bun")
		case pm.Pip:
			fmt.Println("  → Also available via: poetry, pipenv")
		case pm.Maven:
			fmt.Println("  → Add to pom.xml or build.gradle as a dependency.")
		case pm.Cargo:
			fmt.Println("  → Add to Cargo.toml under [dependencies].")
		case pm.Composer:
			fmt.Println("  → Add to composer.json or run `composer require ...`.")
		}
	}
	fmt.Print(formatAvailability(st))
	return 0
}

func cmdDoctor(_ []string) int {
	// Build doctor config from xpm config
	doctorCfg := doctor.Config{
		SkipEnv:       cfg.Doctor.SkipEnv,
		SkipSecurity:  cfg.Doctor.SkipSecurity,
		SkipConflicts: cfg.Doctor.SkipConflicts,
		SkipDrift:     cfg.Doctor.SkipDrift,
	}

	// Run diagnostics
	report := doctor.Run(doctorCfg)

	// Print the report
	doctor.PrintReport(report, doctorCfg)

	// Return non-zero if there are issues
	if doctor.HasIssues(report) {
		return 1
	}
	return 0
}

// askYesNo is a seam for tests.
var askYesNo = promptYesNo

func promptYesNo(label string) (bool, error) {
	if !cfg.Interactive {
		logx.Info("non-interactive mode: auto-answer No for prompt %q", label)
		return false, nil
	}
	prompt := promptui.Select{
		Label: label,
		Items: []string{"Yes", "No"},
	}
	index, _, err := prompt.Run()
	if err != nil {
		return false, err
	}
	return index == 0, nil
}

func preferOrderMap(prefer []string) map[string]int {
	m := map[string]int{}
	const big = 1000
	for _, meta := range pm.AllMetas() {
		m[string(meta.ID)] = big
	}
	for i, name := range prefer {
		m[name] = i
	}
	return m
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pickPreferredPM(candidates []pm.ID, prefer []string) pm.ID {
	if len(prefer) == 0 {
		return ""
	}
	prefSet := map[string]struct{}{}
	for _, p := range prefer {
		prefSet[p] = struct{}{}
	}
	for _, p := range prefer {
		for _, c := range candidates {
			if string(c) == p {
				return c
			}
		}
	}
	return ""
}
