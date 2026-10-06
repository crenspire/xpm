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
		fmt.Printf("To install, copy them to your man path, e.g.: sudo cp %s/*.1 /usr/local/share/man/man1/\n", outputDir)
		return 0
	}

	command := args[0]
	if !showCommandHelp(command) {
		return 1
	}
	return 0
}

// commandInfo describes one command for the usage and man listings.
type commandInfo struct {
	name         string
	aliases      []string
	description  string
	experimental bool
}

// commandTable lists every command Run dispatches, in help order.
var commandTable = []commandInfo{
	{"install", []string{"i"}, "Install packages (several at once) or this project's dependencies", false},
	{"ci", nil, "Frozen install from lockfiles; deletes nothing unasked", false},
	{"run", []string{"r"}, "Run project scripts (package.json, composer.json, pyproject.toml, Cargo.toml)", false},
	{"which", []string{"w"}, "Check which ecosystems have a package", false},
	{"search", []string{"s"}, "Search all registries (TUI on a terminal, plain output otherwise)", false},
	{"info", nil, "Show detailed package information", false},
	{"list", []string{"l"}, "List installed packages for the current project", false},
	{"update", []string{"u"}, "Update packages in the current project", false},
	{"remove", []string{"rm"}, "Remove a package from the current project", false},
	{"doctor", []string{"d"}, "Environment & project diagnostics", false},
	{"config", nil, "View or edit configuration", false},
	{"env", nil, "Manage runtime versions (node, go, ...)", true},
	{"graph", []string{"g"}, "Dependency graph across ecosystems", true},
	{"lock", nil, "Generate or verify the unified lockfile (xpm-lock.yaml)", true},
	{"workspaces", nil, "List detected workspaces/monorepos", true},
	{"version", []string{"-v", "-V", "--version"}, "Show version information", false},
	{"help", []string{"-h", "--help"}, "Show this help message", false},
	{"man", nil, "Show the detailed manual for a command", false},
}

const experimentalNote = "Experimental commands are being reworked; their behaviour and output may change."

// printCommandTable prints commandTable; shared by usage and `xpm man`.
func printCommandTable() {
	for _, cmd := range commandTable {
		name := cmd.name
		if len(cmd.aliases) > 0 {
			name += ", " + strings.Join(cmd.aliases, ", ")
		}
		desc := cmd.description
		if cmd.experimental {
			desc += " (experimental)"
		}
		fmt.Printf("  %s%s%-28s%s %s\n", colorBold, colorCyan, name, colorReset, desc)
	}
}

// listCommands lists all available commands with their descriptions.
func listCommands() {
	fmt.Printf("%s%sAvailable commands:%s\n\n", colorBold, colorYellow, colorReset)

	printCommandTable()
	fmt.Println()
	fmt.Println(experimentalNote)
	fmt.Println()
	fmt.Printf("%sRun%s '%sxpm man <command>%s' for detailed help on a specific command.\n", colorBold, colorReset, colorCyan, colorReset)
}

// showCommandHelp displays detailed help for a specific command. It reports
// false for an unknown command (after listing the commands).
func showCommandHelp(command string) bool {
	// Normalize command name (handle aliases)
	normalized := normalizeCommand(command)

	switch normalized {
	case "install":
		showInstallHelp()
	case "ci":
		showCiHelp()
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
		return false
	}
	return true
}

