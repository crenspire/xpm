package graph

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// syntheticGraph builds a 1001-node graph: one root over 10 layers of 100
// nodes, every node depending on 5 nodes of the next layer (heavy diamond
// sharing; 100 + 9*100*5 = 4600 edges). Versions carry a "v" prefix so
// NormalizeGraph has to re-key every node and edge.
func syntheticGraph() *DepGraph {
	const layers, width, fanout = 10, 100, 5
	g := NewGraph()
	id := func(layer, i int) string { return NodeID("node", fmt.Sprintf("pkg-%d-%d", layer, i), "v1.0.0") }
	root := NewDepNode("node", "app", "v1.0.0")
	g.AddNode(root)
	g.AddRoot(root.ID)
	for l := 0; l < layers; l++ {
		for i := 0; i < width; i++ {
			g.AddNode(NewDepNode("node", fmt.Sprintf("pkg-%d-%d", l, i), "v1.0.0"))
		}
	}
	for i := 0; i < width; i++ {
		g.AddEdge(NewEdge(root.ID, id(0, i)))
	}
	for l := 0; l+1 < layers; l++ {
		for i := 0; i < width; i++ {
			for k := 0; k < fanout; k++ {
				g.AddEdge(NewEdge(id(l, i), id(l+1, (i*7+k*13)%width)))
			}
		}
	}
	return g
}

// graphPipeline is what `xpm graph` does after extraction.
func graphPipeline(w io.Writer) (*DepGraph, error) {
	g := syntheticGraph()
	NormalizeGraph(g)
	PrintTree(g, w, TreeOptions{ShowVersions: true, ShowEcosystem: true})
	return g, WriteJSON(g, w)
}

func TestSyntheticGraphTreeIsLinearInEdges(t *testing.T) {
	g := syntheticGraph()
	NormalizeGraph(g)
	if g.NodeCount() != 1001 || g.EdgeCount() != 4600 {
		t.Fatalf("synthetic graph: %d nodes, %d edges", g.NodeCount(), g.EdgeCount())
	}
	var sb strings.Builder
	PrintTree(g, &sb, TreeOptions{ShowVersions: true})
	// Every node is expanded once, so each edge prints exactly one line.
	if lines := strings.Count(sb.String(), "\n"); lines != 1+4600 {
		t.Errorf("tree has %d lines, want %d", lines, 1+4600)
	}
}

func TestGraphPipelineBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	start := time.Now()
	if _, err := graphPipeline(io.Discard); err != nil {
		t.Fatal(err)
	}
	// The budget is 200 ms; allow 10x for slow, shared CI runners and -race.
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("build+normalize+tree+JSON took %v, budget 200ms", d)
	}
}

func BenchmarkGraphPipeline(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := graphPipeline(io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
