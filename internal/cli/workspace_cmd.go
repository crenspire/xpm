package cli

import (
	"fmt"
	"os"

	"github.com/crenspire/xpm/internal/workspace"
)

// Test seams for workspace commands: nil/empty means the real thing.
var (
	workspaceRunner     workspace.Runner
	workspaceLookPath   func(string) (string, error)
	workspaceExecutable string
)

// loadWorkspaces detects the workspaces under the current directory and
// applies workspace.include / workspace.exclude from the config.
func loadWorkspaces() ([]workspace.Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	all, err := workspace.DetectWorkspaces(cwd)
	if err != nil {
		return nil, fmt.Errorf("detecting workspaces: %w", err)
	}
	return workspace.Filter(all, cfg.Workspace.Include, cfg.Workspace.Exclude), nil
}

// cmdWorkspaces lists detected workspaces.
func cmdWorkspaces(_ []string) int {
	workspaces, err := loadWorkspaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println(workspace.FormatWorkspaces(workspaces))
	return 0
}

// cmdInstallWorkspace installs dependencies in all workspace projects.
func cmdInstallWorkspace(global bool) int {
	if global {
		fmt.Fprintln(os.Stderr, "error: --global cannot be combined with --workspace")
		return 1
	}
	workspaces, err := loadWorkspaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "No workspaces detected.")
		return 1
	}
	err = workspace.Install(workspaces, workspace.InstallOptions{
		Parallel: cfg.Workspace.Parallel,
		Runner:   workspaceRunner,
		LookPath: workspaceLookPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println("\n✓ All workspace installations completed")
	return 0
}

// cmdRunWorkspace runs a task across all workspace projects, passing extra
// to the task in every project.
func cmdRunWorkspace(task string, extra []string) int {
	workspaces, err := loadWorkspaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "No workspaces detected.")
		return 1
	}
	err = workspace.Run(workspaces, task, workspace.RunOptions{
		Parallel:   cfg.Workspace.Parallel,
		Args:       extra,
		Prefer:     cfg.Scripts.Prefer,
		Executable: workspaceExecutable,
		Runner:     workspaceRunner,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println("\n✓ All workspace tasks completed")
	return 0
}
