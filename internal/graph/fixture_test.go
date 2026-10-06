package graph

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixture reads internal/graph/testdata/<rel>.
func fixture(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// wantGraph asserts exact node, edge and root counts, then that every listed
// edge ("from -> to") and root exists. On failure it prints the whole graph.
func wantGraph(t *testing.T, g *DepGraph, nodes, edges, roots int, wantEdges []string, wantRoots []string) {
	t.Helper()
	if g == nil {
		t.Fatal("graph is nil")
	}
	ok := len(g.Nodes) == nodes && len(g.Edges) == edges && len(g.Root) == roots
	have := map[string]bool{}
	for _, e := range g.Edges {
		have[e.From+" -> "+e.To] = true
	}
	for _, e := range wantEdges {
		if !have[e] {
			ok = false
			t.Errorf("missing edge %s", e)
		}
	}
	isRoot := map[string]bool{}
	for _, r := range g.Root {
		isRoot[r] = true
	}
	for _, r := range wantRoots {
		if !isRoot[r] {
			ok = false
			t.Errorf("missing root %s", r)
		}
	}
	for _, e := range g.Edges {
		if g.Nodes[e.From] == nil || g.Nodes[e.To] == nil {
			ok = false
			t.Errorf("edge %s -> %s references a missing node", e.From, e.To)
		}
	}
	if !ok {
		t.Errorf("got %d nodes, %d edges, %d roots; want %d, %d, %d\n%s",
			len(g.Nodes), len(g.Edges), len(g.Root), nodes, edges, roots, dumpGraph(g))
	}
}

func dumpGraph(g *DepGraph) string {
	var lines []string
	for id := range g.Nodes {
		lines = append(lines, "node "+id)
	}
	sort.Strings(lines)
	for _, e := range g.Edges {
		lines = append(lines, "edge "+e.From+" -> "+e.To+" ("+e.Type+")")
	}
	for _, r := range g.Root {
		lines = append(lines, "root "+r)
	}
	return strings.Join(lines, "\n")
}
