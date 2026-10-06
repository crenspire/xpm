package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/graph"
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

// cmdRunWorkspace runs a task across all workspace projects.
func cmdRunWorkspace(task string) int {
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

// cmdGraphWorkspace generates a combined dependency graph for all workspace projects.
func cmdGraphWorkspace(jsonFlag, svgFlag bool, depthFlag string, rest []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	workspaces, err := workspace.DetectWorkspaces(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error detecting workspaces: %v\n", err)
		return 1
	}

	if len(workspaces) == 0 {
		fmt.Println("No workspaces detected.")
		return 1
	}

	// Extract graphs from all projects
	var allGraphs []*graph.DepGraph
	for _, ws := range workspaces {
		for _, project := range ws.Projects {
			// Extract graph for this project
			g, err := graph.ExtractAll(project.Path, graph.ExtractOptions{Warn: func(msg string) { fmt.Fprintln(os.Stderr, "warning:", msg) }})
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to extract graph for %s: %v\n", project.Name, err)
				continue
			}
			if g != nil {
				allGraphs = append(allGraphs, g)
			}
		}
	}

	if len(allGraphs) == 0 {
		fmt.Println("No dependency graphs found in workspaces.")
		return 1
	}

	// Merge all graphs
	merged := allGraphs[0]
	for i := 1; i < len(allGraphs); i++ {
		merged.Merge(allGraphs[i])
	}

	// Normalize merged graph
	merged.Normalize()

	// Load config
	cfg := config.Load()

	// Parse depth
	maxDepth := cfg.Graph.Depth
	if depthFlag != "" {
		if d, err := strconv.Atoi(depthFlag); err == nil {
			maxDepth = d
		}
	}

	// Detect warnings
	warnings := graph.DetectWarnings(merged)

	// Output based on flags
	if jsonFlag {
		if err := graph.WriteJSON(merged, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	} else if svgFlag {
		if err := graph.WriteSVG(merged, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	} else {
		// Default: tree output
		graph.PrintTree(merged, os.Stdout, graph.TreeOptions{ShowVersions: cfg.Graph.ShowVersions, ShowEcosystem: cfg.Graph.ShowEcosystem, MaxDepth: maxDepth})
	}

	// Print warnings
	if len(warnings) > 0 {
		graph.PrintWarnings(warnings, os.Stdout)
	}

	return 0
}
