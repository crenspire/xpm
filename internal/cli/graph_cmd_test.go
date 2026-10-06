package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/graph"
)

// demoLock is an npm v3 lockfile: demo -> debug@2.6.9 -> ms@2.0.0 (nested),
// demo -> ms@2.1.3 (hoisted). Two versions of ms give one warning.
const demoLock = `{
  "name": "demo",
  "version": "1.0.0",
  "lockfileVersion": 3,
  "requires": true,
  "packages": {
    "": {
      "name": "demo",
      "version": "1.0.0",
      "dependencies": {
        "debug": "^2.6.9",
        "ms": "^2.1.3"
      }
    },
    "node_modules/debug": {
      "version": "2.6.9",
      "resolved": "https://registry.npmjs.org/debug/-/debug-2.6.9.tgz",
      "integrity": "sha512-bC7ElrdJaJnPbAP+1EotYvqZsb3ecl5wi6Bfi6BJTUcNowp6cvspg0jXznRTKDjm/E7AdgFBVeAPVMNcKGsHMA==",
      "dependencies": {
        "ms": "2.0.0"
      }
    },
    "node_modules/debug/node_modules/ms": {
      "version": "2.0.0",
      "resolved": "https://registry.npmjs.org/ms/-/ms-2.0.0.tgz",
      "integrity": "sha512-Tpp60P6IUJDTuOq/5Z8cdskzJujfwqfOTkrwIwj7IRISpnkJnT6SyJ4PCPnGMoFjC9ddhal5KVIYtAt97ix05A=="
    },
    "node_modules/ms": {
      "version": "2.1.3",
      "resolved": "https://registry.npmjs.org/ms/-/ms-2.1.3.tgz",
      "integrity": "sha512-6FlzubTLZG3J2a/NVCAleEhjzq5oxgHyaCU9yYXvcLsvoVaHJq/s5xXI6/XXP6tz7R9xAOtHnSO/tXtF3WRTlA=="
    }
  }
}
`

// singleDepLock is an npm v3 lockfile for project name@1.0.0 -> dep.
func singleDepLock(name, dep, version string) string {
	return fmt.Sprintf(`{
  "name": %[1]q,
  "version": "1.0.0",
  "lockfileVersion": 3,
  "requires": true,
  "packages": {
    "": {
      "name": %[1]q,
      "version": "1.0.0",
      "dependencies": {
        %[2]q: "^%[3]s"
      }
    },
    "node_modules/%[2]s": {
      "version": %[3]q,
      "resolved": "https://registry.npmjs.org/%[2]s/-/%[2]s-%[3]s.tgz"
    }
  }
}
`, name, dep, version)
}

// graphProject chdirs into a temp project with the given files and sets a
// config showing versions and ecosystems with unlimited depth.
func graphProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	withConfig(t, config.Config{Graph: config.GraphConfig{ShowVersions: true, ShowEcosystem: true}})
	return dir
}

// runGraph runs cmdGraph and returns its exit code, stdout and stderr.
func runGraph(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() { code = cmdGraph(args) })
	})
	return code, stdout, stderr
}

// noTools fails the test if any build tool would run.
func noTools(t *testing.T) {
	t.Helper()
	old := graphRunner
	graphRunner = func(dir, name string, args ...string) ([]byte, error) {
		t.Errorf("ran %s %v in %s without --exec", name, args, dir)
		return nil, errors.New("not allowed")
	}
	t.Cleanup(func() { graphRunner = old })
}

func TestGraphTreeToStdoutWarningsToStderr(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	noTools(t)
	code, stdout, stderr := runGraph(t)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	want := `demo@1.0.0 (node)
├─ debug@2.6.9 (node)
│  └─ ms@2.0.0 (node)
└─ ms@2.1.3 (node)
`
	if stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
	}
	if !strings.Contains(stderr, "Multiple versions of ms detected in node") {
		t.Errorf("stderr lacks the version warning: %q", stderr)
	}
}

