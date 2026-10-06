package graph

import (
	"fmt"
	"io"
	"strings"
)

// ToDOT converts the graph to GraphViz DOT format. Nodes are sorted by ID and
// edges by (From, To); IDs and labels are escaped for DOT quoted strings.
func ToDOT(graph *DepGraph) string {
	var sb strings.Builder

	sb.WriteString("digraph dependencies {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=rounded];\n\n")

	for _, id := range sortedKeys(graph.Nodes) {
		node := graph.Nodes[id]
		fmt.Fprintf(&sb, "  \"%s\" [label=\"%s\", fillcolor=\"%s\", style=\"rounded,filled\"];\n",
			dotEscape(id), formatNodeLabelForDOT(node), ColorByEcosystem(node.Ecosystem))
	}

	sb.WriteString("\n")

	edges := append([]*DepEdge(nil), graph.Edges...)
	SortEdges(edges)
	for _, edge := range edges {
		style := "solid"
		if edge.Type == "transitive" {
			style = "dashed"
		}
		fmt.Fprintf(&sb, "  \"%s\" -> \"%s\" [style=%s];\n", dotEscape(edge.From), dotEscape(edge.To), style)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// WriteDOT writes the graph in DOT format to the writer.
func WriteDOT(graph *DepGraph, w io.Writer) error {
	if _, err := io.WriteString(w, ToDOT(graph)); err != nil {
		return fmt.Errorf("write DOT: %w", err)
	}
	return nil
}

// dotEscape escapes s for use inside a DOT double-quoted string: backslashes
// and quotes are escaped, and line breaks become the DOT "\n" escape.
func dotEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return r.Replace(s)
}

// formatNodeLabelForDOT returns the escaped three-line DOT label
// "name\nversion\n[ecosystem]" (the version line only if set).
func formatNodeLabelForDOT(node *DepNode) string {
	label := dotEscape(node.Name)
	if node.Version != "" {
		label += `\n` + dotEscape(node.Version)
	}
	return label + `\n[` + dotEscape(node.Ecosystem) + `]`
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
