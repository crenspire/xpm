package graph

import (
	"fmt"
	"sort"
)

// DepGraph represents a unified dependency graph.
type DepGraph struct {
	// Nodes maps node IDs to their node objects.
	Nodes map[string]*DepNode

	// Edges is the list of dependency edges.
	Edges []*DepEdge

	// Root contains the root package IDs (top-level dependencies).
	Root []string
}

// NewGraph creates a new empty dependency graph.
func NewGraph() *DepGraph {
	return &DepGraph{
		Nodes: make(map[string]*DepNode),
		Edges: []*DepEdge{},
		Root:  []string{},
	}
}

// AddNode adds a node to the graph, handling duplicates.
// If a node with the same ID already exists, it updates the existing node.
func (g *DepGraph) AddNode(node *DepNode) {
	if node == nil {
		return
	}
	g.Nodes[node.ID] = node
}

// AddEdge adds an edge to the graph, validating and removing duplicates.
func (g *DepGraph) AddEdge(edge *DepEdge) {
	if edge == nil || !edge.IsValid() {
		return
	}

	// Check for duplicates
	for _, e := range g.Edges {
		if e.Equal(edge) {
			return
		}
	}

	g.Edges = append(g.Edges, edge)
}

// GetNode retrieves a node by ID.
func (g *DepGraph) GetNode(id string) *DepNode {
	return g.Nodes[id]
}

// GetChildren returns the direct dependencies of a node.
func (g *DepGraph) GetChildren(nodeID string) []string {
	var children []string
	for _, edge := range g.Edges {
		if edge.From == nodeID {
			children = append(children, edge.To)
		}
	}
	return children
}

// GetParents returns the nodes that depend on the given node.
func (g *DepGraph) GetParents(nodeID string) []string {
	var parents []string
	for _, edge := range g.Edges {
		if edge.To == nodeID {
			parents = append(parents, edge.From)
		}
	}
	return parents
}

// GetTransitive returns all transitive dependencies of a node (recursive).
func (g *DepGraph) GetTransitive(nodeID string) []string {
	visited := make(map[string]bool)
	var result []string

	var dfs func(string)
	dfs = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		children := g.GetChildren(id)
		for _, child := range children {
			result = append(result, child)
			dfs(child)
		}
	}

	dfs(nodeID)
	return result
}

// Validate checks graph integrity.
func (g *DepGraph) Validate() []error {
	var errors []error

	// Check that all edges reference existing nodes
	for _, edge := range g.Edges {
		if _, exists := g.Nodes[edge.From]; !exists {
			errors = append(errors, fmt.Errorf("edge references missing node: %s", edge.From))
		}
		if _, exists := g.Nodes[edge.To]; !exists {
			errors = append(errors, fmt.Errorf("edge references missing node: %s", edge.To))
		}
	}

	// Check that root nodes exist
	for _, rootID := range g.Root {
		if _, exists := g.Nodes[rootID]; !exists {
			errors = append(errors, fmt.Errorf("root node missing: %s", rootID))
		}
	}

	return errors
}

// Normalize removes duplicates, self-loops, and orphaned nodes.
func (g *DepGraph) Normalize() {
	// Remove duplicate edges
	seen := make(map[string]bool)
	var uniqueEdges []*DepEdge
	for _, edge := range g.Edges {
		key := fmt.Sprintf("%s->%s", edge.From, edge.To)
		if !seen[key] && edge.IsValid() {
			seen[key] = true
			uniqueEdges = append(uniqueEdges, edge)
		}
	}
	g.Edges = uniqueEdges

	// Remove orphaned nodes (nodes with no edges)
	connected := make(map[string]bool)
	for _, edge := range g.Edges {
		connected[edge.From] = true
		connected[edge.To] = true
	}
	// Keep root nodes even if they have no edges
	for _, rootID := range g.Root {
		connected[rootID] = true
	}

	// Remove nodes that aren't connected
	for id := range g.Nodes {
		if !connected[id] {
			delete(g.Nodes, id)
		}
	}
}

// AddRoot adds a root node ID.
func (g *DepGraph) AddRoot(nodeID string) {
	for _, id := range g.Root {
		if id == nodeID {
			return
		}
	}
	g.Root = append(g.Root, nodeID)
}

// Merge merges another graph into this graph.
func (g *DepGraph) Merge(other *DepGraph) {
	// Merge nodes
	for _, node := range other.Nodes {
		g.AddNode(node)
	}

	// Merge edges
	for _, edge := range other.Edges {
		g.AddEdge(edge)
	}

	// Merge roots
	for _, rootID := range other.Root {
		g.AddRoot(rootID)
	}
}

// NodeCount returns the number of nodes in the graph.
func (g *DepGraph) NodeCount() int {
	return len(g.Nodes)
}

// EdgeCount returns the number of edges in the graph.
func (g *DepGraph) EdgeCount() int {
	return len(g.Edges)
}

// GetNodesByEcosystem returns all nodes for a specific ecosystem.
func (g *DepGraph) GetNodesByEcosystem(ecosystem string) []*DepNode {
	var nodes []*DepNode
	for _, node := range g.Nodes {
		if node.Ecosystem == ecosystem {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

// FindNodeByName finds nodes by name (may return multiple versions).
func (g *DepGraph) FindNodeByName(name string) []*DepNode {
	var nodes []*DepNode
	for _, node := range g.Nodes {
		if node.Name == name {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// Clear removes all nodes and edges from the graph, freeing memory.
// This is useful for large graphs that are no longer needed.
func (g *DepGraph) Clear() {
	g.Nodes = make(map[string]*DepNode)
	g.Edges = []*DepEdge{}
	g.Root = []string{}
}

// Trim removes nodes and edges that are not reachable from root nodes.
// This can reduce memory usage for large graphs with many orphaned nodes.
func (g *DepGraph) Trim() {
	if len(g.Root) == 0 {
		// No roots, clear everything
		g.Clear()
		return
	}

	// Mark all nodes reachable from roots
	reachable := make(map[string]bool)
	var visit func(string)
	visit = func(nodeID string) {
		if reachable[nodeID] {
			return
		}
		reachable[nodeID] = true
		for _, childID := range g.GetChildren(nodeID) {
			visit(childID)
		}
	}

	// Start from all root nodes
	for _, rootID := range g.Root {
		visit(rootID)
	}

	// Remove unreachable nodes
	for id := range g.Nodes {
		if !reachable[id] {
			delete(g.Nodes, id)
		}
	}

	// Remove edges involving removed nodes
	var validEdges []*DepEdge
	for _, edge := range g.Edges {
		if reachable[edge.From] && reachable[edge.To] {
			validEdges = append(validEdges, edge)
		}
	}
	g.Edges = validEdges
}
