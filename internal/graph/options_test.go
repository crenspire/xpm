package graph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExtractOptionsRunUsesInjectedRunner(t *testing.T) {
	var gotDir, gotName string
	var gotArgs []string
	opts := ExtractOptions{Run: func(dir, name string, args ...string) ([]byte, error) {
		gotDir, gotName, gotArgs = dir, name, args
		return []byte("out"), nil
	}}
	out, err := opts.run("/proj", "go", "mod", "graph")
	if err != nil || string(out) != "out" {
		t.Fatalf("run = %q, %v", out, err)
	}
	if gotDir != "/proj" || gotName != "go" || !reflect.DeepEqual(gotArgs, []string{"mod", "graph"}) {
		t.Errorf("runner got (%q, %q, %q)", gotDir, gotName, gotArgs)
	}
}

func TestExtractOptionsRunRealCommandUsesDirAndStdoutOnly(t *testing.T) {
	t.Setenv(helperEnv, "pwd")
	dir := t.TempDir()
	out, err := ExtractOptions{}.run(dir, os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	got, _ := filepath.EvalSymlinks(string(out))
	if got != want {
		t.Errorf("stdout = %q, want the working directory %q (stderr must not be mixed in)", out, want)
	}
}

func TestExtractOptionsRunRealCommandFailure(t *testing.T) {
	t.Setenv(helperEnv, "no-such-mode")
	if _, err := (ExtractOptions{}).run(t.TempDir(), os.Args[0]); err == nil {
		t.Fatal("want an error for a non-zero exit")
	}
}

func TestExtractOptionsWarn(t *testing.T) {
	ExtractOptions{}.warn("dropped %d", 1) // nil Warn must not panic
	var got []string
	ExtractOptions{Warn: func(msg string) { got = append(got, msg) }}.warn("skipped %s: %v", "pom.xml", "bad xml")
	if !reflect.DeepEqual(got, []string{"skipped pom.xml: bad xml"}) {
		t.Errorf("warnings = %q", got)
	}
}
