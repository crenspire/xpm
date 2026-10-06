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

// flatPaths runs PathsTo and flattens the result for single-target tests.
func flatPaths(g *DepGraph, name string, limit int) (paths [][]string, truncated bool) {
	for _, tp := range PathsTo(g, name, limit) {
		paths = append(paths, tp.Paths...)
		truncated = truncated || tp.Truncated
	}
	return paths, truncated
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
	paths, trunc := flatPaths(g, "c", 0)
	want := [][]string{ids("app", "c"), ids("app", "a", "c"), ids("app", "b", "c")}
	if trunc || !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v trunc %v, want %v", paths, trunc, want)
	}
}

func TestPathsToCycle(t *testing.T) {
	g := pathsGraph([]string{"app"}, [][2]string{{"app", "a"}, {"a", "b"}, {"b", "a"}})
	paths, trunc := flatPaths(g, "b", 0)
	if trunc || !reflect.DeepEqual(paths, [][]string{ids("app", "a", "b")}) {
		t.Fatalf("paths = %v trunc %v", paths, trunc)
	}
	// A cycle with no root yields nothing, and terminates.
	g = pathsGraph(nil, [][2]string{{"x", "y"}, {"y", "x"}})
	if paths, _ := flatPaths(g, "x", 0); len(paths) != 0 {
		t.Fatalf("paths = %v", paths)
	}
}

func TestPathsToKeepsShortestUnderLimit(t *testing.T) {
	// A long path sorts lexically first but must not displace the short one.
	g := pathsGraph([]string{"app"}, [][2]string{{"app", "a"}, {"a", "b"}, {"b", "c"}, {"c", "t"}, {"app", "z"}, {"z", "t"}})
	got := PathsTo(g, "t", 1)
	if len(got) != 1 || !got[0].Truncated || !reflect.DeepEqual(got[0].Paths, [][]string{ids("app", "z", "t")}) {
		t.Fatalf("got %+v", got)
	}
}

func TestPathsToPerTargetLimit(t *testing.T) {
	g := NewGraph()
	for _, n := range []*DepNode{NewDepNode("node", "app", "1"), NewDepNode("node", "x", "1"), NewDepNode("node", "t", "1"), NewDepNode("node", "t", "2")} {
		g.AddNode(n)
	}
	g.AddRoot("node:app@1")
	for _, e := range [][2]string{{"node:app@1", "node:t@1"}, {"node:app@1", "node:x@1"}, {"node:x@1", "node:t@1"}, {"node:app@1", "node:t@2"}, {"node:x@1", "node:t@2"}} {
		g.AddEdge(&DepEdge{From: e[0], To: e[1]})
	}
	got := PathsTo(g, "t", 1)
	if len(got) != 2 || got[0].Target != "node:t@1" || got[1].Target != "node:t@2" {
		t.Fatalf("got %+v", got)
	}
	for _, tp := range got {
		if len(tp.Paths) != 1 || !tp.Truncated {
			t.Errorf("%s: %+v", tp.Target, tp)
		}
	}
}

func TestPathsToDenseCycleUnlimitedIsBounded(t *testing.T) {
	// Deterministic: the work caps, not the clock, bound the search.
	got := PathsTo(denseCycleGraph(), "target", 0)
	if len(got) != 1 || !got[0].Truncated || len(got[0].Paths) == 0 {
		t.Fatalf("got %+v targets", len(got))
	}
}

// denseCycleGraph: 25 mutually dependent nodes, one root edge in, the target below.
func denseCycleGraph() *DepGraph {
	var edges [][2]string
	name := func(i int) string { return fmt.Sprintf("n%d", i) }
	edges = append(edges, [2]string{"app", name(0)})
	for i := 0; i < 25; i++ {
		for j := 0; j < 25; j++ {
			if i != j {
				edges = append(edges, [2]string{name(i), name(j)})
			}
		}
	}
	edges = append(edges, [2]string{name(24), "target"})
	return pathsGraph([]string{"app"}, edges)
}

func TestPathsToDenseCycle(t *testing.T) {
	// 25 mutually dependent nodes, one root edge in, the target below.
	var edges [][2]string
	name := func(i int) string { return fmt.Sprintf("n%d", i) }
	edges = append(edges, [2]string{"app", name(0)})
	for i := 0; i < 25; i++ {
		for j := 0; j < 25; j++ {
			if i != j {
				edges = append(edges, [2]string{name(i), name(j)})
			}
		}
	}
	edges = append(edges, [2]string{name(24), "target"})
	g := pathsGraph([]string{"app"}, edges)
	start := time.Now()
	got := PathsTo(g, "target", 10)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
	if len(got) != 1 || len(got[0].Paths) != 10 || !got[0].Truncated {
		t.Fatalf("got %d paths, truncated %v", len(got[0].Paths), got[0].Truncated)
	}
	if want := ids("app", "n0", "n24", "target"); !reflect.DeepEqual(got[0].Paths[0], want) {
		t.Errorf("first path %v, want %v", got[0].Paths[0], want)
	}
}

func TestPathsToLimit(t *testing.T) {
	g := pathsGraph([]string{"app"}, [][2]string{{"app", "a"}, {"app", "b"}, {"app", "d"}, {"a", "c"}, {"b", "c"}, {"d", "c"}})
	paths, trunc := flatPaths(g, "c", 2)
	if !trunc || len(paths) != 2 {
		t.Fatalf("paths = %v trunc %v", paths, trunc)
	}
	paths, trunc = flatPaths(g, "c", 3)
	if trunc || len(paths) != 3 {
		t.Fatalf("limit == count: paths = %v trunc %v", paths, trunc)
	}
}

func TestPathsToTargetIsRoot(t *testing.T) {
	g := pathsGraph([]string{"app"}, [][2]string{{"app", "a"}})
	paths, trunc := flatPaths(g, "app", 0)
	if trunc || !reflect.DeepEqual(paths, [][]string{ids("app")}) {
		t.Fatalf("paths = %v", paths)
	}
}

func TestPathsToMissing(t *testing.T) {
	g := pathsGraph([]string{"app"}, nil)
	if got := PathsTo(g, "nope", 0); got != nil {
		t.Fatalf("got %v", got)
	}
	if got := PathsTo(nil, "x", 0); got != nil {
		t.Fatalf("nil graph: %v", got)
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
	paths, trunc := flatPaths(g, "target", 10)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
	if !trunc || len(paths) != 10 {
		t.Fatalf("paths = %d trunc %v", len(paths), trunc)
	}
}
