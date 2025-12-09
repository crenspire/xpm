package cli

import (
	"fmt"
	"os"
	"strings"
)

// Helper functions for colored output in man pages
func colorSection(title string) string {
	return fmt.Sprintf("%s%s%s%s", colorBold, colorYellow, title, colorReset)
}

func colorCommand(cmd string) string {
	return fmt.Sprintf("%s%s%s%s", colorBold, colorCyan, cmd, colorReset)
}

func colorOption(opt string) string {
	return fmt.Sprintf("%s%s%s", colorGreen, opt, colorReset)
}

func colorExample(example string) string {
	return fmt.Sprintf("%s%s%s", colorCyan, example, colorReset)
}

// cmdMan handles the `xpm man` command.
// Without arguments, it lists all available commands.
// With a command name, it shows detailed help for that command.
// With --generate, it generates man pages.
func cmdMan(args []string) int {
	if len(args) == 0 {
		listCommands()
		return 0
	}

	// Check for --generate flag
	if args[0] == "--generate" || args[0] == "-g" {
		outputDir := "man"
		if len(args) > 1 {
			outputDir = args[1]
		}
		if err := GenerateAllManPages(outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "error generating man pages: %v\n", err)
			return 1
		}
		fmt.Printf("\nMan pages generated in: %s\n", outputDir)
		fmt.Println("To install, copy to your man path or run: sudo cp man/*.1 /usr/local/share/man/man1/")
		return 0
	}

	command := args[0]
	showCommandHelp(command)
	return 0
}

// listCommands lists all available commands with their descriptions.
func listCommands() {
	fmt.Printf("%s%sAvailable commands:%s\n\n", colorBold, colorYellow, colorReset)
	
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
	fmt.Println()
	fmt.Printf("%sRun%s '%sxpm man <command>%s' for detailed help on a specific command.\n", colorBold, colorReset, colorCyan, colorReset)
}

// showCommandHelp displays detailed help for a specific command.
func showCommandHelp(command string) {
	// Normalize command name (handle aliases)
	normalized := normalizeCommand(command)
	
	switch normalized {
	case "install":
		showInstallHelp()
	case "run":
		showRunHelp()
	case "which":
		showWhichHelp()
	case "list":
		showListHelp()
	case "update":
		showUpdateHelp()
	case "remove":
		showRemoveHelp()
	case "info":
		showInfoHelp()
	case "lock":
		showLockHelp()
	case "cache":
		showCacheHelp()
	case "graph":
		showGraphHelp()
	case "search":
		showSearchHelp()
	case "workspaces":
		showWorkspacesHelp()
	case "env":
		showEnvHelp()
	case "doctor":
		showDoctorHelp()
	case "config":
		showConfigHelp()
	case "version":
		showVersionHelp()
	case "help":
		showHelpHelp()
	case "man":
		showManHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		listCommands()
	}
}

// normalizeCommand converts aliases to their canonical command names.
func normalizeCommand(cmd string) string {
	aliases := map[string]string{
		"i":    "install",
		"r":    "run",
		"w":    "which",
		"l":    "list",
		"u":    "update",
		"rm":   "remove",
		"g":    "graph",
		"s":    "search",
		"d":    "doctor",
		"-v":   "version",
		"-h":   "help",
		"--help": "help",
	}
	
	if canonical, ok := aliases[cmd]; ok {
		return canonical
	}
	return cmd
}

// showCommandUsage is a helper that displays command usage information.
// This is used by error handlers to show help when errors occur.
func showCommandUsage(command string) {
	normalized := normalizeCommand(command)
	showCommandHelp(normalized)
}

// Individual help functions for each command

