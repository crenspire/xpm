package graph

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runtimeIsWindows() bool { return runtime.GOOS == "windows" }

// projectDir builds a temp project from "testdata path -> file name" pairs.
func projectDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for src, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), fixture(t, src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func polyglotDir(t *testing.T) string {
	return projectDir(t, map[string]string{
		"npm/v3-hoisted/package-lock.json": "package-lock.json",
		"go/app/go.mod":                    "go.mod",
		"maven/app/pom.xml":                "pom.xml",
		"cargo/workspace/Cargo.lock":       "Cargo.lock",
	})
}

func TestExtractAllRunsNoToolWithoutExec(t *testing.T) {
	g, err := ExtractAll(polyglotDir(t), ExtractOptions{Run: failRun(t), Warn: func(m string) { t.Errorf("warning: %s", m) }})
	if err != nil {
		t.Fatal(err)
	}
	// node 13/12/1 + go 5/4/1 + java 7/6/1 + rust 11/15/2
	wantGraph(t, g, 36, 37, 5, nil, []string{
		"node:hoisted-app@1.0.0", "go:example.com/app@", "java:com.example:demo@0.0.1-SNAPSHOT", "rust:app@0.1.0",
	})
}

func TestExtractAllIsDeterministic(t *testing.T) {
	dir := polyglotDir(t)
	render := func() string {
		g, err := ExtractAll(dir, ExtractOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, e := range g.Edges {
			b.WriteString(e.From + ">" + e.To + "\n")
		}
		b.WriteString(strings.Join(g.Root, ","))
		return b.String()
	}
	first := render()
	for i := 0; i < 5; i++ {
		if got := render(); got != first {
			t.Fatalf("run %d differs:\n%s\nvs\n%s", i, got, first)
		}
	}
	if !strings.HasPrefix(first, "node:") {
		t.Errorf("node edges must come first (fixed extractor order), got %.40q", first)
	}
}

func TestExtractAllWarnsAndKeepsStdoutClean(t *testing.T) {
	dir := projectDir(t, map[string]string{"go/app/go.mod": "go.mod"})
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	var warnings []string
	g, err := ExtractAll(dir, ExtractOptions{Warn: func(m string) { warnings = append(warnings, m) }})
	os.Stdout = stdout
	w.Close()
	printed, _ := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(printed) != 0 {
		t.Errorf("ExtractAll wrote to stdout: %q", printed)
	}
	wantGraph(t, g, 5, 4, 1, nil, nil)
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], "node: ") {
		t.Errorf("warnings = %q, want one node: warning", warnings)
	}
}

func TestExtractAllAllFailedIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.lock"), []byte("[[package]"), 0o644); err != nil {
		t.Fatal(err)
	}
	var warned bool
	if _, err := ExtractAll(dir, ExtractOptions{Warn: func(string) { warned = true }}); err == nil || !strings.Contains(err.Error(), "rust:") {
		t.Fatalf("err = %v, want the rust failure", err)
	}
	if warned {
		t.Error("a failure returned as the error must not also be warned")
	}
}

func TestExtractAllNothingDetected(t *testing.T) {
	g, err := ExtractAll(t.TempDir(), ExtractOptions{})
	if err != nil || g == nil || len(g.Nodes) != 0 {
		t.Fatalf("got (%v, %v), want an empty graph", g, err)
	}
}

func TestExtractAllExecRunsToolsInProjectDir(t *testing.T) {
	dir := projectDir(t, map[string]string{"go/app/go.mod": "go.mod", "maven/app/pom.xml": "pom.xml"})
	tgf := fixture(t, "maven/app/tree.tgf")
	modGraph := fixture(t, "go/app/modgraph.txt")
	var ran []string
	opts := ExtractOptions{Exec: true, Run: func(d, name string, args ...string) ([]byte, error) {
		if d != dir {
			t.Errorf("%s ran in %q, want %q", name, d, dir)
		}
		ran = append(ran, name)
		if name == "go" {
			return modGraph, nil
		}
		for _, a := range args {
			if out, ok := strings.CutPrefix(a, "-DoutputFile="); ok {
				return nil, os.WriteFile(out, tgf, 0o644)
			}
		}
		return nil, nil
	}}
	g, err := ExtractAll(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 19, 19, 2, nil, nil) // go mod graph 9/10/1 + tgf 10/9/1
	if strings.Join(ran, ",") != "go,mvn" {
		t.Errorf("ran %v, want [go mvn] in extractor order", ran)
	}
}

func TestDetectEcosystems(t *testing.T) {
	for file, eco := range map[string]string{
		"bun.lock":            "node",
		"package.json":        "node",
		"gradle.lockfile":     "java",
		"settings.gradle.kts": "java",
		"composer.json":       "php",
		"Cargo.toml":          "rust",
		"go.mod":              "go",
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := DetectEcosystems(dir); !got[eco] || len(got) != 1 {
			t.Errorf("%s: detected %v, want only %s", file, got, eco)
		}
	}
}
