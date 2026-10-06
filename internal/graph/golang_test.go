package graph

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGoModDirectAndIndirect(t *testing.T) {
	g, err := parseGoMod(fixture(t, "go/app/go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 5, 4, 1, []string{
		"go:example.com/app@ -> go:github.com/spf13/cobra@v1.8.0", // full module path, not "cobra"
		"go:example.com/app@ -> go:github.com/spf13/pflag@v1.0.5",
	}, []string{"go:example.com/app@"})
	types := map[string]string{}
	for _, e := range g.Edges {
		types[e.To] = e.Type
	}
	if types["go:golang.org/x/mod@v0.17.0"] != "direct" || types["go:github.com/spf13/pflag@v1.0.5"] != "transitive" {
		t.Errorf("edge types = %v; want direct for x/mod, transitive for // indirect pflag", types)
	}
}

func TestParseGoModGraph(t *testing.T) {
	g, err := parseGoModGraph(fixture(t, "go/app/modgraph.txt"))
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 9, 10, 1, []string{
		"go:github.com/spf13/cobra@v1.8.0 -> go:github.com/cpuguy83/go-md2man/v2@v2.0.3",
		"go:github.com/cpuguy83/go-md2man/v2@v2.0.3 -> go:github.com/russross/blackfriday/v2@v2.1.0",
		"go:gopkg.in/yaml.v3@v3.0.1 -> go:gopkg.in/check.v1@v0.0.0-20161208181325-20d25e280405",
	}, []string{"go:example.com/app@"})
}

func TestParseGoMalformed(t *testing.T) {
	for name, data := range map[string]string{
		"no-module":      "go 1.22\n\nrequire example.com/x v1.0.0\n",
		"unclosed-block": "module example.com/app\n\nrequire (\n\texample.com/x v1.0.0\n",
		"bad-version":    "module example.com/app\n\nrequire example.com/x one.two\n",
	} {
		t.Run("go.mod/"+name, func(t *testing.T) {
			if g, err := parseGoMod([]byte(data)); err == nil {
				t.Fatalf("want error, got %s", dumpGraph(g))
			}
		})
	}
	for name, data := range map[string]string{
		"three-fields": "example.com/app github.com/a/b@v1.0.0 extra\n",
		"no-main":      "github.com/a/b@v1.0.0 github.com/c/d@v1.0.0\n",
	} {
		t.Run("graph/"+name, func(t *testing.T) {
			if g, err := parseGoModGraph([]byte(data)); err == nil {
				t.Fatalf("want error, got %s", dumpGraph(g))
			}
		})
	}
}

func TestGoExtractorRunsNothingWithoutExec(t *testing.T) {
	opts := ExtractOptions{Run: func(dir, name string, args ...string) ([]byte, error) {
		t.Fatalf("ran %s %v without Exec", name, args)
		return nil, nil
	}}
	g, err := (&GoExtractor{}).Extract(copyFixtureDir(t, "go/app"), opts)
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 5, 4, 1, nil, nil)
}

func TestGoExtractorExecUsesGoModGraph(t *testing.T) {
	dir := copyFixtureDir(t, "go/app")
	var ran string
	opts := ExtractOptions{Exec: true, Run: func(d, name string, args ...string) ([]byte, error) {
		ran = d + "|" + name + " " + strings.Join(args, " ")
		return os.ReadFile(filepath.Join("testdata", "go", "app", "modgraph.txt"))
	}}
	g, err := (&GoExtractor{}).Extract(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	if ran != dir+"|go mod graph" {
		t.Errorf("ran %q, want %q", ran, dir+"|go mod graph")
	}
	wantGraph(t, g, 9, 10, 1, nil, nil)
}

func TestGoExtractorExecFailureFallsBackWithWarning(t *testing.T) {
	var warnings []string
	opts := ExtractOptions{
		Exec: true,
		Run: func(string, string, ...string) ([]byte, error) {
			return nil, errors.New("go: command not found")
		},
		Warn: func(msg string) { warnings = append(warnings, msg) },
	}
	g, err := (&GoExtractor{}).Extract(copyFixtureDir(t, "go/app"), opts)
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 5, 4, 1, nil, nil)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "command not found") {
		t.Errorf("warnings = %q, want one mentioning the failure", warnings)
	}
}
