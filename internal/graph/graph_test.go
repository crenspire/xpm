package graph

import (
	"reflect"
	"testing"
)

// testNode adds an npm node to g and returns its ID.
func testNode(g *DepGraph, name, version string) string {
	n := NewDepNode("node", name, version)
	g.AddNode(n)
	return n.ID
}

// assertIndexConsistent checks that the adjacency index matches Edges.
func assertIndexConsistent(t *testing.T, g *DepGraph) {
	t.Helper()
	seen := map[[2]string]bool{}
	for _, e := range g.Edges {
		k := [2]string{e.From, e.To}
		if seen[k] {
			t.Errorf("duplicate edge %s -> %s in Edges", e.From, e.To)
		}
		seen[k] = true
		if !g.HasEdge(e.From, e.To) {
			t.Errorf("edge %s -> %s missing from the index", e.From, e.To)
		}
	}
	n := 0
	for from, tos := range g.out {
		for to := range tos {
			n++
			if !seen[[2]string{from, to}] {
				t.Errorf("index has %s -> %s but Edges does not", from, to)
			}
			if _, ok := g.in[to][from]; !ok {
				t.Errorf("reverse index lacks %s -> %s", from, to)
			}
		}
	}
	if n != len(g.Edges) {
		t.Errorf("index has %d edges, Edges has %d", n, len(g.Edges))
	}
}

func TestNodeIDKeepsNameVerbatim(t *testing.T) {
	if got := NodeID("node", "@scope/pkg", "1.0.0"); got != "node:@scope/pkg@1.0.0" {
		t.Errorf("NodeID = %q", got)
	}
	if NodeID("node", "@scope/pkg", "1.0.0") == NodeID("node", "scope-pkg", "1.0.0") {
		t.Error("@scope/pkg and scope-pkg must not collide")
	}
	if got := NodeID("go", "github.com/pkg/errors", "v0.9.1"); got != "go:github.com/pkg/errors@v0.9.1" {
		t.Errorf("NodeID = %q", got)
	}
}

func TestAddEdgeDedupesAndRejectsSelfLoops(t *testing.T) {
	g := NewGraph()
	a, b := testNode(g, "a", "1"), testNode(g, "b", "1")
	g.AddEdge(NewEdge(a, b))
	g.AddEdge(NewTransitiveEdge(a, b)) // same endpoints: ignored, first wins
	g.AddEdge(NewEdge(a, a))
	g.AddEdge(NewEdge("", b))
	g.AddEdge(nil)
	if g.EdgeCount() != 1 || g.Edges[0].Type != "direct" {
		t.Fatalf("edges = %v", g.Edges)
	}
	assertIndexConsistent(t, g)
}

func TestChildrenAndParentsSorted(t *testing.T) {
	g := NewGraph()
	r, z, m, a := testNode(g, "root", "1"), testNode(g, "z", "1"), testNode(g, "m", "1"), testNode(g, "a", "1")
	g.AddEdge(NewEdge(r, z))
	g.AddEdge(NewEdge(r, m))
	g.AddEdge(NewEdge(r, a))
	g.AddEdge(NewEdge(m, a))
	if got, want := g.Children(r), []string{a, m, z}; !reflect.DeepEqual(got, want) {
		t.Errorf("Children = %v, want %v", got, want)
	}
	if got, want := g.GetParents(a), []string{m, r}; !reflect.DeepEqual(got, want) {
		t.Errorf("GetParents = %v, want %v", got, want)
	}
	if got := g.Children("node:missing@1"); len(got) != 0 {
		t.Errorf("Children(missing) = %v", got)
	}
}

func TestGetTransitiveVisitsEachNodeOnce(t *testing.T) {
	g := NewGraph()
	r, a, b, c := testNode(g, "r", "1"), testNode(g, "a", "1"), testNode(g, "b", "1"), testNode(g, "c", "1")
	g.AddEdge(NewEdge(r, a))
	g.AddEdge(NewEdge(r, b))
	g.AddEdge(NewEdge(a, c))
	g.AddEdge(NewEdge(b, c))
	g.AddEdge(NewEdge(c, r)) // cycle back to the start
	if got, want := g.GetTransitive(r), []string{a, c, b}; !reflect.DeepEqual(got, want) {
		t.Errorf("GetTransitive = %v, want %v", got, want)
	}
}

func TestStructLiteralGraphIsIndexedOnDemand(t *testing.T) {
	g := &DepGraph{
		Nodes: map[string]*DepNode{},
		Edges: []*DepEdge{NewEdge("x", "y"), NewEdge("x", "y"), NewEdge("x", "x")},
	}
	if got := g.Children("x"); !reflect.DeepEqual(got, []string{"y"}) {
		t.Errorf("Children = %v", got)
	}
	if g.EdgeCount() != 1 {
		t.Errorf("EdgeCount = %d, want 1 after indexing", g.EdgeCount())
	}
	assertIndexConsistent(t, g)
}

func TestAddRootOnce(t *testing.T) {
	g := NewGraph()
	g.AddRoot("b")
	g.AddRoot("a")
	g.AddRoot("b")
	if !reflect.DeepEqual(g.Root, []string{"b", "a"}) {
		t.Errorf("Root = %v, want insertion order without duplicates", g.Root)
	}
}

func TestMergeDedupesEdgesAndRoots(t *testing.T) {
	g1, g2 := NewGraph(), NewGraph()
	for _, g := range []*DepGraph{g1, g2} {
		r, a := testNode(g, "r", "1"), testNode(g, "a", "1")
		g.AddRoot(r)
		g.AddEdge(NewEdge(r, a))
	}
	b := testNode(g2, "b", "1")
	g2.AddEdge(NewEdge(NodeID("node", "a", "1"), b))
	g1.Merge(g2)
	if g1.NodeCount() != 3 || g1.EdgeCount() != 2 || len(g1.Root) != 1 {
		t.Errorf("merged: %d nodes, %d edges, roots %v", g1.NodeCount(), g1.EdgeCount(), g1.Root)
	}
	assertIndexConsistent(t, g1)
}

func TestTrimAndNormalizeKeepIndexConsistent(t *testing.T) {
	g := NewGraph()
	r, a, x, y := testNode(g, "r", "1"), testNode(g, "a", "1"), testNode(g, "x", "1"), testNode(g, "y", "1")
	testNode(g, "lonely", "1")
	g.AddRoot(r)
	g.AddEdge(NewEdge(r, a))
	g.AddEdge(NewEdge(x, y)) // unreachable from the root

	n := NewGraph()
	n.Merge(g)
	n.Normalize() // drops only "lonely"
	if n.NodeCount() != 4 || n.EdgeCount() != 2 {
		t.Errorf("Normalize: %d nodes, %d edges", n.NodeCount(), n.EdgeCount())
	}
	assertIndexConsistent(t, n)

	g.Trim()
	if g.NodeCount() != 2 || g.EdgeCount() != 1 || g.HasEdge(x, y) {
		t.Errorf("Trim: %d nodes, %d edges", g.NodeCount(), g.EdgeCount())
	}
	assertIndexConsistent(t, g)
}
