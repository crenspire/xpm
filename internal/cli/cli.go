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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/doctor"
	"github.com/crenspire/xpm/internal/env"
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

	// Auto-activate versions from .xpm-env
	if cfg.Env.Enabled {
		manager, err := env.NewManager(cfg)
		if err == nil {
			env.ActivateFromLocalEnv(manager)
		}
	}

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

// filterResultsByLockFiles filters search results based on detected lock files.
// For ecosystems with lock files:
//   - If exactly one lock file: returns that PM's ID for auto-selection
//   - If multiple lock files: filters results to only those PMs
//   - If no lock file: adds all PMs in that ecosystem to allow selection
//
// Returns filtered results and optionally a single PM ID if auto-selection should occur.
func filterResultsByLockFiles(results []search.Result, lockFiles map[pm.Ecosystem][]pm.ID) ([]search.Result, *pm.ID) {
	var filtered []search.Result
	var autoSelect *pm.ID

	// Track which ecosystems we've seen in results
	ecosystemResults := make(map[pm.Ecosystem][]search.Result)

	for _, r := range results {
		eco := pm.EcosystemForManager(r.Manager)
		if eco == "" {
			// Not part of a multi-PM ecosystem (composer, cargo, go, maven, gradle)
			filtered = append(filtered, r)
			continue
		}
		ecosystemResults[eco] = append(ecosystemResults[eco], r)
	}

	// Process each ecosystem
	for eco, ecoResults := range ecosystemResults {
		detectedPMs, hasLockFiles := lockFiles[eco]

		if !hasLockFiles || len(detectedPMs) == 0 {
			// No lock file detected - add all results from this ecosystem
			// The user will be prompted to choose
			filtered = append(filtered, ecoResults...)
			continue
		}

		if len(detectedPMs) == 1 {
			// Exactly one lock file - auto-select this PM
			pm := detectedPMs[0]
			autoSelect = &pm
			// Find the result for this PM (or create one based on existing result)
			for _, r := range ecoResults {
				if r.Manager == pm {
					filtered = append(filtered, r)
					break
				}
			}
			// If no result for that exact PM but we have npm result and user has yarn/pnpm/bun lock
			// Use the npm result but switch the manager
			found := false
			for _, r := range filtered {
				if r.Manager == *autoSelect {
					found = true
					break
				}
			}
			if !found && len(ecoResults) > 0 {
				// Clone the first result but with the detected PM
				r := ecoResults[0]
				r.Manager = *autoSelect
				filtered = append(filtered, r)
			}
			continue
		}

		// Multiple lock files - filter to only those PMs
		pmSet := make(map[pm.ID]bool)
		for _, p := range detectedPMs {
			pmSet[p] = true
		}

		for _, r := range ecoResults {
			if pmSet[r.Manager] {
				filtered = append(filtered, r)
			}
		}
		// Also add results for lock-file PMs that aren't in results
		// (e.g., if npm search found package but user has yarn.lock)
		if len(ecoResults) > 0 {
			for _, p := range detectedPMs {
				found := false
				for _, r := range filtered {
					if r.Manager == p {
						found = true
						break
					}
				}
				if !found {
					r := ecoResults[0]
					r.Manager = p
					filtered = append(filtered, r)
				}
			}
		}
	}

	return filtered, autoSelect
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

func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	global := fs.Bool("global", false, "install globally")
	gShort := fs.Bool("g", false, "install globally (shorthand)")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return 1
	}
	pkgArgs := fs.Args()
	glob := *global || *gShort

	if len(pkgArgs) == 0 {
		// auto-detect project files and run installs
		return autoInstallDetected(glob)
	}

	// Parse package@version syntax
	pkg, requestedVersion := parsePackageVersion(pkgArgs[0])

	// Validate package name (use generic validation since we don't know the manager yet)
	if err := pm.ValidateGenericPackageName(pkg); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid package name: %v\n\n", err)
		showCommandUsage("install")
		return 1
	}

	// Validate version if provided
	if requestedVersion != "" {
		if err := pm.ValidateVersion(requestedVersion); err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid version: %v\n\n", err)
			showCommandUsage("install")
			return 1
		}
	}

	if requestedVersion != "" {
		fmt.Printf("Searching for %q (version %s) across ecosystems...\n\n", pkg, requestedVersion)
	} else {
		fmt.Printf("Searching for %q across ecosystems...\n\n", pkg)
	}

	searchOpts := search.Options{
		Enable: make(map[pm.ID]bool),
	}
	for id := range map[pm.ID]struct{}{
		pm.Npm: {}, pm.Pip: {}, pm.Composer: {}, pm.Cargo: {}, pm.Maven: {},
	} {
		name := string(id)
		enabled := true
		if v, ok := cfg.Search[name]; ok {
			enabled = v
		}
		searchOpts.Enable[id] = enabled
	}

	results, err := search.SearchEverywhere(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	if len(results) == 0 {
		fmt.Println("No matches found for", pkg)
		return 0
	}

	// Detect lock files to influence package manager selection
	cwd, _ := os.Getwd()
	lockFiles := pm.DetectLockFiles(cwd)

	// Filter and enhance results based on lock file detection
	results, autoSelected := filterResultsByLockFiles(results, lockFiles)

	if len(results) == 0 {
		fmt.Println("No matches found for", pkg)
		return 0
	}

	order := preferOrderMap(cfg.Prefer)
	sort.Slice(results, func(i, j int) bool {
		oi := order[string(results[i].Manager)]
		oj := order[string(results[j].Manager)]
		if oi == oj {
			return string(results[i].Manager) < string(results[j].Manager)
		}
		return oi < oj
	})

	var chosen search.Result

	// If auto-selected (single lock file for ecosystem), use that
	if autoSelected != nil {
		for _, r := range results {
			if r.Manager == *autoSelected {
				chosen = r
				lockFile := pm.GetLockFileName(*autoSelected)
				fmt.Printf("Detected %s - using %s\n\n", lockFile, *autoSelected)
				break
			}
		}
	}

	// If not auto-selected, prompt user
	if chosen.Manager == "" {
		items := []string{}
		for _, r := range results {
			label := fmt.Sprintf("%s (%s)", r.Name, r.Manager)
			if v, ok := r.Extra["version"]; ok && v != "" {
				label += " @ " + v
			}
			if r.Info != "" {
				label += " - " + r.Info
			}
			items = append(items, label)
		}

		var index int
		if cfg.Interactive {
			prompt := promptui.Select{
				Label: "Select package manager to install from",
				Items: items,
			}

			idx, _, err := prompt.Run()
			if err != nil {
				fmt.Println("Cancelled.")
				return 1
			}
			index = idx
		} else {
			index = 0
			fmt.Println("Non-interactive mode: picking", items[0])
		}

		chosen = results[index]
	}
	meta, ok := pm.MetaFor(chosen.Manager)
	if !ok {
		fmt.Println("Unsupported manager selected:", chosen.Manager)
		return 1
	}

	fmt.Printf("\nYou chose: %s via %s\n\n", pkg, meta.Name)

	if glob && !meta.SupportsGlobal {
		fmt.Printf("%s does not support global installs in the same way. Ignoring --global.\n\n", meta.Name)
		glob = false
	}

	if glob && chosen.Manager == pm.Pip {
		fmt.Println("Global pip installs often require sudo and can affect system Python.")
		yes, err := askYesNo("Show recommended sudo command instead of running pip?")
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		if yes {
			fmt.Printf("\nRun this manually:\n  sudo pip install %s\n", pkg)
			return 0
		}
		fmt.Println("Proceeding without sudo (may fail if permissions are insufficient)...")
	}

	if !pm.Exists(meta.Binary) {
		fmt.Printf("%s (%s) is not installed on this system.\n", meta.Name, meta.Binary)

		if !cfg.AutoInstallPM {
			fmt.Println("Auto-install is disabled in config. Install it manually and re-run.")
			hint := pm.InstallHint(chosen.Manager)
			if strings.TrimSpace(hint) != "" {
				fmt.Println("Hint:", hint)
			}
			return 1
		}

		yes, err := askYesNo(fmt.Sprintf("Attempt to install %s now?", meta.Name))
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		if !yes {
			fmt.Println("Aborted by user.")
			return 1
		}

		if err := pm.InstallPM(chosen.Manager); err != nil {
			fmt.Fprintln(os.Stderr, "failed to install package manager:", err)
			return 1
		}

		fmt.Printf("\n%s installation finished (or instructions printed). If necessary, ensure it's in PATH and re-run xpm.\n\n", meta.Name)
	}

	adapter, err := pm.NewAdapter(chosen.Manager)
	if err != nil {
		fmt.Fprintln(os.Stderr, "adapter error:", err)
		return 1
	}

	// Merge requested version into extra info (overrides search result version)
	installExtra := make(map[string]string)
	for k, v := range chosen.Extra {
		installExtra[k] = v
	}
	if requestedVersion != "" {
		installExtra["version"] = requestedVersion
	}

	// Validate package name for the specific package manager before installation
	if err := pm.ValidatePackageName(pkg, chosen.Manager); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid package name for %s: %v\n", meta.Name, err)
		return 1
	}

	installPkg := pkg
	if requestedVersion != "" {
		fmt.Printf("Installing %s@%s via %s...\n\n", pkg, requestedVersion, meta.Name)
	} else {
		fmt.Printf("Installing %s via %s...\n\n", pkg, meta.Name)
	}

	if err := adapter.InstallPackage(installPkg, glob, nil, installExtra); err != nil {
		fmt.Fprintln(os.Stderr, "package install failed:", err)
		return 1
	}

	fmt.Println("\nDone ✅")
	return 0
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

	searchOpts := search.Options{
		Enable: make(map[pm.ID]bool),
	}
	for id := range map[pm.ID]struct{}{
		pm.Npm: {}, pm.Pip: {}, pm.Composer: {}, pm.Cargo: {}, pm.Maven: {},
	} {
		name := string(id)
		enabled := true
		if v, ok := cfg.Search[name]; ok {
			enabled = v
		}
		searchOpts.Enable[id] = enabled
	}

	results, err := search.SearchEverywhere(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	if len(results) == 0 {
		fmt.Println("No matches found.")
		return 0
	}

	foundBy := map[pm.ID]search.Result{}
	for _, r := range results {
		foundBy[r.Manager] = r
	}

	fmt.Println("Found in:")
	keys := make([]string, 0, len(foundBy))
	for id := range foundBy {
		keys = append(keys, string(id))
	}
	sort.Strings(keys)

	// Check if npm was found (yarn/pnpm/bun share the same registry)
	_, npmFound := foundBy[pm.Npm]
	// Check if pip was found (poetry/pipenv use the same PyPI registry)
	_, pipFound := foundBy[pm.Pip]

	for _, k := range keys {
		id := pm.ID(k)
		r := foundBy[id]
		line := fmt.Sprintf("- %s: %s", k, r.Name)
		if v, ok := r.Extra["version"]; ok && v != "" {
			line += " @" + v
		}
		if r.Info != "" {
			line += " - " + r.Info
		}
		fmt.Println(line)

		switch id {
		case pm.Npm:
			fmt.Println("  → Also available via: yarn, pnpm, bun")
		case pm.Pip:
			fmt.Println("  → Also available via: poetry, pipenv")
		case pm.Maven:
			fmt.Println("  → Add to pom.xml as dependency.")
		case pm.Cargo:
			fmt.Println("  → Add to Cargo.toml under [dependencies].")
		case pm.Composer:
			fmt.Println("  → Add to composer.json or run `composer require ...`.")
		}
	}

	// Collect registries where not found (excluding yarn/pnpm/bun if npm was found, poetry/pipenv if pip was found)
	var notFound []string
	for _, meta := range pm.AllMetas() {
		if _, ok := foundBy[meta.ID]; !ok {
			// Skip yarn/pnpm/bun if npm was found (they share the same registry)
			if npmFound && (meta.ID == pm.Yarn || meta.ID == pm.Pnpm || meta.ID == pm.Bun) {
				continue
			}
			// Skip poetry/pipenv if pip was found (they use the same PyPI registry)
			if pipFound && (meta.ID == pm.Poetry || meta.ID == pm.Pipenv) {
				continue
			}
			// Only show ecosystems that were actually searched
			if search.Enabled(searchOpts, meta.ID) {
				notFound = append(notFound, meta.Name)
			}
		}
	}

	if len(notFound) > 0 {
		fmt.Println()
		fmt.Println("Not found in:")
		for _, name := range notFound {
			fmt.Println("-", name)
		}
	}

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

	if !pm.Exists(meta.Binary) {
		fmt.Printf("%s (%s) is not installed on this system.\n", meta.Name, meta.Binary)

		if !cfg.AutoInstallPM {
			fmt.Println("Auto-install is disabled in config. Install it manually and re-run.")
			hint := pm.InstallHint(chosenPM)
			if strings.TrimSpace(hint) != "" {
				fmt.Println("Hint:", hint)
			}
			return fmt.Errorf("%s missing", meta.Name)
		}

		yes, err := askYesNo(fmt.Sprintf("Attempt to install %s now?", meta.Name))
		if err != nil {
			return fmt.Errorf("cancelled")
		}
		if !yes {
			return fmt.Errorf("aborted by user")
		}

		if err := pm.InstallPM(chosenPM); err != nil {
			return fmt.Errorf("failed to install %s: %w", meta.Name, err)
		}

		fmt.Printf("\n%s installation finished (or instructions printed). If necessary, ensure it's in PATH and re-run xpm.\n\n", meta.Name)
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
