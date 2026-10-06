package graph

import (
	"strings"
	"testing"
)

// treeGraph adds name@1.0.0 npm nodes and edges "from>to" to a new graph with
// the given roots.
func treeGraph(roots []string, edges ...string) *DepGraph {
	g := NewGraph()
	id := func(name string) string {
		n := NewDepNode("node", name, "1.0.0")
		if g.GetNode(n.ID) == nil {
			g.AddNode(n)
		}
		return n.ID
	}
	for _, r := range roots {
		g.AddRoot(id(r))
	}
	for _, e := range edges {
		parts := strings.SplitN(e, ">", 2)
		g.AddEdge(NewEdge(id(parts[0]), id(parts[1])))
	}
	return g
}

func TestPrintTreeGolden(t *testing.T) {
	versions := TreeOptions{ShowVersions: true}
	for _, c := range []struct {
		name string
		g    *DepGraph
		opts TreeOptions
		want string
	}{
		{
			name: "diamond marks the repeated subtree",
			g:    treeGraph([]string{"app"}, "app>a", "app>b", "a>c", "b>c", "c>d"),
			opts: versions,
			want: `app@1.0.0
├─ a@1.0.0
│  └─ c@1.0.0
│     └─ d@1.0.0
└─ b@1.0.0
   └─ c@1.0.0 (*)
`,
		},
		{
			name: "shared leaf is not marked",
			g:    treeGraph([]string{"app"}, "app>y", "app>x", "x>leaf", "y>leaf"),
			opts: TreeOptions{},
			want: `app
├─ x
│  └─ leaf
└─ y
   └─ leaf
`,
		},
		{
			name: "cycle",
			g:    treeGraph([]string{"app"}, "app>a", "a>b", "b>a"),
			opts: versions,
			want: `app@1.0.0
└─ a@1.0.0
   └─ b@1.0.0
      └─ a@1.0.0 (cycle)
`,
		},
		{
			name: "depth 1 shows direct dependencies only",
			g:    treeGraph([]string{"app"}, "app>a", "app>b", "a>c", "b>c", "c>d"),
			opts: TreeOptions{ShowVersions: true, MaxDepth: 1},
			want: `app@1.0.0
├─ a@1.0.0
└─ b@1.0.0
`,
		},
		{
			name: "depth 2 never marks unexpanded nodes",
			g:    treeGraph([]string{"app"}, "app>a", "app>b", "a>c", "b>c", "c>d"),
			opts: TreeOptions{MaxDepth: 2},
			want: `app
├─ a
│  └─ c
└─ b
   └─ c
`,
		},
		{
			name: "multiple roots in root order",
			g:    treeGraph([]string{"web", "api"}, "web>shared", "api>shared", "shared>leaf"),
			opts: TreeOptions{ShowVersions: true, ShowEcosystem: true},
			want: `web@1.0.0 (node)
└─ shared@1.0.0 (node)
   └─ leaf@1.0.0 (node)
api@1.0.0 (node)
└─ shared@1.0.0 (node) (*)
`,
		},
		{
			name: "no roots: parentless nodes, sorted",
			g:    treeGraph(nil, "b>x", "a>x"),
			opts: TreeOptions{},
			want: `a
└─ x
b
└─ x
`,
		},
		{
			name: "empty graph prints nothing",
			g:    NewGraph(),
			opts: versions,
			want: ``,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var sb strings.Builder
			PrintTree(c.g, &sb, c.opts)
			if got := sb.String(); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}
