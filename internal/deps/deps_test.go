package deps

import (
	"reflect"
	"testing"

	"github.com/crenspire/xpm/internal/graph"
	"github.com/crenspire/xpm/internal/pm"
)

func testGraph() *graph.DepGraph {
	g := graph.NewGraph()
	proj := graph.NewDepNode("node", "app", "1.0.0").WithMetadata("project", "true")
	member := graph.NewDepNode("node", "pkg-a", "1.0.0").WithMetadata("project", "true")
	goRoot := graph.NewDepNode("go", "example.com/mod", "")
	zed := graph.NewDepNode("node", "zed", "2.0.0")
	alpha := graph.NewDepNode("node", "alpha", "1.0.0")
	dupAlpha := graph.NewDepNode("node", "alpha", "1.0.0")
	dupAlpha.ID = "node:alpha@1.0.0#dup" // same triple, distinct node
	trans := graph.NewDepNode("node", "trans", "3.0.0")
	gdep := graph.NewDepNode("go", "golang.org/x/mod", "v0.20.0")
	for _, n := range []*graph.DepNode{proj, member, goRoot, zed, alpha, dupAlpha, trans, gdep} {
		g.AddNode(n)
	}
	g.AddRoot(proj.ID)
	g.AddRoot(goRoot.ID)
	g.AddEdge(graph.NewEdge(proj.ID, zed.ID))
	g.AddEdge(graph.NewEdge(proj.ID, alpha.ID))
	g.AddEdge(graph.NewEdge(proj.ID, dupAlpha.ID))
	g.AddEdge(graph.NewEdge(proj.ID, member.ID))
	g.AddEdge(graph.NewEdge(member.ID, trans.ID))
	g.AddEdge(graph.NewEdge(goRoot.ID, gdep.ID))
	return g
}

func TestDirect(t *testing.T) {
	want := []Dep{
		{"go", "golang.org/x/mod", "v0.20.0"},
		{"node", "alpha", "1.0.0"},
		{"node", "zed", "2.0.0"},
	}
	if got := Direct(testGraph()); !reflect.DeepEqual(got, want) {
		t.Errorf("Direct = %v, want %v", got, want)
	}
}

func TestAll(t *testing.T) {
	want := []Dep{
		{"go", "golang.org/x/mod", "v0.20.0"},
		{"node", "alpha", "1.0.0"},
		{"node", "trans", "3.0.0"},
		{"node", "zed", "2.0.0"},
	}
	if got := All(testGraph()); !reflect.DeepEqual(got, want) {
		t.Errorf("All = %v, want %v", got, want)
	}
}

func TestEmptyGraph(t *testing.T) {
	if got := Direct(nil); got != nil {
		t.Errorf("Direct(nil) = %v", got)
	}
	if got := All(graph.NewGraph()); len(got) != 0 {
		t.Errorf("All(empty) = %v", got)
	}
}

func TestManagerAndOSV(t *testing.T) {
	mgr := map[string]pm.ID{
		"node": pm.Npm, "python": pm.Pip, "php": pm.Composer,
		"rust": pm.Cargo, "java": pm.Maven, "go": pm.GoMod,
	}
	osv := map[string]string{
		"node": "npm", "python": "PyPI", "php": "Packagist",
		"rust": "crates.io", "go": "Go", "java": "Maven",
	}
	for eco, want := range mgr {
		if got, ok := Manager(eco); !ok || got != want {
			t.Errorf("Manager(%q) = %q, %v", eco, got, ok)
		}
	}
	for eco, want := range osv {
		if got, ok := OSVEcosystem(eco); !ok || got != want {
			t.Errorf("OSVEcosystem(%q) = %q, %v", eco, got, ok)
		}
	}
	if _, ok := Manager("ruby"); ok {
		t.Error("Manager(ruby) ok")
	}
	if _, ok := OSVEcosystem("ruby"); ok {
		t.Error("OSVEcosystem(ruby) ok")
	}
}

func TestPinned(t *testing.T) {
	cases := map[string]bool{
		"1.2.3":               true,
		"v1.2.3":              true,
		"1.0.0-rc.1":          true,
		"^1.2.0":              false,
		"~1.2.0":              false,
		">=1.0":               false,
		"1.x":                 false,
		"1.2.x":               false,
		"${ver}":              false,
		"":                    false,
		"latest":              false,
		"workspace:*":         false,
		"file:../x":           false,
		"link:../x":           false,
		"git+https://x/y#1.0": false,
		"1.0 || 2.0":          false,
		"*":                   false,
	}
	for v, want := range cases {
		if got := Pinned(v); got != want {
			t.Errorf("Pinned(%q) = %v, want %v", v, got, want)
		}
	}
}
