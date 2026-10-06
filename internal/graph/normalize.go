package graph

import (
	"sort"
	"strings"
)

// NormalizeGraph normalizes versions, re-keys every node by its canonical
// ID (merging nodes that collide), and drops self-loops and duplicate edges.
func NormalizeGraph(graph *DepGraph) {
	NormalizeVersions(graph)
}

// DeduplicateNodes re-keys every node by NodeID(ecosystem, name, version)
// and merges nodes that end up with the same ID. Edges and roots are
// remapped once, in O(N + E).
func DeduplicateNodes(graph *DepGraph) {
	graph.rekey()
}

// RemoveSelfLoops removes edges from a node to itself.
func RemoveSelfLoops(graph *DepGraph) {
	graph.reindex()
}

// RemoveDuplicateEdges keeps only the first edge for each From -> To pair.
func RemoveDuplicateEdges(graph *DepGraph) {
	graph.reindex()
}

// RemoveOrphanedNodes removes nodes that are neither a root nor an edge endpoint.
func RemoveOrphanedNodes(graph *DepGraph) {
	connected := make(map[string]bool, len(graph.Nodes))
	for _, edge := range graph.Edges {
		connected[edge.From] = true
		connected[edge.To] = true
	}
	for _, rootID := range graph.Root {
		connected[rootID] = true
	}
	for id := range graph.Nodes {
		if !connected[id] {
			delete(graph.Nodes, id)
		}
	}
}

// NormalizeVersions strips a leading "=" or a "v" before a digit from every
// version ("v1.2.0" -> "1.2.0") and re-keys nodes, edges and roots to the new
// IDs. Nodes that collide after normalization are merged.
func NormalizeVersions(graph *DepGraph) {
	for _, node := range graph.Nodes {
		node.Version = normalizeVersion(node.Version)
	}
	graph.rekey()
}

// rekey rebuilds Nodes keyed by each node's canonical ID, merges colliding
// nodes (the node with the smallest old key wins; missing metadata keys are
// copied from the others), and rewrites edge endpoints and roots through the
// old->new ID map. Edges are copied, never mutated, because Merge shares edge
// pointers between graphs.
func (g *DepGraph) rekey() {
	remap := make(map[string]string, len(g.Nodes))
	nodes := make(map[string]*DepNode, len(g.Nodes))
	for _, oldID := range sortedKeys(g.Nodes) {
		node := g.Nodes[oldID]
		newID := NodeID(node.Ecosystem, node.Name, node.Version)
		remap[oldID] = newID
		if primary, ok := nodes[newID]; ok {
			for k, v := range node.Metadata {
				if _, exists := primary.Metadata[k]; !exists {
					if primary.Metadata == nil {
						primary.Metadata = make(map[string]string)
					}
					primary.Metadata[k] = v
				}
			}
			continue
		}
		node.ID = newID
		nodes[newID] = node
	}
	g.Nodes = nodes

	mapID := func(id string) string {
		if newID, ok := remap[id]; ok {
			return newID
		}
		return id
	}
	edges := make([]*DepEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, &DepEdge{From: mapID(e.From), To: mapID(e.To), Type: e.Type})
	}
	g.Edges = edges
	roots := make([]string, 0, len(g.Root))
	for _, id := range g.Root {
		roots = append(roots, mapID(id))
	}
	g.Root = roots
	g.reindex()
}

// normalizeVersion normalizes a version string.
func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "=")
	if len(version) > 1 && (version[0] == 'v' || version[0] == 'V') && version[1] >= '0' && version[1] <= '9' {
		version = version[1:]
	}
	return version
}

// SortNodes sorts nodes by ecosystem, then name, then version.
func SortNodes(nodes []*DepNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Ecosystem != nodes[j].Ecosystem {
			return nodes[i].Ecosystem < nodes[j].Ecosystem
		}
		if nodes[i].Name != nodes[j].Name {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].Version < nodes[j].Version
	})
}

// SortEdges sorts edges by from node, then to node.
func SortEdges(edges []*DepEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
}
