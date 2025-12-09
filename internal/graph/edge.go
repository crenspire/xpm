package graph

import "fmt"

// DepEdge represents a dependency edge in the graph.
type DepEdge struct {
	// From is the source node ID.
	From string

	// To is the target node ID.
	To string

	// Type is the edge type: "direct" or "transitive".
	Type string
}

// NewEdge creates a new edge between two nodes.
func NewEdge(from, to string) *DepEdge {
	return &DepEdge{
		From: from,
		To:   to,
		Type: "direct",
	}
}

// NewTransitiveEdge creates a new transitive edge.
func NewTransitiveEdge(from, to string) *DepEdge {
	return &DepEdge{
		From: from,
		To:   to,
		Type: "transitive",
	}
}

// IsValid checks if the edge is valid (no self-loops).
func (e *DepEdge) IsValid() bool {
	return e.From != "" && e.To != "" && e.From != e.To
}

// String returns a string representation of the edge.
func (e *DepEdge) String() string {
	return fmt.Sprintf("%s -> %s (%s)", e.From, e.To, e.Type)
}

// Equal checks if two edges are equal.
func (e *DepEdge) Equal(other *DepEdge) bool {
	return e.From == other.From && e.To == other.To
}

