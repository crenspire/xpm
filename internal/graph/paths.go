package graph

import (
	"container/heap"
)

// maxPathPops bounds the work spent enumerating paths to one target, so a
// dense cyclic graph cannot run away. Hitting it marks the result truncated.
const maxPathPops = 50000

// maxPathPushes bounds the queue growth (and so the memory) per target.
const maxPathPushes = 100000

// TargetPaths holds the dependency paths to one node.
type TargetPaths struct {
	// Target is the node ID.
	Target string
	// Paths lists node-ID paths from a root to Target, shortest first, ties
	// broken lexically. Empty when no root reaches Target.
	Paths [][]string
	// Truncated reports that more paths exist than were returned.
	Truncated bool
}

// PathsTo returns, for every node named name (all versions, in ID order), the
// dependency paths from g's roots to that node, each a list of node IDs from a
// start to the target. A start is a root or a node without parents. Paths are
// enumerated shortest first and never revisit a node within one path (cycles
// are cut). At most limit paths are returned per target (limit <= 0 means no
// limit); Truncated reports that more exist. A target that is itself a start
// yields the one-node path [target]. It returns nil when no node has that name.
func PathsTo(g *DepGraph, name string, limit int) []TargetPaths {
	if g == nil {
		return nil
	}
	nodes := g.FindNodeByName(name)
	if len(nodes) == 0 {
		return nil
	}
	isRoot := make(map[string]bool, len(g.Root))
	for _, id := range g.Root {
		isRoot[id] = true
	}
	out := make([]TargetPaths, 0, len(nodes))
	for _, n := range nodes {
		paths, truncated := pathsToTarget(g, isRoot, n.ID, limit)
		if paths == nil {
			paths = [][]string{}
		}
		out = append(out, TargetPaths{Target: n.ID, Paths: paths, Truncated: truncated})
	}
	return out
}

// pathLink is a backward partial path: head is the node reached so far, next
// leads back to the target. Links are shared between partials.
type pathLink struct {
	id   string
	next *pathLink
	n    int // links in the chain, head included
}

func (l *pathLink) contains(id string) bool {
	for ; l != nil; l = l.next {
		if l.id == id {
			return true
		}
	}
	return false
}

// compareLinks orders two chains deterministically, head first.
func compareLinks(a, b *pathLink) int {
	for a != nil && b != nil {
		if a.id != b.id {
			if a.id < b.id {
				return -1
			}
			return 1
		}
		a, b = a.next, b.next
	}
	switch {
	case a == nil && b != nil:
		return -1
	case a != nil && b == nil:
		return 1
	}
	return 0
}

type pathItem struct {
	link *pathLink
	key  int // chain length plus the shortest distance from a start to the head
}

type pathQueue []pathItem

func (q pathQueue) Len() int { return len(q) }
func (q pathQueue) Less(i, j int) bool {
	if q[i].key != q[j].key {
		return q[i].key < q[j].key
	}
	// Among equally short candidates go deepest first, so complete paths come
	// out quickly instead of the queue growing breadth-wise.
	if q[i].link.n != q[j].link.n {
		return q[i].link.n > q[j].link.n
	}
	return compareLinks(q[i].link, q[j].link) < 0
}
func (q pathQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *pathQueue) Push(x any)   { *q = append(*q, x.(pathItem)) }
func (q *pathQueue) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}

// pathsToTarget enumerates paths to one target in nondecreasing length.
func pathsToTarget(g *DepGraph, isRoot map[string]bool, target string, limit int) (paths [][]string, truncated bool) {
	// Ancestors of the target (including it): the only nodes worth visiting.
	anc := map[string]bool{target: true}
	queue := []string{target}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, p := range g.GetParents(id) {
			if !anc[p] {
				anc[p] = true
				queue = append(queue, p)
			}
		}
	}
	isStart := func(id string) bool { return isRoot[id] || len(g.GetParents(id)) == 0 }

	// dist: shortest distance from any start to each ancestor (forward BFS).
	dist := map[string]int{}
	for id := range anc {
		if isStart(id) {
			dist[id] = 0
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, c := range g.Children(id) {
			if _, seen := dist[c]; anc[c] && !seen {
				dist[c] = dist[id] + 1
				queue = append(queue, c)
			}
		}
	}
	if _, ok := dist[target]; !ok {
		return nil, false
	}

	pq := &pathQueue{{link: &pathLink{id: target, n: 1}, key: 1 + dist[target]}}
	pushes, capped := 1, false
	for pops := 0; pq.Len() > 0; pops++ {
		if pops >= maxPathPops {
			return paths, true
		}
		it := heap.Pop(pq).(pathItem)
		head := it.link
		if isStart(head.id) {
			if limit > 0 && len(paths) == limit {
				return paths, true
			}
			p := make([]string, 0, head.n)
			for l := head; l != nil; l = l.next {
				p = append(p, l.id)
			}
			paths = append(paths, p)
			continue
		}
		for _, parent := range g.GetParents(head.id) {
			d, ok := dist[parent]
			if !ok || head.contains(parent) {
				continue
			}
			if pushes >= maxPathPushes {
				capped = true
				break
			}
			pushes++
			heap.Push(pq, pathItem{link: &pathLink{id: parent, next: head, n: head.n + 1}, key: head.n + 1 + d})
		}
	}
	return paths, capped
}
