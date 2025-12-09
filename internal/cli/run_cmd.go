package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/crenspire/xpm/internal/scripts"
)

// cmdRun handles the `xpm run` command.
// Without arguments, it lists available scripts from all detected config files.
// With a script name, it runs that script using the appropriate package manager.
func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	workspaceFlag := fs.Bool("workspace", false, "run task in all workspace projects")
	wShort := fs.Bool("w", false, "run task in all workspace projects (shorthand)")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	runArgs := fs.Args()
	workspace := *workspaceFlag || *wShort

	// Handle workspace run
	if workspace {
		if len(runArgs) == 0 {
			fmt.Fprintln(os.Stderr, "error: task name required for workspace run")
			fmt.Fprintln(os.Stderr)
			showCommandUsage("run")
			return 1
		}
		return cmdRunWorkspace(runArgs[0])
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error getting current directory:", err)
		return 1
	}

	// Load scripts from all detected config files
	// Use scripts.prefer from config for precedence
	merged, err := scripts.LoadAllScripts(cwd, cfg.Scripts.Prefer)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error loading scripts:", err)
		return 1
	}

	// If no scripts found anywhere
	if merged.IsEmpty() {
		fmt.Fprintln(os.Stderr, "No tasks found.")
		fmt.Fprintln(os.Stderr, "\nSupported config files:")
		fmt.Fprintln(os.Stderr, "  - package.json (npm/yarn/pnpm/bun scripts)")
		fmt.Fprintln(os.Stderr, "  - composer.json (PHP/Composer scripts)")
		fmt.Fprintln(os.Stderr, "  - pyproject.toml ([tool.upm.scripts] section)")
		fmt.Fprintln(os.Stderr, "  - Cargo.toml ([package.metadata.upm.scripts] section)")
		return 1
	}

	// If no script name provided, list available scripts
	if len(runArgs) == 0 {
		fmt.Println(merged.FormatList())
		fmt.Printf("\nRun a task: xpm run <task>\n")
		fmt.Printf("Pass arguments: xpm run <task> -- <args>\n")
		return 0
	}

	scriptName := runArgs[0]
	script, found := merged.GetScript(scriptName)
	if !found {
		fmt.Fprintf(os.Stderr, "error: task %q not found\n\n", scriptName)
		showCommandUsage("run")
		return 1
	}

	// Collect extra arguments after --
	var extraArgs []string
	for i, arg := range runArgs {
		if arg == "--" && i+1 < len(runArgs) {
			extraArgs = runArgs[i+1:]
			break
		}
	}

	// Run the script
	if err := scripts.RunScript(*script, extraArgs); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "error running task:", err)
		return 1
	}

	return 0
}
