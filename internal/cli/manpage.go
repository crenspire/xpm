package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

// GenerateManPage generates a man page for a specific command.
func GenerateManPage(command string) (string, error) {
	normalized := normalizeCommand(command)

	manPageTemplate := `.\" Man page for xpm {{.Command}}
.TH "XPM-{{upper .Command}}" "1" "{{.Date}}" "xpm {{.Version}}" "User Commands"
.SH NAME
xpm {{.Command}} \- {{.Description}}
.SH SYNOPSIS
{{.Synopsis}}
.SH DESCRIPTION
{{.Description}}
.PP
{{.FullDescription}}
.SH OPTIONS
{{.Options}}
.SH EXAMPLES
{{.Examples}}
.SH EXIT STATUS
0 on success; 1 on errors, invalid arguments, cancelled install prompts, no matches, or a refused non-interactive choice.
2 on a \fBgraph\fR or \fBcompletion\fR usage error (bad flag or argument).
\fBoutdated\fR: 0 all current, 1 some outdated, 2 usage error or incomplete check.
\fBaudit\fR: 0 no known vulnerabilities, 1 vulnerabilities found, 2 usage error or the check could not be completed.
\fBinstall\fR (without packages), \fBci\fR, \fBlist\fR, \fBupdate\fR, \fBremove\fR and \fBrun\fR pass through the underlying tool's exit code; \fBrun -w\fR exits 1 if any project fails.
.SH ENVIRONMENT
.TP
\fBXPM_NO_CACHE\fR (any non-empty value, e.g. 1)
Skip the on-disk registry lookup cache.
.TP
\fBXPM_CACHE_DIR\fR=\fIdir\fR
Keep the lookup cache in \fIdir\fR/lookups instead of the OS cache folder.
.SH FILES
~/.config/xpm/xpmrc.json (Windows: %APPDATA%\expm\expmrc.json)
.SH SEE ALSO
{{.SeeAlso}}
.SH AUTHOR
Crenspire
.SH COPYRIGHT
Copyright (C) 2024 Crenspire
`

	tmpl, err := template.New("manpage").Funcs(template.FuncMap{"upper": strings.ToUpper}).Parse(manPageTemplate)
	if err != nil {
		return "", err
	}

	data := getManPageData(normalized)

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// manPageData holds data for generating a man page.
type manPageData struct {
	Command         string
	Description     string
	Synopsis        string
	FullDescription string
	Options         string
	Examples        string
	SeeAlso         string
	Date            string
	Version         string
}

// getManPageData returns man page data for a command.
func getManPageData(command string) manPageData {
	data := manPageData{
		Command: command,
		Date:    "2024",
		Version: versionString(),
	}

	switch command {
	case "install":
		data.Description = "Install packages or project dependencies"
		data.Synopsis = `.B xpm install
[\fB-g\fR|\fB--global\fR] [\fB-w\fR|\fB--workspace\fR] [\fIpackage\fR[@\fIversion\fR] ...] [\fB--\fR]`
		data.FullDescription = `Install one or several packages, or, without packages, the dependencies of the detected project.
Several packages are installed in order; xpm stops at the first failure.
Without packages, the detected project's tool runs its install (with several projects, a terminal asks which one or all; a script runs all).
\fB-g\fR without packages is an error: project dependencies are never global.
Exit status 1 when no registry has a match or an install fails.
.PP
How xpm picks a tool: inside a project, an exact hit in the project's own ecosystem is installed with the project's tool (its lockfile decides, e.g. yarn.lock selects yarn).
Outside a project, and for every \fB-g\fR install, exact hits from a single ecosystem are installed; with several, the \fBprefer\fR config list decides, else a terminal shows a menu and a script gets an error listing the candidates.
.PP
A Go module path (github.com/gin-gonic/gin) runs \fBgo get\fR (with \fB-g\fR, \fBgo install\fR \fImodule\fR@latest or @\fIversion\fR).
A Maven or Gradle hit only prints the dependency snippet to add.`
		data.Options = `.TP
\fB-g\fR, \fB--global\fR
Install globally (if the tool supports it); may come before or after the packages.
.TP
\fB-w\fR, \fB--workspace\fR
Install dependencies in every workspace project (honours workspace.include/exclude and workspace.parallel); cannot be combined with packages or \fB-g\fR.
.TP
\fB--\fR
Ends flag parsing; names starting with - are still rejected by name validation.
.PP
Any other flag, including \fB--global=false\fR, is rejected.`
		data.Examples = `.B xpm install
Install dependencies for detected projects
.PP
.B xpm install axios
Install axios (searches all ecosystems)
.PP
.B xpm install axios lodash
Install several packages in order
.PP
.B xpm install axios@1.0.0
Install specific version
.PP
.B xpm install typescript -g
Install globally
.PP
.B xpm install github.com/gin-gonic/gin
go get a Go module`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm ci\fR(1), \fBxpm update\fR(1), \fBxpm remove\fR(1)`

	case "ci":
		data.Description = "Frozen install from lockfiles; deletes nothing unasked"
		data.Synopsis = `.B xpm ci`
		data.FullDescription = `Install every detected project's dependencies from its lockfile, using the tool's strict form where one exists:
\fBnpm ci\fR, \fBpnpm\fR/\fByarn\fR/\fBbun install --frozen-lockfile\fR, \fByarn install --immutable\fR (yarn 2+), \fBpipenv install --deploy\fR, \fBcargo build --locked\fR, \fBgo mod download\fR.
Other tools have no frozen flag and run their normal install: \fBcomposer install\fR, \fBpip install -r requirements.txt\fR (\fBpip install .\fR for pyproject.toml), \fBpoetry install\fR, \fBmvn install\fR, \fBgradle build\fR.
.PP
xpm checks every project's tool before running any install.
It deletes nothing unasked: it never deletes lockfiles, and asks before removing node_modules/ (yarn, pnpm, bun) or Composer's vendor/.
ci takes no arguments.`
		data.Options = ""
		data.Examples = `.B xpm ci
Frozen install for every detected project`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "run":
		data.Description = "Run project scripts"
		data.Synopsis = `.B xpm run
[\fB-w\fR|\fB--workspace\fR] [\fItask\fR] [\fB--\fR \fIargs\fR]`
		data.FullDescription = `Run scripts defined in package.json, composer.json, pyproject.toml, or Cargo.toml.
If no task is specified, lists all available scripts.
.PP
pyproject.toml: [tool.xpm.scripts] ([tool.upm.scripts] when that table is absent or empty), else [tool.poetry.scripts], else [project.scripts].
Cargo.toml: [package.metadata.xpm.scripts] ([package.metadata.upm.scripts] fallback).
package.json scripts run with npm, yarn, pnpm or bun run; composer.json scripts with \fBcomposer run-script\fR; pyproject.toml and Cargo.toml scripts run with \fBsh -c\fR.
Arguments after \fB--\fR are passed on: to npm and pnpm as \fBrun\fR \fItask\fR \fB--\fR \fIargs\fR, to yarn and bun as \fBrun\fR \fItask\fR \fIargs\fR, to composer as \fBrun-script\fR \fItask\fR \fB--\fR \fIargs\fR.
For pyproject.toml and Cargo.toml scripts they are separate, unexpanded arguments appended to the script command as "$@" (a script that already uses "$@" receives them twice).
With \fB-w\fR they go to the task in every project.
The exit status is the script's.`
		data.Options = `.TP
\fB-w\fR, \fB--workspace\fR
Run task in all workspace projects (experimental)`
		data.Examples = `.B xpm run
List all available scripts
.PP
.B xpm run dev
Run the 'dev' script
.PP
.B xpm run build -- --watch
Run 'build' with --watch argument`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "which":
		data.Description = "Check which ecosystems have a package"
		data.Synopsis = `.B xpm which
\fIpackage\fR`
		data.FullDescription = `Search for a package across all supported ecosystems and show where it's available.
Registries that do not answer within 2.5 s (by default) are listed as Unavailable.
Exit status 1 when nothing matches or every registry fails.`
		data.Options = ""
		data.Examples = `.B xpm which lodash
Find lodash across all registries`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm search\fR(1), \fBxpm info\fR(1)`

	case "list":
		data.Description = "List installed packages"
		data.Synopsis = `.B xpm list`
		data.FullDescription = `List installed packages for the current project.`
		data.Options = ""
		data.Examples = `.B xpm list
List all installed packages`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm info\fR(1)`

	case "outdated":
		data.Description = "Show dependencies with newer versions"
		data.Synopsis = `.B xpm outdated
[\fB--json\fR] [\fB--all\fR] [\fB--workspace\fR|\fB-w\fR]`
		data.FullDescription = `Show which of the project's dependencies have a newer version in their registry, across ecosystems.
Versions come from the lockfiles; only dependencies with a locked version are checked, the rest are counted in a note on stderr (add a lockfile).
Registries used: npm, PyPI, Packagist, the crates.io sparse index, Maven Central and proxy.golang.org. Go modules matched by GOPRIVATE or GONOPROXY are not looked up, nor are registries turned off in the config. Lookups use the registry lookup cache and the registry timeouts from the config.
The table lists every dependency that is not current, with a summary line. With \fB--json\fR, stdout is one JSON document with \fBdependencies\fR and \fBunchecked\fR arrays.
A package a registry does not know (for example a private package) is shown as \fBnot found\fR and does not change the exit status; a lookup that failed is shown as \fBunavailable\fR.
Takes no arguments; an unknown flag or an argument is a usage error (exit status 2).`
		data.Options = `.TP
\fB--json\fR
Print the result as JSON
.TP
\fB--all\fR
Check every locked package, not only direct dependencies
.TP
\fB--workspace\fR, \fB-w\fR
Combine all workspace projects`
		data.Examples = `.B xpm outdated
Show outdated direct dependencies
.PP
.B xpm outdated --all --json
Check every locked package and print JSON`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm update\fR(1), \fBxpm list\fR(1)`

	case "audit":
		data.Description = "Check dependencies for known vulnerabilities"
		data.Synopsis = `.B xpm audit
[\fB--json\fR] [\fB--timeout\fR \fIduration\fR] [\fB--workspace\fR|\fB-w\fR]`
		data.FullDescription = `Check the project's locked dependencies against the OSV.dev vulnerability database, across ecosystems (npm, PyPI, Packagist, crates.io, Go and Maven).
Only dependencies with a locked version are checked; the rest are counted in a note on stderr, and listed with a reason under \fBunchecked\fR in the JSON output.
Privacy: package names and versions from the lockfiles are sent to api.osv.dev, one batch request per 1000 packages plus one details request per distinct vulnerability found. Nothing else is sent.
\fBxpm doctor\fR is separate: it keeps running the ecosystems' own audit tools (npm audit, pip-audit and so on), which do not go through OSV.
Each vulnerable package is listed with its vulnerabilities, severity and fixed versions. With \fB--json\fR, stdout is one JSON document with \fBscanned\fR, \fBvulnerable\fR and \fBunchecked\fR.
Takes no arguments; an unknown flag or an argument is a usage error (exit status 2).`
		data.Options = `.TP
\fB--json\fR
Print the result as JSON
.TP
\fB--timeout\fR \fIduration\fR
Time limit for the OSV.dev queries (default 30s; a bare number is seconds)
.TP
\fB--workspace\fR, \fB-w\fR
Combine all workspace projects`
		data.Examples = `.B xpm audit
Audit the locked dependencies
.PP
.B xpm audit --json --timeout 10s
Audit with a 10 second limit and print JSON`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm outdated\fR(1), \fBxpm doctor\fR(1)`

	case "update":
		data.Description = "Update packages"
		data.Synopsis = `.B xpm update
[\fIpackage\fR]`
		data.FullDescription = `Update packages in the current project. If no package is specified, updates all packages.`
		data.Options = ""
		data.Examples = `.B xpm update
Update all packages
.PP
.B xpm update axios
Update axios to latest version`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "remove":
		data.Description = "Remove a package"
		data.Synopsis = `.B xpm remove
\fIpackage\fR`
		data.FullDescription = `Remove a package from the current project.`
		data.Options = ""
		data.Examples = `.B xpm remove axios
Remove axios from project`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "info":
		data.Description = "Show detailed package information"
		data.Synopsis = `.B xpm info
\fIpackage\fR`
		data.FullDescription = `Show detailed package information from all ecosystems.
Registries that do not answer within 2.5 s (by default) are listed as Unavailable.
Exit status 1 when nothing matches or every registry fails.`
		data.Options = ""
		data.Examples = `.B xpm info express
Get detailed info about express`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm which\fR(1), \fBxpm search\fR(1)`

	case "lock":
		data.Description = "Generate or verify unified lockfile (experimental)"
		data.Synopsis = `.B xpm lock
[\fB--verify\fR]`
		data.FullDescription = `Generate or verify unified lockfile (xpm-lock.yaml) that consolidates metadata from all ecosystem lockfiles.
This command is experimental and is being reworked; its behaviour and output may change.`
		data.Options = `.TP
\fB--verify\fR
Verify lockfiles haven't changed since last generation.
Each recorded lockfile is reported as unchanged, changed or missing.
Supported lockfiles on disk that xpm-lock.yaml does not record are reported as added.
Unreadable entries and recorded paths outside the project are errors.
Exit status 0 when every recorded lockfile is unchanged and none was added (or there is nothing to verify); 1 otherwise, including when xpm-lock.yaml does not exist.`
		data.Examples = `.B xpm lock
Generate unified lockfile
.PP
.B xpm lock --verify
Check if lockfiles changed`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "graph":
		data.Description = "Show unified dependency graph (experimental)"
		data.Synopsis = `.B xpm graph
[\fIpackage\fR] [\fB--json\fR] [\fB--svg\fR] [\fB--depth\fR \fIN\fR] [\fB--exec\fR] [\fB--workspace\fR|\fB-w\fR]`
		data.FullDescription = `Show unified dependency graph across all ecosystems.
This command is experimental and is being reworked; its behaviour and output may change.
Without \fB--exec\fR, only the files on disk are read.
Flags may come before or after the package name.
A usage error (unknown flag, negative \fB--depth\fR, \fB--json\fR with \fB--svg\fR, more than one package) exits with status 2.`
		data.Options = `.TP
\fB--json\fR
Export graph as JSON (cannot be combined with \fB--svg\fR)
.TP
\fB--svg\fR
Generate SVG visualization (needs GraphViz)
.TP
\fB--depth\fR \fIN\fR
Tree depth below the roots (default: \fBgraph.depth\fR; 0 = unlimited)
.TP
\fB--exec\fR
Run mvn/gradle/go to resolve full trees (default: files only)
.TP
\fB--workspace\fR, \fB-w\fR
Generate combined graph for all workspaces`
		data.Examples = `.B xpm graph
Show full dependency graph
.PP
.B xpm graph react
Show graph for react
.PP
.B xpm graph --json
Export as JSON`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm list\fR(1)`

	case "search":
		data.Description = "Search all registries"
		data.Synopsis = `.B xpm search
[\fIquery\fR ...]`
		data.FullDescription = `Search all ecosystems' registries. Several words are one query.
The interactive TUI opens only when stdin and stdout are terminals and the \fBinteractive\fR and \fBsearchUI.enabled\fR settings are true; otherwise results are printed as plain text, and a query is required.
In plain output, registries that do not answer within 2.5 s (by default) are listed as Unavailable.
Plain output exits with status 1 when nothing matches or every registry fails.`
		data.Options = ""
		data.Examples = `.B xpm search
Start interactive search (terminal only)
.PP
.B xpm search axios
Search for axios`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm which\fR(1), \fBxpm info\fR(1)`

	case "workspaces":
		data.Description = "List detected workspaces/monorepos (experimental)"
		data.Synopsis = `.B xpm workspaces`
		data.FullDescription = `List detected workspaces/monorepos.
Related: \fBxpm install --workspace\fR installs dependencies in every workspace project and \fBxpm run --workspace\fR runs a task in each.
This command is experimental and is being reworked; its behaviour and output may change.`
		data.Options = ""
		data.Examples = `.B xpm workspaces
List all detected workspaces
.PP
.B xpm install --workspace
Install dependencies in every workspace project`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm run\fR(1), \fBxpm install\fR(1)`

	case "env":
		data.Description = "Manage runtime versions (experimental)"
		data.Synopsis = `.B xpm env
\fIcommand\fR [\fIarguments\fR]`
		data.FullDescription = `Manage runtime versions (node, python, go, java, rust, bun, deno, php).
This command is experimental and is being reworked; its behaviour and output may change.`
		data.Options = `.TP
\fBinstall\fR \fIruntime@version\fR
Install a runtime version
.TP
\fBuse\fR \fIruntime@version\fR
Switch to a runtime version
.TP
\fBlist\fR
List installed versions
.TP
\fBls-remote\fR \fIruntime\fR
List available remote versions
.TP
\fBcurrent\fR
Show active runtime versions
.TP
\fBremove\fR \fIruntime@version\fR
Remove a runtime version`
		data.Examples = `.B xpm env install node@20.11.0
Install Node.js 20.11.0
.PP
.B xpm env use node@20.11.0
Switch to Node.js 20.11.0`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "doctor":
		data.Description = "Comprehensive environment & project diagnostics"
		data.Synopsis = `.B xpm doctor`
		data.FullDescription = `Run comprehensive diagnostics on your environment and project.`
		data.Options = ""
		data.Examples = `.B xpm doctor
Run full diagnostic check`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm config\fR(1)`

	case "config":
		data.Description = "View or edit configuration"
		data.Synopsis = `.B xpm config
[\fIsubcommand\fR]`
		data.FullDescription = `View or edit xpm configuration.`
		data.Options = `.TP
\fBshow\fR
Display current configuration
.TP
\fBpath\fR
Show config file path
.TP
\fBedit\fR
Open config in editor
.TP
\fBset\fR \fIkey\fR \fIvalue\fR
Set a configuration value (keys below)
.TP
\fBreset\fR
Reset to default configuration
.PP
Keys accepted by \fBset\fR (values are validated before anything is written; unknown keys are rejected):
.TP
\fBprefer\fR
Comma-separated manager IDs: npm, yarn, pnpm, bun, pip, poetry, pipenv, composer, cargo, gomod, maven, gradle
.TP
\fBautoInstallPM\fR
true/false (also yes/no, on/off, 1/0)
.TP
\fBinteractive\fR
true/false (also yes/no, on/off, 1/0)
.TP
\fBsearch.\fR\fIid\fR
true/false (also yes/no, on/off, 1/0): enable or disable one registry; id is one of npm, pip, composer, cargo, maven`
		data.Examples = `.B xpm config show
Show current config
.PP
.B xpm config set interactive false
Never prompt
.PP
.B xpm config set prefer npm,pip
Prefer npm, then pip`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm doctor\fR(1)`

	case "version":
		data.Description = "Show version information"
		data.Synopsis = `.B xpm version
.br
.B xpm -V
| \fB--version\fR | \fB-v\fR`
		data.FullDescription = `Show xpm version information.
\fB-v\fR shows the version only when it is the sole argument; before a command it means \fB--verbose\fR.`
		data.Options = ""
		data.Examples = `.B xpm version
Show version
.PP
.B xpm -v
Show version (shorthand)`
		data.SeeAlso = `\fBxpm\fR(1)`

	case "help":
		data.Description = "Show help message"
		data.Synopsis = `.B xpm help
[\fB-h\fR|\fB--help\fR]`
		data.FullDescription = `Show brief help message.`
		data.Options = `Global flags go before the command:
.TP
\fB-v\fR, \fB--verbose\fR
Verbose logging (e.g. xpm -v install axios)
.TP
\fB-V\fR, \fB--version\fR
Show version`
		data.Examples = `.B xpm help
Show help`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm man\fR(1)`

	case "man":
		data.Description = "Show detailed manual for commands"
		data.Synopsis = `.B xpm man
[\fIcommand\fR]`
		data.FullDescription = `Show detailed manual for commands. Without arguments, lists all available commands.`
		data.Options = ""
		data.Examples = `.B xpm man
List commands
.PP
.B xpm man install
Show install command help`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm help\fR(1)`

	case "completion":
		data.Description = "Print a shell completion script (bash, zsh, fish)"
		data.Synopsis = `.B xpm completion
\fI<bash|zsh|fish>\fR`
		data.FullDescription = `Print a completion script for the given shell on standard output. It completes command names and aliases, the flags of each command, and the arguments of \fBman\fR, \fBcompletion\fR, \fBconfig\fR and \fBenv\fR; everything else falls back to file completion.
Exactly one argument is required; anything else is a usage error (exit status 2).`
		data.Options = `.TP
\fBbash\fR
\fBsource <(xpm completion bash)\fR, or save the output to /usr/local/etc/bash_completion.d/xpm or ~/.local/share/bash-completion/completions/xpm.
.TP
\fBzsh\fR
\fBxpm completion zsh > "${fpath[1]}/_xpm"\fR
.TP
\fBfish\fR
\fBxpm completion fish > ~/.config/fish/completions/xpm.fish\fR`
		data.Examples = `.B source <(xpm completion bash)
Enable completion in the current bash session
.PP
.B xpm completion fish > ~/.config/fish/completions/xpm.fish
Install fish completion`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm help\fR(1)`

	default:
		data.Description = "xpm command"
		data.Synopsis = `.B xpm
\fIcommand\fR`
		data.FullDescription = `Cross-ecosystem package manager front end.`
		data.Options = ""
		data.Examples = ""
		data.SeeAlso = `\fBxpm help\fR(1), \fBxpm man\fR(1)`
	}

	return data
}

// InstallManPage installs a man page to the system man path.
func InstallManPage(command string) error {
	content, err := GenerateManPage(command)
	if err != nil {
		return fmt.Errorf("failed to generate man page: %w", err)
	}

	manPath, err := getManPath()
	if err != nil {
		return fmt.Errorf("failed to determine man path: %w", err)
	}

	manDir := filepath.Join(manPath, "man1")
	if err := os.MkdirAll(manDir, 0755); err != nil {
		return fmt.Errorf("failed to create man directory: %w", err)
	}

	manFile := filepath.Join(manDir, fmt.Sprintf("xpm-%s.1", command))
	if err := os.WriteFile(manFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write man page: %w", err)
	}

	fmt.Printf("Installed man page: %s\n", manFile)
	return nil
}

// getManPath returns the system man path.
func getManPath() (string, error) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		// Try common locations
		paths := []string{
			"/usr/local/share/man",
			"/usr/share/man",
			filepath.Join(os.Getenv("HOME"), ".local", "share", "man"),
		}

		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}

		// Default to user-local if available
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share", "man"), nil
	}

	// Windows or other - use user directory
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "man"), nil
}

// GenerateAllManPages generates man pages for all commands.
func GenerateAllManPages(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for _, c := range commandTable {
		cmd := c.name
		content, err := GenerateManPage(cmd)
		if err != nil {
			return fmt.Errorf("failed to generate man page for %s: %w", cmd, err)
		}

		manFile := filepath.Join(outputDir, fmt.Sprintf("xpm-%s.1", cmd))
		if err := os.WriteFile(manFile, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write man page for %s: %w", cmd, err)
		}

		fmt.Printf("Generated: %s\n", manFile)
	}

	return nil
}