// normalizeCommand converts aliases to their canonical command names.
func normalizeCommand(cmd string) string {
	for _, c := range commandTable {
		if cmd == c.name {
			return c.name
		}
		for _, a := range c.aliases {
			if cmd == a {
				return c.name
			}
		}
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
	fmt.Printf("%s\n", colorCommand("xpm install [-g|--global] [package[@version] ...] [--]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Install one or several packages, or, without packages, this project's dependencies.")
	fmt.Println("  Several packages are installed in order; xpm stops at the first failure.")
	fmt.Println("  Without packages, the detected project's tool runs its install (with several")
	fmt.Println("  projects, a terminal asks which one or all; a script runs all). -g without")
	fmt.Println("  packages is an error: project dependencies are never global.")
	fmt.Println("  Exit status 1 when no registry has a match or an install fails.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("HOW XPM PICKS A TOOL:"))
	fmt.Println("  Inside a project, an exact hit in the project's own ecosystem is installed with")
	fmt.Println("  the project's tool (its lockfile decides, e.g. yarn.lock -> yarn). Outside a")
	fmt.Println("  project, and for every -g install, exact hits from a single ecosystem are")
	fmt.Println("  installed; with several, the \"prefer\" config list decides, else a terminal shows")
	fmt.Println("  a menu and a script gets an error listing the candidates.")
	fmt.Println("  A Go module path (github.com/gin-gonic/gin) runs go get (with -g, go install ...@latest")
	fmt.Println("  or @version). A Maven or Gradle hit only prints the dependency snippet to add.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("OPTIONS:"))
	fmt.Printf("  %s       Install globally (if the tool supports it); may come before or after the packages\n", colorOption("-g, --global"))
	fmt.Printf("  %s                 Ends flag parsing; names starting with - are still rejected\n", colorOption("--"))
	fmt.Println("  Any other flag, including --global=false, is rejected.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                    Install dependencies for detected projects\n", colorExample("xpm install"))
	fmt.Printf("  %s              Install axios (searches all ecosystems)\n", colorExample("xpm install axios"))
	fmt.Printf("  %s       Install several packages in order\n", colorExample("xpm install axios lodash"))
	fmt.Printf("  %s        Install specific version\n", colorExample("xpm install axios@1.0.0"))
	fmt.Printf("  %s   Install globally\n", colorExample("xpm install typescript -g"))
	fmt.Printf("  %s   go get a Go module\n", colorExample("xpm install github.com/gin-gonic/gin"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("i, install"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                         Frozen install from lockfiles (deletes nothing unasked)\n", colorCommand("xpm ci"))
	fmt.Printf("  %s                     Update installed packages\n", colorCommand("xpm update"))
	fmt.Printf("  %s                     Remove a package\n", colorCommand("xpm remove"))
}

func showCiHelp() {
	fmt.Printf("%s\n", colorCommand("xpm ci"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Install every detected project's dependencies from its lockfile, using the")
	fmt.Println("  tool's strict form where one exists: npm ci, pnpm/yarn/bun install --frozen-lockfile,")
	fmt.Println("  yarn install --immutable (yarn 2+), pipenv install --deploy, cargo build --locked,")
	fmt.Println("  go mod download. Other tools have no frozen flag and run their normal install:")
	fmt.Println("  composer install, pip install -r requirements.txt (pip install . for pyproject.toml),")
	fmt.Println("  poetry install, mvn install, gradle build.")
	fmt.Println("  xpm checks every project's tool before running any install. It deletes nothing")
	fmt.Println("  unasked: it never deletes lockfiles, and asks before removing node_modules/")
	fmt.Println("  (yarn, pnpm, bun) or Composer's vendor/. ci takes no arguments.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                         Frozen install for every detected project\n", colorExample("xpm ci"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s                    Install packages or project dependencies\n", colorCommand("xpm install"))
}

func showRunHelp() {
	fmt.Printf("%s\n", colorCommand("xpm run [task] [-- <args>]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Run project scripts from package.json, composer.json, pyproject.toml, or Cargo.toml.")
	fmt.Println("  pyproject.toml: [tool.xpm.scripts] ([tool.upm.scripts] when that table is absent or")
	fmt.Println("  empty), else [tool.poetry.scripts], else [project.scripts]. Cargo.toml:")
	fmt.Println("  [package.metadata.xpm.scripts] ([package.metadata.upm.scripts] fallback).")
	fmt.Println("  package.json scripts run with npm, yarn, pnpm or bun run; composer.json scripts with")
	fmt.Println("  composer run-script; pyproject.toml and Cargo.toml scripts run with sh -c.")
	fmt.Println("  The exit status is the script's.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                        List available scripts\n", colorCommand("xpm run"))
	fmt.Printf("  %s                 Run a specific task\n", colorCommand("xpm run <task>"))
	fmt.Printf("  %s       Run task with additional arguments\n", colorCommand("xpm run <task> -- <args>"))
	fmt.Printf("  %s              Run task in every workspace project (flag before the task)\n", colorCommand("xpm run -w <task>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("OPTIONS:"))
	fmt.Printf("  %s                 Run task in all workspace projects (experimental)\n", colorOption("-w, --workspace"))
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
	fmt.Println("  Registries that do not answer within 2.5 s (by default) are listed as Unavailable.")
	fmt.Println("  Exit status 1 when nothing matches or every registry fails.")
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
	fmt.Println("  Registries that do not answer within 2.5 s (by default) are listed as Unavailable.")
	fmt.Println("  Exit status 1 when nothing matches or every registry fails.")
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
	fmt.Println("  Generate or verify unified lockfile (xpm-lock.yaml) (experimental)")
	fmt.Println("  This command is experimental and is being reworked; its behaviour and output may change.")
	fmt.Println("  --verify reports each recorded lockfile as unchanged, changed or missing, and each")
	fmt.Println("  supported lockfile on disk that xpm-lock.yaml does not record as added. Unreadable")
	fmt.Println("  entries and recorded paths outside the project are errors. Exit status 1 unless")
	fmt.Println("  every lockfile is unchanged.")
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

func showGraphHelp() {
	fmt.Printf("%s\n", colorCommand("xpm graph [package] [options]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Show unified dependency graph across all ecosystems (experimental)")
	fmt.Println("  This command is experimental and is being reworked; its behaviour and output may change.")
	fmt.Println("  Without --exec, only the files on disk are read. Flags may come before or after the")
	fmt.Println("  package name. A usage error (unknown flag, negative --depth, --json with --svg, more")
	fmt.Println("  than one package) exits with status 2.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                      Show dependency graph for all projects\n", colorCommand("xpm graph"))
	fmt.Printf("  %s             Show graph for specific package\n", colorCommand("xpm graph <package>"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("OPTIONS:"))
	fmt.Printf("  %s                         Export graph as JSON (cannot be combined with --svg)\n", colorOption("--json"))
	fmt.Printf("  %s                          Generate SVG visualization (needs GraphViz)\n", colorOption("--svg"))
	fmt.Printf("  %s                   Tree depth below the roots (default: graph.depth; 0 = unlimited)\n", colorOption("--depth N"))
	fmt.Printf("  %s                         Run mvn/gradle/go to resolve full trees (default: files only)\n", colorOption("--exec"))
	fmt.Printf("  %s                    Generate combined graph for all workspaces (-w)\n", colorOption("--workspace"))
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
	fmt.Printf("%s\n", colorCommand("xpm search [query ...]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Search all ecosystems' registries. Several words are one query.")
	fmt.Println("  The interactive TUI opens only when stdin and stdout are terminals and the")
	fmt.Println("  interactive and searchUI.enabled settings are true; otherwise results are printed")
	fmt.Println("  as plain text, and a query is required.")
	fmt.Println("  In plain output, registries that do not answer within 2.5 s (by default) are listed as Unavailable.")
	fmt.Println("  Plain output exits with status 1 when nothing matches or every registry fails.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                     Open interactive search UI (terminal only)\n", colorCommand("xpm search"))
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
	fmt.Println("  List detected workspaces/monorepos (experimental)")
	fmt.Println("  This command is experimental and is being reworked; its behaviour and output may change.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("SYNTAX:"))
	fmt.Printf("  %s                 List all detected workspaces\n", colorCommand("xpm workspaces"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("RELATED COMMANDS:"))
	fmt.Printf("  %s     Run task across workspaces\n", colorCommand("xpm run --workspace <task>"))
}

func showEnvHelp() {
	fmt.Printf("%s\n", colorCommand("xpm env <command> [arguments]"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("DESCRIPTION:"))
	fmt.Println("  Manage runtime versions (node, python, go, java, rust, bun, deno) (experimental)")
	fmt.Println("  This command is experimental and is being reworked; its behaviour and output may change.")
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
	fmt.Printf("  %s              Set a configuration value (see KEYS)\n", colorCommand("set <key> <value>"))
	fmt.Printf("  %s                          Reset to default configuration\n", colorCommand("reset"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("KEYS (config set):"))
	fmt.Printf("  %s       Comma-separated manager IDs: npm, yarn, pnpm, bun, pip, poetry,\n", colorOption("prefer"))
	fmt.Println("                 pipenv, composer, cargo, gomod, maven, gradle")
	fmt.Printf("  %s  true/false (also yes/no, on/off, 1/0)\n", colorOption("autoInstallPM"))
	fmt.Printf("  %s    true/false (also yes/no, on/off, 1/0)\n", colorOption("interactive"))
	fmt.Printf("  %s  true/false (also yes/no, on/off, 1/0): enable or disable one registry;\n                 id is one of npm, pip, composer, cargo, maven\n", colorOption("search.<id>"))
	fmt.Println("  Values are validated before anything is written; unknown keys are rejected.")
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                Show current config\n", colorExample("xpm config show"))
	fmt.Printf("  %s Never prompt\n", colorExample("xpm config set interactive false"))
	fmt.Printf("  %s   Prefer npm, then pip\n", colorExample("xpm config set prefer npm,pip"))
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
	fmt.Printf("  %s\n", colorCommand("xpm -V"))
	fmt.Printf("  %s\n", colorCommand("xpm --version"))
	fmt.Printf("  %s                         -v alone; with a command, -v means --verbose\n", colorCommand("xpm -v"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("EXAMPLES:"))
	fmt.Printf("  %s                    Show version\n", colorExample("xpm version"))
	fmt.Printf("  %s                         Show version (shorthand)\n", colorExample("xpm -v"))
	fmt.Println()
	fmt.Printf("%s\n", colorSection("ALIASES:"))
	fmt.Printf("  %s\n", colorCommand("version, -V, --version, -v (alone)"))
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
	fmt.Printf("%s\n", colorSection("GLOBAL FLAGS (before the command):"))
	fmt.Printf("  %s                 Verbose logging, e.g. xpm -v install axios\n", colorOption("-v, --verbose"))
	fmt.Printf("  %s                 Show version\n", colorOption("-V, --version"))
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
