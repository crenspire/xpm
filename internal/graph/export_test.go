package graph

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestToDOTEscapesAndSorts(t *testing.T) {
	g := NewGraph()
	weird := NewDepNode("node", `we"ird\name`, "1.0.0\nx")
	plain := NewDepNode("node", "plain", "2.0.0")
	g.AddNode(weird)
	g.AddNode(plain)
	g.AddEdge(NewEdge(weird.ID, plain.ID))
	g.AddEdge(NewTransitiveEdge(plain.ID, weird.ID))

	want := `digraph dependencies {
  rankdir=LR;
  node [shape=box, style=rounded];

  "node:plain@2.0.0" [label="plain\n2.0.0\n[node]", fillcolor="#339933", style="rounded,filled"];
  "node:we\"ird\\name@1.0.0\nx" [label="we\"ird\\name\n1.0.0\nx\n[node]", fillcolor="#339933", style="rounded,filled"];

  "node:plain@2.0.0" -> "node:we\"ird\\name@1.0.0\nx" [style=dashed];
  "node:we\"ird\\name@1.0.0\nx" -> "node:plain@2.0.0" [style=solid];
}
`
	if got := ToDOT(g); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDotEscape(t *testing.T) {
	for in, want := range map[string]string{
		`a"b`: `a\"b`, `a\b`: `a\\b`, "a\nb": `a\nb`, "a\r\nb": `a\nb`, `\"`: `\\\"`, "plain": "plain",
	} {
		if got := dotEscape(in); got != want {
			t.Errorf("dotEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToJSONIsValidAndDeterministic(t *testing.T) {
	build := func(order []string) *DepGraph {
		g := NewGraph()
		for _, name := range order {
			g.AddNode(NewDepNode("node", name, "1.0.0").WithMetadata("resolved", "https://r/"+name))
		}
		g.AddRoot(NodeID("node", "app", "1.0.0"))
		for _, to := range order {
			g.AddEdge(NewEdge(NodeID("node", "app", "1.0.0"), NodeID("node", to, "1.0.0")))
		}
		return g
	}
	a, err := ToJSON(build([]string{"app", "zeta", "@scope/pkg", "alpha"}))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ToJSON(build([]string{"alpha", "@scope/pkg", "app", "zeta"}))
	if string(a) != string(b) {
		t.Errorf("JSON depends on insertion order:\n%s\n---\n%s", a, b)
	}
	var parsed JSONGraph
	if err := json.Unmarshal(a, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, a)
	}
	var ids []string
	for _, n := range parsed.Nodes {
		ids = append(ids, n.ID)
	}
	want := []string{"node:@scope/pkg@1.0.0", "node:alpha@1.0.0", "node:app@1.0.0", "node:zeta@1.0.0"}
	if !reflect.DeepEqual(ids, want) || len(parsed.Edges) != 3 || parsed.Edges[0].To != "node:@scope/pkg@1.0.0" {
		t.Errorf("nodes %v edges %v", ids, parsed.Edges)
	}
}

func TestWriteJSONEmptyGraph(t *testing.T) {
	var sb strings.Builder
	if err := WriteJSON(&DepGraph{Nodes: map[string]*DepNode{}}, &sb); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"nodes\": [],\n  \"edges\": [],\n  \"roots\": []\n}\n"
	if sb.String() != want {
		t.Errorf("got %q, want %q", sb.String(), want)
	}
}

// fakeDot makes WriteSVG run the test binary in the given helper mode.
func fakeDot(t *testing.T, mode string) {
	t.Helper()
	old := dotCommand
	dotCommand = func() (*exec.Cmd, error) {
		cmd := exec.Command(os.Args[0], "-Tsvg")
		cmd.Env = append(os.Environ(), helperEnv+"="+mode)
		return cmd, nil
	}
	t.Cleanup(func() { dotCommand = old })
}

func TestWriteSVGPipesDOTThroughDot(t *testing.T) {
	fakeDot(t, "dot")
	g := treeGraph([]string{"app"}, "app>lib")
	var sb strings.Builder
	if err := WriteSVG(g, &sb); err != nil {
		t.Fatal(err)
	}
	want := "<svg args=\"-Tsvg\">\n" + ToDOT(g) + "</svg>\n"
	if sb.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", sb.String(), want)
	}
}

func TestWriteSVGReportsDotStderr(t *testing.T) {
	fakeDot(t, "dot-fail")
	var sb strings.Builder
	err := WriteSVG(NewGraph(), &sb)
	if err == nil || !strings.Contains(err.Error(), "syntax error in line 1") {
		t.Errorf("err = %v, want dot's stderr in it", err)
	}
}

func TestWriteSVGWithoutGraphViz(t *testing.T) {
	old := dotCommand
	dotCommand = func() (*exec.Cmd, error) { return nil, ErrGraphVizNotFound }
	t.Cleanup(func() { dotCommand = old })
	if err := WriteSVG(NewGraph(), &strings.Builder{}); !errors.Is(err, ErrGraphVizNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestDetectWarningsSortedAndPrintedToWriter(t *testing.T) {
	g := NewGraph()
	for _, v := range []string{"2.1.3", "2.0.0"} {
		g.AddNode(NewDepNode("node", "ms", v))
	}
	for _, v := range []string{"4.0.0", "3.0.0"} {
		g.AddNode(NewDepNode("node", "chalk", v))
	}
	g.AddNode(NewDepNode("python", "six", "1.16.0"))
	g.AddNode(NewDepNode("node", "six", "1.0.0"))

	ws := DetectWarnings(g)
	var got []string
	for _, w := range ws {
		got = append(got, w.Type+" "+w.Package)
	}
	want := []string{"ecosystem_conflict six", "version_conflict chalk", "version_conflict ms"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %v, want %v", got, want)
	}

	var sb strings.Builder
	PrintWarnings(ws[2:], &sb)
	wantOut := "\n⚠ Warnings:\n\n  ⚠ Multiple versions of ms detected in node\n     Package: ms\n     Details: [2.0.0 2.1.3]\n\n"
	if sb.String() != wantOut {
		t.Errorf("PrintWarnings = %q, want %q", sb.String(), wantOut)
	}
	sb.Reset()
	PrintWarnings(nil, &sb)
	if sb.Len() != 0 {
		t.Errorf("no warnings must print nothing, got %q", sb.String())
	}
}
