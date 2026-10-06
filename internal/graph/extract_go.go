package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// GoExtractor extracts dependencies from Go modules.
type GoExtractor struct{}

func (e *GoExtractor) Name() string {
	return "go"
}

func (e *GoExtractor) Supports(file string) bool {
	return file == "go.mod"
}

// Extract parses go.mod: the module is the root, with an edge to every
// require ("transitive" for // indirect ones). With opts.Exec it runs
// `go mod graph` for the full module graph and falls back to go.mod, with a
// warning, when that fails.
func (e *GoExtractor) Extract(dir string, opts ExtractOptions) (*DepGraph, error) {
	if opts.Exec {
		out, err := opts.run(dir, "go", "mod", "graph")
		if err == nil {
			g, perr := parseGoModGraph(out)
			if perr == nil {
				return g, nil
			}
			err = perr
		}
		opts.warn("go: `go mod graph` failed, using go.mod requires only: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	return parseGoMod(data)
}

// parseGoMod builds a one-level graph from go.mod.
func parseGoMod(data []byte) (*DepGraph, error) {
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return nil, fmt.Errorf("go.mod: %w", err)
	}
	if f.Module == nil || f.Module.Mod.Path == "" {
		return nil, fmt.Errorf("go.mod: no module directive")
	}
	g := NewGraph()
	root := NewDepNode("go", f.Module.Mod.Path, "")
	g.AddNode(root)
	g.AddRoot(root.ID)
	for _, r := range f.Require {
		n := NewDepNode("go", r.Mod.Path, r.Mod.Version)
		g.AddNode(n)
		if r.Indirect {
			n.WithMetadata("indirect", "true")
			g.AddEdge(NewTransitiveEdge(root.ID, n.ID))
		} else {
			g.AddEdge(NewEdge(root.ID, n.ID))
		}
	}
	return g, nil
}

// parseGoModGraph parses `go mod graph` output: one "from to" pair per line,
// each "path@version" except main modules (no "@"), which become the roots.
// The go and toolchain pseudo-modules are skipped.
func parseGoModGraph(out []byte) (*DepGraph, error) {
	g := NewGraph()
	node := func(tok string) string {
		path, version, _ := strings.Cut(tok, "@")
		id := NodeID("go", path, version)
		if g.GetNode(id) == nil {
			g.AddNode(NewDepNode("go", path, version))
		}
		if version == "" {
			g.AddRoot(id)
		}
		return id
	}
	for i, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			return nil, fmt.Errorf("go mod graph line %d: want 2 fields, got %q", i+1, line)
		}
		if strings.HasPrefix(f[1], "go@") || strings.HasPrefix(f[1], "toolchain@") {
			continue
		}
		from, to := node(f[0]), node(f[1])
		g.AddEdge(NewEdge(from, to))
	}
	if len(g.Root) == 0 {
		return nil, fmt.Errorf("go mod graph: no main module in output")
	}
	return g, nil
}
