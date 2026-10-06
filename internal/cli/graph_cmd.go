package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/graph"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/workspace"
)

// Seams for tests.
var (
	// extractGraph extracts one project's dependency graph.
	extractGraph = graph.ExtractAll
	// writeSVG renders a graph as SVG through GraphViz.
	writeSVG = graph.WriteSVG
	// graphRunner runs build tools for --exec; nil runs the real commands.
	graphRunner func(dir, name string, args ...string) ([]byte, error)
)

// graphArgs is `xpm graph`'s command line.
type graphArgs struct {
	JSON, SVG, Exec, Workspace bool
	Depth                      int
	Package                    string
}

// errFlagReported wraps a flag parse error that the flag package has already
// printed, with the usage, to stderr.
type errFlagReported struct{ err error }

func (e errFlagReported) Error() string { return e.err.Error() }
func (e errFlagReported) Unwrap() error { return e.err }

// parseGraphArgs parses flags anywhere on the line (`xpm graph react --json`).
// The depth default comes from the config. Errors are usage errors (exit 2);
// flag parse errors come back as errFlagReported.
func parseGraphArgs(args []string, defaultDepth int) (graphArgs, error) {
	var a graphArgs
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&a.JSON, "json", false, "print the graph as JSON")
	fs.BoolVar(&a.SVG, "svg", false, "print the graph as SVG (requires GraphViz)")
	fs.IntVar(&a.Depth, "depth", defaultDepth, "tree depth below the roots (0 = unlimited)")
	fs.BoolVar(&a.Exec, "exec", false, "run mvn/gradle/go to resolve full trees")
	fs.BoolVar(&a.Workspace, "workspace", false, "combine the graphs of all workspace projects")
	fs.BoolVar(&a.Workspace, "w", false, "shorthand for --workspace")

	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return graphArgs{}, errFlagReported{err}
		}
		rest := fs.Args()
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			positional = append(positional, rest...)
			break
		}
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}

	switch {
	case a.Depth < 0:
		return graphArgs{}, fmt.Errorf("--depth must be a non-negative number (0 = unlimited), got %d", a.Depth)
	case a.JSON && a.SVG:
		return graphArgs{}, errors.New("--json and --svg cannot be combined")
	case len(positional) > 1:
		return graphArgs{}, fmt.Errorf("graph takes at most one package name, got %d", len(positional))
	case len(positional) == 1:
		a.Package = positional[0]
	}
	return a, nil
}

// cmdGraph prints the project's dependency graph. stdout carries only the
// requested output (tree, JSON or SVG); warnings and status go to stderr.
// With --json or --svg, stdout is always a complete document, even for an
// empty graph.
func cmdGraph(args []string) int {
	a, err := parseGraphArgs(args, cfg.Graph.Depth)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		if !errors.As(err, new(errFlagReported)) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	opts := graph.ExtractOptions{
		Exec: a.Exec,
		Run:  graphRunner,
		Warn: func(msg string) { fmt.Fprintf(os.Stderr, "warning: %s\n", msg) },
	}

	var g *graph.DepGraph
	if a.Workspace {
		g, err = workspaceGraph(cwd, opts)
	} else {
		g, err = extractGraph(cwd, opts)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if g == nil {
		g = graph.NewGraph()
	}
	graph.NormalizeGraph(g)

	if a.Package != "" {
		if g, err = graph.Subgraph(g, a.Package); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	return renderGraph(g, a)
}

// workspaceGraph merges the graphs of every workspace root and project
// under root, after workspace.include / workspace.exclude from the config.
// A workspace whose root lockfile covers its members (npm/pnpm/yarn/bun
// workspaces, Cargo workspaces) is extracted at its root only; members have
// no lockfile of their own. Other workspaces (go.work, Python, Gradle,
// Composer, and Maven reactors, whose root pom.xml does not list the
// modules' dependencies) are extracted at the root and at every project.
// Each directory is extracted once, in detection order, even when it is
// listed under several ecosystems (one Workspace per ecosystem). A directory
// that fails to extract is reported as a warning and skipped.
func workspaceGraph(root string, opts graph.ExtractOptions) (*graph.DepGraph, error) {
	detected, err := workspace.DetectWorkspaces(root)
	if err != nil {
		return nil, fmt.Errorf("detecting workspaces: %w", err)
	}
	workspaces := workspace.Filter(detected, cfg.Workspace.Include, cfg.Workspace.Exclude)
	if len(workspaces) == 0 {
		return nil, errors.New("no workspaces detected")
	}
	var dirs []string
	seen := map[string]bool{}
	add := func(dir string) {
		dir = filepath.Clean(dir)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	for _, ws := range workspaces {
		add(ws.Root)
		if rootCoversMembers(ws) {
			continue
		}
		for _, p := range ws.Projects {
			add(p.Path)
		}
	}

	merged := graph.NewGraph()
	for _, dir := range dirs {
		g, err := extractGraph(dir, opts)
		if err != nil {
			if opts.Warn != nil {
				opts.Warn(fmt.Sprintf("skipping %s: %v", dir, err))
			}
			continue
		}
		merged.Merge(g)
	}
	return merged, nil
}

// rootCoversMembers reports whether ws's root lockfile already lists its
// members' dependencies: a workspace installed once at its root, except a
// Maven reactor, which has no lockfile.
func rootCoversMembers(ws workspace.Workspace) bool {
	return ws.RootPM != "" && ws.RootPM != pm.Maven
}

// renderGraph writes g to stdout in the requested format and its warnings
// to stderr.
func renderGraph(g *graph.DepGraph, a graphArgs) int {
	if len(g.Nodes) == 0 {
		fmt.Fprintln(os.Stderr, "No dependencies found.")
		if !a.JSON && !a.SVG {
			return 0
		}
	}

	switch {
	case a.JSON:
		if err := graph.WriteJSON(g, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	case a.SVG:
		if err := writeSVG(g, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	default:
		graph.PrintTree(g, os.Stdout, graph.TreeOptions{
			ShowVersions:  cfg.Graph.ShowVersions,
			ShowEcosystem: cfg.Graph.ShowEcosystem,
			MaxDepth:      a.Depth,
		})
	}

	graph.PrintWarnings(graph.DetectWarnings(g), os.Stderr)
	return 0
}
