package graph

import (
	"reflect"
	"testing"
)

func TestNormalizeVersionsRekeysNodesEdgesAndRoots(t *testing.T) {
	g := NewGraph()
	root := testNode(g, "app", "v1.0.0")
	dep := testNode(g, "dep", "=2.0.0")
	g.AddRoot(root)
	g.AddEdge(NewEdge(root, dep))

	NormalizeVersions(g)

	wantRoot, wantDep := "node:app@1.0.0", "node:dep@2.0.0"
	if g.GetNode(wantRoot) == nil || g.GetNode(wantDep) == nil || g.NodeCount() != 2 {
		t.Fatalf("nodes = %v", sortedKeys(g.Nodes))
	}
	if g.GetNode(wantRoot).ID != wantRoot {
		t.Errorf("node.ID = %q, want %q", g.GetNode(wantRoot).ID, wantRoot)
	}
	if !reflect.DeepEqual(g.Root, []string{wantRoot}) {
		t.Errorf("Root = %v", g.Root)
	}
	if !reflect.DeepEqual(g.Children(wantRoot), []string{wantDep}) {
		t.Errorf("Children = %v", g.Children(wantRoot))
	}
	if len(g.Validate()) != 0 {
		t.Errorf("Validate = %v", g.Validate())
	}
	assertIndexConsistent(t, g)
}

func TestNormalizeVersionsMergesCollidingNodes(t *testing.T) {
	g := NewGraph()
	app := testNode(g, "app", "1.0.0")
	a := NewDepNode("node", "lib", "1.2.0").WithMetadata("resolved", "r1")
	b := NewDepNode("node", "lib", "v1.2.0").WithMetadata("integrity", "sha512-x")
	g.AddNode(a)
	g.AddNode(b)
	c := testNode(g, "c", "1.0.0")
	g.AddRoot(app)
	g.AddEdge(NewEdge(app, a.ID))
	g.AddEdge(NewEdge(app, b.ID)) // becomes a duplicate of app -> lib@1.2.0
	g.AddEdge(NewEdge(b.ID, c))
	g.AddEdge(NewEdge(a.ID, b.ID)) // becomes a self-loop

	NormalizeGraph(g)

	lib := g.GetNode("node:lib@1.2.0")
	if g.NodeCount() != 3 || lib == nil {
		t.Fatalf("nodes = %v", sortedKeys(g.Nodes))
	}
	if lib.GetMetadata("resolved") != "r1" || lib.GetMetadata("integrity") != "sha512-x" {
		t.Errorf("metadata not merged: %v", lib.Metadata)
	}
	if g.EdgeCount() != 2 || !g.HasEdge(app, "node:lib@1.2.0") || !g.HasEdge("node:lib@1.2.0", c) {
		t.Errorf("edges = %v", g.Edges)
	}
	assertIndexConsistent(t, g)
}

func TestNormalizeVersionKeepsNonNumericV(t *testing.T) {
	for in, want := range map[string]string{
		"v1.2.3": "1.2.3", "=1.0.0": "1.0.0", " 2.0.0 ": "2.0.0", "V3": "3",
		"=v1.0.0": "1.0.0", "very-new": "very-new", "v": "v", "": "",
	} {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeduplicateNodesRemapsMisKeyedNodes(t *testing.T) {
	g := NewGraph()
	good := NewDepNode("node", "x", "1.0.0")
	g.AddNode(good)
	// A node stored under a stale key (as the old NodeID mangling produced).
	stale := NewDepNode("node", "x", "1.0.0")
	stale.ID = "node:x-stale@1.0.0"
	g.AddNode(stale)
	parent := testNode(g, "p", "1.0.0")
	g.AddRoot(stale.ID)
	g.AddEdge(NewEdge(parent, stale.ID))

	DeduplicateNodes(g)

	if g.NodeCount() != 2 || !reflect.DeepEqual(g.Root, []string{good.ID}) || !g.HasEdge(parent, good.ID) {
		t.Errorf("nodes %v, roots %v, edges %v", sortedKeys(g.Nodes), g.Root, g.Edges)
	}
	assertIndexConsistent(t, g)
}
