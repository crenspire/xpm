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
.TH XPM {{.Command}} "1" "{{.Date}}" "xpm {{.Version}}" "User Commands"
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
.SH SEE ALSO
{{.SeeAlso}}
.SH AUTHOR
Crenspire
.SH COPYRIGHT
Copyright (C) 2024 Crenspire
`

	tmpl, err := template.New("manpage").Parse(manPageTemplate)
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
	Command        string
	Description    string
	Synopsis       string
	FullDescription string
	Options        string
	Examples       string
	SeeAlso        string
	Date           string
	Version        string
}

// getManPageData returns man page data for a command.
func getManPageData(command string) manPageData {
	data := manPageData{
		Command: command,
		Date:    "2024",
		Version: Version,
	}

	switch command {
	case "install":
		data.Description = "Install packages or project dependencies"
		data.Synopsis = `.B xpm install
[\fIpackage[@version]\fR] [\fB-g\fR|\fB--global\fR]`
		data.FullDescription = `Install packages from all supported ecosystems or install dependencies for detected projects.
If no package is specified, xpm will auto-detect project types and install their dependencies.`
		data.Options = `.TP
\fB-g\fR, \fB--global\fR
Install globally (if supported by the package manager)`
		data.Examples = `.B xpm install
Install dependencies for detected projects
.PP
.B xpm install axios
Install axios (searches all ecosystems)
.PP
.B xpm install axios@1.0.0
Install specific version`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm ci\fR(1), \fBxpm update\fR(1), \fBxpm remove\fR(1)`

	case "run":
		data.Description = "Run project scripts"
		data.Synopsis = `.B xpm run
[\fItask\fR] [\fB--\fR \fIargs\fR] [\fB-w\fR|\fB--workspace\fR]`
		data.FullDescription = `Run scripts defined in package.json, composer.json, pyproject.toml, or Cargo.toml.
If no task is specified, lists all available scripts.`
		data.Options = `.TP
\fB-w\fR, \fB--workspace\fR
Run task in all workspace projects`
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
		data.FullDescription = `Search for a package across all supported ecosystems and show where it's available.`
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
		data.FullDescription = `Show detailed package information from all ecosystems.`
		data.Options = ""
		data.Examples = `.B xpm info express
Get detailed info about express`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm which\fR(1), \fBxpm search\fR(1)`

	case "lock":
		data.Description = "Generate or verify unified lockfile"
		data.Synopsis = `.B xpm lock
[\fB--verify\fR]`
		data.FullDescription = `Generate or verify unified lockfile (xpm-lock.yaml) that consolidates metadata from all ecosystem lockfiles.`
		data.Options = `.TP
\fB--verify\fR
Verify lockfiles haven't changed since last generation`
		data.Examples = `.B xpm lock
Generate unified lockfile
.PP
.B xpm lock --verify
Check if lockfiles changed`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "cache":
		data.Description = "Manage dependency cache"
		data.Synopsis = `.B xpm cache
\fIsubcommand\fR`
		data.FullDescription = `Manage the global dependency cache.`
		data.Options = `.TP
\fBtree\fR
Show cache structure and contents
.TP
\fBsize\fR
Show total cache size and statistics
.TP
\fBclean\fR
Clear the entire cache
.TP
\fBgc\fR
Run garbage collection (removes old/unused items)
.TP
\fBverify\fR
Verify integrity of cached items
.TP
\fBrepair\fR
Attempt to repair corrupted cache entries
.TP
\fBpath\fR
Show the cache directory path`
		data.Examples = `.B xpm cache tree
Show cache tree
.PP
.B xpm cache clean
Clear all cached artifacts`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "graph":
		data.Description = "Show unified dependency graph"
		data.Synopsis = `.B xpm graph
[\fIpackage\fR] [\fB--json\fR] [\fB--svg\fR] [\fB--workspace\fR]`
		data.FullDescription = `Show unified dependency graph across all ecosystems.`
		data.Options = `.TP
\fB--json\fR
Export graph as JSON
.TP
\fB--svg\fR
Generate SVG visualization
.TP
\fB--workspace\fR
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
		data.Description = "Interactive TUI package search"
		data.Synopsis = `.B xpm search
[\fIquery\fR]`
		data.FullDescription = `Interactive TUI package search across all ecosystems.`
		data.Options = ""
		data.Examples = `.B xpm search
Start interactive search
.PP
.B xpm search axios
Search for axios`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm which\fR(1), \fBxpm info\fR(1)`

	case "workspaces":
		data.Description = "List detected workspaces/monorepos"
		data.Synopsis = `.B xpm workspaces`
		data.FullDescription = `List detected workspaces/monorepos.`
		data.Options = ""
		data.Examples = `.B xpm workspaces
List all detected workspaces`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm install\fR(1)`

	case "env":
		data.Description = "Manage runtime versions"
		data.Synopsis = `.B xpm env
\fIcommand\fR [\fIarguments\fR]`
		data.FullDescription = `Manage runtime versions (node, python, go, java, rust, bun, deno).`
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
Set a configuration value
.TP
\fBreset\fR
Reset to default configuration`
		data.Examples = `.B xpm config show
Show current config
.PP
.B xpm config set interactive false
Set interactive mode to false`
		data.SeeAlso = `\fBxpm\fR(1), \fBxpm doctor\fR(1)`

	case "version":
		data.Description = "Show version information"
		data.Synopsis = `.B xpm version
[\fB-v\fR|\fB--version\fR]`
		data.FullDescription = `Show xpm version information.`
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
		data.Options = ""
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

	default:
		data.Description = "xpm command"
		data.Synopsis = `.B xpm
\fIcommand\fR`
		data.FullDescription = `Universal Package Manager for multiple ecosystems.`
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
	commands := []string{
		"install", "run", "which", "list", "update", "remove", "info",
		"lock", "cache", "graph", "search", "workspaces", "env",
		"doctor", "config", "version", "help", "man",
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for _, cmd := range commands {
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

