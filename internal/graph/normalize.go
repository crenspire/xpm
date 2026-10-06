package graph

import (
	"sort"
	"strings"
)

// NormalizeGraph normalizes the graph by removing duplicates, self-loops, and normalizing versions.
func NormalizeGraph(graph *DepGraph) {
	DeduplicateNodes(graph)
	RemoveSelfLoops(graph)
	RemoveDuplicateEdges(graph)
	NormalizeVersions(graph)
}

// DeduplicateNodes merges nodes with the same ecosystem:name@version.
func DeduplicateNodes(graph *DepGraph) {
	// Group nodes by ecosystem:name@version
	nodeGroups := make(map[string][]*DepNode)
	for _, node := range graph.Nodes {
		key := node.Ecosystem + ":" + node.Name + "@" + node.Version
		nodeGroups[key] = append(nodeGroups[key], node)
	}

	// Merge duplicate nodes
	for _, nodes := range nodeGroups {
		if len(nodes) <= 1 {
			continue
		}

		// Keep the first node, merge metadata from others
		primary := nodes[0]
		for i := 1; i < len(nodes); i++ {
			duplicate := nodes[i]
			// Merge metadata
			for k, v := range duplicate.Metadata {
				if _, exists := primary.Metadata[k]; !exists {
					primary.Metadata[k] = v
				}
			}
			// Update edges to point to primary
			for _, edge := range graph.Edges {
				if edge.From == duplicate.ID {
					edge.From = primary.ID
				}
				if edge.To == duplicate.ID {
					edge.To = primary.ID
				}
			}
			// Remove duplicate node
			delete(graph.Nodes, duplicate.ID)
			// Update root if needed
			for j, rootID := range graph.Root {
				if rootID == duplicate.ID {
					graph.Root[j] = primary.ID
				}
			}
		}
	}
}

// RemoveSelfLoops removes edges from a node to itself.
func RemoveSelfLoops(graph *DepGraph) {
	var validEdges []*DepEdge
	for _, edge := range graph.Edges {
		if edge.IsValid() {
			validEdges = append(validEdges, edge)
		}
	}
	graph.Edges = validEdges
}

// RemoveDuplicateEdges keeps only unique edges.
func RemoveDuplicateEdges(graph *DepGraph) {
	seen := make(map[string]bool)
	var uniqueEdges []*DepEdge

	for _, edge := range graph.Edges {
		key := edge.From + "->" + edge.To
		if !seen[key] {
			seen[key] = true
			uniqueEdges = append(uniqueEdges, edge)
		}
	}

	graph.Edges = uniqueEdges
}

// RemoveOrphanedNodes removes nodes with no connections (optional).
func RemoveOrphanedNodes(graph *DepGraph) {
	connected := make(map[string]bool)

	// Mark all nodes connected by edges
	for _, edge := range graph.Edges {
		connected[edge.From] = true
		connected[edge.To] = true
	}

	// Keep root nodes even if they have no edges
	for _, rootID := range graph.Root {
		connected[rootID] = true
	}

	// Remove orphaned nodes
	for id := range graph.Nodes {
		if !connected[id] {
			delete(graph.Nodes, id)
		}
	}
}

// NormalizeVersions standardizes version formats.
func NormalizeVersions(graph *DepGraph) {
	for _, node := range graph.Nodes {
		node.Version = normalizeVersion(node.Version)
		// Update ID with normalized version
		node.ID = NodeID(node.Ecosystem, node.Name, node.Version)
	}
}

// normalizeVersion normalizes a version string.
func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	// Remove leading 'v' if present
	if strings.HasPrefix(version, "v") {
		version = version[1:]
	}
	// Remove leading '=' if present
	if strings.HasPrefix(version, "=") {
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
