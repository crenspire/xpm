package graph

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func pathsGraph(roots []string, edges [][2]string) *DepGraph {
	g := NewGraph()
	seen := map[string]bool{}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			g.AddNode(NewDepNode("node", id, "1.0.0"))
		}
	}
	for _, r := range roots {
		add(r)
		g.AddRoot(NodeID("node", r, "1.0.0"))
	}
	for _, e := range edges {
		add(e[0])
		add(e[1])
		g.AddEdge(&DepEdge{From: NodeID("node", e[0], "1.0.0"), To: NodeID("node", e[1], "1.0.0")})
	}
	return g
}

func ids(names ...string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = NodeID("node", n, "1.0.0")
	}
	return out
}

func TestPathsToDiamond(t *testing.T) {
	g := pathsGraph([]string{"app"}, [][2]string{{"app", "a"}, {"app", "b"}, {"a", "c"}, {"b", "c"}, {"app", "c"}})
	paths, trunc := PathsTo(g, "c", 0)
	want := [][]string{ids("app", "c"), ids("app", "a", "c"), ids("app", "b", "c")}
	if trunc || !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v trunc %v, want %v", paths, trunc, want)
	}
}

func TestPathsToCycle(t *testing.T) {
	g := pathsGraph([]string{"app"}, [][2]string{{"app", "a"}, {"a", "b"}, {"b", "a"}})
	paths, trunc := PathsTo(g, "b", 0)
	if trunc || !reflect.DeepEqual(paths, [][]string{ids("app", "a", "b")}) {
		t.Fatalf("paths = %v trunc %v", paths, trunc)
	}
	// A cycle with no root yields nothing, and terminates.
	g = pathsGraph(nil, [][2]string{{"x", "y"}, {"y", "x"}})
	if paths, _ := PathsTo(g, "x", 0); len(paths) != 0 {
		t.Fatalf("paths = %v", paths)
	}
}

func TestPathsToLimit(t *testing.T) {
	g := pathsGraph([]string{"app"}, [][2]string{{"app", "a"}, {"app", "b"}, {"app", "d"}, {"a", "c"}, {"b", "c"}, {"d", "c"}})
	paths, trunc := PathsTo(g, "c", 2)
	if !trunc || len(paths) != 2 {
		t.Fatalf("paths = %v trunc %v", paths, trunc)
	}
	paths, trunc = PathsTo(g, "c", 3)
	if trunc || len(paths) != 3 {
		t.Fatalf("limit == count: paths = %v trunc %v", paths, trunc)
	}
}

func TestPathsToTargetIsRoot(t *testing.T) {
	g := pathsGraph([]string{"app"}, [][2]string{{"app", "a"}})
	paths, trunc := PathsTo(g, "app", 0)
	if trunc || !reflect.DeepEqual(paths, [][]string{ids("app")}) {
		t.Fatalf("paths = %v", paths)
	}
}

func TestPathsToMissing(t *testing.T) {
	g := pathsGraph([]string{"app"}, nil)
	if paths, trunc := PathsTo(g, "nope", 0); paths != nil || trunc {
		t.Fatalf("paths = %v trunc %v", paths, trunc)
	}
	if paths, trunc := PathsTo(nil, "x", 0); paths != nil || trunc {
		t.Fatalf("nil graph: %v %v", paths, trunc)
	}
}

func TestPathsToLayeredPerf(t *testing.T) {
	// 30 layers of 2 nodes, fully connected: 2^30 paths. A limit must bound the work.
	var edges [][2]string
	layer := func(l, i int) string { return fmt.Sprintf("l%d_%d", l, i) }
	for i := 0; i < 2; i++ {
		edges = append(edges, [2]string{"app", layer(0, i)})
	}
	for l := 0; l < 29; l++ {
		for i := 0; i < 2; i++ {
			for j := 0; j < 2; j++ {
				edges = append(edges, [2]string{layer(l, i), layer(l+1, j)})
			}
		}
	}
	edges = append(edges, [2]string{layer(29, 0), "target"}, [2]string{layer(29, 1), "target"})
	g := pathsGraph([]string{"app"}, edges)
	start := time.Now()
	paths, trunc := PathsTo(g, "target", 10)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
	if !trunc || len(paths) != 10 {
		t.Fatalf("paths = %d trunc %v", len(paths), trunc)
	}
}
