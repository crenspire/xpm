// Package cli implements the command-line interface for xpm.
//
// This package handles argument parsing, command dispatch, and user interaction.
// It provides commands for installing packages, checking package availability,
// and diagnosing the local environment.
//
// Commands:
//   - install: Install packages or project dependencies
//   - which: Check which ecosystems have a package
//   - doctor: Check installed package managers
//   - version: Show version information
//   - help: Show usage information
package cli

import (
	"fmt"
	"os"
	"os/exec"
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
	fmt.Printf("     Universal Package Manager")
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

	commands := []struct {
		name        string
		aliases     []string
		description string
	}{
		{"install", []string{"i"}, "Install packages or project dependencies"},
		{"run", []string{"r"}, "Run project scripts (from package.json, etc.)"},
		{"which", []string{"w"}, "Check which ecosystems have a package"},
		{"list", []string{"l"}, "List installed packages for current project"},
		{"update", []string{"u"}, "Update packages in the current project"},
		{"remove", []string{"rm"}, "Remove a package from the current project"},
		{"info", nil, "Show detailed package information"},
		{"lock", nil, "Generate or verify unified lockfile (xpm-lock.yaml)"},
		{"cache", nil, "Manage dependency cache (tree, size, clean, gc)"},
		{"graph", []string{"g"}, "Show unified dependency graph across all ecosystems"},
		{"search", []string{"s"}, "Interactive TUI package search"},
		{"workspaces", nil, "List detected workspaces/monorepos"},
		{"env", nil, "Manage runtime versions (node, python, go, java, rust, bun, deno)"},
		{"doctor", []string{"d"}, "Comprehensive environment & project diagnostics"},
		{"config", nil, "View or edit configuration"},
		{"version", []string{"-v"}, "Show version information"},
		{"help", []string{"-h"}, "Show this help message"},
		{"man", nil, "Show detailed manual for commands"},
	}

	for _, cmd := range commands {
		name := cmd.name
		if len(cmd.aliases) > 0 {
			name += ", " + strings.Join(cmd.aliases, ", ")
		}
		fmt.Printf("  %s%s%-20s%s %s\n", colorBold, colorCyan, name, colorReset, cmd.description)
	}

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
	case "cc":
		return cmdCache([]string{"clean"})
	case "cg":
		return cmdCache([]string{"gc"})
	case "cache":
		return cmdCache(rest)
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

// cmdCleanInstall performs a clean install by removing lockfiles and dependency directories
// before running the normal install command.
func cmdCleanInstall(args []string) int {
	fmt.Println("Cleaning project before install...")

	// Remove Node.js artifacts
	if fileExists("package-lock.json") {
		fmt.Println("  Removing package-lock.json")
		os.Remove("package-lock.json")
	}
	if fileExists("yarn.lock") {
		fmt.Println("  Removing yarn.lock")
		os.Remove("yarn.lock")
	}
	if fileExists("pnpm-lock.yaml") {
		fmt.Println("  Removing pnpm-lock.yaml")
		os.Remove("pnpm-lock.yaml")
	}
	if fileExists("bun.lockb") {
		fmt.Println("  Removing bun.lockb")
		os.Remove("bun.lockb")
	}
	if fileExists("node_modules") {
		fmt.Println("  Removing node_modules/")
		os.RemoveAll("node_modules")
	}

	// Remove Python artifacts
	if fileExists("poetry.lock") {
		fmt.Println("  Removing poetry.lock")
		os.Remove("poetry.lock")
	}
	if fileExists("Pipfile.lock") {
		fmt.Println("  Removing Pipfile.lock")
		os.Remove("Pipfile.lock")
	}

	// Remove PHP artifacts
	if fileExists("composer.lock") {
		fmt.Println("  Removing composer.lock")
		os.Remove("composer.lock")
	}
	if fileExists("vendor") {
		fmt.Println("  Removing vendor/")
		os.RemoveAll("vendor")
	}

	// Remove Rust artifacts
	if fileExists("Cargo.lock") {
		fmt.Println("  Removing Cargo.lock")
		os.Remove("Cargo.lock")
	}

	fmt.Println()
	fmt.Println("Running install...")
	fmt.Println()

	// Now run the normal install
	return cmdInstall(args)
}

func cmdWhich(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: missing package name")
		fmt.Fprintln(os.Stderr)
		showCommandUsage("which")
		return 1
	}
	pkg := args[0]

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

func askYesNo(label string) (bool, error) {
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

type projectTarget struct {
	Label string
	Kind  string
	PMs   []pm.ID
}

func detectProjectTargets() []projectTarget {
	var targets []projectTarget

	if fileExists("package.json") {
		targets = append(targets, projectTarget{
			Label: "Node (package.json)",
			Kind:  "node",
			PMs:   []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun},
		})
	}
	if fileExists("composer.json") {
		targets = append(targets, projectTarget{
			Label: "PHP (composer.json)",
			Kind:  "composer",
			PMs:   []pm.ID{pm.Composer},
		})
	}
	if fileExists("Cargo.toml") {
		targets = append(targets, projectTarget{
			Label: "Rust (Cargo.toml)",
			Kind:  "cargo",
			PMs:   []pm.ID{pm.Cargo},
		})
	}
	if fileExists("go.mod") {
		targets = append(targets, projectTarget{
			Label: "Go (go.mod)",
			Kind:  "gomod",
			PMs:   []pm.ID{pm.GoMod},
		})
	}
	if fileExists("pom.xml") {
		targets = append(targets, projectTarget{
			Label: "Java (pom.xml)",
			Kind:  "maven",
			PMs:   []pm.ID{pm.Maven},
		})
	}
	if fileExists("build.gradle") || fileExists("build.gradle.kts") {
		targets = append(targets, projectTarget{
			Label: "Java (Gradle build.gradle)",
			Kind:  "gradle",
			PMs:   []pm.ID{pm.Gradle},
		})
	}
	if fileExists("requirements.txt") {
		targets = append(targets, projectTarget{
			Label: "Python (requirements.txt)",
			Kind:  "pip-req",
			PMs:   []pm.ID{pm.Pip},
		})
	}
	if fileExists("pyproject.toml") {
		targets = append(targets, projectTarget{
			Label: "Python (pyproject.toml)",
			Kind:  "pip-pyproject",
			PMs:   []pm.ID{pm.Pip},
		})
	}

	return targets
}

func autoInstallDetected(global bool) int {
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println("No known dependency files found (package.json, composer.json, etc).")
		return 1
	}

	// If multiple ecosystems
	if len(targets) > 1 && cfg.Interactive {
		items := []string{}
		for _, t := range targets {
			items = append(items, t.Label)
		}
		items = append(items, "Run All")

		prompt := promptui.Select{
			Label: "Detected multiple project types",
			Items: items,
		}
		idx, _, err := prompt.Run()
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}

		if idx == len(items)-1 {
			// Run all
			for _, t := range targets {
				if err := installForTarget(t, global); err != nil {
					fmt.Fprintln(os.Stderr, "error:", err)
					return 1
				}
			}
			return 0
		}

		if err := installForTarget(targets[idx], global); err != nil {
			printError(err)
			return 1
		}
		return 0
	}

	// Single target or non-interactive: run all
	if len(targets) == 1 {
		if err := installForTarget(targets[0], global); err != nil {
			printError(err)
			return 1
		}
		return 0
	}

	// Non-interactive and multiple: run all
	fmt.Println("Non-interactive mode: running installs for all detected project types.")
	for _, t := range targets {
		if err := installForTarget(t, global); err != nil {
			printError(err)
			return 1
		}
	}
	return 0
}

