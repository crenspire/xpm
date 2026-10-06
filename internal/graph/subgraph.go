package graph

import "fmt"

// Subgraph returns the part of g reachable from every node named name (all
// versions of it become roots, sorted by ID). Nodes are shared with g. It
// returns an error when no node has that name.
func Subgraph(g *DepGraph, name string) (*DepGraph, error) {
	matches := g.FindNodeByName(name)
	if len(matches) == 0 {
		return nil, fmt.Errorf("package %q not found in the dependency graph", name)
	}
	starts := make([]string, 0, len(matches))
	for _, n := range matches {
		starts = append(starts, n.ID)
	}
	reachable := g.reachableFrom(starts)

	sub := NewGraph()
	for _, id := range sortedKeys(reachable) {
		if node := g.Nodes[id]; node != nil {
			sub.AddNode(node)
		}
	}
	for _, e := range g.Edges {
		if reachable[e.From] {
			sub.AddEdge(e)
		}
	}
	for _, id := range starts {
		sub.AddRoot(id)
	}
	return sub, nil
}