func showInstallHelp() {
	fmt.Printf("%s\n", colorCommand("xpm install [package[@version]] [options]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Install packages or project dependencies.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                    Auto-detect and install project dependencies\n", colorCommand("xpm install"))
	fmt.Printf("  %s          Install a package (searches all ecosystems)\n", colorCommand("xpm install <package>"))
	fmt.Printf("  %s Install a specific version\n", colorCommand("xpm install <package>@<version>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("OPTIONS:"))
	fmt.Printf("  %s                   Install globally (if supported)\n", colorOption("-g, --global"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                    Install dependencies for detected projects\n", colorExample("xpm install"))
	fmt.Printf("  %s              Install axios (searches all ecosystems)\n", colorExample("xpm install axios"))
	fmt.Printf("  %s        Install specific version\n", colorExample("xpm install axios@1.0.0"))
	fmt.Printf("  %s      Install globally\n", colorExample("xpm install -g typescript"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("i, install"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                         Clean install (removes lockfiles first)\n", colorCommand("xpm ci"))
	fmt.Printf("  %s                     Update installed packages\n", colorCommand("xpm update"))
	fmt.Printf("  %s                     Remove a package\n", colorCommand("xpm remove"))
}

func showRunHelp() {
	fmt.Printf("%s\n", colorCommand("xpm run [task] [-- <args>]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Run project scripts from package.json, composer.json, pyproject.toml, or Cargo.toml.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                        List available scripts\n", colorCommand("xpm run"))
	fmt.Printf("  %s                 Run a specific task\n", colorCommand("xpm run <task>"))
	fmt.Printf("  %s       Run task with additional arguments\n", colorCommand("xpm run <task> -- <args>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("OPTIONS:"))
	fmt.Printf("  %s                 Run task in all workspace projects\n", colorOption("-w, --workspace"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                        List all available scripts\n", colorExample("xpm run"))
	fmt.Printf("  %s                    Run the 'dev' script\n", colorExample("xpm run dev"))
	fmt.Printf("  %s        Run 'build' with --watch argument\n", colorExample("xpm run build -- --watch"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("r, run"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                    Install project dependencies\n", colorCommand("xpm install"))
}

func showWhichHelp() {
	fmt.Printf("%s\n", colorCommand("xpm which <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Check which package ecosystems have a package available.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s\n", colorCommand("xpm which <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s               Find lodash across all registries\n", colorExample("xpm which lodash"))
	fmt.Printf("  %s              Check if express exists in any ecosystem\n", colorExample("xpm which express"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("w, which"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                     Interactive package search\n", colorCommand("xpm search"))
	fmt.Printf("  %s                       Get detailed package information\n", colorCommand("xpm info"))
}

func showListHelp() {
	fmt.Printf("%s\n", colorCommand("xpm list"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  List installed packages for the current project.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s\n", colorCommand("xpm list"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                       List all installed packages\n", colorExample("xpm list"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("l, list"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                       Show detailed package information\n", colorCommand("xpm info"))
}

func showUpdateHelp() {
	fmt.Printf("%s\n", colorCommand("xpm update [package]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Update packages in the current project.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                     Update all packages\n", colorCommand("xpm update"))
	fmt.Printf("  %s            Update a specific package\n", colorCommand("xpm update <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                     Update all packages\n", colorExample("xpm update"))
	fmt.Printf("  %s               Update axios to latest version\n", colorExample("xpm update axios"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("u, update"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                    Install packages\n", colorCommand("xpm install"))
}

func showRemoveHelp() {
	fmt.Printf("%s\n", colorCommand("xpm remove <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Remove a package from the current project.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s\n", colorCommand("xpm remove <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s               Remove axios from project\n", colorExample("xpm remove axios"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("rm, remove"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                    Install packages\n", colorCommand("xpm install"))
}

func showInfoHelp() {
	fmt.Printf("%s\n", colorCommand("xpm info <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Show detailed package information from all ecosystems.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s\n", colorCommand("xpm info <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s               Get detailed info about express\n", colorExample("xpm info express"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                      Check package availability\n", colorCommand("xpm which"))
	fmt.Printf("  %s                     Interactive package search\n", colorCommand("xpm search"))
}

func showLockHelp() {
	fmt.Printf("%s\n", colorCommand("xpm lock [--verify]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Generate or verify unified lockfile (xpm-lock.yaml).")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                       Generate xpm-lock.yaml\n", colorCommand("xpm lock"))
	fmt.Printf("  %s              Verify lockfiles haven't changed\n", colorCommand("xpm lock --verify"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                       Generate unified lockfile\n", colorExample("xpm lock"))
	fmt.Printf("  %s              Check if lockfiles changed\n", colorExample("xpm lock --verify"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                    Install dependencies\n", colorCommand("xpm install"))
}

func showCacheHelp() {
	fmt.Printf("%s\n", colorCommand("xpm cache <subcommand>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Manage dependency cache.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SUBCOMMANDS:"))
	fmt.Printf("  %s                           Show cache structure and contents\n", colorCommand("tree"))
	fmt.Printf("  %s                           Show total cache size and statistics\n", colorCommand("size"))
	fmt.Printf("  %s                          Clear the entire cache\n", colorCommand("clean"))
	fmt.Printf("  %s                             Run garbage collection (removes old/unused items)\n", colorCommand("gc"))
	fmt.Printf("  %s                         Verify integrity of cached items\n", colorCommand("verify"))
	fmt.Printf("  %s                         Attempt to repair corrupted cache entries\n", colorCommand("repair"))
	fmt.Printf("  %s                           Show the cache directory path\n", colorCommand("path"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                 Show cache tree\n", colorExample("xpm cache tree"))
	fmt.Printf("  %s                Clear all cached artifacts\n", colorExample("xpm cache clean"))
	fmt.Printf("  %s                   Clean up old cached items\n", colorExample("xpm cache gc"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s                             Alias for 'cache clean'\n", colorCommand("cc"))
	fmt.Printf("  %s                             Alias for 'cache gc'\n", colorCommand("cg"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                    Install dependencies (uses cache)\n", colorCommand("xpm install"))
}

func showGraphHelp() {
	fmt.Printf("%s\n", colorCommand("xpm graph [package] [options]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Show unified dependency graph across all ecosystems.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                      Show dependency graph for all projects\n", colorCommand("xpm graph"))
	fmt.Printf("  %s             Show graph for specific package\n", colorCommand("xpm graph <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("OPTIONS:"))
	fmt.Printf("  %s                         Export graph as JSON\n", colorOption("--json"))
	fmt.Printf("  %s                          Generate SVG visualization\n", colorOption("--svg"))
	fmt.Printf("  %s                    Generate combined graph for all workspaces\n", colorOption("--workspace"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                      Show full dependency graph\n", colorExample("xpm graph"))
	fmt.Printf("  %s                Show graph for react\n", colorExample("xpm graph react"))
	fmt.Printf("  %s               Export as JSON\n", colorExample("xpm graph --json"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("g, graph"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                       List installed packages\n", colorCommand("xpm list"))
}

func showSearchHelp() {
	fmt.Printf("%s\n", colorCommand("xpm search [query]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Interactive TUI package search across all ecosystems.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                     Open interactive search UI\n", colorCommand("xpm search"))
	fmt.Printf("  %s             Search with pre-filled query\n", colorCommand("xpm search <query>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                     Start interactive search\n", colorExample("xpm search"))
	fmt.Printf("  %s               Search for axios\n", colorExample("xpm search axios"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("s, search"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                      Check package availability\n", colorCommand("xpm which"))
	fmt.Printf("  %s                       Get detailed package information\n", colorCommand("xpm info"))
}

func showWorkspacesHelp() {
	fmt.Printf("%s\n", colorCommand("xpm workspaces"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  List detected workspaces/monorepos.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                 List all detected workspaces\n", colorCommand("xpm workspaces"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s        Install in all workspaces\n", colorCommand("xpm install --workspace"))
	fmt.Printf("  %s     Run task across workspaces\n", colorCommand("xpm run --workspace <task>"))
}

func showEnvHelp() {
	fmt.Printf("%s\n", colorCommand("xpm env <command> [arguments]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Manage runtime versions (node, python, go, java, rust, bun, deno).")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("COMMANDS:"))
	fmt.Printf("  %s    Install a runtime version\n", colorCommand("install <runtime>@<version>"))
	fmt.Printf("  %s        Switch to a runtime version\n", colorCommand("use <runtime>@<version>"))
	fmt.Printf("  %s                           List installed versions\n", colorCommand("list"))
	fmt.Printf("  %s            List available remote versions\n", colorCommand("ls-remote <runtime>"))
	fmt.Printf("  %s                        Show active runtime versions\n", colorCommand("current"))
	fmt.Printf("  %s     Remove a runtime version\n", colorCommand("remove <runtime>@<version>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s   Install Node.js 20.11.0\n", colorExample("xpm env install node@20.11.0"))
	fmt.Printf("  %s       Switch to Node.js 20.11.0\n", colorExample("xpm env use node@20.11.0"))
	fmt.Printf("  %s                   List all installed runtimes\n", colorExample("xpm env list"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                    Install packages\n", colorCommand("xpm install"))
}

func showDoctorHelp() {
	fmt.Printf("%s\n", colorCommand("xpm doctor"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Comprehensive environment & project diagnostics.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s\n", colorCommand("xpm doctor"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                     Run full diagnostic check\n", colorExample("xpm doctor"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("d, doctor"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                     View or edit configuration\n", colorCommand("xpm config"))
}

func showConfigHelp() {
	fmt.Printf("%s\n", colorCommand("xpm config [subcommand]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  View or edit configuration.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SUBCOMMANDS:"))
	fmt.Printf("  %s                           Display current configuration\n", colorCommand("show"))
	fmt.Printf("  %s                           Show config file path\n", colorCommand("path"))
	fmt.Printf("  %s                           Open config in editor\n", colorCommand("edit"))
	fmt.Printf("  %s              Set a configuration value\n", colorCommand("set <key> <value>"))
	fmt.Printf("  %s                          Reset to default configuration\n", colorCommand("reset"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                Show current config\n", colorExample("xpm config show"))
	fmt.Printf("  %s\n", colorExample("xpm config set interactive false"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                      Check environment\n", colorCommand("xpm doctor"))
}

func showVersionHelp() {
	fmt.Printf("%s\n", colorCommand("xpm version"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Show version information.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s\n", colorCommand("xpm version"))
	fmt.Printf("  %s\n", colorCommand("xpm -v"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                    Show version\n", colorExample("xpm version"))
	fmt.Printf("  %s                         Show version (shorthand)\n", colorExample("xpm -v"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("-v, --version, version"))
}

func showHelpHelp() {
	fmt.Printf("%s\n", colorCommand("xpm help"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Show help message.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s\n", colorCommand("xpm help"))
	fmt.Printf("  %s\n", colorCommand("xpm -h"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                       Show help\n", colorExample("xpm help"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("-h, --help, help"))
}

func showManHelp() {
	fmt.Printf("%s\n", colorCommand("xpm man [command]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Show detailed manual for commands.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                        List all available commands\n", colorCommand("xpm man"))
	fmt.Printf("  %s              Show detailed help for a command\n", colorCommand("xpm man <command>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                        List commands\n", colorExample("xpm man"))
	fmt.Printf("  %s                Show install command help\n", colorExample("xpm man install"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                       Show brief help\n", colorCommand("xpm help"))
}

