package graph

import (
	"io"
	"sort"
	"strings"
)

// TreeOptions controls PrintTree.
type TreeOptions struct {
	ShowVersions  bool // name@version instead of name
	ShowEcosystem bool // append " (ecosystem)"
	MaxDepth      int  // dependency levels below the roots; 0 = unlimited
}

// Markers appended to tree lines.
const (
	treeSeenMarker  = " (*)"     // subtree already printed above
	treeCycleMarker = " (cycle)" // node is its own ancestor
)

// PrintTree prints the dependency tree of every root, in root order, with
// children sorted by ID. A node whose subtree was already printed at least
// as deep as it could be printed here is shown once more with " (*)" and not
// expanded again, so shared dependencies (diamonds) print in O(nodes + edges)
// lines without a depth limit and O(nodes * MaxDepth) with one. A node first
// met deep (and cut off by MaxDepth) is expanded again where it appears with
// more depth left. A dependency that leads back to one of its ancestors is
// shown with " (cycle)". If the graph has no roots, nodes without parents
// are used as roots.
func PrintTree(g *DepGraph, w io.Writer, opts TreeOptions) {
	p := treePrinter{g: g, opts: opts, expanded: map[string]int{}, onPath: map[string]bool{}}
	for _, id := range treeRoots(g) {
		p.visit(id, "", "", 0)
	}
	_, _ = io.WriteString(w, p.sb.String())
}

// treeRoots returns g.Root (existing nodes only) or, without roots, the
// nodes that have no parents, sorted.
func treeRoots(g *DepGraph) []string {
	var roots []string
	for _, id := range g.Root {
		if g.Nodes[id] != nil {
			roots = append(roots, id)
		}
	}
	if len(g.Root) > 0 {
		return roots
	}
	for id := range g.Nodes {
		if len(g.GetParents(id)) == 0 {
			roots = append(roots, id)
		}
	}
	sort.Strings(roots)
	return roots
}

type treePrinter struct {
	g        *DepGraph
	opts     TreeOptions
	sb       strings.Builder
	expanded map[string]int  // levels below the node already printed above
	onPath   map[string]bool // ancestors of the node being printed
}

// visit prints id as "<prefix><connector><label>" and then its children
// with prefix childPrefix-extended; depth is 0 for roots.
func (p *treePrinter) visit(id, prefix, connector string, depth int) {
	children := p.children(id)
	remaining := p.remaining(depth)
	printed, wasExpanded := p.expanded[id]
	marker := ""
	switch {
	case p.onPath[id]:
		marker = treeCycleMarker
	case remaining > 0 && wasExpanded && printed >= remaining && len(children) > 0:
		marker = treeSeenMarker
	}
	p.sb.WriteString(prefix)
	p.sb.WriteString(connector)
	p.sb.WriteString(formatNodeLabel(p.g.Nodes[id], p.opts.ShowVersions, p.opts.ShowEcosystem))
	p.sb.WriteString(marker)
	p.sb.WriteByte('\n')
	if marker != "" || remaining == 0 {
		return
	}

	if remaining > printed {
		p.expanded[id] = remaining
	}
	p.onPath[id] = true
	childPrefix := prefix
	switch connector {
	case "├─ ":
		childPrefix += "│  "
	case "└─ ":
		childPrefix += "   "
	}
	for i, child := range children {
		conn := "├─ "
		if i == len(children)-1 {
			conn = "└─ "
		}
		p.visit(child, childPrefix, conn, depth+1)
	}
	p.onPath[id] = false
}

// treeUnlimited is the remaining depth when MaxDepth is 0 (no limit).
const treeUnlimited = int(^uint(0) >> 1)

// remaining returns how many levels below a node at depth may be printed.
func (p *treePrinter) remaining(depth int) int {
	if p.opts.MaxDepth <= 0 {
		return treeUnlimited
	}
	if depth >= p.opts.MaxDepth {
		return 0
	}
	return p.opts.MaxDepth - depth
}

// children returns the sorted children of id that exist as nodes.
func (p *treePrinter) children(id string) []string {
	all := p.g.Children(id)
	kept := all[:0]
	for _, c := range all {
		if p.g.Nodes[c] != nil {
			kept = append(kept, c)
		}
	}
	return kept
}

// formatNodeLabel formats a node for display.
func formatNodeLabel(node *DepNode, showVersions, showEcosystem bool) string {
	label := node.Name
	if showVersions && node.Version != "" {
		label += "@" + node.Version
	}
	if showEcosystem {
		label += " (" + node.Ecosystem + ")"
	}
	return label
}
