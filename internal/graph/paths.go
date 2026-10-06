package graph

import (
	"sort"
	"strings"
)

// PathsTo returns dependency paths from g's roots to every node named name
// (all versions), each path a list of node IDs from a root to the target.
// It walks parents depth-first from each target (parents in sorted order),
// never revisiting a node within one path (cycles are cut), and stops after
// limit paths (limit <= 0 means no limit). truncated reports whether more
// paths exist. Paths are returned sorted by length, then lexically by IDs.
// A target that is itself a root yields the one-node path [target].
func PathsTo(g *DepGraph, name string, limit int) (paths [][]string, truncated bool) {
	if g == nil {
		return nil, false
	}
	isRoot := make(map[string]bool, len(g.Root))
	for _, id := range g.Root {
		isRoot[id] = true
	}

	onPath := map[string]bool{}
	var stack []string // target first, then successive parents
	full := false      // limit+1 paths collected

	var walk func(id string)
	walk = func(id string) {
		if full {
			return
		}
		onPath[id] = true
		stack = append(stack, id)
		defer func() {
			onPath[id] = false
			stack = stack[:len(stack)-1]
		}()

		parents := g.GetParents(id)
		if isRoot[id] || len(parents) == 0 {
			p := make([]string, len(stack))
			for i, s := range stack {
				p[len(stack)-1-i] = s
			}
			paths = append(paths, p)
			if limit > 0 && len(paths) > limit {
				full = true
			}
			return
		}
		for _, parent := range parents {
			if onPath[parent] {
				continue
			}
			walk(parent)
			if full {
				return
			}
		}
	}

	for _, n := range g.FindNodeByName(name) {
		walk(n.ID)
		if full {
			break
		}
	}
	if full {
		paths = paths[:limit]
		truncated = true
	}
	sort.SliceStable(paths, func(i, j int) bool {
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) < len(paths[j])
		}
		return strings.Join(paths[i], "\x00") < strings.Join(paths[j], "\x00")
	})
	return paths, truncated
}