func TestGraphJSONStdoutIsValidJSON(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	code, stdout, stderr := runGraph(t, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var g graph.JSONGraph
	if err := json.Unmarshal([]byte(stdout), &g); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(g.Nodes) != 4 || len(g.Edges) != 3 || len(g.Roots) != 1 || g.Roots[0] != "node:demo@1.0.0" {
		t.Errorf("got %d nodes, %d edges, roots %v", len(g.Nodes), len(g.Edges), g.Roots)
	}
	if !strings.Contains(stderr, "Multiple versions of ms") {
		t.Errorf("warnings must go to stderr, got %q", stderr)
	}
}

func TestGraphEmptyProject(t *testing.T) {
	graphProject(t, nil)

	code, stdout, stderr := runGraph(t)
	if code != 0 || stdout != "" || !strings.Contains(stderr, "No dependencies found.") {
		t.Errorf("tree: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	code, stdout, stderr = runGraph(t, "--json")
	want := "{\n  \"nodes\": [],\n  \"edges\": [],\n  \"roots\": []\n}\n"
	if code != 0 || stdout != want || !strings.Contains(stderr, "No dependencies found.") {
		t.Errorf("json: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

// fakeSVG replaces GraphViz with fn.
func fakeSVG(t *testing.T, fn func(g *graph.DepGraph, w io.Writer) error) {
	t.Helper()
	old := writeSVG
	writeSVG = fn
	t.Cleanup(func() { writeSVG = old })
}

func TestGraphSVGStreamsToStdout(t *testing.T) {
	dir := graphProject(t, map[string]string{"package-lock.json": demoLock})
	fakeSVG(t, func(g *graph.DepGraph, w io.Writer) error {
		_, err := fmt.Fprintf(w, "<svg nodes=\"%d\"/>\n", g.NodeCount())
		return err
	})
	code, stdout, stderr := runGraph(t, "--svg")
	if code != 0 || stdout != "<svg nodes=\"4\"/>\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "graph.svg")); !os.IsNotExist(err) {
		t.Errorf("graph.svg must not be written (stat err %v)", err)
	}
}

func TestGraphSVGWithoutGraphViz(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	fakeSVG(t, func(*graph.DepGraph, io.Writer) error { return graph.ErrGraphVizNotFound })
	code, stdout, stderr := runGraph(t, "--svg")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "GraphViz is not installed") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestGraphUsageErrorsExit2(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	old := extractGraph
	extractGraph = func(string, graph.ExtractOptions) (*graph.DepGraph, error) {
		t.Error("must not extract after a usage error")
		return graph.NewGraph(), nil
	}
	t.Cleanup(func() { extractGraph = old })
	for _, args := range [][]string{
		{"--depth", "abc"},
		{"--depth", "-1"},
		{"--depth=1.5"},
		{"--json", "--svg"},
		{"react", "vue"},
		{"--bogus"},
	} {
		code, stdout, stderr := runGraph(t, args...)
		if code != 2 || stdout != "" || stderr == "" {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}

func TestGraphDepthAndPackageArgument(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--depth", "1"}, "demo@1.0.0 (node)\n├─ debug@2.6.9 (node)\n└─ ms@2.1.3 (node)\n"},
		{[]string{"debug"}, "debug@2.6.9 (node)\n└─ ms@2.0.0 (node)\n"},
		{[]string{"debug", "--depth", "0"}, "debug@2.6.9 (node)\n└─ ms@2.0.0 (node)\n"},
		{[]string{"ms"}, "ms@2.0.0 (node)\nms@2.1.3 (node)\n"},
	} {
		code, stdout, stderr := runGraph(t, c.args...)
		if code != 0 || stdout != c.want {
			t.Errorf("%v: exit %d, stdout:\n%s\nwant:\n%s\nstderr %q", c.args, code, stdout, c.want, stderr)
		}
	}
}

func TestGraphPackageNotFound(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	code, stdout, stderr := runGraph(t, "left-pad")
	if code != 1 || stdout != "" || !strings.Contains(stderr, `"left-pad" not found`) {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestGraphPassesExecAndWarnings(t *testing.T) {
	graphProject(t, nil)
	var ran []string
	oldRunner := graphRunner
	graphRunner = func(dir, name string, args ...string) ([]byte, error) {
		ran = append(ran, name)
		return nil, nil
	}
	t.Cleanup(func() { graphRunner = oldRunner })

	var execs []bool
	old := extractGraph
	extractGraph = func(dir string, opts graph.ExtractOptions) (*graph.DepGraph, error) {
		execs = append(execs, opts.Exec)
		if opts.Run != nil {
			_, _ = opts.Run(dir, "go", "mod", "graph")
		}
		opts.Warn("pom.xml: no <dependencies>")
		return graph.NewGraph(), nil
	}
	t.Cleanup(func() { extractGraph = old })

	_, stdout1, stderr1 := runGraph(t)
	_, stdout2, _ := runGraph(t, "--exec")
	if len(execs) != 2 || execs[0] || !execs[1] {
		t.Errorf("Exec per run = %v, want [false true]", execs)
	}
	if strings.Join(ran, ",") != "go,go" {
		t.Errorf("Run seam not passed through: ran %v", ran)
	}
	if !strings.Contains(stderr1, "warning: pom.xml: no <dependencies>") || stdout1 != "" || stdout2 != "" {
		t.Errorf("warnings must go to stderr only: stdout %q / %q, stderr %q", stdout1, stdout2, stderr1)
	}
}

func TestGraphRunsNoToolsWithoutExec(t *testing.T) {
	graphProject(t, map[string]string{
		"package-lock.json": demoLock,
		"go.mod":            "module example.com/demo\n\ngo 1.22\n\nrequire (\n\tgithub.com/pkg/errors v0.9.1\n\tgolang.org/x/mod v0.23.0 // indirect\n)\n",
	})
	noTools(t)
	code, stdout, stderr := runGraph(t)
	if code != 0 || !strings.Contains(stdout, "github.com/pkg/errors") || !strings.Contains(stdout, "debug@2.6.9") {
		t.Errorf("exit %d, stdout:\n%s\nstderr %q", code, stdout, stderr)
	}
}

func TestGraphWorkspaceMergesProjects(t *testing.T) {
	graphProject(t, map[string]string{
		"package.json":                   `{"name": "mono", "version": "1.0.0", "private": true, "workspaces": ["packages/*"]}`,
		"package-lock.json":              singleDepLock("mono", "typescript", "5.4.5"),
		"packages/web/package.json":      `{"name": "web", "version": "1.0.0"}`,
		"packages/web/package-lock.json": singleDepLock("web", "ms", "2.1.3"),
		"packages/api/package.json":      `{"name": "api", "version": "1.0.0"}`,
		"packages/api/package-lock.json": singleDepLock("api", "ms", "2.1.3"),
	})
	noTools(t)
	code, stdout, stderr := runGraph(t, "-w", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var g graph.JSONGraph
	if err := json.Unmarshal([]byte(stdout), &g); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	// mono, typescript, web, api, and one shared ms node.
	if len(g.Nodes) != 5 || len(g.Edges) != 3 || len(g.Roots) != 3 {
		t.Errorf("got %d nodes, %d edges, roots %v", len(g.Nodes), len(g.Edges), g.Roots)
	}

	code, stdout, _ = runGraph(t, "--workspace")
	for _, want := range []string{"mono@1.0.0 (node)", "web@1.0.0 (node)", "api@1.0.0 (node)", "└─ ms@2.1.3 (node)"} {
		if code != 0 || !strings.Contains(stdout, want) {
			t.Errorf("exit %d, tree lacks %q:\n%s", code, want, stdout)
		}
	}
}

func TestGraphWorkspaceNoneDetected(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	code, stdout, stderr := runGraph(t, "-w")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "no workspaces detected") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

// recordExtracts replaces extraction with a recorder of the directories
// extracted (relative to root, slash-separated).
func recordExtracts(t *testing.T, root string) *[]string {
	t.Helper()
	var dirs []string
	old := extractGraph
	extractGraph = func(dir string, _ graph.ExtractOptions) (*graph.DepGraph, error) {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, filepath.ToSlash(rel))
		return graph.NewGraph(), nil
	}
	t.Cleanup(func() { extractGraph = old })
	return &dirs
}

func TestGraphWorkspaceExtractsEachDirectoryOnce(t *testing.T) {
	// packages/web is both an npm workspace member and a go.work module, so
	// it is listed under two ecosystems; the root is the root of both. Getwd
	// (not the temp path) is the root: macOS temp dirs sit behind a symlink.
	graphProject(t, map[string]string{
		"package.json":              `{"name": "mono", "private": true, "workspaces": ["packages/*"]}`,
		"go.work":                   "go 1.22\n\nuse ./packages/web\n",
		"packages/web/package.json": `{"name": "web", "version": "1.0.0"}`,
		"packages/web/go.mod":       "module example.com/web\n\ngo 1.22\n",
	})
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dirs := recordExtracts(t, root)
	if code, _, stderr := runGraph(t, "-w"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if got := strings.Join(*dirs, ","); got != ".,packages/web" {
		t.Errorf("extracted %q, want each directory once: \".,packages/web\"", got)
	}
}

func TestGraphWorkspaceAppliesIncludeExclude(t *testing.T) {
	graphProject(t, map[string]string{
		"package.json":              `{"name": "mono", "private": true, "workspaces": ["packages/*"]}`,
		"packages/web/package.json": `{"name": "web", "version": "1.0.0"}`,
		"packages/api/package.json": `{"name": "api", "version": "1.0.0"}`,
	})
	withConfig(t, config.Config{
		Graph:     config.GraphConfig{ShowVersions: true, ShowEcosystem: true},
		Workspace: config.WorkspaceConfig{Exclude: []string{"packages/api"}},
	})
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dirs := recordExtracts(t, root)
	if code, _, stderr := runGraph(t, "-w"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if got := strings.Join(*dirs, ","); got != ".,packages/web" {
		t.Errorf("extracted %q, want \".,packages/web\" (packages/api excluded)", got)
	}
}

func TestGraphWorkspaceSkipsFailingDirectory(t *testing.T) {
	graphProject(t, map[string]string{
		"package.json":              `{"name": "mono", "private": true, "workspaces": ["packages/*"]}`,
		"packages/web/package.json": `{"name": "web", "version": "1.0.0"}`,
	})
	old := extractGraph
	extractGraph = func(dir string, _ graph.ExtractOptions) (*graph.DepGraph, error) {
		if filepath.Base(dir) == "web" {
			return nil, errors.New("broken lockfile")
		}
		return graph.NewGraph(), nil
	}
	t.Cleanup(func() { extractGraph = old })
	code, stdout, stderr := runGraph(t, "-w")
	if code != 0 || stdout != "" || !strings.Contains(stderr, "warning: skipping ") || !strings.Contains(stderr, "broken lockfile") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}
