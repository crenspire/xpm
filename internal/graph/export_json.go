package graph

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSONGraph represents the graph in JSON format.
type JSONGraph struct {
	Nodes []JSONNode `json:"nodes"`
	Edges []JSONEdge `json:"edges"`
	Roots []string   `json:"roots"`
}

// JSONNode represents a node in JSON format.
type JSONNode struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Ecosystem string            `json:"ecosystem"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// JSONEdge represents an edge in JSON format.
type JSONEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// ToJSON converts the graph to JSON format.
func ToJSON(graph *DepGraph) ([]byte, error) {
	jsonGraph := JSONGraph{
		Nodes: make([]JSONNode, 0, len(graph.Nodes)),
		Edges: make([]JSONEdge, 0, len(graph.Edges)),
		Roots: graph.Root,
	}

	// Convert nodes
	for _, node := range graph.Nodes {
		jsonNode := JSONNode{
			ID:        node.ID,
			Name:      node.Name,
			Version:   node.Version,
			Ecosystem: node.Ecosystem,
			Metadata:  node.Metadata,
		}
		if len(jsonNode.Metadata) == 0 {
			jsonNode.Metadata = nil
		}
		jsonGraph.Nodes = append(jsonGraph.Nodes, jsonNode)
	}

	// Convert edges
	for _, edge := range graph.Edges {
		jsonGraph.Edges = append(jsonGraph.Edges, JSONEdge{
			From: edge.From,
			To:   edge.To,
			Type: edge.Type,
		})
	}

	return json.MarshalIndent(jsonGraph, "", "  ")
}

// WriteJSON writes the graph as JSON to the writer.
func WriteJSON(graph *DepGraph, w io.Writer) error {
	data, err := ToJSON(graph)
	if err != nil {
		return fmt.Errorf("failed to marshal graph: %w", err)
	}

	_, err = w.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	return nil
}
