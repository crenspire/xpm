package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/crenspire/xpm/internal/graph"
	"github.com/crenspire/xpm/internal/search"
)

const defaultWhyLimit = 10

// whyArgs is `xpm why`'s command line.
type whyArgs struct {
	JSON, Workspace bool
	Limit           int
	Package         string
}

// parseWhyArgs parses flags anywhere on the line and exactly one package.
func parseWhyArgs(args []string) (whyArgs, error) {
	a := whyArgs{}
	fs := flag.NewFlagSet("why", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&a.JSON, "json", false, "print the paths as JSON")
	fs.IntVar(&a.Limit, "limit", defaultWhyLimit, "maximum number of paths to show (0 = all)")
	fs.BoolVar(&a.Workspace, "workspace", false, "combine all workspace projects")
	fs.BoolVar(&a.Workspace, "w", false, "shorthand for --workspace")

	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return whyArgs{}, errFlagReported{err}
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
	case a.Limit < 0:
		return whyArgs{}, fmt.Errorf("--limit must be a non-negative number (0 = all), got %d", a.Limit)
	case len(positional) == 0:
		return whyArgs{}, errors.New("why needs a package name: xpm why <package>")
	case len(positional) > 1:
		return whyArgs{}, fmt.Errorf("why takes exactly one package name, got %d", len(positional))
	}
	a.Package = positional[0]
	return a, nil
}

// whyReport is the JSON document of `xpm why`.
type whyReport struct {
	Package   string     `json:"package"`
	Paths     [][]string `json:"paths"`
	Truncated bool       `json:"truncated"`
}

// cmdWhy shows the dependency paths from the project's roots to a package.
// Exit status: 0 found, 1 not in the graph, 2 usage error or unreadable project.
func cmdWhy(args []string) int {
	a, err := parseWhyArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		if !errors.As(err, new(errFlagReported)) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		return 2
	}
	g, err := loadDepGraph(a.Workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	graph.NormalizeGraph(g)

	paths, truncated := graph.PathsTo(g, a.Package, a.Limit)
	if len(paths) == 0 {
		fmt.Fprintf(os.Stderr, "error: package %q not found in the dependency graph\n", search.SanitizeText(a.Package))
		return 1
	}

	if a.JSON {
		rep := whyReport{Package: a.Package, Paths: paths, Truncated: truncated}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		return 0
	}
	printWhy(g, paths, truncated)
	return 0
}

// printWhy writes one block per target version (sorted by node ID).
func printWhy(g *graph.DepGraph, paths [][]string, truncated bool) {
	byTarget := map[string][][]string{}
	for _, p := range paths {
		t := p[len(p)-1]
		byTarget[t] = append(byTarget[t], p)
	}
	targets := make([]string, 0, len(byTarget))
	for id := range byTarget {
		targets = append(targets, id)
	}
	sort.Strings(targets)

	label := func(id string) string {
		if n := g.GetNode(id); n != nil {
			return search.SanitizeText(n.ShortString())
		}
		return search.SanitizeText(id)
	}
	for _, t := range targets {
		eco := ""
		if n := g.GetNode(t); n != nil {
			eco = search.SanitizeText(n.Ecosystem)
		}
		fmt.Printf("%s (%s)\n", label(t), eco)
		for _, p := range byTarget[t] {
			names := make([]string, len(p))
			for i, id := range p {
				names[i] = label(id)
			}
			fmt.Printf("  %s\n", strings.Join(names, " > "))
		}
	}
	if truncated {
		fmt.Println("  … more paths (use --limit 0 to show all)")
	}
}
