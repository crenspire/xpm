package graph

import (
	"fmt"
	"sort"
)

// DepGraph represents a unified dependency graph.
//
// Edges is the canonical, exported edge list (used by the JSON and DOT
// exporters). The unexported adjacency index mirrors it for O(1) edge
// dedupe and O(degree) child/parent lookups. Mutate edges only through
// AddEdge or the graph's methods; code that rewrites Edges wholesale must
// call reindex afterwards.
type DepGraph struct {
	// Nodes maps node IDs to their node objects.
	Nodes map[string]*DepNode

	// Edges is the list of dependency edges, in insertion order.
	Edges []*DepEdge

	// Root contains the root package IDs, in insertion order.
	Root []string

	out     map[string]map[string]*DepEdge // from -> to -> edge
	in      map[string]map[string]struct{} // to -> from
	rootSet map[string]struct{}
	indexed int // len(Edges) when the index was last consistent
}

// NewGraph creates a new empty dependency graph.
func NewGraph() *DepGraph {
	g := &DepGraph{
		Nodes: make(map[string]*DepNode),
		Edges: []*DepEdge{},
		Root:  []string{},
	}
	g.reindex()
	return g
}

// reindex rebuilds the adjacency index and root set from Edges and Root,
// dropping invalid edges (empty endpoints, self-loops), duplicate edges and
// duplicate roots. The first occurrence of each edge and root wins, so the
// order of Edges and Root is preserved.
func (g *DepGraph) reindex() {
	edges := g.Edges
	g.Edges = make([]*DepEdge, 0, len(edges))
	g.out = make(map[string]map[string]*DepEdge)
	g.in = make(map[string]map[string]struct{})
	g.indexed = 0
	for _, e := range edges {
		g.addEdgeIndexed(e)
	}

	roots := g.Root
	g.Root = make([]string, 0, len(roots))
	g.rootSet = make(map[string]struct{}, len(roots))
	for _, id := range roots {
		g.AddRoot(id)
	}
}

// ensureIndex rebuilds the index if Edges was changed behind the graph's
// back (for example a DepGraph built as a struct literal).
func (g *DepGraph) ensureIndex() {
	if g.out == nil || g.rootSet == nil || g.indexed != len(g.Edges) {
		g.reindex()
	}
}

func (g *DepGraph) addEdgeIndexed(e *DepEdge) {
	if e == nil || !e.IsValid() {
		return
	}
	if _, dup := g.out[e.From][e.To]; dup {
		return
	}
	if g.out[e.From] == nil {
		g.out[e.From] = make(map[string]*DepEdge)
	}
	g.out[e.From][e.To] = e
	if g.in[e.To] == nil {
		g.in[e.To] = make(map[string]struct{})
	}
	g.in[e.To][e.From] = struct{}{}
	g.Edges = append(g.Edges, e)
	g.indexed = len(g.Edges)
}

// AddNode adds a node to the graph. A node with the same ID replaces the
// existing one.
func (g *DepGraph) AddNode(node *DepNode) {
	if node == nil {
		return
	}
	if g.Nodes == nil {
		g.Nodes = make(map[string]*DepNode)
	}
	g.Nodes[node.ID] = node
}

// AddEdge adds an edge, ignoring invalid edges (self-loops, empty
// endpoints) and edges already present (same From and To). O(1).
func (g *DepGraph) AddEdge(edge *DepEdge) {
	g.ensureIndex()
	g.addEdgeIndexed(edge)
}

// HasEdge reports whether the graph has an edge from -> to.
func (g *DepGraph) HasEdge(from, to string) bool {
	g.ensureIndex()
	_, ok := g.out[from][to]
	return ok
}

// GetNode retrieves a node by ID.
func (g *DepGraph) GetNode(id string) *DepNode {
	return g.Nodes[id]
}

// Children returns the IDs of the direct dependencies of id, sorted.
func (g *DepGraph) Children(id string) []string {
	g.ensureIndex()
	return sortedKeys(g.out[id])
}

// GetChildren returns the direct dependencies of a node, sorted.
func (g *DepGraph) GetChildren(nodeID string) []string {
	return g.Children(nodeID)
}