func installForTarget(t projectTarget, global bool) error {
	fmt.Printf("Detected: %s\n", t.Label)

	var chosenPM pm.ID

	if len(t.PMs) == 1 {
		chosenPM = t.PMs[0]
	} else {
		// multiple PM candidates (Node)
		// try to respect cfg.Prefer
		preferred := pickPreferredPM(t.PMs, cfg.Prefer)
		if preferred != "" && !cfg.Interactive {
			chosenPM = preferred
			fmt.Println("Non-interactive: using preferred PM", chosenPM)
		} else if cfg.Interactive {
			items := []string{}
			for _, id := range t.PMs {
				items = append(items, string(id))
			}
			prompt := promptui.Select{
				Label: "Select package manager for " + t.Label,
				Items: items,
			}
			idx, _, err := prompt.Run()
			if err != nil {
				return fmt.Errorf("cancelled")
			}
			chosenPM = t.PMs[idx]
		} else {
			chosenPM = t.PMs[0]
		}
	}

	meta, ok := pm.MetaFor(chosenPM)
	if !ok {
		return fmt.Errorf("no meta for PM %s", chosenPM)
	}

	if global && !meta.SupportsGlobal {
		fmt.Printf("%s does not support global installs in the same way. Ignoring --global.\n", meta.Name)
		global = false
	}

	if err := ensurePM(chosenPM); err != nil {
		return err
	}

	fmt.Printf("Running dependency install for %s using %s...\n", t.Label, meta.Name)
	switch t.Kind {
	case "node":
		return runBinary(meta.Binary, []string{"install"})
	case "composer":
		return runBinary(meta.Binary, []string{"install"})
	case "cargo":
		return runBinary(meta.Binary, []string{"build"})
	case "gomod":
		return runBinary(meta.Binary, []string{"mod", "tidy"})
	case "maven":
		return runBinary(meta.Binary, []string{"install"})
	case "gradle":
		return runBinary(meta.Binary, []string{"build"})
	case "pip-req":
		return runBinary(meta.Binary, []string{"install", "-r", "requirements.txt"})
	case "pip-pyproject":
		return runBinary(meta.Binary, []string{"install", "."})
	default:
		return fmt.Errorf("unknown project kind %s", t.Kind)
	}
}

func runBinary(bin string, args []string) error {
	logx.Info("running project install: %s %v", bin, args)
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
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
