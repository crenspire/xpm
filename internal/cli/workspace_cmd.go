package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/graph"
	"github.com/crenspire/xpm/internal/workspace"
)

// cmdWorkspaces lists detected workspaces.
func cmdWorkspaces(args []string) int {
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
		return 0
	}

	fmt.Println(workspace.FormatWorkspaces(workspaces))
	return 0
}

// cmdInstallWorkspace installs dependencies in all workspace projects.
//
//nolint:unused // wired in P6 (--workspace)
//lint:ignore U1000 wired in P6 (--workspace)
func cmdInstallWorkspace(global bool) int {
	if global {
		fmt.Fprintln(os.Stderr, "warning: --global flag is ignored for workspace installs")
	}

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

	parallel := cfg.Workspace.Parallel
	if err := workspace.InstallWorkspaces(workspaces, parallel); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Println("\n✓ All workspace installations completed")
	return 0
}

// cmdRunWorkspace runs a task across all workspace projects.
func cmdRunWorkspace(task string) int {
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

	parallel := cfg.Workspace.Parallel
	if err := workspace.RunInWorkspaces(workspaces, task, parallel); err != nil {
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
			g, err := graph.ExtractAll(project.Path)
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
		fmt.Println() // Add newline after JSON
	} else if svgFlag {
		outputPath := "graph.svg"
		if err := graph.GenerateSVG(graph.ToDOT(merged), outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Printf("SVG graph written to %s\n", outputPath)
	} else {
		// Default: tree output
		graph.PrintTree(merged, os.Stdout, cfg.Graph.ShowVersions, cfg.Graph.ShowEcosystem, maxDepth)
	}

	// Print warnings
	if len(warnings) > 0 {
		graph.PrintWarnings(warnings, os.Stdout)
	}

	return 0
}