// GetParents returns the IDs of the nodes that depend on nodeID, sorted.
func (g *DepGraph) GetParents(nodeID string) []string {
	g.ensureIndex()
	parents := make([]string, 0, len(g.in[nodeID]))
	for id := range g.in[nodeID] {
		parents = append(parents, id)
	}
	sort.Strings(parents)
	return parents
}

// GetTransitive returns every node reachable from nodeID (excluding nodeID
// itself unless it is part of a cycle), each once, in depth-first order with
// sorted children.
func (g *DepGraph) GetTransitive(nodeID string) []string {
	visited := map[string]bool{nodeID: true}
	var result []string
	var dfs func(string)
	dfs = func(id string) {
		for _, child := range g.Children(id) {
			if visited[child] {
				continue
			}
			visited[child] = true
			result = append(result, child)
			dfs(child)
		}
	}
	dfs(nodeID)
	return result
}

// Validate checks graph integrity.
func (g *DepGraph) Validate() []error {
	var errs []error
	for _, edge := range g.Edges {
		if _, exists := g.Nodes[edge.From]; !exists {
			errs = append(errs, fmt.Errorf("edge references missing node: %s", edge.From))
		}
		if _, exists := g.Nodes[edge.To]; !exists {
			errs = append(errs, fmt.Errorf("edge references missing node: %s", edge.To))
		}
	}
	for _, rootID := range g.Root {
		if _, exists := g.Nodes[rootID]; !exists {
			errs = append(errs, fmt.Errorf("root node missing: %s", rootID))
		}
	}
	return errs
}

// Normalize removes invalid and duplicate edges, then nodes that are neither
// a root nor an edge endpoint.
func (g *DepGraph) Normalize() {
	g.reindex()
	RemoveOrphanedNodes(g)
}

// AddRoot adds a root node ID (once). O(1).
func (g *DepGraph) AddRoot(nodeID string) {
	if g.rootSet == nil {
		g.rootSet = make(map[string]struct{}, len(g.Root))
		for _, id := range g.Root {
			g.rootSet[id] = struct{}{}
		}
	}
	if _, ok := g.rootSet[nodeID]; ok {
		return
	}
	g.rootSet[nodeID] = struct{}{}
	g.Root = append(g.Root, nodeID)
}

// Merge merges another graph into this graph. Nodes of other replace nodes
// with the same ID; edges and roots are added once.
func (g *DepGraph) Merge(other *DepGraph) {
	if other == nil {
		return
	}
	for _, id := range sortedKeys(other.Nodes) {
		g.AddNode(other.Nodes[id])
	}
	for _, edge := range other.Edges {
		g.AddEdge(edge)
	}
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

// GetNodesByEcosystem returns all nodes for a specific ecosystem, sorted by ID.
func (g *DepGraph) GetNodesByEcosystem(ecosystem string) []*DepNode {
	var nodes []*DepNode
	for _, node := range g.Nodes {
		if node.Ecosystem == ecosystem {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

// FindNodeByName finds nodes by name (may return multiple versions), sorted by ID.
func (g *DepGraph) FindNodeByName(name string) []*DepNode {
	var nodes []*DepNode
	for _, node := range g.Nodes {
		if node.Name == name {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

// Clear removes all nodes, edges and roots.
func (g *DepGraph) Clear() {
	g.Nodes = make(map[string]*DepNode)
	g.Edges = []*DepEdge{}
	g.Root = []string{}
	g.reindex()
}

// Trim removes nodes and edges that are not reachable from root nodes.
func (g *DepGraph) Trim() {
	if len(g.Root) == 0 {
		g.Clear()
		return
	}
	reachable := g.reachableFrom(g.Root)
	for id := range g.Nodes {
		if !reachable[id] {
			delete(g.Nodes, id)
		}
	}
	kept := make([]*DepEdge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if reachable[edge.From] && reachable[edge.To] {
			kept = append(kept, edge)
		}
	}
	g.Edges = kept
	g.reindex()
}

// reachableFrom returns the set of IDs reachable from starts (inclusive).
func (g *DepGraph) reachableFrom(starts []string) map[string]bool {
	g.ensureIndex()
	reachable := make(map[string]bool)
	stack := append([]string(nil), starts...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if reachable[id] {
			continue
		}
		reachable[id] = true
		for child := range g.out[id] {
			if !reachable[child] {
				stack = append(stack, child)
			}
		}
	}
	return reachable
}

// sortedKeys returns the keys of m in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
