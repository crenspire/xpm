package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/graph"
)

// cmdGraph handles the graph command.
func cmdGraph(args []string) int {
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "output as JSON")
	svgOutput := fs.Bool("svg", false, "output as SVG (requires GraphViz)")
	depthFlag := fs.String("depth", "", "limit tree depth")
	workspaceFlag := fs.Bool("workspace", false, "generate graph for all workspace projects")
	wShort := fs.Bool("w", false, "generate graph for all workspace projects (shorthand)")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	workspace := *workspaceFlag || *wShort

	// Handle workspace graph
	if workspace {
		return cmdGraphWorkspace(*jsonOutput, *svgOutput, *depthFlag, fs.Args())
	}

	// Get current directory
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Load config
	cfg := config.Load()

	// Parse depth
	maxDepth := cfg.Graph.Depth
	if *depthFlag != "" {
		if d, err := strconv.Atoi(*depthFlag); err == nil {
			maxDepth = d
		}
	}

	// Extract dependencies
	var depGraph *graph.DepGraph
	packageArg := fs.Arg(0)

	if packageArg != "" {
		// Extract graph for specific package
		depGraph, err = graph.ExtractAll(dir)
		if err == nil {
			depGraph, err = graph.Subgraph(depGraph, packageArg)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	} else {
		// Extract all dependencies
		depGraph, err = graph.ExtractAll(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	}

	if depGraph == nil || len(depGraph.Nodes) == 0 {
		fmt.Println("No dependencies found.")
		return 0
	}

	// Normalize graph
	graph.NormalizeGraph(depGraph)

	// Detect warnings
	warnings := graph.DetectWarnings(depGraph)

	// Output based on format
	if *jsonOutput {
		if err := graph.WriteJSON(depGraph, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Println() // Add newline after JSON
	} else if *svgOutput {
		outputPath := "graph.svg"
		if err := graph.GenerateSVG(graph.ToDOT(depGraph), outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Printf("SVG graph written to %s\n", outputPath)
	} else {
		// Default: tree output
		graph.PrintTree(depGraph, os.Stdout, cfg.Graph.ShowVersions, cfg.Graph.ShowEcosystem, maxDepth)
	}

	// Print warnings
	if len(warnings) > 0 {
		graph.PrintWarnings(warnings, os.Stdout)
	}

	return 0
}
