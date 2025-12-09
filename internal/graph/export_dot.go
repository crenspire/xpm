package graph

import (
	"fmt"
	"io"
	"strings"
)

// ToDOT converts the graph to GraphViz DOT format.
func ToDOT(graph *DepGraph) string {
	var sb strings.Builder

	sb.WriteString("digraph dependencies {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=rounded];\n\n")

	// Add nodes with colors by ecosystem
	ecosystemColors := map[string]string{
		"node":   "#339933",
		"python": "#3776ab",
		"php":    "#777bb4",
		"rust":   "#000000",
		"go":     "#00add8",
		"java":   "#ed8b00",
	}

	for _, node := range graph.Nodes {
		color := ecosystemColors[node.Ecosystem]
		if color == "" {
			color = "#666666"
		}

		label := formatNodeLabelForDOT(node)
		sb.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\", fillcolor=\"%s\", style=\"rounded,filled\"];\n",
			node.ID, label, color))
	}

	sb.WriteString("\n")

	// Add edges
	for _, edge := range graph.Edges {
		style := "solid"
		if edge.Type == "transitive" {
			style = "dashed"
		}
		sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [style=%s];\n",
			edge.From, edge.To, style))
	}

	sb.WriteString("}\n")

	return sb.String()
}

// WriteDOT writes the graph in DOT format to the writer.
func WriteDOT(graph *DepGraph, w io.Writer) error {
	dot := ToDOT(graph)
	_, err := fmt.Fprint(w, dot)
	return err
}

// formatNodeLabelForDOT formats a node label for DOT output.
func formatNodeLabelForDOT(node *DepNode) string {
	var parts []string
	parts = append(parts, node.Name)
	if node.Version != "" {
		parts = append(parts, fmt.Sprintf("\\n%s", node.Version))
	}
	parts = append(parts, fmt.Sprintf("\\n[%s]", node.Ecosystem))
	return strings.Join(parts, "")
}

// ColorByEcosystem returns a color for an ecosystem.
func ColorByEcosystem(ecosystem string) string {
	colors := map[string]string{
		"node":   "#339933",
		"python": "#3776ab",
		"php":    "#777bb4",
		"rust":   "#000000",
		"go":     "#00add8",
		"java":   "#ed8b00",
	}
	if color, ok := colors[ecosystem]; ok {
		return color
	}
	return "#666666"
}

