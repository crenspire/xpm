package graph

import (
	"reflect"
	"strings"
	"testing"
)

func TestSubgraphAllVersionsBecomeRoots(t *testing.T) {
	g := NewGraph()
	app := testNode(g, "app", "1.0.0")
	d1, d2 := testNode(g, "debug", "2.6.9"), testNode(g, "debug", "4.3.4")
	ms1, ms2 := testNode(g, "ms", "2.0.0"), testNode(g, "ms", "2.1.3")
	other := testNode(g, "other", "1.0.0")
	g.AddRoot(app)
	for _, e := range [][2]string{{app, d2}, {app, other}, {other, d1}, {d1, ms1}, {d2, ms2}} {
		g.AddEdge(NewEdge(e[0], e[1]))
	}

	sub, err := Subgraph(g, "debug")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sub.Root, []string{d1, d2}) {
		t.Errorf("Root = %v", sub.Root)
	}
	if got := sortedKeys(sub.Nodes); !reflect.DeepEqual(got, []string{d1, d2, ms1, ms2}) {
		t.Errorf("Nodes = %v", got)
	}
	if sub.EdgeCount() != 2 || !sub.HasEdge(d1, ms1) || !sub.HasEdge(d2, ms2) {
		t.Errorf("Edges = %v", sub.Edges)
	}
	assertIndexConsistent(t, sub)
}

func TestSubgraphNotFound(t *testing.T) {
	_, err := Subgraph(NewGraph(), "left-pad")
	if err == nil || !strings.Contains(err.Error(), `"left-pad" not found`) {
		t.Errorf("err = %v", err)
	}
}

func TestSubgraphWithCycle(t *testing.T) {
	g := NewGraph()
	a, b := testNode(g, "a", "1"), testNode(g, "b", "1")
	g.AddEdge(NewEdge(a, b))
	g.AddEdge(NewEdge(b, a))
	sub, err := Subgraph(g, "b")
	if err != nil {
		t.Fatal(err)
	}
	if sub.NodeCount() != 2 || sub.EdgeCount() != 2 || !reflect.DeepEqual(sub.Root, []string{b}) {
		t.Errorf("sub: %v %v %v", sortedKeys(sub.Nodes), sub.Edges, sub.Root)
	}
}
