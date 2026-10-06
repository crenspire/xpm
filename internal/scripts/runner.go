package scripts

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/crenspire/xpm/internal/pm"
)

// RunScript executes a script using the appropriate method based on its source.
// It pipes output to the terminal in real-time.
func RunScript(script ScriptDefinition, extraArgs []string) error {
	var cmd *exec.Cmd

	switch script.Source {
	case SourceNPM:
		cmd = buildNodeCommand(script, extraArgs)
	case SourceComposer:
		cmd = buildComposerCommand(script, extraArgs)
	case SourcePython:
		cmd = buildPythonCommand(script, extraArgs)
	case SourceCargo:
		cmd = buildCargoCommand(script, extraArgs)
	default:
		cmd = buildShellCommand(script, extraArgs)
	}

	// Connect to terminal for live I/O
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Print what we're running
	meta, _ := pm.MetaFor(script.PM)
	if meta.Name != "" {
		fmt.Printf("Running %q via %s...\n\n", script.Name, meta.Name)
	} else {
		fmt.Printf("Running %q from %s...\n\n", script.Name, script.Path)
	}

	return cmd.Run()
}

// buildNodeCommand creates the command for Node.js package managers.
func buildNodeCommand(script ScriptDefinition, extraArgs []string) *exec.Cmd {
	switch script.PM {
	case pm.Npm:
		args := []string{"run", script.Name}
		if len(extraArgs) > 0 {
			args = append(args, "--")
			args = append(args, extraArgs...)
		}
		return exec.Command("npm", args...)

	case pm.Yarn:
		// Yarn doesn't need -- to pass args
		args := []string{"run", script.Name}
		args = append(args, extraArgs...)
		return exec.Command("yarn", args...)

	case pm.Pnpm:
		args := []string{"run", script.Name}
		if len(extraArgs) > 0 {
			args = append(args, "--")
			args = append(args, extraArgs...)
		}
		return exec.Command("pnpm", args...)

	case pm.Bun:
		args := []string{"run", script.Name}
		args = append(args, extraArgs...)
		return exec.Command("bun", args...)

	default:
		// Fallback to npm
		args := []string{"run", script.Name}
		if len(extraArgs) > 0 {
			args = append(args, "--")
			args = append(args, extraArgs...)
		}
		return exec.Command("npm", args...)
	}
}

// buildComposerCommand creates the command for Composer scripts.
func buildComposerCommand(script ScriptDefinition, extraArgs []string) *exec.Cmd {
	args := []string{"run-script", script.Name}
	if len(extraArgs) > 0 {
		args = append(args, "--")
		args = append(args, extraArgs...)
	}
	return exec.Command("composer", args...)
}

// shellCommand runs command through `sh -c`. Extra arguments are passed as
// positional parameters and appended as "$@", so the shell never re-splits
// or evaluates them. Trailing whitespace is trimmed first so a script ending
// in a newline does not put "$@" on a line of its own. On Windows this needs an `sh` on PATH (Git for Windows,
// MSYS2 or WSL), exactly as before; xpm does not translate scripts to cmd.exe.
func shellCommand(command string, extraArgs []string) *exec.Cmd {
	if len(extraArgs) == 0 {
		return exec.Command("sh", "-c", command)
	}
	args := append([]string{"-c", strings.TrimRight(command, " \t\r\n") + ` "$@"`, "sh"}, extraArgs...)
	return exec.Command("sh", args...)
}

// buildPythonCommand creates the command for Python scripts.
// Python scripts are executed directly via shell.
func buildPythonCommand(script ScriptDefinition, extraArgs []string) *exec.Cmd {
	return shellCommand(script.Command, extraArgs)
}

// buildCargoCommand creates the command for Cargo scripts.
// Cargo scripts are executed directly via shell.
func buildCargoCommand(script ScriptDefinition, extraArgs []string) *exec.Cmd {
	return shellCommand(script.Command, extraArgs)
}

// buildShellCommand creates a generic shell command.
func buildShellCommand(script ScriptDefinition, extraArgs []string) *exec.Cmd {
	return shellCommand(script.Command, extraArgs)
}

// RunScriptByName finds and runs a script by name from the merged scripts.
func RunScriptByName(merged *MergedScripts, name string, extraArgs []string) error {
	script, found := merged.GetScript(name)
	if !found {
		return fmt.Errorf("script %q not found", name)
	}
	return RunScript(*script, extraArgs)
}
