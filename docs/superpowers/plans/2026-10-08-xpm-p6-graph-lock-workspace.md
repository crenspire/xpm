# Plan F — P6 Graph / Lock / Workspace Correctness

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `xpm graph`, `xpm lock`, `xpm workspaces`/`run --workspace`, and `xpm doctor` tell the truth: graphs parsed from real lockfile formats with correct node/edge counts, clean stdout, no build tools run unless asked; a stable `xpm-lock.yaml` that detects added/removed lockfiles; workspace commands that never `os.Chdir` from goroutines; doctor checks that never report a failed audit as OK. Delete the unused dependency cache.

**Architecture:**
- **Graph (`internal/graph`, Tasks 1–7).** `DepGraph` gets an adjacency index (O(1) edge dedupe, O(deg) children). Node IDs keep the real package name (`ecosystem:name@version`, no `/`→`-` mangling). Each ecosystem parser is a pure function over file bytes, tested against `internal/graph/testdata/<ecosystem>/...` fixtures in the real formats, with expected node/edge counts. Running `mvn`/`gradle`/`go` happens only with `ExtractOptions{Exec: true}` (`xpm graph --exec`), through a command seam. Exporters: a depth-limited tree printer that marks repeated subtrees `(*)`, escaped DOT, SVG streamed through `dot -Tsvg` stdin→stdout, valid JSON.
- **Lock (`internal/lock`, Tasks 8–9).** `xpm-lock.yaml` v2 keyed by lockfile path relative to the project root; no timestamps; byte-identical output when nothing changed (no rewrite); `--verify` reports added and removed lockfiles; every `File` path is checked to stay inside the project.
- **Cache (Task 10).** `internal/cache` and the `xpm cache` implementation are deleted (roadmap recommendation).
- **Doctor (`internal/doctor`, Task 11).** Audit tool failure ⇒ "unavailable"; pip-audit object format; no false-positive missing lockfiles for PEP 621 / plain requirements projects; drift by content, never mtime; each missing lockfile counted once.
- **Workspaces (`internal/workspace`, Tasks 12–13).** Detection skips `node_modules`/`.git`/`.venv`/`vendor`, supports `go.work` block form via `golang.org/x/mod/modfile`, `**` and `!negation` globs, include/exclude filters, deterministic order. Execution uses `cmd.Dir` per project via a command seam, installs a Node workspace once at its root, fixed ecosystem order.
- **Docs (Task 14).** README sections for graph/lock/workspaces/cache/doctor match the code.

**Tech Stack:** Go 1.22 (module floor), stdlib `encoding/json`, `gopkg.in/yaml.v3` and `github.com/BurntSushi/toml` (already dependencies), `golang.org/x/mod/modfile` v0.23.0 (new, see Rulings), golangci-lint v2.5.0.

**Spec:** `docs/superpowers/plans/2026-10-06-xpm-roadmap.md` — section "P6 — Graph / lock / workspace correctness" items 1–8 and its exit criteria, plus the P6 items deferred from P3 (`.superpowers/overnight/p3-ledger.md`: workspace/install.go cross-ecosystem map ranging).

**Out of scope:** `internal/cli/cli.go`, `install.go`, `run_cmd.go`, `man.go`, `manpage.go`, `internal/config`, `internal/scripts`, `internal/pm` (owned by P4 in parallel), `internal/env` (P5), `.golangci.yml` (shared; the controller removes the lint ratchet after all phases merge).

## Global Constraints

- Go module floor `go 1.22`; no APIs newer than 1.22 (no t.Chdir, os.CopyFS, range-over-int/func). New module dependencies only if clearly justified and only from golang.org/x/* (record a Ruling).
- Commit messages must NOT contain Co-Authored-By, "Generated with", or Claude-Session lines (a local commit-msg hook rejects them).
- Every task ends with `go build ./... && go vet ./... && go test ./...` green and `gofmt -l .` empty; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...` reports 0 issues.
- No network in unit tests (httptest / fakes / seams). Never run real package managers or real installers in tests; never write to the real $HOME, ~/.xpm, or real user caches in tests (use t.TempDir + env/seams).
- Never `git push`, tag, release, publish, or create repos. Never touch the main checkout at /opt/personal/upm (except reading).
- Keep docs honest: README/man claims must match code; verify claims.
- CI must stay green on linux/macos/windows (Go 1.22 + stable; macOS skips 1.22): guard OS-specific tests with runtime.GOOS skips where needed.
- File ownership: edit only `internal/graph/**`, `internal/lock/**`, `internal/workspace/**`, `internal/cache/**`, `internal/doctor/**`, `internal/cli/{graph,lock,workspace,cache}_cmd.go` (+ their `_test.go`), README sections for graph/lock/workspaces/cache/doctor, `go.mod`/`go.sum` (Task 5 only), and testdata under those packages. Never edit `internal/cli/cli.go`.
- Work in `/opt/personal/upm/.worktrees/p6` on branch `feat/p6-graph-lock-workspace`; `git add` only the files your task touched.

House rules that follow from the lint config (`.golangci.yml`):
- In new or edited import blocks, put `github.com/crenspire/xpm/...` imports in their own last group (goimports `local-prefixes`).
- Write to an `io.Writer` other than `os.Stdout`/`os.Stderr` only via `_, _ =` or a `strings.Builder` (errcheck).
- Wrap errors with `%w` and compare them with `errors.Is`/`errors.As` (errorlint).
- Tests use `t.TempDir()`; tests that need a working directory use a helper that `os.Chdir`s and restores via `t.Cleanup` (no `t.Chdir`), and are never `t.Parallel()`.

## Rulings made while planning

- Ruling: add `golang.org/x/mod` v0.23.0 (the newest release whose go.mod floor is `go 1.22.0`) for `go.work`/`go.mod` parsing — the roadmap prescribes `modfile`; it parses block forms, comments and quoting correctly; resolvable from the module cache offline. `go get` rewrites `go 1.22` to `go 1.22.0` (same floor). — Cost if wrong: one small dependency to remove and a hand parser to write.
- Ruling: delete `internal/cache` and the `xpm cache` implementation; `cache_cmd.go` keeps a tiny `cmdCache` that prints that the command was removed (package managers manage their own caches) and returns 1, so `cli.go` (P4-owned) needs no edit. P4/P7 must remove the `cache`/`cc`/`cg` dispatch and `usage()` lines, `config.CacheConfig` and `cache.*` keys, and man-page text. — Cost if wrong: re-adding a feature nobody used; history keeps the code.
- Ruling: `graph` runs no external tool by default. With `--exec` it may run `mvn dependency:tree -DoutputType=tgf`, `gradle dependencies --console=plain`, and `go mod graph` (all via a seam). Without `--exec`, Maven = `pom.xml` direct deps, Gradle = `gradle.lockfile`, Go = `go.mod` requires (direct vs `// indirect`). — Cost if wrong: users must pass `--exec` for full Java/Go trees.
- Ruling: `xpm-lock.yaml` scans the project root only (not subdirectories); schema `version: 2`, `locks` keyed by relative slash path; v1 files are read (keys re-derived from `file`) and reported as needing regeneration only if contents differ. — Cost if wrong: monorepo nested lockfiles need a later recursive mode.
- Ruling: `xpm run --workspace` runs each project's task by re-executing the current binary (`os.Executable()`) as `xpm run <task>` with `cmd.Dir = project.Path`, because the script runner (`internal/scripts`, P4-owned) has no working-directory parameter and `os.Chdir` from goroutines is the bug being fixed. Parallel runs buffer each project's output and print it whole, prefixed by a `[name]` header, when the project finishes. — Cost if wrong: one extra process per project (~ms).
- Ruling: `xpm install --workspace` cannot be wired in P6 (flag parsing lives in P4-owned `install.go`). P6 exposes `cmdInstallWorkspace(global bool) int` in `workspace_cmd.go` (fully tested, no `nolint:unused`) backed by `workspace.Install`; P7 wires the flag in one line. — Cost if wrong: the flag stays unwired until P7.
- Ruling: include/exclude come from `cfg.Workspace.Include/Exclude` (globs matched against the project path relative to the workspace root, slash-separated, with `path.Match` semantics plus `**`). P4 must keep the `workspace.include`, `workspace.exclude`, `workspace.parallel` config keys. — Cost if wrong: if P4 deletes them, the controller fixes one line in `workspace_cmd.go` at merge.
- Ruling: doctor drift is decided by content, never mtime: (a) npm `package-lock.json` v2/v3 root entry and pnpm `importers["."]` vs the manifest's declared dependency names; (b) Cargo/go: every manifest dependency name present in the lockfile; (c) composer/poetry: lockfile exists and is parseable; otherwise "unknown", never "outdated". — Cost if wrong: some drift goes undetected rather than falsely reported.
- Ruling: the lint ratchet (`new-from-rev` in `.golangci.yml`) is NOT removed in P6 — the file is shared by parallel phases; the controller removes it after P4–P6 merge (record in the phase report). — Cost if wrong: one-line follow-up.

## Interfaces fixed up front (tasks consume these exact names)

```go
// internal/graph (Task 1 produces; Tasks 2–7 consume)
type ExtractOptions struct {
	Exec bool                                      // allow running mvn/gradle/go
	Run  func(dir, name string, args ...string) ([]byte, error) // nil = real exec (stdout only); tests inject fakes
	Warn func(msg string)                          // non-fatal warnings; nil = discard
}
func NodeID(ecosystem, name, version string) string          // "ecosystem:name@version", name verbatim
func (g *DepGraph) AddEdge(e *DepEdge)                       // O(1) dedupe
func (g *DepGraph) Children(id string) []string              // sorted, deduped
func ExtractAll(dir string, opts ExtractOptions) (*DepGraph, error)        // Task 6 final signature
func Subgraph(g *DepGraph, name string) (*DepGraph, error)                 // replaces ExtractForPackage
func PrintTree(g *DepGraph, w io.Writer, opts TreeOptions)                 // Task 2
func WriteSVG(g *DepGraph, w io.Writer) error                              // Task 2, dot stdin→w via seam

// internal/workspace (Task 12/13 produce; Task 7 consumes only DetectWorkspaces, unchanged signature)
func DetectWorkspaces(root string) ([]Workspace, error)
```

## Task map

| Task | Package / files | Summary |
|---|---|---|
| 1 | internal/graph core | adjacency index, verbatim IDs, NormalizeVersions re-keys nodes/edges/roots, Subgraph, ExtractOptions |
| 2 | internal/graph exporters | tree printer depth + `(*)`, DOT escaping, SVG via stdin seam, JSON, 1k-node budget test |
| 3 | internal/graph node (npm) | npm v1/v2/v3 fixtures, hoisted resolution |
| 4 | internal/graph node (pnpm, yarn) | pnpm v6/v9 (+snapshots), yarn v1 + berry fixtures |
| 5 | internal/graph cargo/go/python/composer | Cargo by name+version, go.mod via modfile, `go mod graph` (exec), poetry, composer |
| 6 | internal/graph java + ExtractAll | Maven pom/tgf, Gradle lockfile/output, exec gating, detection |
| 7 | internal/cli/graph_cmd.go | `--exec`, stdout hygiene, `--svg` streaming, `--depth` validation, workspace graph |
| 8 | internal/lock | v2 schema, relpath keys, no churn, verify added/removed, containment |
| 9 | internal/cli/lock_cmd.go | CLI output for added/removed/unchanged, no rewrite when identical |
| 10 | internal/cache, cache_cmd.go | delete package; removal stub |
| 11 | internal/doctor | audits unavailable, pip-audit object format, false positives, drift by content, counted once |
| 12 | internal/workspace detection | skip dirs, go.work via modfile, globs, include/exclude, deterministic order |
| 13 | internal/workspace exec + workspace_cmd.go | cmd.Dir, node root install, fixed ecosystem order, run via re-exec, cmdInstallWorkspace |
| 14 | README | graph/lock/workspaces/cache/doctor sections |


---

## Section A — Graph core, exporters, `xpm graph` (Tasks 1, 2, 7)

## Review Focus

1. **Diamond-heavy real lockfiles** (a typical npm/pnpm tree shares most transitive deps). Expected: `xpm graph` prints each subtree once, marks repeats `(*)`, and finishes in well under 200 ms for 1k nodes / ~5k edges. Pinned by `TestSyntheticGraphTreeIsLinearInEdges` (exactly 1 + E lines), `TestPrintTreeGolden/diamond…` (Task 2), `TestGraphPipelineBudget` + `BenchmarkGraphPipeline` (Task 2).
2. **Redirected output** — `xpm graph --json > g.json`, `xpm graph --svg > g.svg`, including projects with no dependencies and projects that produce warnings. Expected: stdout holds only the document (valid JSON / SVG); warnings, "No dependencies found." and errors go to stderr. Pinned by `TestGraphJSONStdoutIsValidJSON`, `TestGraphEmptyProject`, `TestGraphSVGStreamsToStdout`, `TestGraphTreeToStdoutWarningsToStderr` (Task 7).
3. **Names that used to collide or break output** — `@scope/pkg` vs `scope-pkg`, Go module paths, `v1.2.0` vs `1.2.0` of the same package, and names containing `"`/`\`. Expected: distinct nodes for distinct names, one merged node for version spellings, valid DOT. Pinned by `TestNodeIDKeepsNameVerbatim`, `TestNormalizeVersionsMergesCollidingNodes` (Task 1), `TestToDOTEscapesAndSorts` (Task 2).

---

### Task 1: Graph core — adjacency index, verbatim node IDs, re-keying normalization, `Subgraph`, `ExtractOptions`

**Files:**
- Modify: `internal/graph/node.go` (`NodeID` and its import block)
- Replace: `internal/graph/graph.go`, `internal/graph/normalize.go`
- Create: `internal/graph/options.go`, `internal/graph/subgraph.go`
- Modify: `internal/graph/extract_all.go` (delete `ExtractForPackage` and `contains`)
- Modify: `internal/cli/graph_cmd.go` (the one `ExtractForPackage` call)
- Test: `internal/graph/main_test.go`, `internal/graph/options_test.go`, `internal/graph/graph_test.go`, `internal/graph/normalize_test.go`, `internal/graph/subgraph_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces (Tasks 2–7 rely on these):
  - `type ExtractOptions struct { Exec bool; Run func(dir, name string, args ...string) ([]byte, error); Warn func(msg string) }`
  - `func (o ExtractOptions) run(dir, name string, args ...string) ([]byte, error)` — `o.Run` if set, else `exec.Command(name, args...)` with `cmd.Dir = dir`, stdout only (`cmd.Output()`), error wrapped as `"<name>: %w"`.
  - `func (o ExtractOptions) warn(format string, a ...any)` — formats and calls `o.Warn`; no-op when nil.
  - `func NodeID(ecosystem, name, version string) string` — `"ecosystem:name@version"`, name verbatim.
  - `func (g *DepGraph) AddEdge(e *DepEdge)` — O(1) dedupe on (From, To), first edge wins; ignores nil, empty endpoints, self-loops.
  - `func (g *DepGraph) HasEdge(from, to string) bool`
  - `func (g *DepGraph) Children(id string) []string` — sorted, deduped. `GetChildren` is kept as an alias.
  - `func (g *DepGraph) GetParents(id string) []string` — sorted.
  - `func (g *DepGraph) reindex()` — rebuilds the index from `Edges`/`Root`, dropping invalid/duplicate edges and duplicate roots, order preserved. **Any code that assigns `g.Edges`/`g.Root` wholesale must call it.** Never mutate `e.From`/`e.To` of an edge already in a graph (edges are shared between graphs by `Merge`); build new edges instead.
  - `func (g *DepGraph) reachableFrom(starts []string) map[string]bool`
  - `func sortedKeys[V any](m map[string]V) []string`
  - `func Subgraph(g *DepGraph, name string) (*DepGraph, error)` — replaces `ExtractForPackage`.
  - `NormalizeGraph`, `NormalizeVersions`, `DeduplicateNodes` re-key nodes, edges, roots and the index (`(*DepGraph).rekey()`).
  - Test helpers (package `graph`, usable by Tasks 2–6 tests): `const helperEnv = "XPM_GRAPH_TEST_HELPER"`, `TestMain`/`helperMain` (the test binary acts as a fake command when `helperEnv` is set), `testNode(g, name, version) string`, `assertIndexConsistent(t, g)`. **Tasks 3–6 must not declare another `TestMain` in package `graph`**; they may add helper modes to `helperMain`.
- `ExtractAll`'s signature is NOT changed here (Task 6 changes it to `ExtractAll(dir string, opts ExtractOptions)`).

Design notes:
- `Edges` stays the canonical exported slice (JSON/DOT read it); the unexported `out` (from → to → edge) and `in` (to → from) maps mirror it. Every mutator maintains both; wholesale rewrites go through `reindex()`. `ensureIndex()` also rebuilds when the index is missing or `len(Edges)` changed behind the graph's back (struct-literal graphs, external appends), so reads are always consistent for append-only misuse. `assertIndexConsistent` proves consistency after every mutator in the tests.
- `rekey()` is the one O(N + E) remap used by `NormalizeVersions` and `DeduplicateNodes`: nodes are visited in sorted old-key order, the first node to claim a canonical ID wins and missing metadata keys are copied from the others; edges are re-created (not mutated) through the old→new map and `reindex()` drops the self-loops and duplicates a merge creates. This replaces the O(N·E)-per-duplicate loop and fixes `NormalizeVersions` changing `node.ID` without re-keying the map, edges or roots.
- `normalizeVersion` strips a leading `=` and a `v`/`V` only when a digit follows (`very-new` stays).
- `Subgraph` makes every version of the name a root (sorted by ID), copies the reachable nodes and the edges leaving them, in `g.Edges` order. Unknown name → `package "<name>" not found in the dependency graph`.
- `AddRoot` is O(1) via `rootSet`; roots keep insertion order.

- [ ] **Step 1: Write the failing tests**

Create `internal/graph/main_test.go`:

```go
package graph

import (
	"fmt"
	"os"
	"testing"
)

// helperEnv makes the test binary act as a fake external command (see
// helperMain) instead of running the tests. This works on every OS, unlike
// shell-script fakes.
const helperEnv = "XPM_GRAPH_TEST_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(helperMain(mode))
	}
	os.Exit(m.Run())
}

// helperMain implements the fake commands:
//   - "pwd": print the working directory to stdout and noise to stderr.
func helperMain(mode string) int {
	switch mode {
	case "pwd":
		wd, err := os.Getwd()
		if err != nil {
			return 3
		}
		fmt.Print(wd)
		fmt.Fprint(os.Stderr, "stderr noise")
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
	return 2
}
```

Create `internal/graph/options_test.go`:

```go
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
```

Create `internal/graph/graph_test.go`:

```go
package graph

import (
	"reflect"
	"testing"
)

// testNode adds an npm node to g and returns its ID.
func testNode(g *DepGraph, name, version string) string {
	n := NewDepNode("node", name, version)
	g.AddNode(n)
	return n.ID
}

// assertIndexConsistent checks that the adjacency index matches Edges.
func assertIndexConsistent(t *testing.T, g *DepGraph) {
	t.Helper()
	seen := map[[2]string]bool{}
	for _, e := range g.Edges {
		k := [2]string{e.From, e.To}
		if seen[k] {
			t.Errorf("duplicate edge %s -> %s in Edges", e.From, e.To)
		}
		seen[k] = true
		if !g.HasEdge(e.From, e.To) {
			t.Errorf("edge %s -> %s missing from the index", e.From, e.To)
		}
	}
	n := 0
	for from, tos := range g.out {
		for to := range tos {
			n++
			if !seen[[2]string{from, to}] {
				t.Errorf("index has %s -> %s but Edges does not", from, to)
			}
			if _, ok := g.in[to][from]; !ok {
				t.Errorf("reverse index lacks %s -> %s", from, to)
			}
		}
	}
	if n != len(g.Edges) {
		t.Errorf("index has %d edges, Edges has %d", n, len(g.Edges))
	}
}

func TestNodeIDKeepsNameVerbatim(t *testing.T) {
	if got := NodeID("node", "@scope/pkg", "1.0.0"); got != "node:@scope/pkg@1.0.0" {
		t.Errorf("NodeID = %q", got)
	}
	if NodeID("node", "@scope/pkg", "1.0.0") == NodeID("node", "scope-pkg", "1.0.0") {
		t.Error("@scope/pkg and scope-pkg must not collide")
	}
	if got := NodeID("go", "github.com/pkg/errors", "v0.9.1"); got != "go:github.com/pkg/errors@v0.9.1" {
		t.Errorf("NodeID = %q", got)
	}
}

func TestAddEdgeDedupesAndRejectsSelfLoops(t *testing.T) {
	g := NewGraph()
	a, b := testNode(g, "a", "1"), testNode(g, "b", "1")
	g.AddEdge(NewEdge(a, b))
	g.AddEdge(NewTransitiveEdge(a, b)) // same endpoints: ignored, first wins
	g.AddEdge(NewEdge(a, a))
	g.AddEdge(NewEdge("", b))
	g.AddEdge(nil)
	if g.EdgeCount() != 1 || g.Edges[0].Type != "direct" {
		t.Fatalf("edges = %v", g.Edges)
	}
	assertIndexConsistent(t, g)
}

func TestChildrenAndParentsSorted(t *testing.T) {
	g := NewGraph()
	r, z, m, a := testNode(g, "root", "1"), testNode(g, "z", "1"), testNode(g, "m", "1"), testNode(g, "a", "1")
	g.AddEdge(NewEdge(r, z))
	g.AddEdge(NewEdge(r, m))
	g.AddEdge(NewEdge(r, a))
	g.AddEdge(NewEdge(m, a))
	if got, want := g.Children(r), []string{a, m, z}; !reflect.DeepEqual(got, want) {
		t.Errorf("Children = %v, want %v", got, want)
	}
	if got, want := g.GetParents(a), []string{m, r}; !reflect.DeepEqual(got, want) {
		t.Errorf("GetParents = %v, want %v", got, want)
	}
	if got := g.Children("node:missing@1"); len(got) != 0 {
		t.Errorf("Children(missing) = %v", got)
	}
}

func TestGetTransitiveVisitsEachNodeOnce(t *testing.T) {
	g := NewGraph()
	r, a, b, c := testNode(g, "r", "1"), testNode(g, "a", "1"), testNode(g, "b", "1"), testNode(g, "c", "1")
	g.AddEdge(NewEdge(r, a))
	g.AddEdge(NewEdge(r, b))
	g.AddEdge(NewEdge(a, c))
	g.AddEdge(NewEdge(b, c))
	g.AddEdge(NewEdge(c, r)) // cycle back to the start
	if got, want := g.GetTransitive(r), []string{a, c, b}; !reflect.DeepEqual(got, want) {
		t.Errorf("GetTransitive = %v, want %v", got, want)
	}
}

func TestStructLiteralGraphIsIndexedOnDemand(t *testing.T) {
	g := &DepGraph{
		Nodes: map[string]*DepNode{},
		Edges: []*DepEdge{NewEdge("x", "y"), NewEdge("x", "y"), NewEdge("x", "x")},
	}
	if got := g.Children("x"); !reflect.DeepEqual(got, []string{"y"}) {
		t.Errorf("Children = %v", got)
	}
	if g.EdgeCount() != 1 {
		t.Errorf("EdgeCount = %d, want 1 after indexing", g.EdgeCount())
	}
	assertIndexConsistent(t, g)
}

func TestAddRootOnce(t *testing.T) {
	g := NewGraph()
	g.AddRoot("b")
	g.AddRoot("a")
	g.AddRoot("b")
	if !reflect.DeepEqual(g.Root, []string{"b", "a"}) {
		t.Errorf("Root = %v, want insertion order without duplicates", g.Root)
	}
}

func TestMergeDedupesEdgesAndRoots(t *testing.T) {
	g1, g2 := NewGraph(), NewGraph()
	for _, g := range []*DepGraph{g1, g2} {
		r, a := testNode(g, "r", "1"), testNode(g, "a", "1")
		g.AddRoot(r)
		g.AddEdge(NewEdge(r, a))
	}
	b := testNode(g2, "b", "1")
	g2.AddEdge(NewEdge(NodeID("node", "a", "1"), b))
	g1.Merge(g2)
	if g1.NodeCount() != 3 || g1.EdgeCount() != 2 || len(g1.Root) != 1 {
		t.Errorf("merged: %d nodes, %d edges, roots %v", g1.NodeCount(), g1.EdgeCount(), g1.Root)
	}
	assertIndexConsistent(t, g1)
}

func TestTrimAndNormalizeKeepIndexConsistent(t *testing.T) {
	g := NewGraph()
	r, a, x, y := testNode(g, "r", "1"), testNode(g, "a", "1"), testNode(g, "x", "1"), testNode(g, "y", "1")
	testNode(g, "lonely", "1")
	g.AddRoot(r)
	g.AddEdge(NewEdge(r, a))
	g.AddEdge(NewEdge(x, y)) // unreachable from the root

	n := NewGraph()
	n.Merge(g)
	n.Normalize() // drops only "lonely"
	if n.NodeCount() != 4 || n.EdgeCount() != 2 {
		t.Errorf("Normalize: %d nodes, %d edges", n.NodeCount(), n.EdgeCount())
	}
	assertIndexConsistent(t, n)

	g.Trim()
	if g.NodeCount() != 2 || g.EdgeCount() != 1 || g.HasEdge(x, y) {
		t.Errorf("Trim: %d nodes, %d edges", g.NodeCount(), g.EdgeCount())
	}
	assertIndexConsistent(t, g)
}
```

Create `internal/graph/normalize_test.go`:

```go
package graph

import (
	"reflect"
	"testing"
)

func TestNormalizeVersionsRekeysNodesEdgesAndRoots(t *testing.T) {
	g := NewGraph()
	root := testNode(g, "app", "v1.0.0")
	dep := testNode(g, "dep", "=2.0.0")
	g.AddRoot(root)
	g.AddEdge(NewEdge(root, dep))

	NormalizeVersions(g)

	wantRoot, wantDep := "node:app@1.0.0", "node:dep@2.0.0"
	if g.GetNode(wantRoot) == nil || g.GetNode(wantDep) == nil || g.NodeCount() != 2 {
		t.Fatalf("nodes = %v", sortedKeys(g.Nodes))
	}
	if g.GetNode(wantRoot).ID != wantRoot {
		t.Errorf("node.ID = %q, want %q", g.GetNode(wantRoot).ID, wantRoot)
	}
	if !reflect.DeepEqual(g.Root, []string{wantRoot}) {
		t.Errorf("Root = %v", g.Root)
	}
	if !reflect.DeepEqual(g.Children(wantRoot), []string{wantDep}) {
		t.Errorf("Children = %v", g.Children(wantRoot))
	}
	if len(g.Validate()) != 0 {
		t.Errorf("Validate = %v", g.Validate())
	}
	assertIndexConsistent(t, g)
}

func TestNormalizeVersionsMergesCollidingNodes(t *testing.T) {
	g := NewGraph()
	app := testNode(g, "app", "1.0.0")
	a := NewDepNode("node", "lib", "1.2.0").WithMetadata("resolved", "r1")
	b := NewDepNode("node", "lib", "v1.2.0").WithMetadata("integrity", "sha512-x")
	g.AddNode(a)
	g.AddNode(b)
	c := testNode(g, "c", "1.0.0")
	g.AddRoot(app)
	g.AddEdge(NewEdge(app, a.ID))
	g.AddEdge(NewEdge(app, b.ID)) // becomes a duplicate of app -> lib@1.2.0
	g.AddEdge(NewEdge(b.ID, c))
	g.AddEdge(NewEdge(a.ID, b.ID)) // becomes a self-loop

	NormalizeGraph(g)

	lib := g.GetNode("node:lib@1.2.0")
	if g.NodeCount() != 3 || lib == nil {
		t.Fatalf("nodes = %v", sortedKeys(g.Nodes))
	}
	if lib.GetMetadata("resolved") != "r1" || lib.GetMetadata("integrity") != "sha512-x" {
		t.Errorf("metadata not merged: %v", lib.Metadata)
	}
	if g.EdgeCount() != 2 || !g.HasEdge(app, "node:lib@1.2.0") || !g.HasEdge("node:lib@1.2.0", c) {
		t.Errorf("edges = %v", g.Edges)
	}
	assertIndexConsistent(t, g)
}

func TestNormalizeVersionKeepsNonNumericV(t *testing.T) {
	for in, want := range map[string]string{
		"v1.2.3": "1.2.3", "=1.0.0": "1.0.0", " 2.0.0 ": "2.0.0", "V3": "3",
		"=v1.0.0": "1.0.0", "very-new": "very-new", "v": "v", "": "",
	} {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeduplicateNodesRemapsMisKeyedNodes(t *testing.T) {
	g := NewGraph()
	good := NewDepNode("node", "x", "1.0.0")
	g.AddNode(good)
	// A node stored under a stale key (as the old NodeID mangling produced).
	stale := NewDepNode("node", "x", "1.0.0")
	stale.ID = "node:x-stale@1.0.0"
	g.AddNode(stale)
	parent := testNode(g, "p", "1.0.0")
	g.AddRoot(stale.ID)
	g.AddEdge(NewEdge(parent, stale.ID))

	DeduplicateNodes(g)

	if g.NodeCount() != 2 || !reflect.DeepEqual(g.Root, []string{good.ID}) || !g.HasEdge(parent, good.ID) {
		t.Errorf("nodes %v, roots %v, edges %v", sortedKeys(g.Nodes), g.Root, g.Edges)
	}
	assertIndexConsistent(t, g)
}
```

Create `internal/graph/subgraph_test.go`:

```go
package graph

import (
	"reflect"
	"strings"
	"testing"
)

func TestSubgraphAllVersionsBecomeRoots(t *testing.T) {
	g := NewGraph()
	app := testNode(g, "app", "1.0.0")
	d1, d2 := testNode(g, "debug", "2.6.9"), testNode(g, "debug", "4.3.4")
	ms1, ms2 := testNode(g, "ms", "2.0.0"), testNode(g, "ms", "2.1.3")
	other := testNode(g, "other", "1.0.0")
	g.AddRoot(app)
	for _, e := range [][2]string{{app, d2}, {app, other}, {other, d1}, {d1, ms1}, {d2, ms2}} {
		g.AddEdge(NewEdge(e[0], e[1]))
	}

	sub, err := Subgraph(g, "debug")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sub.Root, []string{d1, d2}) {
		t.Errorf("Root = %v", sub.Root)
	}
	if got := sortedKeys(sub.Nodes); !reflect.DeepEqual(got, []string{d1, d2, ms1, ms2}) {
		t.Errorf("Nodes = %v", got)
	}
	if sub.EdgeCount() != 2 || !sub.HasEdge(d1, ms1) || !sub.HasEdge(d2, ms2) {
		t.Errorf("Edges = %v", sub.Edges)
	}
	assertIndexConsistent(t, sub)
}

func TestSubgraphNotFound(t *testing.T) {
	_, err := Subgraph(NewGraph(), "left-pad")
	if err == nil || !strings.Contains(err.Error(), `"left-pad" not found`) {
		t.Errorf("err = %v", err)
	}
}

func TestSubgraphWithCycle(t *testing.T) {
	g := NewGraph()
	a, b := testNode(g, "a", "1"), testNode(g, "b", "1")
	g.AddEdge(NewEdge(a, b))
	g.AddEdge(NewEdge(b, a))
	sub, err := Subgraph(g, "b")
	if err != nil {
		t.Fatal(err)
	}
	if sub.NodeCount() != 2 || sub.EdgeCount() != 2 || !reflect.DeepEqual(sub.Root, []string{b}) {
		t.Errorf("sub: %v %v %v", sortedKeys(sub.Nodes), sub.Edges, sub.Root)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/graph/`
Expected: FAIL (build failed) with errors including `undefined: ExtractOptions`, `g.HasEdge undefined`, `undefined: Subgraph`, `undefined: sortedKeys`.

- [ ] **Step 3: Implement**

In `internal/graph/node.go`, replace the import block

```go
import (
	"fmt"
	"strings"
)
```

with

```go
import "fmt"
```

and replace `NodeID` (comment included) with:

```go
// NodeID generates a unique node ID from ecosystem, name, and version.
// Format: ecosystem:name@version, with the name kept verbatim so that
// "@scope/pkg" and "scope-pkg" (or two Go module paths) never collide.
func NodeID(ecosystem, name, version string) string {
	return ecosystem + ":" + name + "@" + version
}
```

Replace `internal/graph/graph.go` with:

```go
package graph

import (
	"fmt"
	"sort"
)

// DepGraph represents a unified dependency graph.
//
// Edges is the canonical, exported edge list (used by the JSON and DOT
// exporters). The unexported adjacency index mirrors it for O(1) edge
// dedupe and O(degree) child/parent lookups. Mutate edges only through
// AddEdge or the graph's methods; code that rewrites Edges wholesale must
// call reindex afterwards.
type DepGraph struct {
	// Nodes maps node IDs to their node objects.
	Nodes map[string]*DepNode

	// Edges is the list of dependency edges, in insertion order.
	Edges []*DepEdge

	// Root contains the root package IDs, in insertion order.
	Root []string

	out     map[string]map[string]*DepEdge // from -> to -> edge
	in      map[string]map[string]struct{} // to -> from
	rootSet map[string]struct{}
	indexed int // len(Edges) when the index was last consistent
}

// NewGraph creates a new empty dependency graph.
func NewGraph() *DepGraph {
	g := &DepGraph{
		Nodes: make(map[string]*DepNode),
		Edges: []*DepEdge{},
		Root:  []string{},
	}
	g.reindex()
	return g
}

// reindex rebuilds the adjacency index and root set from Edges and Root,
// dropping invalid edges (empty endpoints, self-loops), duplicate edges and
// duplicate roots. The first occurrence of each edge and root wins, so the
// order of Edges and Root is preserved.
func (g *DepGraph) reindex() {
	edges := g.Edges
	g.Edges = make([]*DepEdge, 0, len(edges))
	g.out = make(map[string]map[string]*DepEdge)
	g.in = make(map[string]map[string]struct{})
	g.indexed = 0
	for _, e := range edges {
		g.addEdgeIndexed(e)
	}

	roots := g.Root
	g.Root = make([]string, 0, len(roots))
	g.rootSet = make(map[string]struct{}, len(roots))
	for _, id := range roots {
		g.AddRoot(id)
	}
}

// ensureIndex rebuilds the index if Edges was changed behind the graph's
// back (for example a DepGraph built as a struct literal).
func (g *DepGraph) ensureIndex() {
	if g.out == nil || g.rootSet == nil || g.indexed != len(g.Edges) {
		g.reindex()
	}
}

func (g *DepGraph) addEdgeIndexed(e *DepEdge) {
	if e == nil || !e.IsValid() {
		return
	}
	if _, dup := g.out[e.From][e.To]; dup {
		return
	}
	if g.out[e.From] == nil {
		g.out[e.From] = make(map[string]*DepEdge)
	}
	g.out[e.From][e.To] = e
	if g.in[e.To] == nil {
		g.in[e.To] = make(map[string]struct{})
	}
	g.in[e.To][e.From] = struct{}{}
	g.Edges = append(g.Edges, e)
	g.indexed = len(g.Edges)
}

// AddNode adds a node to the graph. A node with the same ID replaces the
// existing one.
func (g *DepGraph) AddNode(node *DepNode) {
	if node == nil {
		return
	}
	if g.Nodes == nil {
		g.Nodes = make(map[string]*DepNode)
	}
	g.Nodes[node.ID] = node
}

// AddEdge adds an edge, ignoring invalid edges (self-loops, empty
// endpoints) and edges already present (same From and To). O(1).
func (g *DepGraph) AddEdge(edge *DepEdge) {
	g.ensureIndex()
	g.addEdgeIndexed(edge)
}

// HasEdge reports whether the graph has an edge from -> to.
func (g *DepGraph) HasEdge(from, to string) bool {
	g.ensureIndex()
	_, ok := g.out[from][to]
	return ok
}

// GetNode retrieves a node by ID.
func (g *DepGraph) GetNode(id string) *DepNode {
	return g.Nodes[id]
}

// Children returns the IDs of the direct dependencies of id, sorted.
func (g *DepGraph) Children(id string) []string {
	g.ensureIndex()
	return sortedKeys(g.out[id])
}

// GetChildren returns the direct dependencies of a node, sorted.
func (g *DepGraph) GetChildren(nodeID string) []string {
	return g.Children(nodeID)
}

// GetParents returns the IDs of the nodes that depend on nodeID, sorted.
func (g *DepGraph) GetParents(nodeID string) []string {
	g.ensureIndex()
	parents := make([]string, 0, len(g.in[nodeID]))
	for id := range g.in[nodeID] {
		parents = append(parents, id)
	}
	sort.Strings(parents)
	return parents
}

// GetTransitive returns every node reachable from nodeID (excluding nodeID
// itself unless it is part of a cycle), each once, in depth-first order with
// sorted children.
func (g *DepGraph) GetTransitive(nodeID string) []string {
	visited := map[string]bool{nodeID: true}
	var result []string
	var dfs func(string)
	dfs = func(id string) {
		for _, child := range g.Children(id) {
			if visited[child] {
				continue
			}
			visited[child] = true
			result = append(result, child)
			dfs(child)
		}
	}
	dfs(nodeID)
	return result
}

// Validate checks graph integrity.
func (g *DepGraph) Validate() []error {
	var errs []error
	for _, edge := range g.Edges {
		if _, exists := g.Nodes[edge.From]; !exists {
			errs = append(errs, fmt.Errorf("edge references missing node: %s", edge.From))
		}
		if _, exists := g.Nodes[edge.To]; !exists {
			errs = append(errs, fmt.Errorf("edge references missing node: %s", edge.To))
		}
	}
	for _, rootID := range g.Root {
		if _, exists := g.Nodes[rootID]; !exists {
			errs = append(errs, fmt.Errorf("root node missing: %s", rootID))
		}
	}
	return errs
}

// Normalize removes invalid and duplicate edges, then nodes that are neither
// a root nor an edge endpoint.
func (g *DepGraph) Normalize() {
	g.reindex()
	RemoveOrphanedNodes(g)
}

// AddRoot adds a root node ID (once). O(1).
func (g *DepGraph) AddRoot(nodeID string) {
	if g.rootSet == nil {
		g.rootSet = make(map[string]struct{}, len(g.Root))
		for _, id := range g.Root {
			g.rootSet[id] = struct{}{}
		}
	}
	if _, ok := g.rootSet[nodeID]; ok {
		return
	}
	g.rootSet[nodeID] = struct{}{}
	g.Root = append(g.Root, nodeID)
}

// Merge merges another graph into this graph. Nodes of other replace nodes
// with the same ID; edges and roots are added once.
func (g *DepGraph) Merge(other *DepGraph) {
	if other == nil {
		return
	}
	for _, id := range sortedKeys(other.Nodes) {
		g.AddNode(other.Nodes[id])
	}
	for _, edge := range other.Edges {
		g.AddEdge(edge)
	}
	for _, rootID := range other.Root {
		g.AddRoot(rootID)
	}
}

// NodeCount returns the number of nodes in the graph.
func (g *DepGraph) NodeCount() int {
	return len(g.Nodes)
}

// EdgeCount returns the number of edges in the graph.
func (g *DepGraph) EdgeCount() int {
	return len(g.Edges)
}

// GetNodesByEcosystem returns all nodes for a specific ecosystem, sorted by ID.
func (g *DepGraph) GetNodesByEcosystem(ecosystem string) []*DepNode {
	var nodes []*DepNode
	for _, node := range g.Nodes {
		if node.Ecosystem == ecosystem {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

// FindNodeByName finds nodes by name (may return multiple versions), sorted by ID.
func (g *DepGraph) FindNodeByName(name string) []*DepNode {
	var nodes []*DepNode
	for _, node := range g.Nodes {
		if node.Name == name {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

// Clear removes all nodes, edges and roots.
func (g *DepGraph) Clear() {
	g.Nodes = make(map[string]*DepNode)
	g.Edges = []*DepEdge{}
	g.Root = []string{}
	g.reindex()
}

// Trim removes nodes and edges that are not reachable from root nodes.
func (g *DepGraph) Trim() {
	if len(g.Root) == 0 {
		g.Clear()
		return
	}
	reachable := g.reachableFrom(g.Root)
	for id := range g.Nodes {
		if !reachable[id] {
			delete(g.Nodes, id)
		}
	}
	kept := make([]*DepEdge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if reachable[edge.From] && reachable[edge.To] {
			kept = append(kept, edge)
		}
	}
	g.Edges = kept
	g.reindex()
}

// reachableFrom returns the set of IDs reachable from starts (inclusive).
func (g *DepGraph) reachableFrom(starts []string) map[string]bool {
	g.ensureIndex()
	reachable := make(map[string]bool)
	stack := append([]string(nil), starts...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if reachable[id] {
			continue
		}
		reachable[id] = true
		for child := range g.out[id] {
			if !reachable[child] {
				stack = append(stack, child)
			}
		}
	}
	return reachable
}

// sortedKeys returns the keys of m in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

Replace `internal/graph/normalize.go` with:

```go
package graph

import (
	"sort"
	"strings"
)

// NormalizeGraph normalizes versions, re-keys every node by its canonical
// ID (merging nodes that collide), and drops self-loops and duplicate edges.
func NormalizeGraph(graph *DepGraph) {
	NormalizeVersions(graph)
}

// DeduplicateNodes re-keys every node by NodeID(ecosystem, name, version)
// and merges nodes that end up with the same ID. Edges and roots are
// remapped once, in O(N + E).
func DeduplicateNodes(graph *DepGraph) {
	graph.rekey()
}

// RemoveSelfLoops removes edges from a node to itself.
func RemoveSelfLoops(graph *DepGraph) {
	graph.reindex()
}

// RemoveDuplicateEdges keeps only the first edge for each From -> To pair.
func RemoveDuplicateEdges(graph *DepGraph) {
	graph.reindex()
}

// RemoveOrphanedNodes removes nodes that are neither a root nor an edge endpoint.
func RemoveOrphanedNodes(graph *DepGraph) {
	connected := make(map[string]bool, len(graph.Nodes))
	for _, edge := range graph.Edges {
		connected[edge.From] = true
		connected[edge.To] = true
	}
	for _, rootID := range graph.Root {
		connected[rootID] = true
	}
	for id := range graph.Nodes {
		if !connected[id] {
			delete(graph.Nodes, id)
		}
	}
}

// NormalizeVersions strips a leading "=" or a "v" before a digit from every
// version ("v1.2.0" -> "1.2.0") and re-keys nodes, edges and roots to the new
// IDs. Nodes that collide after normalization are merged.
func NormalizeVersions(graph *DepGraph) {
	for _, node := range graph.Nodes {
		node.Version = normalizeVersion(node.Version)
	}
	graph.rekey()
}

// rekey rebuilds Nodes keyed by each node's canonical ID, merges colliding
// nodes (the node with the smallest old key wins; missing metadata keys are
// copied from the others), and rewrites edge endpoints and roots through the
// old->new ID map. Edges are copied, never mutated, because Merge shares edge
// pointers between graphs.
func (g *DepGraph) rekey() {
	remap := make(map[string]string, len(g.Nodes))
	nodes := make(map[string]*DepNode, len(g.Nodes))
	for _, oldID := range sortedKeys(g.Nodes) {
		node := g.Nodes[oldID]
		newID := NodeID(node.Ecosystem, node.Name, node.Version)
		remap[oldID] = newID
		if primary, ok := nodes[newID]; ok {
			for k, v := range node.Metadata {
				if _, exists := primary.Metadata[k]; !exists {
					if primary.Metadata == nil {
						primary.Metadata = make(map[string]string)
					}
					primary.Metadata[k] = v
				}
			}
			continue
		}
		node.ID = newID
		nodes[newID] = node
	}
	g.Nodes = nodes

	mapID := func(id string) string {
		if newID, ok := remap[id]; ok {
			return newID
		}
		return id
	}
	edges := make([]*DepEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, &DepEdge{From: mapID(e.From), To: mapID(e.To), Type: e.Type})
	}
	g.Edges = edges
	roots := make([]string, 0, len(g.Root))
	for _, id := range g.Root {
		roots = append(roots, mapID(id))
	}
	g.Root = roots
	g.reindex()
}

// normalizeVersion normalizes a version string.
func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "=")
	if len(version) > 1 && (version[0] == 'v' || version[0] == 'V') && version[1] >= '0' && version[1] <= '9' {
		version = version[1:]
	}
	return version
}

// SortNodes sorts nodes by ecosystem, then name, then version.
func SortNodes(nodes []*DepNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Ecosystem != nodes[j].Ecosystem {
			return nodes[i].Ecosystem < nodes[j].Ecosystem
		}
		if nodes[i].Name != nodes[j].Name {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].Version < nodes[j].Version
	})
}

// SortEdges sorts edges by from node, then to node.
func SortEdges(edges []*DepEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
}
```

Create `internal/graph/options.go`:

```go
package graph

import (
	"fmt"
	"os/exec"
)

// ExtractOptions controls how dependency graphs are extracted.
type ExtractOptions struct {
	// Exec allows running build tools (mvn, gradle, go) to resolve full
	// trees. Without it extractors only parse files.
	Exec bool
	// Run runs name with args in dir and returns its stdout. nil runs the
	// real command (stdout only; stderr is discarded). Tests inject fakes.
	Run func(dir, name string, args ...string) ([]byte, error)
	// Warn receives non-fatal warnings. nil discards them.
	Warn func(msg string)
}

// run runs a command through Run, or for real with cmd.Dir = dir.
func (o ExtractOptions) run(dir, name string, args ...string) ([]byte, error) {
	if o.Run != nil {
		return o.Run(dir, name, args...)
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// warn formats a warning and passes it to Warn, if set.
func (o ExtractOptions) warn(format string, a ...any) {
	if o.Warn != nil {
		o.Warn(fmt.Sprintf(format, a...))
	}
}
```

Create `internal/graph/subgraph.go`:

```go
package graph

import "fmt"

// Subgraph returns the part of g reachable from every node named name (all
// versions of it become roots, sorted by ID). Nodes are shared with g. It
// returns an error when no node has that name.
func Subgraph(g *DepGraph, name string) (*DepGraph, error) {
	matches := g.FindNodeByName(name)
	if len(matches) == 0 {
		return nil, fmt.Errorf("package %q not found in the dependency graph", name)
	}
	starts := make([]string, 0, len(matches))
	for _, n := range matches {
		starts = append(starts, n.ID)
	}
	reachable := g.reachableFrom(starts)

	sub := NewGraph()
	for _, id := range sortedKeys(reachable) {
		if node := g.Nodes[id]; node != nil {
			sub.AddNode(node)
		}
	}
	for _, e := range g.Edges {
		if reachable[e.From] {
			sub.AddEdge(e)
		}
	}
	for _, id := range starts {
		sub.AddRoot(id)
	}
	return sub, nil
}
```

In `internal/graph/extract_all.go`, delete everything from the line `// ExtractForPackage extracts dependencies for a specific package.` to the end of the file (that removes `ExtractForPackage` and `contains`). The file must end right after the closing brace of `DetectEcosystems`.

In `internal/cli/graph_cmd.go`, replace

```go
		depGraph, err = graph.ExtractForPackage(dir, packageArg)
```

with

```go
		depGraph, err = graph.ExtractAll(dir)
		if err == nil {
			depGraph, err = graph.Subgraph(depGraph, packageArg)
		}
```

(Task 7 rewrites this file; this keeps the build green.)

- [ ] **Step 4: Run the tests and the full gate**

Run: `go test ./internal/graph/ -v -run 'NodeID|AddEdge|Children|Transitive|StructLiteral|AddRoot|Merge|Trim|Normalize|Dedup|Subgraph|ExtractOptions'`
Expected: every listed test `--- PASS`, then `ok  	github.com/crenspire/xpm/internal/graph`.

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: all packages `ok` (or `no test files`), `gofmt -l` prints nothing, lint prints `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/graph/node.go internal/graph/graph.go internal/graph/normalize.go internal/graph/options.go internal/graph/subgraph.go internal/graph/extract_all.go internal/graph/main_test.go internal/graph/options_test.go internal/graph/graph_test.go internal/graph/normalize_test.go internal/graph/subgraph_test.go internal/cli/graph_cmd.go
git commit -m "graph: adjacency index, verbatim node IDs, re-keying normalization, Subgraph, ExtractOptions"
```

---

### Task 2: Exporters — depth-aware tree with `(*)`, escaped deterministic DOT, SVG through `dot` stdin, valid JSON, warnings to a writer

**Files:**
- Replace: `internal/graph/export_tree.go`, `internal/graph/export_dot.go`, `internal/graph/export_svg.go`, `internal/graph/export_json.go`, `internal/graph/warnings.go`
- Modify: `internal/graph/main_test.go` (add the `dot` and `dot-fail` helper modes)
- Modify: `internal/cli/graph_cmd.go`, `internal/cli/workspace_cmd.go` (call sites of the changed `PrintTree`/removed `GenerateSVG`; Task 7 rewrites them)
- Test: `internal/graph/export_tree_test.go`, `internal/graph/export_test.go`, `internal/graph/perf_test.go`

**Interfaces:**
- Consumes (Task 1): `DepGraph.Children`, `GetParents`, `sortedKeys`, `SortEdges`, `NormalizeGraph`, `helperEnv`/`helperMain`/`TestMain`.
- Produces:
  - `type TreeOptions struct { ShowVersions, ShowEcosystem bool; MaxDepth int }` (MaxDepth = levels below the roots, 0 = unlimited)
  - `func PrintTree(g *DepGraph, w io.Writer, opts TreeOptions)`
  - `func ToDOT(g *DepGraph) string`, `func WriteDOT(g *DepGraph, w io.Writer) error`, `func dotEscape(s string) string`
  - `var ErrGraphVizNotFound error`; `var dotCommand func() (*exec.Cmd, error)` (seam); `func WriteSVG(g *DepGraph, w io.Writer) error`
  - `func ToJSON(g *DepGraph) ([]byte, error)`, `func WriteJSON(g *DepGraph, w io.Writer) error` (trailing newline; empty lists are `[]`)
  - `func DetectWarnings(g *DepGraph) []Warning` (sorted by type, package, message), `func PrintWarnings(ws []Warning, w io.Writer)`
  - Removed: `GenerateSVG`, `CheckGraphViz`, `PrintTreeForPackage` (no callers after this task; `Subgraph` + `PrintTree` replaces the latter).
  - Test helpers: `treeGraph(roots []string, edges ...string) *DepGraph`, `fakeDot(t, mode)`, `syntheticGraph()`.

Design notes:
- Tree: roots print without a connector, children with `├─ `/`└─ `, grandchildren under `│  `/`   ` (fixes the old bug where first-level children were printed as roots because `prefix == ""`). A node whose children were already printed is shown again with ` (*)` and not expanded; a node that is its own ancestor gets ` (cycle)`. Leaves never get `(*)` (nothing was elided), and a node cut off by `MaxDepth` is not "expanded", so it is expanded in full where it appears higher up. Every expanded node prints each of its edges once, so output is ≤ roots + E lines. No roots → parentless nodes, sorted.
- DOT: nodes sorted by ID, edges by (From, To); `\` → `\\`, `"` → `\"`, CR/LF → `\n` in IDs and labels; label lines joined by the DOT `\n` escape after escaping each part.
- SVG: `dotCommand()` does `exec.LookPath("dot")` (→ `ErrGraphVizNotFound`) and returns `dot -Tsvg`; `WriteSVG` sets stdin to `ToDOT(g)`, stdout to `w`, captures stderr into the error. No temp files. Tests re-exec the test binary as a fake `dot` (works on Windows).
- The perf budget test lives here (it measures graph core + exporters; the CLI adds only flag parsing). 1001 nodes / 4600 edges, every node shared by ~5 parents; measured ~9 ms/op on an M1 Pro.

- [ ] **Step 1: Write the failing tests**

In `internal/graph/main_test.go`, replace the whole file with:

```go
package graph

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// helperEnv makes the test binary act as a fake external command (see
// helperMain) instead of running the tests. This works on every OS, unlike
// shell-script fakes.
const helperEnv = "XPM_GRAPH_TEST_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(helperMain(mode))
	}
	os.Exit(m.Run())
}

// helperMain implements the fake commands:
//   - "pwd": print the working directory to stdout and noise to stderr.
//   - "dot": a fake `dot -Tsvg`; wraps stdin in <svg>...</svg> on stdout.
//   - "dot-fail": print a dot-style syntax error to stderr and exit 1.
func helperMain(mode string) int {
	switch mode {
	case "dot":
		in, err := io.ReadAll(os.Stdin)
		if err != nil || !strings.HasPrefix(string(in), "digraph ") {
			fmt.Fprintf(os.Stderr, "fake dot: bad input %q\n", in)
			return 1
		}
		fmt.Printf("<svg args=%q>\n%s</svg>\n", strings.Join(os.Args[1:], " "), in)
		return 0
	case "dot-fail":
		_, _ = io.ReadAll(os.Stdin)
		fmt.Fprintln(os.Stderr, "Error: <stdin>: syntax error in line 1 near 'x'")
		return 1
	case "pwd":
		wd, err := os.Getwd()
		if err != nil {
			return 3
		}
		fmt.Print(wd)
		fmt.Fprint(os.Stderr, "stderr noise")
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
	return 2
}
```

Create `internal/graph/export_tree_test.go`:

```go
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
```

Create `internal/graph/export_test.go`:

```go
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
```

Create `internal/graph/perf_test.go`:

```go
package graph

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// syntheticGraph builds a 1001-node graph: one root over 10 layers of 100
// nodes, every node depending on 5 nodes of the next layer (heavy diamond
// sharing; 100 + 9*100*5 = 4600 edges). Versions carry a "v" prefix so
// NormalizeGraph has to re-key every node and edge.
func syntheticGraph() *DepGraph {
	const layers, width, fanout = 10, 100, 5
	g := NewGraph()
	id := func(layer, i int) string { return NodeID("node", fmt.Sprintf("pkg-%d-%d", layer, i), "v1.0.0") }
	root := NewDepNode("node", "app", "v1.0.0")
	g.AddNode(root)
	g.AddRoot(root.ID)
	for l := 0; l < layers; l++ {
		for i := 0; i < width; i++ {
			g.AddNode(NewDepNode("node", fmt.Sprintf("pkg-%d-%d", l, i), "v1.0.0"))
		}
	}
	for i := 0; i < width; i++ {
		g.AddEdge(NewEdge(root.ID, id(0, i)))
	}
	for l := 0; l+1 < layers; l++ {
		for i := 0; i < width; i++ {
			for k := 0; k < fanout; k++ {
				g.AddEdge(NewEdge(id(l, i), id(l+1, (i*7+k*13)%width)))
			}
		}
	}
	return g
}

// graphPipeline is what `xpm graph` does after extraction.
func graphPipeline(w io.Writer) (*DepGraph, error) {
	g := syntheticGraph()
	NormalizeGraph(g)
	PrintTree(g, w, TreeOptions{ShowVersions: true, ShowEcosystem: true})
	return g, WriteJSON(g, w)
}

func TestSyntheticGraphTreeIsLinearInEdges(t *testing.T) {
	g := syntheticGraph()
	NormalizeGraph(g)
	if g.NodeCount() != 1001 || g.EdgeCount() != 4600 {
		t.Fatalf("synthetic graph: %d nodes, %d edges", g.NodeCount(), g.EdgeCount())
	}
	var sb strings.Builder
	PrintTree(g, &sb, TreeOptions{ShowVersions: true})
	// Every node is expanded once, so each edge prints exactly one line.
	if lines := strings.Count(sb.String(), "\n"); lines != 1+4600 {
		t.Errorf("tree has %d lines, want %d", lines, 1+4600)
	}
}

func TestGraphPipelineBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	start := time.Now()
	if _, err := graphPipeline(io.Discard); err != nil {
		t.Fatal(err)
	}
	// The budget is 200 ms; allow 10x for slow, shared CI runners and -race.
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("build+normalize+tree+JSON took %v, budget 200ms", d)
	}
}

func BenchmarkGraphPipeline(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := graphPipeline(io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/graph/`
Expected: FAIL (build failed) with errors including `undefined: TreeOptions`, `undefined: dotEscape`, `undefined: dotCommand`, `undefined: ErrGraphVizNotFound`.

- [ ] **Step 3: Implement**

Replace `internal/graph/export_tree.go` with:

```go
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
// children sorted by ID. A node whose subtree was already printed is shown
// once more with " (*)" and not expanded again, so shared dependencies
// (diamonds) print in O(nodes + edges) lines; a dependency that leads back to
// one of its ancestors is shown with " (cycle)". If the graph has no roots,
// nodes without parents are used as roots.
func PrintTree(g *DepGraph, w io.Writer, opts TreeOptions) {
	p := treePrinter{g: g, opts: opts, expanded: map[string]bool{}, onPath: map[string]bool{}}
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
	expanded map[string]bool // children already printed somewhere above
	onPath   map[string]bool // ancestors of the node being printed
}

// visit prints id as "<prefix><connector><label>" and then its children
// with prefix childPrefix-extended; depth is 0 for roots.
func (p *treePrinter) visit(id, prefix, connector string, depth int) {
	children := p.children(id)
	marker := ""
	switch {
	case p.onPath[id]:
		marker = treeCycleMarker
	case p.expanded[id] && len(children) > 0:
		marker = treeSeenMarker
	}
	p.sb.WriteString(prefix)
	p.sb.WriteString(connector)
	p.sb.WriteString(formatNodeLabel(p.g.Nodes[id], p.opts.ShowVersions, p.opts.ShowEcosystem))
	p.sb.WriteString(marker)
	p.sb.WriteByte('\n')
	if marker != "" || (p.opts.MaxDepth > 0 && depth >= p.opts.MaxDepth) {
		return
	}

	p.expanded[id] = true
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
```

Replace `internal/graph/export_dot.go` with:

```go
package graph

import (
	"fmt"
	"io"
	"strings"
)

// ToDOT converts the graph to GraphViz DOT format. Nodes are sorted by ID and
// edges by (From, To); IDs and labels are escaped for DOT quoted strings.
func ToDOT(graph *DepGraph) string {
	var sb strings.Builder

	sb.WriteString("digraph dependencies {\n")
	sb.WriteString("  rankdir=LR;\n")
	sb.WriteString("  node [shape=box, style=rounded];\n\n")

	for _, id := range sortedKeys(graph.Nodes) {
		node := graph.Nodes[id]
		fmt.Fprintf(&sb, "  \"%s\" [label=\"%s\", fillcolor=\"%s\", style=\"rounded,filled\"];\n",
			dotEscape(id), formatNodeLabelForDOT(node), ColorByEcosystem(node.Ecosystem))
	}

	sb.WriteString("\n")

	edges := append([]*DepEdge(nil), graph.Edges...)
	SortEdges(edges)
	for _, edge := range edges {
		style := "solid"
		if edge.Type == "transitive" {
			style = "dashed"
		}
		fmt.Fprintf(&sb, "  \"%s\" -> \"%s\" [style=%s];\n", dotEscape(edge.From), dotEscape(edge.To), style)
	}

	sb.WriteString("}\n")
	return sb.String()
}

// WriteDOT writes the graph in DOT format to the writer.
func WriteDOT(graph *DepGraph, w io.Writer) error {
	if _, err := io.WriteString(w, ToDOT(graph)); err != nil {
		return fmt.Errorf("write DOT: %w", err)
	}
	return nil
}

// dotEscape escapes s for use inside a DOT double-quoted string: backslashes
// and quotes are escaped, and line breaks become the DOT "\n" escape.
func dotEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`)
	return r.Replace(s)
}

// formatNodeLabelForDOT returns the escaped three-line DOT label
// "name\nversion\n[ecosystem]" (the version line only if set).
func formatNodeLabelForDOT(node *DepNode) string {
	label := dotEscape(node.Name)
	if node.Version != "" {
		label += `\n` + dotEscape(node.Version)
	}
	return label + `\n[` + dotEscape(node.Ecosystem) + `]`
}

// ColorByEcosystem returns a color for an ecosystem.
func ColorByEcosystem(ecosystem string) string {
	colors := map[string]string{
		"node":   "#339933",
		"python": "#3776ab",
		"php":    "#777bb4",
		"rust":   "#000000",
		"go":     "#00add8",
		"java":   "#ed8b00",
	}
	if color, ok := colors[ecosystem]; ok {
		return color
	}
	return "#666666"
}
```

Replace `internal/graph/export_svg.go` with:

```go
package graph

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ErrGraphVizNotFound is returned by WriteSVG when GraphViz's dot is not on PATH.
var ErrGraphVizNotFound = errors.New("GraphViz is not installed (no `dot` in PATH); install it from https://graphviz.org/download/ to use --svg")

// dotCommand returns the command that turns DOT on stdin into SVG on stdout.
// Tests replace it with a fake.
var dotCommand = func() (*exec.Cmd, error) {
	path, err := exec.LookPath("dot")
	if err != nil {
		return nil, ErrGraphVizNotFound
	}
	return exec.Command(path, "-Tsvg"), nil
}

// WriteSVG renders the graph as SVG by piping its DOT form into `dot -Tsvg`
// and streaming dot's stdout to w. dot's stderr is included in the error.
func WriteSVG(graph *DepGraph, w io.Writer) error {
	cmd, err := dotCommand()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stdin = strings.NewReader(ToDOT(graph))
	cmd.Stdout = w
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("dot -Tsvg failed: %w: %s", err, msg)
		}
		return fmt.Errorf("dot -Tsvg failed: %w", err)
	}
	return nil
}
```

Replace `internal/graph/export_json.go` with:

```go
package graph

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSONGraph represents the graph in JSON format.
type JSONGraph struct {
	Nodes []JSONNode `json:"nodes"`
	Edges []JSONEdge `json:"edges"`
	Roots []string   `json:"roots"`
}

// JSONNode represents a node in JSON format.
type JSONNode struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Ecosystem string            `json:"ecosystem"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// JSONEdge represents an edge in JSON format.
type JSONEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// ToJSON converts the graph to indented JSON: nodes sorted by ID, edges by
// (From, To), roots in graph order. Empty lists are [] (never null).
func ToJSON(graph *DepGraph) ([]byte, error) {
	jsonGraph := JSONGraph{
		Nodes: make([]JSONNode, 0, len(graph.Nodes)),
		Edges: make([]JSONEdge, 0, len(graph.Edges)),
		Roots: append([]string{}, graph.Root...),
	}

	for _, id := range sortedKeys(graph.Nodes) {
		node := graph.Nodes[id]
		jsonNode := JSONNode{
			ID:        node.ID,
			Name:      node.Name,
			Version:   node.Version,
			Ecosystem: node.Ecosystem,
			Metadata:  node.Metadata,
		}
		if len(jsonNode.Metadata) == 0 {
			jsonNode.Metadata = nil
		}
		jsonGraph.Nodes = append(jsonGraph.Nodes, jsonNode)
	}

	edges := append([]*DepEdge(nil), graph.Edges...)
	SortEdges(edges)
	for _, edge := range edges {
		jsonGraph.Edges = append(jsonGraph.Edges, JSONEdge{
			From: edge.From,
			To:   edge.To,
			Type: edge.Type,
		})
	}

	return json.MarshalIndent(jsonGraph, "", "  ")
}

// WriteJSON writes the graph as JSON, followed by a newline, to the writer.
func WriteJSON(graph *DepGraph, w io.Writer) error {
	data, err := ToJSON(graph)
	if err != nil {
		return fmt.Errorf("failed to marshal graph: %w", err)
	}

	_, err = w.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	return nil
}
```

Replace `internal/graph/warnings.go` with:

```go
package graph

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Warning represents a dependency warning.
type Warning struct {
	Type    string
	Message string
	Package string
	Details []string
}

// DetectWarnings detects various issues in the dependency graph. The result
// is sorted by type, then package, then message.
func DetectWarnings(graph *DepGraph) []Warning {
	var warnings []Warning

	warnings = append(warnings, DetectVersionConflicts(graph)...)
	warnings = append(warnings, DetectEcosystemConflicts(graph)...)
	warnings = append(warnings, DetectMissingDependencies(graph)...)

	sort.SliceStable(warnings, func(i, j int) bool {
		a, b := warnings[i], warnings[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		return a.Message < b.Message
	})
	return warnings
}

// DetectVersionConflicts finds multiple versions of the same package.
func DetectVersionConflicts(graph *DepGraph) []Warning {
	var warnings []Warning

	// Group nodes by ecosystem and name
	byEcosystemAndName := make(map[string]map[string][]*DepNode)
	for _, node := range graph.Nodes {
		key := node.Ecosystem
		if byEcosystemAndName[key] == nil {
			byEcosystemAndName[key] = make(map[string][]*DepNode)
		}
		byEcosystemAndName[key][node.Name] = append(byEcosystemAndName[key][node.Name], node)
	}

	// Check for multiple versions
	for ecosystem, byName := range byEcosystemAndName {
		for name, nodes := range byName {
			if len(nodes) > 1 {
				versions := make([]string, len(nodes))
				for i, node := range nodes {
					versions[i] = node.Version
				}
				sort.Strings(versions)

				warnings = append(warnings, Warning{
					Type:    "version_conflict",
					Message: fmt.Sprintf("Multiple versions of %s detected in %s", name, ecosystem),
					Package: name,
					Details: versions,
				})
			}
		}
	}

	return warnings
}

// DetectEcosystemConflicts finds the same package in different ecosystems.
func DetectEcosystemConflicts(graph *DepGraph) []Warning {
	var warnings []Warning

	// Group nodes by name
	byName := make(map[string][]*DepNode)
	for _, node := range graph.Nodes {
		byName[node.Name] = append(byName[node.Name], node)
	}

	// Check for same name in different ecosystems
	for name, nodes := range byName {
		if len(nodes) <= 1 {
			continue
		}

		ecosystems := make(map[string]bool)
		for _, node := range nodes {
			ecosystems[node.Ecosystem] = true
		}

		if len(ecosystems) > 1 {
			ecoList := make([]string, 0, len(ecosystems))
			for eco := range ecosystems {
				ecoList = append(ecoList, eco)
			}
			sort.Strings(ecoList)

			warnings = append(warnings, Warning{
				Type:    "ecosystem_conflict",
				Message: fmt.Sprintf("Package %s found in multiple ecosystems", name),
				Package: name,
				Details: ecoList,
			})
		}
	}

	return warnings
}

// DetectMissingDependencies finds potential missing dependencies.
func DetectMissingDependencies(graph *DepGraph) []Warning {
	var warnings []Warning

	// Check for edges pointing to non-existent nodes
	for _, edge := range graph.Edges {
		if graph.GetNode(edge.To) == nil {
			warnings = append(warnings, Warning{
				Type:    "missing_dependency",
				Message: fmt.Sprintf("Dependency %s referenced but not found", edge.To),
				Package: edge.From,
				Details: []string{edge.To},
			})
		}
		if graph.GetNode(edge.From) == nil {
			warnings = append(warnings, Warning{
				Type:    "missing_dependency",
				Message: fmt.Sprintf("Source node %s not found", edge.From),
				Package: edge.To,
				Details: []string{edge.From},
			})
		}
	}

	return warnings
}

// PrintWarnings prints warnings to w (the CLI passes os.Stderr).
func PrintWarnings(warnings []Warning, w io.Writer) {
	if len(warnings) == 0 {
		return
	}
	var sb strings.Builder
	sb.WriteString("\n⚠ Warnings:\n\n")
	for _, warning := range warnings {
		fmt.Fprintf(&sb, "  ⚠ %s\n", warning.Message)
		if warning.Package != "" {
			fmt.Fprintf(&sb, "     Package: %s\n", warning.Package)
		}
		if len(warning.Details) > 0 {
			fmt.Fprintf(&sb, "     Details: %v\n", warning.Details)
		}
		sb.WriteString("\n")
	}
	_, _ = io.WriteString(w, sb.String())
}
```

Adapt the two CLI call sites (Task 7 rewrites both; this keeps the build green). In **both** `internal/cli/graph_cmd.go` (graph variable `depGraph`) and `internal/cli/workspace_cmd.go` (graph variable `merged`), with `<g>` standing for that variable:

1. Delete the line `		fmt.Println() // Add newline after JSON` (WriteJSON now ends with a newline).
2. Replace

```go
		outputPath := "graph.svg"
		if err := graph.GenerateSVG(graph.ToDOT(<g>), outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		fmt.Printf("SVG graph written to %s\n", outputPath)
```

with

```go
		if err := graph.WriteSVG(<g>, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
```

3. Replace `graph.PrintTree(<g>, os.Stdout, cfg.Graph.ShowVersions, cfg.Graph.ShowEcosystem, maxDepth)` with

```go
graph.PrintTree(<g>, os.Stdout, graph.TreeOptions{ShowVersions: cfg.Graph.ShowVersions, ShowEcosystem: cfg.Graph.ShowEcosystem, MaxDepth: maxDepth})
```

- [ ] **Step 4: Run the tests, the benchmark and the full gate**

Run: `go test ./internal/graph/ -v -run 'PrintTree|DOT|Dot|JSON|SVG|Warnings|Synthetic|Budget'`
Expected: `--- PASS` for `TestPrintTreeGolden` (8 subtests), `TestToDOTEscapesAndSorts`, `TestDotEscape`, `TestToJSONIsValidAndDeterministic`, `TestWriteJSONEmptyGraph`, `TestWriteSVGPipesDOTThroughDot`, `TestWriteSVGReportsDotStderr`, `TestWriteSVGWithoutGraphViz`, `TestDetectWarningsSortedAndPrintedToWriter`, `TestSyntheticGraphTreeIsLinearInEdges`, `TestGraphPipelineBudget`.

Run: `go test -run '^$' -bench GraphPipeline -benchmem ./internal/graph/`
Expected: one `BenchmarkGraphPipeline` line well under `200000000 ns/op` (≈ 10 ms/op on a laptop).

Run: `go test -race ./internal/graph/`
Expected: `ok`.

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: all `ok`, no gofmt output, `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/graph/export_tree.go internal/graph/export_dot.go internal/graph/export_svg.go internal/graph/export_json.go internal/graph/warnings.go internal/graph/main_test.go internal/graph/export_tree_test.go internal/graph/export_test.go internal/graph/perf_test.go internal/cli/graph_cmd.go internal/cli/workspace_cmd.go
git commit -m "graph: tree marks repeated subtrees, escaped DOT, SVG via dot stdin, deterministic JSON"
```

---

### Task 7: `xpm graph` — clean stdout, `--exec`, `--svg` streaming, `--depth` validation, workspace graph

**Files:**
- Replace: `internal/cli/graph_cmd.go`
- Modify: `internal/cli/workspace_cmd.go` (move `cmdGraphWorkspace` out: delete it and the imports only it used)
- Test: `internal/cli/graph_cmd_test.go`

**Interfaces:**
- Consumes:
  - Task 6: `func graph.ExtractAll(dir string, opts graph.ExtractOptions) (*graph.DepGraph, error)` (final signature; it must call `opts.Run` only when `opts.Exec` is true, and report non-fatal problems through `opts.Warn`).
  - Task 1: `graph.ExtractOptions`, `graph.NewGraph`, `graph.NormalizeGraph`, `graph.Subgraph`, `(*DepGraph).Merge`.
  - Task 2: `graph.PrintTree`, `graph.TreeOptions`, `graph.WriteJSON`, `graph.WriteSVG`, `graph.ErrGraphVizNotFound`, `graph.JSONGraph`, `graph.DetectWarnings`, `graph.PrintWarnings`.
  - Tasks 3 and 5 (through `ExtractAll`): an npm v3 `package-lock.json` yields the project node `demo@1.0.0` as root with edges to its direct deps (from the `""` entry), nested `node_modules/debug/node_modules/ms` resolved for `debug`; a `go.mod` without `--exec` yields its `require`d module paths.
  - Task 12: `workspace.DetectWorkspaces(root)` (unchanged signature) still detects npm `"workspaces": ["packages/*"]`.
  - Existing test helpers in package `cli`: `captureStdout`, `captureStderr`, `withConfig` (`helpers_test.go`), `chdir` (`startup_test.go`).
- Produces:
  - `func cmdGraph(args []string) int` (same signature; `cli.go` unchanged).
  - Seams: `var extractGraph = graph.ExtractAll`, `var writeSVG = graph.WriteSVG`, `var graphRunner func(dir, name string, args ...string) ([]byte, error)` (nil = real commands).
  - `type graphArgs struct { JSON, SVG, Exec, Workspace bool; Depth int; Package string }`, `func parseGraphArgs(args []string, defaultDepth int) (graphArgs, error)`, `func workspaceGraph(root string, opts graph.ExtractOptions) (*graph.DepGraph, error)`, `func renderGraph(g *graph.DepGraph, a graphArgs) int`.
  - Removed: `cmdGraphWorkspace` (folded into `cmdGraph` via `workspaceGraph`).

Behavior:
- Flags anywhere (`xpm graph react --json`, as the man page shows): `--json`, `--svg`, `--depth N` (default `cfg.Graph.Depth`, 0 = unlimited), `--exec`, `--workspace`/`-w`, optional package name. `--` ends flags.
- Usage errors exit **2** with `error: …` on stderr and nothing on stdout: unknown flag, non-integer or negative `--depth`, `--json` with `--svg`, more than one package. `-h` exits 0.
- Order: extract (or workspace merge) → `NormalizeGraph` → `Subgraph` if a package was named (unknown package → exit 1) → render.
- stdout carries only the tree / JSON / SVG. Warnings (`graph.PrintWarnings`), extractor warnings (`warning: …`), `No dependencies found.` and errors go to stderr.
- Empty graph: tree mode prints nothing on stdout and exits 0; `--json` prints `{"nodes": [], "edges": [], "roots": []}` (indented) and `--svg` renders the empty digraph, so `xpm graph --json > g.json` is always valid JSON.
- `--svg` streams SVG to stdout; no `graph.svg` is written. Missing GraphViz → `error: GraphViz is not installed …` on stderr, exit 1.
- Config comes from the package-level `cfg` (loaded by `Run` in `cli.go`), not `config.Load()`.
- Workspace graph: extracts each workspace root and each project directory once (a single root lockfile — npm/pnpm/Cargo workspaces — is picked up), in detection order, merges them; a directory that fails extraction becomes a `warning: skipping <dir>: <err>`; no workspaces → `error: no workspaces detected`, exit 1.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/graph_cmd_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run Graph`
Expected: FAIL (build failed) with `undefined: graphRunner`, `undefined: extractGraph`, `undefined: writeSVG`.

- [ ] **Step 3: Implement**

Replace `internal/cli/graph_cmd.go` with:

```go
package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/crenspire/xpm/internal/graph"
	"github.com/crenspire/xpm/internal/workspace"
)

// Seams for tests.
var (
	// extractGraph extracts one project's dependency graph.
	extractGraph = graph.ExtractAll
	// writeSVG renders a graph as SVG through GraphViz.
	writeSVG = graph.WriteSVG
	// graphRunner runs build tools for --exec; nil runs the real commands.
	graphRunner func(dir, name string, args ...string) ([]byte, error)
)

// graphArgs is `xpm graph`'s command line.
type graphArgs struct {
	JSON, SVG, Exec, Workspace bool
	Depth                      int
	Package                    string
}

// parseGraphArgs parses flags anywhere on the line (`xpm graph react --json`).
// The depth default comes from the config. Errors are usage errors (exit 2).
func parseGraphArgs(args []string, defaultDepth int) (graphArgs, error) {
	var a graphArgs
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&a.JSON, "json", false, "print the graph as JSON")
	fs.BoolVar(&a.SVG, "svg", false, "print the graph as SVG (requires GraphViz)")
	fs.IntVar(&a.Depth, "depth", defaultDepth, "tree depth below the roots (0 = unlimited)")
	fs.BoolVar(&a.Exec, "exec", false, "run mvn/gradle/go to resolve full trees")
	fs.BoolVar(&a.Workspace, "workspace", false, "combine the graphs of all workspace projects")
	fs.BoolVar(&a.Workspace, "w", false, "shorthand for --workspace")

	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return graphArgs{}, err
		}
		rest := fs.Args()
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			positional = append(positional, rest...)
			break
		}
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}

	switch {
	case a.Depth < 0:
		return graphArgs{}, fmt.Errorf("--depth must be 0 (unlimited) or a positive number, got %d", a.Depth)
	case a.JSON && a.SVG:
		return graphArgs{}, errors.New("--json and --svg cannot be combined")
	case len(positional) > 1:
		return graphArgs{}, fmt.Errorf("graph takes at most one package name, got %d", len(positional))
	case len(positional) == 1:
		a.Package = positional[0]
	}
	return a, nil
}

// cmdGraph prints the project's dependency graph. stdout carries only the
// requested output (tree, JSON or SVG); warnings and status go to stderr.
// With --json or --svg, stdout is always a complete document, even for an
// empty graph.
func cmdGraph(args []string) int {
	a, err := parseGraphArgs(args, cfg.Graph.Depth)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	opts := graph.ExtractOptions{
		Exec: a.Exec,
		Run:  graphRunner,
		Warn: func(msg string) { fmt.Fprintf(os.Stderr, "warning: %s\n", msg) },
	}

	var g *graph.DepGraph
	if a.Workspace {
		g, err = workspaceGraph(cwd, opts)
	} else {
		g, err = extractGraph(cwd, opts)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if g == nil {
		g = graph.NewGraph()
	}
	graph.NormalizeGraph(g)

	if a.Package != "" {
		if g, err = graph.Subgraph(g, a.Package); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	return renderGraph(g, a)
}

// workspaceGraph merges the graphs of every workspace root and project
// under root (each directory once, in detection order). A project that fails
// to extract is reported as a warning and skipped.
func workspaceGraph(root string, opts graph.ExtractOptions) (*graph.DepGraph, error) {
	workspaces, err := workspace.DetectWorkspaces(root)
	if err != nil {
		return nil, fmt.Errorf("detecting workspaces: %w", err)
	}
	if len(workspaces) == 0 {
		return nil, errors.New("no workspaces detected")
	}
	var dirs []string
	seen := map[string]bool{}
	add := func(dir string) {
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	for _, ws := range workspaces {
		add(ws.Root)
		for _, p := range ws.Projects {
			add(p.Path)
		}
	}

	merged := graph.NewGraph()
	for _, dir := range dirs {
		g, err := extractGraph(dir, opts)
		if err != nil {
			opts.Warn(fmt.Sprintf("skipping %s: %v", dir, err))
			continue
		}
		merged.Merge(g)
	}
	return merged, nil
}

// renderGraph writes g to stdout in the requested format and its warnings
// to stderr.
func renderGraph(g *graph.DepGraph, a graphArgs) int {
	if len(g.Nodes) == 0 {
		fmt.Fprintln(os.Stderr, "No dependencies found.")
		if !a.JSON && !a.SVG {
			return 0
		}
	}

	switch {
	case a.JSON:
		if err := graph.WriteJSON(g, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	case a.SVG:
		if err := writeSVG(g, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	default:
		graph.PrintTree(g, os.Stdout, graph.TreeOptions{
			ShowVersions:  cfg.Graph.ShowVersions,
			ShowEcosystem: cfg.Graph.ShowEcosystem,
			MaxDepth:      a.Depth,
		})
	}

	graph.PrintWarnings(graph.DetectWarnings(g), os.Stderr)
	return 0
}
```

In `internal/cli/workspace_cmd.go`, delete `cmdGraphWorkspace` — everything from the line `// cmdGraphWorkspace generates a combined dependency graph for all workspace projects.` to the end of the file — and replace the import block with:

```go
import (
	"fmt"
	"os"

	"github.com/crenspire/xpm/internal/workspace"
)
```

- [ ] **Step 4: Run the tests and the full gate**

Run: `go test ./internal/cli/ -v -run Graph`
Expected: `--- PASS` for `TestGraphTreeToStdoutWarningsToStderr`, `TestGraphJSONStdoutIsValidJSON`, `TestGraphEmptyProject`, `TestGraphSVGStreamsToStdout`, `TestGraphSVGWithoutGraphViz`, `TestGraphUsageErrorsExit2`, `TestGraphDepthAndPackageArgument`, `TestGraphPackageNotFound`, `TestGraphPassesExecAndWarnings`, `TestGraphRunsNoToolsWithoutExec`, `TestGraphWorkspaceMergesProjects`, `TestGraphWorkspaceNoneDetected`.

Run: `go build ./... && go vet ./... && go test ./... && go test -race ./internal/cli/ ./internal/graph/ && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: all `ok`, no gofmt output, `0 issues.`

Manual smoke (optional, any project with a `package-lock.json`):
`go run ./cmd/xpm graph --json > /tmp/g.json && python3 -m json.tool /tmp/g.json > /dev/null && echo valid` → `valid`;
`go run ./cmd/xpm graph --depth -1; echo $?` → `error: --depth must be 0 (unlimited) or a positive number, got -1` then `2`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/graph_cmd.go internal/cli/graph_cmd_test.go internal/cli/workspace_cmd.go
git commit -m "graph: clean stdout, --exec gate, streamed --svg, validated --depth, workspace graph in graph_cmd"
```

---

## Rulings (section A)

- Ruling: `--depth 0` means unlimited and N counts levels below the roots (`--depth 1` = roots + direct deps) — matches the config doc (`graph.depth`, "0 = unlimited") and the header (`MaxDepth 0 = unlimited`) — Cost if wrong: users expecting npm's `--depth=0` (direct deps only) get the full tree.
- Ruling: empty graph → tree mode prints nothing on stdout (`No dependencies found.` on stderr, exit 0); `--json` prints `{"nodes": [], "edges": [], "roots": []}`; `--svg` renders the empty digraph — so redirected output is always a valid document — Cost if wrong: scripts that tested for "No dependencies found." on stdout.
- Ruling: usage errors (unknown flag, bad/negative `--depth`, `--json`+`--svg`, two package names) exit 2; runtime errors (extraction, unknown package, missing GraphViz) exit 1; flags are accepted after the package name — Cost if wrong: one exit-code change.
- Ruling: `(*)` marks only nodes whose children were elided; repeated leaves print plainly; back-edges print ` (cycle)`; a graph with no roots uses its parentless nodes — Cost if wrong: cosmetic golden-test churn.
- Ruling: `DepGraph.Edges` stays exported and canonical; the index is unexported and rebuilt by `reindex()`/`ensureIndex()` (detects missing index and length changes, not in-place endpoint edits). Tasks 3–6 must add edges with `AddEdge` and never edit `From`/`To` in place — Cost if wrong: a parser that mutates edges in place gets stale `Children` until the next `reindex`.
- Ruling: `AddEdge` keeps the first edge for a (From, To) pair (its `Type` wins); `AddNode` still replaces; `Merge` adds nodes in sorted-ID order — Cost if wrong: a later "direct" edge does not upgrade an earlier "transitive" one.
- Ruling: `normalizeVersion` strips `v` only before a digit (old code turned `very-new` into `ery-new`) — Cost if wrong: none known.
- Ruling: the perf budget test/benchmark live in Task 2 (`internal/graph/perf_test.go`), timed with a 2 s bound (10× the 200 ms budget for -race/shared CI) and skipped under `-short`; the exact line-count test (1 + E lines) runs always — Cost if wrong: a 10× regression could slip through CI timing, but the line-count test still catches exponential output.
- Ruling: the SVG seam is `graph.dotCommand` (unexported, helper-process fake in graph tests); the CLI has its own seams (`extractGraph`, `writeSVG`, `graphRunner`) rather than an exported graph seam — Cost if wrong: none.
- Ruling: `internal/graph/main_test.go` (Task 1) owns `TestMain` for package `graph`, re-executing the test binary as fake commands (`XPM_GRAPH_TEST_HELPER=pwd|dot|dot-fail`); Tasks 3–6 add modes there instead of another `TestMain` — Cost if wrong: a duplicate `TestMain` compile error at merge.
- Ruling: `xpm graph -w` also extracts each workspace root (not only member projects) and dedupes directories — npm/pnpm/Cargo workspaces keep one lockfile at the root — Cost if wrong: root-level deps appear in the workspace graph.
- Ruling: `GenerateSVG`, `CheckGraphViz`, `PrintTreeForPackage`, `ExtractForPackage` are deleted (no callers); `GetChildren` stays as an alias of `Children` — Cost if wrong: none (internal package).

## Conflicts / notes for the controller

- Task map lists "1k-node budget" under Task 7; this section implements it in Task 2 (graph-level). Proposed fix: change the Task 7 row to drop "1k-node budget" and add it to Task 2's row.
- **Task 6 must keep the build green when it changes `ExtractAll`'s signature**: it has to update the three existing callers, which at that point are `internal/cli/graph_cmd.go` (two `graph.ExtractAll(dir)` calls, after Task 1's edit) and `internal/cli/workspace_cmd.go` (`graph.ExtractAll(project.Path)` in `cmdGraphWorkspace`) — pass `graph.ExtractOptions{}`. Task 7 then replaces all three.
- Task 7's exact-output tests assume Task 3 makes the lockfile's own project (`demo@1.0.0`, from `name`/`version` and the `""` entry) the root with edges to its direct dependencies, and resolves `node_modules/debug/node_modules/ms` for `debug`. If Task 3 chooses different roots, `TestGraphTreeToStdoutWarningsToStderr`, `TestGraphJSONStdoutIsValidJSON` (4 nodes / 3 edges / 1 root) and `TestGraphDepthAndPackageArgument` need the same adjustment.
- `ExtractOptions.warn` is declared as `warn(format string, a ...any)` (dispatch spec); the header only fixes the struct — no conflict.

---

## Section C — Workspaces (Tasks 12–13)

### Review Focus

1. **A monorepo whose dependency directories hold manifests.** Examples: `node_modules/**/package.json`, `vendor/**/go.mod`, `vendor/**/composer.json`, `.venv/**/pyproject.toml`, `target/package/*/Cargo.toml`, `testdata/**/go.mod`. **Expected:** none of them is listed, installed or run. Walks and wildcards never enter `node_modules`, `vendor`, `venv`, `target`, `testdata`, `__pycache__` or any hidden directory. A pattern that names one of these directories literally still reaches it. **Pinned by:** `TestGoModulesFallbackSkipsVendorAndFixtures`, `TestNpmWorkspacesDoubleStarAndNegation`, `TestPythonSkipsVirtualenvsAndReadsUVWorkspace`, `TestComposerPathRepositoryGlob`, `TestCargoMembersExcludeAndRootPackage` and `TestExpandGlobsLiteralSkippedDirIsReachable`.
2. **`xpm install --workspace` / `cmdInstallWorkspace` on an npm, yarn, pnpm or bun workspace.** **Expected:** one `<pm> install` runs at the workspace root, never one per package. The manager comes from the root lock file in pm's fixed order (npm, yarn, pnpm, bun) or from `pnpm-workspace.yaml`, so the result is the same on every run. Cargo runs `cargo fetch` and Maven runs `mvn -q dependency:resolve`, each once at the root. Nothing is built. A missing tool fails only its own project; the other projects still install. **Pinned by:** `TestInstallCommandsAndDirs`, `TestInstallMissingToolFailsOnlyThatProject` and `TestCmdInstallWorkspaceInstallsNodeRootOnce`.
3. **`xpm run --workspace <task>` with `workspace.parallel: true`.** **Expected:** xpm re-executes itself as `<exe> run <task>` with `cmd.Dir` set to each project. It never calls `os.Chdir`. Each project's output is printed in one piece after a `[name] $ cmd` header. Projects without the task are skipped, with a note on stderr. The command fails if no project has the task, and the exit code is 1 if any project fails. **Pinned by:** `TestExecuteParallelDoesNotInterleave`, `TestRunReexecsPerProjectAndSkipsMissingTask`, `TestRunAggregatesFailures`, `TestNoChdirInWorkspacePackage` and `TestCmdRunWorkspaceFailureExitsNonZero`.

---

### Task 12: Workspace detection — skipped directories, go.work via modfile, `**`/`!` globs, include/exclude filter, fixed order

**Files:**
- Create: `internal/workspace/glob.go`, `internal/workspace/filter.go`
- Replace (whole file): `internal/workspace/types.go`, `internal/workspace/detect.go`, `internal/workspace/node.go`, `internal/workspace/go.go`, `internal/workspace/cargo.go`, `internal/workspace/python.go`, `internal/workspace/composer.go`, `internal/workspace/list.go`
- Modify: `internal/workspace/java.go` (one line: `RootPM` on the Maven workspace)
- Test (create): `internal/workspace/detect_test.go`, `internal/workspace/glob_test.go`, `internal/workspace/filter_test.go`
- Not touched: `install.go` and `run.go` (Task 13 replaces them; they still compile after this task), `internal/cli/*`, `go.mod`/`go.sum`.

**Interfaces:**
- Consumes:
  - `golang.org/x/mod/modfile` v0.23.0, added to go.mod by Task 5: `modfile.ParseWork(file string, data []byte, fix VersionFixer) (*WorkFile, error)`, `(*WorkFile).Use[i].Path` and `modfile.ModulePath(mod []byte) string`.
  - `pm.DetectLockFilesForEcosystem(dir string, eco pm.Ecosystem) []pm.ID` (order: package-lock.json, yarn.lock, pnpm-lock.yaml, bun.lock, bun.lockb) and `pm.MetaFor`. internal/pm itself is not edited.
- Produces (package `workspace`):
  - `func DetectWorkspaces(root string) ([]Workspace, error)`: **signature unchanged** (Task 7 consumes it). It now returns **one Workspace per ecosystem**, in the order node, python, rust, go, java, php. The old merged `"mixed"` workspace is gone. Projects are sorted by slash path relative to `Root` and de-duplicated by `Path`.
  - `type Workspace struct { Root string; Projects []Project; Ecosystem string; RootPM pm.ID }`. `RootPM` is new. It is set for npm/yarn/pnpm/bun workspaces (the root's manager), Cargo workspaces (`pm.Cargo`) and Maven reactors (`pm.Maven`).
  - `type Project`: unchanged.
  - `func Filter(workspaces []Workspace, include, exclude []string) []Workspace`
  - `func FormatWorkspaces(workspaces []Workspace) string`: prints in the given order.
  - Unexported helpers that Task 13 relies on: `expandGlobs(root string, patterns []string) []string`, `matchPath(pattern, rel string) bool`, `walkDirs(root string, visit func(dir string) bool)`, `relSlash(root, p string) string`, `isFile(p string) bool`, `const uvID pm.ID = "uv"`.
  - Removed: `GroupByEcosystem`, `validateWorkspace`, `mergeWorkspaces`, `detectNodePM`, `readGoModuleName`, `detectPnpmWorkspace`, `detectPoetryWorkspace`, `detectRootPythonWorkspace`, `isProjectInList` and `isComposerProjectInList`. Their only callers were inside this package.

- [ ] **Step 1: Write the failing tests**

Create `internal/workspace/detect_test.go`. The `writeTree` helper turns a `map[path]content` into a temp dir. Each test asserts exact project lists.

```go
package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
)

// writeTree creates files (slash-separated paths relative to a new temp dir)
// with the given contents and returns the dir.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// rels returns each project's path relative to its workspace root.
func rels(ws *Workspace) []string {
	if ws == nil {
		return nil
	}
	out := []string{}
	for _, p := range ws.Projects {
		out = append(out, relSlash(ws.Root, p.Path))
	}
	return out
}

func names(ws *Workspace) []string {
	out := []string{}
	for _, p := range ws.Projects {
		out = append(out, p.Name)
	}
	return out
}

// detectOne runs DetectWorkspaces and returns the workspace of ecosystem eco.
func detectOne(t *testing.T, root, eco string) *Workspace {
	t.Helper()
	all, err := DetectWorkspaces(root)
	if err != nil {
		t.Fatal(err)
	}
	for i := range all {
		if all[i].Ecosystem == eco {
			return &all[i]
		}
	}
	return nil
}

func TestGoWorkBlockAndSingleLineForms(t *testing.T) {
	root := writeTree(t, map[string]string{
		"go.work": `go 1.22

// shared libraries first
use (
	./svc/api
	"./libs/common" // quoted path
)

use ./tools
`,
		"svc/api/go.mod":     "module example.com/api\n\ngo 1.22\n",
		"svc/api/go.sum":     "",
		"libs/common/go.mod": "// header comment\nmodule \"example.com/common\"\n\ngo 1.22\n",
		"tools/go.mod":       "module example.com/tools\n",
		"unused/go.mod":      "module example.com/unused\n",
	})
	ws := detectOne(t, root, "go")
	if ws == nil {
		t.Fatal("no go workspace detected")
	}
	if got, want := rels(ws), []string{"libs/common", "svc/api", "tools"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
	if got, want := names(ws), []string{"example.com/common", "example.com/api", "example.com/tools"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	if ws.Projects[1].Lockfile == "" || ws.Projects[0].Lockfile != "" {
		t.Errorf("lockfiles = %q, %q; want only svc/api to have go.sum", ws.Projects[0].Lockfile, ws.Projects[1].Lockfile)
	}
	if ws.RootPM != "" {
		t.Errorf("RootPM = %q, want per-module installs", ws.RootPM)
	}
}

func TestGoModulesFallbackSkipsVendorAndFixtures(t *testing.T) {
	root := writeTree(t, map[string]string{
		"svc/go.mod":                        "module example.com/svc\n",
		"svc/inner/go.mod":                  "module example.com/svc/inner\n",
		"vendor/github.com/acme/lib/go.mod": "module github.com/acme/lib\n",
		"testdata/fixture/go.mod":           "module fixture\n",
		".cache/mod/go.mod":                 "module cached\n",
		"web/node_modules/esbuild/go.mod":   "module esbuild\n",
		"deep/a/b/c/d/e/f/go.mod":           "module too.deep\n",
		"tools/gen/go.mod":                  "module example.com/gen\n",
	})
	ws := detectOne(t, root, "go")
	if got, want := rels(ws), []string{"svc", "tools/gen"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
}

func TestNpmWorkspacesDoubleStarAndNegation(t *testing.T) {
	root := writeTree(t, map[string]string{
		"package.json": `{
  "name": "acme",
  "private": true,
  "workspaces": ["packages/**", "!packages/legacy/**", "apps/*"]
}`,
		"yarn.lock":                "# yarn lockfile v1\n",
		"packages/ui/package.json": `{"name": "@acme/ui"}`,
		"packages/ui/node_modules/react/package.json": `{"name": "react"}`,
		"packages/tools/cli/package.json":             `{"name": "@acme/cli"}`,
		"packages/legacy/old/package.json":            `{"name": "@acme/old"}`,
		"packages/README.md":                          "docs\n",
		"apps/web/package.json":                       `{"name": "web"}`,
		"apps/docs/index.md":                          "no manifest\n",
		"node_modules/@acme/ui/package.json":          `{"name": "@acme/ui"}`,
		"node_modules/left-pad/package.json":          `{"name": "left-pad"}`,
	})
	ws := detectOne(t, root, "node")
	if ws == nil {
		t.Fatal("no node workspace detected")
	}
	if got, want := rels(ws), []string{"apps/web", "packages/tools/cli", "packages/ui"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
	if got, want := names(ws), []string{"web", "@acme/cli", "@acme/ui"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	if ws.RootPM != pm.Yarn {
		t.Errorf("RootPM = %q, want yarn (from root yarn.lock)", ws.RootPM)
	}
	if ws.Projects[0].Lockfile != filepath.Join(root, "yarn.lock") || ws.Projects[0].PM != pm.Yarn {
		t.Errorf("member = %+v, want the root yarn.lock and yarn", ws.Projects[0])
	}
}

func TestNpmWorkspacesObjectFormAndLockPrecedence(t *testing.T) {
	root := writeTree(t, map[string]string{
		"package.json":            `{"workspaces": {"packages": ["libs/*"], "nohoist": ["**/react"]}}`,
		"package-lock.json":       `{"lockfileVersion": 3}`,
		"yarn.lock":               "",
		"libs/a/package.json":     `{"name": "a"}`,
		"libs/b/package.json":     `{}`,
		"libs/b/sub/package.json": `{"name": "not-a-member"}`,
	})
	ws := detectOne(t, root, "node")
	if got, want := names(ws), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v (unnamed member falls back to its dir)", got, want)
	}
	if ws.RootPM != pm.Npm {
		t.Errorf("RootPM = %q, want npm (package-lock.json precedes yarn.lock)", ws.RootPM)
	}
}

func TestPnpmWorkspaceNegation(t *testing.T) {
	root := writeTree(t, map[string]string{
		"package.json":               `{"name": "root"}`,
		"pnpm-workspace.yaml":        "packages:\n  - 'packages/*'\n  - '!**/test/**'\n",
		"packages/a/package.json":    `{"name": "a"}`,
		"packages/test/package.json": `{"name": "test-utils"}`,
	})
	ws := detectOne(t, root, "node")
	if got, want := rels(ws), []string{"packages/a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
	if ws.RootPM != pm.Pnpm {
		t.Errorf("RootPM = %q, want pnpm", ws.RootPM)
	}
}

func TestCargoMembersExcludeAndRootPackage(t *testing.T) {
	root := writeTree(t, map[string]string{
		"Cargo.toml": `[package]
name = "app"
version = "0.1.0"

[workspace]
members = ["crates/*"]
exclude = ["crates/experimental"]
`,
		"Cargo.lock":                                "version = 3\n",
		"crates/core/Cargo.toml":                    "[package]\nname = \"acme-core\"\nversion = \"0.1.0\"\n",
		"crates/cli/Cargo.toml":                     "[package]\nname = \"acme-cli\"\nversion = \"0.1.0\"\n",
		"crates/experimental/Cargo.toml":            "[package]\nname = \"acme-exp\"\nversion = \"0.1.0\"\n",
		"crates/notes/README.md":                    "not a crate\n",
		"target/package/acme-core-0.1.0/Cargo.toml": "[package]\nname = \"acme-core\"\n",
	})
	ws := detectOne(t, root, "rust")
	if got, want := names(ws), []string{"app", "acme-cli", "acme-core"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	if ws.RootPM != pm.Cargo {
		t.Errorf("RootPM = %q, want cargo", ws.RootPM)
	}
}

func TestPythonSkipsVirtualenvsAndReadsUVWorkspace(t *testing.T) {
	root := writeTree(t, map[string]string{
		"pyproject.toml": `[project]
name = "monorepo"

[tool.uv.workspace]
members = ["services/*"]
exclude = ["services/scratch"]
`,
		"uv.lock":                                             "version = 1\n",
		"services/api/pyproject.toml":                         "[project]\nname = \"api\"\n",
		"services/scratch/pyproject.toml":                     "[project]\nname = \"scratch\"\n",
		"tools/poetry-tool/pyproject.toml":                    "[tool.poetry]\nname = \"poetry-tool\"\n",
		"tools/poetry-tool/poetry.lock":                       "",
		".venv/lib/python3.12/site-packages/x/pyproject.toml": "[tool.poetry]\nname = \"x\"\n",
		"venv/lib/site-packages/y/pyproject.toml":             "[tool.poetry]\nname = \"y\"\n",
	})
	ws := detectOne(t, root, "python")
	if got, want := names(ws), []string{"monorepo", "api", "poetry-tool"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	if ws.Projects[0].PM != uvID || ws.Projects[2].PM != pm.Poetry {
		t.Errorf("PMs = %q, %q; want uv, poetry", ws.Projects[0].PM, ws.Projects[2].PM)
	}
}

func TestComposerPathRepositoryGlob(t *testing.T) {
	root := writeTree(t, map[string]string{
		"composer.json": `{"repositories": [
  {"type": "path", "url": "libs/*"},
  {"type": "vcs", "url": "https://github.com/acme/x"}
]}`,
		"libs/money/composer.json":                `{"name": "acme/money"}`,
		"libs/money/vendor/psr/log/composer.json": `{"name": "psr/log"}`,
		"vendor/symfony/console/composer.json":    `{"name": "symfony/console"}`,
	})
	ws := detectOne(t, root, "php")
	if got, want := names(ws), []string{"acme/money"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
}

func TestDetectWorkspacesFixedEcosystemOrder(t *testing.T) {
	root := writeTree(t, map[string]string{
		"go.work":             "go 1.22\nuse ./svc\n",
		"svc/go.mod":          "module example.com/svc\n",
		"Cargo.toml":          "[workspace]\nmembers = [\"crates/a\"]\n",
		"crates/a/Cargo.toml": "[package]\nname = \"a\"\n",
		"package.json":        `{"workspaces": ["web"]}`,
		"web/package.json":    `{"name": "web"}`,
	})
	for i := 0; i < 3; i++ {
		all, err := DetectWorkspaces(root)
		if err != nil {
			t.Fatal(err)
		}
		var ecos []string
		for _, ws := range all {
			ecos = append(ecos, ws.Ecosystem)
		}
		if want := []string{"node", "rust", "go"}; !reflect.DeepEqual(ecos, want) {
			t.Fatalf("ecosystems = %v, want %v", ecos, want)
		}
	}
}
```

Create `internal/workspace/glob_test.go`:

```go
package workspace

import (
	"reflect"
	"testing"
)

func TestMatchPath(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		{"packages/*", "packages/ui", true},
		{"packages/*", "packages/ui/sub", false},
		{"packages/**", "packages/ui/sub", true},
		{"packages/**", "packages", true},
		{"**/test/**", "packages/test", true},
		{"**/test/**", "packages/test/unit", true},
		{"**/test/**", "packages/testing", false},
		{"./apps/*/", "apps/web", true},
		{"apps/w?b", "apps/web", true},
		{"apps/[a-c]*", "apps/web", false},
		{".", ".", true},
		{"*", ".", false},
		{"**", ".", true},
	}
	for _, c := range cases {
		if got := matchPath(c.pattern, c.rel); got != c.want {
			t.Errorf("matchPath(%q, %q) = %v, want %v", c.pattern, c.rel, got, c.want)
		}
	}
}

func TestExpandGlobsLiteralSkippedDirIsReachable(t *testing.T) {
	root := writeTree(t, map[string]string{
		"vendor/acme/a/x":   "",
		"vendor/acme/b/x":   "",
		"pkgs/a/vendor/z/x": "",
		"pkgs/.hidden/y/x":  "",
	})
	got := []string{}
	for _, d := range expandGlobs(root, []string{"vendor/acme/*", "pkgs/**"}) {
		got = append(got, relSlash(root, d))
	}
	want := []string{"pkgs", "pkgs/a", "vendor/acme/a", "vendor/acme/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandGlobs = %v, want %v", got, want)
	}
}
```

Create `internal/workspace/filter_test.go`:

```go
package workspace

import (
	"path/filepath"
	"reflect"
	"testing"
)

func filterFixture() []Workspace {
	root := filepath.FromSlash("/repo")
	proj := func(rel string) Project {
		return Project{Name: rel, Path: filepath.Join(root, filepath.FromSlash(rel))}
	}
	return []Workspace{
		{Root: root, Ecosystem: "node", Projects: []Project{
			proj("apps/web"), proj("packages/legacy-ui"), proj("packages/tools/cli"), proj("packages/ui"),
		}},
		{Root: root, Ecosystem: "go", Projects: []Project{proj("services/api")}},
		{Root: root, Ecosystem: "python", Projects: []Project{proj(".")}},
	}
}

func filtered(ws []Workspace) map[string][]string {
	out := map[string][]string{}
	for _, w := range ws {
		for _, p := range w.Projects {
			out[w.Ecosystem] = append(out[w.Ecosystem], p.Name)
		}
	}
	return out
}

func TestFilter(t *testing.T) {
	cases := []struct {
		name             string
		include, exclude []string
		want             map[string][]string
	}{
		{"no patterns keeps all", nil, nil, map[string][]string{
			"node":   {"apps/web", "packages/legacy-ui", "packages/tools/cli", "packages/ui"},
			"go":     {"services/api"},
			"python": {"."},
		}},
		{"include double star, exclude wins, empty workspaces dropped",
			[]string{"packages/**"}, []string{"packages/legacy-*"},
			map[string][]string{"node": {"packages/tools/cli", "packages/ui"}}},
		{"single star stays in one segment", []string{"packages/*"}, nil,
			map[string][]string{"node": {"packages/legacy-ui", "packages/ui"}}},
		{"exclude only", nil, []string{"apps/*", "services/**", "."},
			map[string][]string{"node": {"packages/legacy-ui", "packages/tools/cli", "packages/ui"}}},
		{"root project matched by dot", []string{"."}, nil,
			map[string][]string{"python": {"."}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := filterFixture()
			got := filtered(Filter(in, c.include, c.exclude))
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Filter = %v, want %v", got, c.want)
			}
			if len(in[0].Projects) != 4 {
				t.Fatalf("Filter modified its input")
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/workspace/`
Expected: FAIL. The build fails with `undefined: relSlash`, `undefined: Filter`, `undefined: expandGlobs`, `undefined: matchPath`, `ws.RootPM undefined` and `undefined: uvID`.

- [ ] **Step 3: Add the glob/walk helpers**

Create `internal/workspace/glob.go`:

```go
package workspace

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// maxWalkDepth bounds every directory walk and every "**" expansion, counted
// in path segments below the workspace root.
const maxWalkDepth = 6

// skippedDirs are never descended into by a walk, a "*" or a "**": they hold
// installed dependencies, virtualenvs, build output or test fixtures, and may
// contain manifests that are not workspace projects. A pattern that names one
// literally (e.g. "vendor/acme") still reaches it.
var skippedDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"venv":         true,
	"target":       true,
	"testdata":     true,
	"__pycache__":  true,
}

// skipDir reports whether a walk must not enter a directory with this name.
// Hidden directories (.git, .venv, .idea, ...) are always skipped.
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || skippedDirs[name]
}

// walkDirs visits every directory below root (not root itself), skipping
// skipDir names and anything deeper than maxWalkDepth. When visit returns
// true the directory is a project and its subtree is not entered.
func walkDirs(root string, visit func(dir string) bool) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == root {
			return nil
		}
		if skipDir(d.Name()) {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil || len(strings.Split(filepath.ToSlash(rel), "/")) > maxWalkDepth {
			return filepath.SkipDir
		}
		if visit(p) {
			return filepath.SkipDir
		}
		return nil
	})
}

// cleanPattern normalises a workspace glob: slash-separated, no leading "./",
// no trailing "/".
func cleanPattern(p string) string {
	p = filepath.ToSlash(strings.TrimSpace(p))
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return strings.TrimSuffix(p, "/")
}

// matchPath reports whether the slash-separated relative path rel matches
// pattern: path.Match per segment, plus "**" matching zero or more segments.
func matchPath(pattern, rel string) bool {
	pattern, rel = cleanPattern(pattern), cleanPattern(rel)
	if rel == "" || rel == "." {
		// The workspace root matches only "." or a pattern of "**"s.
		return pattern == "." || strings.Trim(strings.ReplaceAll(pattern, "**", ""), "/") == ""
	}
	if pattern == "" || pattern == "." {
		return false
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			if len(rest) == 0 {
				return true
			}
			for i := 0; i <= len(name); i++ {
				if matchSegments(rest, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// expandGlobs returns the directories under root matched by patterns, as
// absolute paths sorted by their relative slash path. Patterns are relative
// to root and slash-separated; "*", "?" and "[...]" match within one segment,
// "**" matches any number of segments (up to maxWalkDepth). A pattern starting
// with "!" removes the directories it matches from the result, whatever its
// position in the list (npm/pnpm/cargo semantics for the common cases).
// Wildcards never enter skipDir directories; root itself is never returned.
func expandGlobs(root string, patterns []string) []string {
	var include, exclude []string
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") {
			exclude = append(exclude, cleanPattern(p[1:]))
		} else if c := cleanPattern(p); c != "" && c != "." {
			include = append(include, c)
		}
	}
	found := map[string]bool{}
	for _, p := range include {
		expandSegments(root, strings.Split(p, "/"), "", 0, found)
	}
	var rels []string
	for rel := range found {
		if rel == "" || excluded(rel, exclude) {
			continue
		}
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		out = append(out, filepath.Join(root, filepath.FromSlash(rel)))
	}
	return out
}

// excluded reports whether rel matches any exclusion pattern, or lies inside
// a directory that one names (Cargo's exclude is a list of paths).
func excluded(rel string, patterns []string) bool {
	for _, p := range patterns {
		if matchPath(p, rel) || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func expandSegments(root string, segs []string, rel string, depth int, found map[string]bool) {
	if len(segs) == 0 {
		if isDir(filepath.Join(root, filepath.FromSlash(rel))) {
			found[rel] = true
		}
		return
	}
	seg := segs[0]
	switch {
	case seg == "**":
		expandSegments(root, segs[1:], rel, depth, found)
		if depth >= maxWalkDepth {
			return
		}
		for _, name := range subdirs(root, rel) {
			expandSegments(root, segs, joinRel(rel, name), depth+1, found)
		}
	case strings.ContainsAny(seg, "*?["):
		for _, name := range subdirs(root, rel) {
			if ok, err := path.Match(seg, name); err == nil && ok {
				expandSegments(root, segs[1:], joinRel(rel, name), depth+1, found)
			}
		}
	default:
		expandSegments(root, segs[1:], joinRel(rel, seg), depth+1, found)
	}
}

// subdirs lists the directories in root/rel that wildcards may enter.
func subdirs(root, rel string) []string {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !skipDir(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

func joinRel(rel, name string) string {
	if rel == "" {
		return name
	}
	return path.Clean(rel + "/" + name)
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// relSlash returns p relative to root, slash-separated; "." for root itself.
func relSlash(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}
```

Create `internal/workspace/filter.go`:

```go
package workspace

// Filter keeps the projects whose path relative to their workspace root
// (slash-separated, "." for the root itself) matches an include pattern and
// no exclude pattern. Patterns use path.Match syntax per segment plus "**"
// for any number of segments. An empty include list keeps every project;
// exclude wins over include. Workspaces left without projects are dropped.
// The input is not modified.
func Filter(workspaces []Workspace, include, exclude []string) []Workspace {
	if len(include) == 0 && len(exclude) == 0 {
		return workspaces
	}
	var out []Workspace
	for _, ws := range workspaces {
		var kept []Project
		for _, p := range ws.Projects {
			rel := relSlash(ws.Root, p.Path)
			if (len(include) == 0 || anyMatch(include, rel)) && !anyMatch(exclude, rel) {
				kept = append(kept, p)
			}
		}
		if len(kept) > 0 {
			ws.Projects = kept
			out = append(out, ws)
		}
	}
	return out
}

func anyMatch(patterns []string, rel string) bool {
	for _, p := range patterns {
		if matchPath(p, rel) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Replace the types and the detector driver**

Replace `internal/workspace/types.go`:

```go
// Package workspace provides workspace/monorepo detection and management.
package workspace

import (
	"github.com/crenspire/xpm/internal/pm"
)

// Workspace represents a detected workspace/monorepo of one ecosystem.
type Workspace struct {
	Root      string
	Projects  []Project
	Ecosystem string // "node", "python", "rust", "go", "java", "php"
	// RootPM is set when the workspace is installed once, at Root, by this
	// package manager (npm/yarn/pnpm/bun workspaces, Cargo workspaces, Maven
	// reactors). Empty means each project is installed in its own directory.
	RootPM pm.ID
}

// Project represents a single project within a workspace.
type Project struct {
	Name      string
	Path      string
	Ecosystem string
	Manifest  string // path to manifest file
	Lockfile  string // path to lockfile (if exists)
	PM        pm.ID  // detected package manager
}
```

Replace `internal/workspace/detect.go`:

```go
package workspace

import (
	"fmt"
	"path/filepath"
	"sort"
)

// detectors run in this order, which is also the order of the returned
// workspaces: node, python, rust, go, java, php.
var detectors = []func(root string) (*Workspace, error){
	DetectNodeWorkspace,
	DetectPythonWorkspace,
	DetectCargoWorkspace,
	DetectGoWorkspace,
	DetectJavaWorkspace,
	DetectComposerWorkspace,
}

// DetectWorkspaces detects the workspaces rooted at root: at most one per
// ecosystem, in a fixed ecosystem order, each with its projects sorted by
// path relative to root and de-duplicated. A detector that fails (for
// example on a malformed manifest) contributes nothing.
func DetectWorkspaces(root string) ([]Workspace, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("invalid root path: %w", err)
	}
	var out []Workspace
	for _, detect := range detectors {
		ws, err := detect(absRoot)
		if err != nil || ws == nil {
			continue
		}
		finalize(ws)
		if len(ws.Projects) > 0 {
			out = append(out, *ws)
		}
	}
	return out, nil
}

// finalize sorts a workspace's projects by relative slash path and drops
// repeated project directories (first one wins).
func finalize(ws *Workspace) {
	sort.SliceStable(ws.Projects, func(i, j int) bool {
		return relSlash(ws.Root, ws.Projects[i].Path) < relSlash(ws.Root, ws.Projects[j].Path)
	})
	seen := map[string]bool{}
	kept := ws.Projects[:0]
	for _, p := range ws.Projects {
		if seen[p.Path] {
			continue
		}
		seen[p.Path] = true
		kept = append(kept, p)
	}
	ws.Projects = kept
}
```

- [ ] **Step 5: Replace the ecosystem detectors**

Replace `internal/workspace/node.go`:

```go
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectNodeWorkspace detects npm/yarn/bun workspaces (package.json
// "workspaces", as an array or as {"packages": [...]}) and pnpm workspaces
// (pnpm-workspace.yaml "packages"). Every member needs its own package.json.
func DetectNodeWorkspace(root string) (*Workspace, error) {
	patterns, rootPM, err := nodeWorkspacePatterns(root)
	if err != nil || len(patterns) == 0 {
		return nil, err
	}
	lockfile := nodeLockfile(root, rootPM)
	var projects []Project
	for _, dir := range expandGlobs(root, patterns) {
		manifest := filepath.Join(dir, "package.json")
		data, err := os.ReadFile(manifest)
		if err != nil {
			continue
		}
		var pkg struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &pkg) != nil {
			continue
		}
		name := pkg.Name
		if name == "" {
			name = filepath.Base(dir)
		}
		projects = append(projects, Project{
			Name:      name,
			Path:      dir,
			Ecosystem: "node",
			Manifest:  manifest,
			Lockfile:  lockfile,
			PM:        rootPM,
		})
	}
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "node", RootPM: rootPM}, nil
}

// nodeWorkspacePatterns returns the member globs and the package manager
// that owns the workspace root. pnpm-workspace.yaml wins over package.json.
func nodeWorkspacePatterns(root string) ([]string, pm.ID, error) {
	if data, err := os.ReadFile(filepath.Join(root, "pnpm-workspace.yaml")); err == nil {
		var cfg struct {
			Packages []string `yaml:"packages"`
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, "", fmt.Errorf("pnpm-workspace.yaml: %w", err)
		}
		return cfg.Packages, pm.Pnpm, nil
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, "", nil
	}
	var pkg struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, "", fmt.Errorf("package.json: %w", err)
	}
	if len(pkg.Workspaces) == 0 {
		return nil, "", nil
	}
	var patterns []string
	if json.Unmarshal(pkg.Workspaces, &patterns) != nil {
		var obj struct {
			Packages []string `json:"packages"`
		}
		if err := json.Unmarshal(pkg.Workspaces, &obj); err != nil {
			return nil, "", fmt.Errorf("package.json workspaces: %w", err)
		}
		patterns = obj.Packages
	}
	return patterns, rootNodePM(root), nil
}

// rootNodePM picks the root's package manager from its lock files in pm's
// fixed order (package-lock.json, yarn.lock, pnpm-lock.yaml, bun.lock,
// bun.lockb); npm when there is none.
func rootNodePM(root string) pm.ID {
	if ids := pm.DetectLockFilesForEcosystem(root, pm.EcosystemNode); len(ids) > 0 {
		return ids[0]
	}
	return pm.Npm
}

// nodeLockfile returns the root lock file written by id, or "".
func nodeLockfile(root string, id pm.ID) string {
	names := map[pm.ID][]string{
		pm.Npm:  {"package-lock.json"},
		pm.Yarn: {"yarn.lock"},
		pm.Pnpm: {"pnpm-lock.yaml"},
		pm.Bun:  {"bun.lock", "bun.lockb"},
	}[id]
	for _, n := range names {
		if p := filepath.Join(root, n); isFile(p) {
			return p
		}
	}
	return ""
}
```

Replace `internal/workspace/go.go`:

```go
package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectGoWorkspace detects the modules listed by go.work "use" directives
// (single-line and block forms). Without a go.work it falls back to the
// go.mod files below root (not root itself), skipping vendor/, testdata/,
// hidden directories and the other skipDir names.
func DetectGoWorkspace(root string) (*Workspace, error) {
	goWork := filepath.Join(root, "go.work")
	data, err := os.ReadFile(goWork)
	if err != nil {
		return detectGoModules(root)
	}
	wf, err := modfile.ParseWork(goWork, data, nil)
	if err != nil {
		return nil, fmt.Errorf("go.work: %w", err)
	}
	var projects []Project
	for _, use := range wf.Use {
		dir := filepath.FromSlash(use.Path)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		if p, ok := goProject(dir); ok {
			projects = append(projects, p)
		}
	}
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "go"}, nil
}

// detectGoModules finds go.mod files below root; nested modules inside a
// found module are not separate projects.
func detectGoModules(root string) (*Workspace, error) {
	var projects []Project
	walkDirs(root, func(dir string) bool {
		p, ok := goProject(dir)
		if ok {
			projects = append(projects, p)
		}
		return ok
	})
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "go"}, nil
}

// goProject describes the module in dir, named by its go.mod module path.
func goProject(dir string) (Project, bool) {
	manifest := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(manifest)
	if err != nil {
		return Project{}, false
	}
	name := modfile.ModulePath(data)
	if name == "" {
		name = filepath.Base(dir)
	}
	lockfile := ""
	if p := filepath.Join(dir, "go.sum"); isFile(p) {
		lockfile = p
	}
	return Project{Name: name, Path: filepath.Clean(dir), Ecosystem: "go",
		Manifest: manifest, Lockfile: lockfile, PM: pm.GoMod}, true
}
```

Replace `internal/workspace/cargo.go`:

```go
package workspace

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectCargoWorkspace detects a Cargo workspace: [workspace] members (globs)
// minus [workspace] exclude (paths or globs), plus the root package when the
// root Cargo.toml also has a [package].
func DetectCargoWorkspace(root string) (*Workspace, error) {
	rootManifest := filepath.Join(root, "Cargo.toml")
	if !isFile(rootManifest) {
		return nil, nil
	}
	var cfg struct {
		Package struct {
			Name string `toml:"name"`
		} `toml:"package"`
		Workspace struct {
			Members []string `toml:"members"`
			Exclude []string `toml:"exclude"`
		} `toml:"workspace"`
	}
	if _, err := toml.DecodeFile(rootManifest, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Workspace.Members) == 0 {
		return nil, nil
	}
	lockfile := ""
	if p := filepath.Join(root, "Cargo.lock"); isFile(p) {
		lockfile = p
	}
	patterns := append([]string{}, cfg.Workspace.Members...)
	for _, ex := range cfg.Workspace.Exclude {
		patterns = append(patterns, "!"+ex)
	}
	var projects []Project
	if cfg.Package.Name != "" {
		projects = append(projects, Project{Name: cfg.Package.Name, Path: root, Ecosystem: "rust",
			Manifest: rootManifest, Lockfile: lockfile, PM: pm.Cargo})
	}
	for _, dir := range expandGlobs(root, patterns) {
		manifest := filepath.Join(dir, "Cargo.toml")
		var pkg struct {
			Package struct {
				Name string `toml:"name"`
			} `toml:"package"`
		}
		if _, err := toml.DecodeFile(manifest, &pkg); err != nil {
			continue
		}
		name := pkg.Package.Name
		if name == "" {
			name = filepath.Base(dir)
		}
		projects = append(projects, Project{Name: name, Path: dir, Ecosystem: "rust",
			Manifest: manifest, Lockfile: lockfile, PM: pm.Cargo})
	}
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "rust", RootPM: pm.Cargo}, nil
}
```

Replace `internal/workspace/python.go`:

```go
package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/crenspire/xpm/internal/pm"
)

// uvID names uv, which internal/pm has no ID for; it is only used to pick
// the install command (`uv sync`).
const uvID pm.ID = "uv"

// pyproject is the subset of pyproject.toml workspace detection reads.
type pyproject struct {
	Project struct {
		Name string `toml:"name"`
	} `toml:"project"`
	Tool struct {
		Poetry struct {
			Name string `toml:"name"`
		} `toml:"poetry"`
		UV struct {
			Workspace struct {
				Members []string `toml:"members"`
				Exclude []string `toml:"exclude"`
			} `toml:"workspace"`
		} `toml:"uv"`
	} `toml:"tool"`
}

// DetectPythonWorkspace detects Python projects: uv workspace members
// ([tool.uv.workspace] members/exclude), Poetry projects anywhere below root
// (bounded walk), pyproject.toml projects in src/*, packages/*, apps/*,
// libs/*, and the root pyproject.toml itself.
func DetectPythonWorkspace(root string) (*Workspace, error) {
	var projects []Project
	add := func(dir string) {
		if p := createPythonProject(dir, filepath.Join(dir, "pyproject.toml")); p != nil {
			projects = append(projects, *p)
		}
	}

	var rootCfg pyproject
	if _, err := toml.DecodeFile(filepath.Join(root, "pyproject.toml"), &rootCfg); err == nil {
		uvws := rootCfg.Tool.UV.Workspace
		patterns := append([]string{}, uvws.Members...)
		for _, ex := range uvws.Exclude {
			patterns = append(patterns, "!"+ex)
		}
		for _, dir := range expandGlobs(root, patterns) {
			add(dir)
		}
		add(root)
	}

	walkDirs(root, func(dir string) bool {
		var cfg pyproject
		if _, err := toml.DecodeFile(filepath.Join(dir, "pyproject.toml"), &cfg); err != nil {
			return false
		}
		if cfg.Tool.Poetry.Name == "" {
			return false
		}
		add(dir)
		return true
	})

	for _, dir := range expandGlobs(root, []string{"src/*", "packages/*", "apps/*", "libs/*"}) {
		add(dir)
	}

	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "python"}, nil
}

// createPythonProject creates a Project from a directory's pyproject.toml,
// or returns nil when there is none or it does not parse.
func createPythonProject(projectDir, pyprojectPath string) *Project {
	var cfg pyproject
	if _, err := toml.DecodeFile(pyprojectPath, &cfg); err != nil {
		return nil
	}
	name := cfg.Project.Name
	if name == "" {
		name = cfg.Tool.Poetry.Name
	}
	if name == "" {
		name = filepath.Base(projectDir)
	}
	pmID, lockfile := detectPythonPM(projectDir)
	return &Project{
		Name:      name,
		Path:      projectDir,
		Ecosystem: "python",
		Manifest:  pyprojectPath,
		Lockfile:  lockfile,
		PM:        pmID,
	}
}

// detectPythonPM picks a Python project's manager in a fixed order:
// uv.lock, poetry.lock, Pipfile.lock, Pipfile, a [tool.poetry] table, pip.
// The lock file is returned when there is one.
func detectPythonPM(projectPath string) (pm.ID, string) {
	for _, lf := range []struct {
		name string
		id   pm.ID
	}{
		{"uv.lock", uvID},
		{"poetry.lock", pm.Poetry},
		{"Pipfile.lock", pm.Pipenv},
	} {
		if p := filepath.Join(projectPath, lf.name); isFile(p) {
			return lf.id, p
		}
	}
	if isFile(filepath.Join(projectPath, "Pipfile")) {
		return pm.Pipenv, ""
	}
	if data, err := os.ReadFile(filepath.Join(projectPath, "pyproject.toml")); err == nil &&
		strings.Contains(string(data), "[tool.poetry]") {
		return pm.Poetry, ""
	}
	return pm.Pip, ""
}
```

Replace `internal/workspace/composer.go`:

```go
package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectComposerWorkspace detects PHP packages: "path" repositories in the
// root composer.json (their url may be a glob, as Composer allows) and
// composer.json projects in packages/*, modules/*, src/*.
func DetectComposerWorkspace(root string) (*Workspace, error) {
	var projects []Project
	add := func(dir string) {
		if p := createComposerProject(dir, filepath.Join(dir, "composer.json")); p != nil {
			projects = append(projects, *p)
		}
	}
	for _, dir := range composerPathRepos(root) {
		add(dir)
	}
	for _, dir := range expandGlobs(root, []string{"packages/*", "modules/*", "src/*"}) {
		add(dir)
	}
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "php"}, nil
}

// composerPathRepos returns the directories named by the root composer.json's
// {"type": "path", "url": ...} repositories.
func composerPathRepos(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return nil
	}
	var cfg struct {
		Repositories json.RawMessage `json:"repositories"`
	}
	if json.Unmarshal(data, &cfg) != nil || len(cfg.Repositories) == 0 {
		return nil
	}
	type repo struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	// "repositories" is a list or an object keyed by name.
	var list []repo
	if json.Unmarshal(cfg.Repositories, &list) != nil {
		var byName map[string]repo
		if json.Unmarshal(cfg.Repositories, &byName) != nil {
			return nil
		}
		for _, r := range byName {
			list = append(list, r)
		}
	}
	var dirs []string
	for _, r := range list {
		if r.Type != "path" || r.URL == "" {
			continue
		}
		if filepath.IsAbs(r.URL) {
			if isDir(r.URL) {
				dirs = append(dirs, filepath.Clean(r.URL))
			}
			continue
		}
		dirs = append(dirs, expandGlobs(root, []string{r.URL})...)
	}
	return dirs
}

// createComposerProject creates a Project from a composer.json, or returns
// nil when there is none or it does not parse.
func createComposerProject(projectDir, composerJSONPath string) *Project {
	data, err := os.ReadFile(composerJSONPath)
	if err != nil {
		return nil
	}
	var cfg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	name := cfg.Name
	if name == "" {
		name = filepath.Base(projectDir)
	}
	lockfile := ""
	if p := filepath.Join(projectDir, "composer.lock"); isFile(p) {
		lockfile = p
	}
	return &Project{
		Name:      name,
		Path:      projectDir,
		Ecosystem: "php",
		Manifest:  composerJSONPath,
		Lockfile:  lockfile,
		PM:        pm.Composer,
	}
}
```

In `internal/workspace/java.go`, `detectMavenWorkspace`, change the final return to:

```go
	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "java",
		RootPM:    pm.Maven, // a reactor resolves from its root pom
	}
```

Replace `internal/workspace/list.go`:

```go
package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FormatWorkspaces formats workspaces for CLI output, in the order given
// (DetectWorkspaces returns them in a fixed ecosystem order).
func FormatWorkspaces(workspaces []Workspace) string {
	if len(workspaces) == 0 {
		return "No workspaces detected."
	}
	var b strings.Builder
	b.WriteString("Detected Workspaces\n")
	b.WriteString(strings.Repeat("─", 30) + "\n")
	for _, ws := range workspaces {
		eco := ws.Ecosystem
		if eco == "" {
			eco = "unknown"
		}
		fmt.Fprintf(&b, "\n%s:\n", eco)
		for _, p := range ws.Projects {
			rel := relSlash(ws.Root, p.Path)
			if rel == "." {
				rel = filepath.Base(p.Path)
			}
			fmt.Fprintf(&b, "  %-30s → %s\n", rel, filepath.Base(p.Manifest))
		}
	}
	return b.String()
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test -count=1 ./internal/workspace/`
Expected: `ok  	github.com/crenspire/xpm/internal/workspace`

Run: `grep -n 'golang.org/x/mod' go.mod`
Expected: `golang.org/x/mod v0.23.0` in the main `require` block, without `// indirect`. Task 5 already imports modfile. If Task 5 left `// indirect`, report it to the controller rather than editing go.mod here, because go.mod belongs to Task 5.

- [ ] **Step 7: Full verification**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: every package `ok` (or `[no test files]`), no gofmt output, and `0 issues.`
Optional, if the toolchain is cached: `GOTOOLCHAIN=go1.22.12 go vet ./... && GOTOOLCHAIN=go1.22.12 go test ./internal/workspace/`. Expected: `ok`.

- [ ] **Step 8: Commit**

```bash
git add internal/workspace/glob.go internal/workspace/filter.go internal/workspace/types.go \
  internal/workspace/detect.go internal/workspace/node.go internal/workspace/go.go \
  internal/workspace/cargo.go internal/workspace/python.go internal/workspace/composer.go \
  internal/workspace/java.go internal/workspace/list.go \
  internal/workspace/detect_test.go internal/workspace/glob_test.go internal/workspace/filter_test.go
git commit -m "workspace: skip dependency dirs, parse go.work with modfile, ** and ! globs, include/exclude filter, fixed order"
```

---

### Task 13: Workspace execution via `cmd.Dir`, one install at the Node/Cargo/Maven root, run via re-exec, `workspace_cmd.go`

**Files:**
- Create: `internal/workspace/exec.go`
- Replace (whole file): `internal/workspace/install.go`, `internal/workspace/run.go`
- Replace: `internal/cli/workspace_cmd.go`. This is its content **after Task 7** has moved `cmdGraphWorkspace` to `graph_cmd.go`. This task does not reference or modify `cmdGraphWorkspace`. If the implementer finds it still in `workspace_cmd.go`, Task 7 has not landed: keep that function byte-for-byte below the new code, and keep the `strconv`, `config` and `graph` imports it needs.
- Test (create): `internal/workspace/exec_test.go`, `internal/workspace/install_test.go`, `internal/workspace/run_test.go`, `internal/cli/workspace_cmd_test.go`

**Interfaces:**
- Consumes:
  - From Task 12: `DetectWorkspaces`, `Workspace.RootPM`, `relSlash`, `isFile`, `uvID`, `writeTree` (test helper in `detect_test.go`) and `Filter`.
  - `scripts.LoadAllScripts(dir string, prefer []string) (*scripts.MergedScripts, error)` and `(*MergedScripts).GetScript(name) (*ScriptDefinition, bool)`. These are read-only use; internal/scripts is not edited.
  - `cfg.Workspace.Include/Exclude/Parallel` and `cfg.Scripts.Prefer` (config owned by P4).
  - The cli test helpers `chdir`, `withConfig`, `captureStdout` and `captureStderr`.
- Produces (package `workspace`):
  - `type Command struct { Dir, Name string; Args, Env []string }` and `func (c Command) String() string`, which renders as `"[ENV=.. ]name args..."`.
  - `type Runner func(ctx context.Context, c Command, stdout, stderr io.Writer) error`. This is the seam that replaces the `func(ctx, dir, stdout, stderr, name, args...)` shape in the dispatch. It is the same seam with `Env` added, because Go installs need `GOWORK=off`.
  - `func ExecRunner(ctx context.Context, c Command, stdout, stderr io.Writer) error`: `exec.CommandContext` with `cmd.Dir = c.Dir`.
  - `type InstallOptions struct { Parallel bool; Runner Runner; LookPath func(string) (string, error); Stdout, Stderr io.Writer }`
  - `func Install(workspaces []Workspace, opts InstallOptions) error`
  - `type RunOptions struct { Parallel bool; Prefer []string; Executable string; Runner Runner; Stdout, Stderr io.Writer }`
  - `func Run(workspaces []Workspace, task string, opts RunOptions) error`
  - Removed: `InstallWorkspaces`, `RunInWorkspaces`, `installProject`, `runInProject` and their sequential/parallel helpers. Their only caller was `workspace_cmd.go`.
- Produces (package `cli`):
  - `func cmdWorkspaces(args []string) int`, `func cmdRunWorkspace(task string) int` and `func cmdInstallWorkspace(global bool) int`. The signatures are unchanged.
  - Seams `workspaceRunner workspace.Runner`, `workspaceLookPath func(string) (string, error)` and `workspaceExecutable string`.
  - `func loadWorkspaces() ([]workspace.Workspace, error)`
  - `cmdInstallWorkspace` has **no** `nolint:unused`/`lint:ignore` markers. This follows the header ruling. Verified: golangci-lint runs with tests enabled by default (`run.tests: true`), and `unused` counts the reference from `workspace_cmd_test.go`. With the test file present, lint reports `0 issues.`. With it removed, lint flags the seam that only `cmdInstallWorkspace` reads. P7 wires `--workspace` in `install.go` with one line: `return cmdInstallWorkspace(global)`.

**Install commands** (none of them builds anything):

| Workspace / project | Where | Command |
|---|---|---|
| node workspace (`RootPM` npm/yarn/pnpm/bun) | workspace root, once | `<npm\|yarn\|pnpm\|bun> install` |
| Cargo workspace | workspace root, once | `cargo fetch` |
| Maven reactor | workspace root, once | `mvn -q dependency:resolve` |
| Go module | each module | `GOWORK=off go mod download` |
| Python with `uv.lock` | each project | `uv sync` |
| Python with poetry.lock / `[tool.poetry]` | each project | `poetry install` |
| Python with Pipfile(.lock) | each project | `pipenv install` |
| Python, pip, with `requirements.txt` | each project | `pip install -r requirements.txt` |
| Python, pip, no `requirements.txt` | — | skipped, with the stderr note `[name] skipped: nothing to install for pip` |
| Composer | each project | `composer install` |
| Gradle subproject | each project | `gradle -q dependencies` |

- [ ] **Step 1: Write the failing tests**

Create `internal/workspace/exec_test.go`. It contains a recording fake `Runner`, the grep test for `os.Chdir`, and the no-interleave test:

```go
package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// recorder is a fake Runner that records every command, prints
// "<label>-1" and "<label>-2" lines (yielding in between, to provoke
// interleaving) and fails for the directories in fail.
type recorder struct {
	mu   sync.Mutex
	cmds []Command
	fail map[string]bool
}

func (r *recorder) run(_ context.Context, c Command, stdout, _ io.Writer) error {
	r.mu.Lock()
	r.cmds = append(r.cmds, c)
	r.mu.Unlock()
	name := filepath.Base(c.Dir)
	_, _ = fmt.Fprintf(stdout, "%s-1\n", name)
	runtime.Gosched() // let other projects write in between
	_, _ = fmt.Fprintf(stdout, "%s-2\n", name)
	if r.fail[c.Dir] {
		return errors.New("exit status 1")
	}
	return nil
}

// lines renders recorded commands as "<dir relative to root>: <command>",
// in the order they ran.
func (r *recorder) lines(root string) []string {
	var out []string
	for _, c := range r.cmds {
		out = append(out, relSlash(root, c.Dir)+": "+c.String())
	}
	return out
}

func TestNoChdirInWorkspacePackage(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "os.Chdir") {
			t.Errorf("%s calls os.Chdir; use Command.Dir", f)
		}
	}
}

func TestExecuteParallelDoesNotInterleave(t *testing.T) {
	rec := &recorder{}
	var steps []step
	for _, n := range []string{"a", "b", "c", "d"} {
		steps = append(steps, step{label: n, cmd: Command{Dir: filepath.Join("/w", n), Name: "x"}})
	}
	var out strings.Builder
	if err := execute(context.Background(), steps, execOptions{parallel: true, run: rec.run, stdout: &out, stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b", "c", "d"} {
		block := fmt.Sprintf("[%s] $ x\n%s-1\n%s-2\n", n, n, n)
		if !strings.Contains(out.String(), block) {
			t.Errorf("output lacks the contiguous block %q:\n%s", block, out.String())
		}
	}
}
```

Create `internal/workspace/install_test.go`:

```go
package workspace

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func installFixture(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		// node: pnpm workspace, installed once at the root
		"package.json":            `{"name": "acme", "private": true, "workspaces": ["packages/*"]}`,
		"pnpm-lock.yaml":          "lockfileVersion: '9.0'\n",
		"packages/a/package.json": `{"name": "a"}`,
		"packages/b/package.json": `{"name": "b"}`,
		// python: per project, command chosen by its files
		"apps/etl/pyproject.toml":  "[project]\nname = \"etl\"\n",
		"apps/etl/uv.lock":         "version = 1\n",
		"apps/ml/pyproject.toml":   "[project]\nname = \"ml\"\n",
		"apps/ml/requirements.txt": "numpy==2.1.0\n",
		"apps/bare/pyproject.toml": "[project]\nname = \"bare\"\n",
		// rust: Cargo workspace, fetched once at the root
		"Cargo.toml":          "[workspace]\nmembers = [\"crates/*\"]\n",
		"crates/x/Cargo.toml": "[package]\nname = \"x\"\n",
		// go: per module
		"go.work":           "go 1.22\n\nuse (\n\t./svc/api\n\t./svc/worker\n)\n",
		"svc/api/go.mod":    "module example.com/api\n",
		"svc/worker/go.mod": "module example.com/worker\n",
		// java: Maven reactor, resolved once at the root
		"pom.xml":      "<project><modules><module>core</module></modules></project>",
		"core/pom.xml": "<project><artifactId>core</artifactId></project>",
		// php: per project
		"packages/php-lib/composer.json": `{"name": "acme/php-lib"}`,
	})
}

func TestInstallCommandsAndDirs(t *testing.T) {
	root := installFixture(t)
	wd, _ := os.Getwd()
	all, err := DetectWorkspaces(root)
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	var stderr strings.Builder
	err = Install(all, InstallOptions{
		Runner:   rec.run,
		LookPath: func(f string) (string, error) { return "/usr/bin/" + f, nil },
		Stdout:   io.Discard,
		Stderr:   &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		".: pnpm install",
		"apps/etl: uv sync",
		"apps/ml: pip install -r requirements.txt",
		".: cargo fetch",
		"svc/api: GOWORK=off go mod download",
		"svc/worker: GOWORK=off go mod download",
		".: mvn -q dependency:resolve",
		"packages/php-lib: composer install",
	}
	if got := rec.lines(root); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(stderr.String(), "[bare] skipped: nothing to install for pip") {
		t.Errorf("stderr = %q, want a skip note for apps/bare", stderr.String())
	}
	if now, _ := os.Getwd(); now != wd {
		t.Errorf("working directory changed from %s to %s", wd, now)
	}
}

func TestInstallMissingToolFailsOnlyThatProject(t *testing.T) {
	root := installFixture(t)
	all, err := DetectWorkspaces(root)
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{fail: map[string]bool{filepath.Join(root, "svc", "worker"): true}}
	err = Install(all, InstallOptions{
		Parallel: true,
		Runner:   rec.run,
		LookPath: func(f string) (string, error) {
			if f == "composer" {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + f, nil
		},
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	for _, s := range []string{
		"[acme/php-lib] composer is not installed",
		"[example.com/worker] GOWORK=off go mod download: exit status 1",
	} {
		if !strings.Contains(msg, s) {
			t.Errorf("error %q lacks %q", msg, s)
		}
	}
	if len(rec.cmds) != 7 {
		t.Errorf("ran %d commands, want 7 (all but composer)", len(rec.cmds))
	}
}

func TestInstallNothingToDo(t *testing.T) {
	ws := []Workspace{{Root: "/r", Ecosystem: "python", Projects: []Project{{Name: "p", Path: t.TempDir(), PM: "pip"}}}}
	err := Install(ws, InstallOptions{Runner: (&recorder{}).run, Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || err.Error() != "no workspace projects to install" {
		t.Fatalf("err = %v", err)
	}
}
```

Create `internal/workspace/run_test.go`:

```go
package workspace

import (
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func runFixture(t *testing.T) []Workspace {
	t.Helper()
	root := writeTree(t, map[string]string{
		"package.json":            `{"workspaces": ["packages/*"]}`,
		"packages/a/package.json": `{"name": "a", "scripts": {"build": "tsc", "test": "vitest"}}`,
		"packages/b/package.json": `{"name": "b", "scripts": {"test": "vitest"}}`,
		"packages/c/package.json": `{"name": "c", "scripts": {"build": "tsc"}}`,
		// packages/c is also a PHP package: it must run once, not twice
		"packages/c/composer.json": `{"name": "acme/c", "scripts": {"build": "make"}}`,
	})
	all, err := DetectWorkspaces(root)
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func TestRunReexecsPerProjectAndSkipsMissingTask(t *testing.T) {
	all := runFixture(t)
	root := all[0].Root
	rec := &recorder{}
	var stdout, stderr strings.Builder
	err := Run(all, "build", RunOptions{Executable: "/opt/xpm", Runner: rec.run, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"packages/a: /opt/xpm run build", "packages/c: /opt/xpm run build"}
	if got := rec.lines(root); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
	if got := stderr.String(); got != "[b] skipped: no task \"build\"\n" {
		t.Errorf("stderr = %q", got)
	}
	if !strings.HasPrefix(stdout.String(), "[a] $ /opt/xpm run build\na-1\na-2\n") {
		t.Errorf("stdout = %q, want the [a] header then its streamed output", stdout.String())
	}
}

func TestRunNoProjectHasTask(t *testing.T) {
	err := Run(runFixture(t), "deploy", RunOptions{Executable: "/opt/xpm", Runner: (&recorder{}).run, Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || err.Error() != `no workspace project defines task "deploy"` {
		t.Fatalf("err = %v", err)
	}
}

func TestRunAggregatesFailures(t *testing.T) {
	all := runFixture(t)
	root := all[0].Root
	rec := &recorder{fail: map[string]bool{filepath.Join(root, "packages", "a"): true}}
	err := Run(all, "test", RunOptions{Parallel: true, Executable: "/opt/xpm", Runner: rec.run, Stdout: io.Discard, Stderr: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "[a] /opt/xpm run test: exit status 1") {
		t.Fatalf("err = %v", err)
	}
	if len(rec.cmds) != 2 {
		t.Errorf("ran %d projects, want 2 (a failing does not stop b)", len(rec.cmds))
	}
}
```

Create `internal/cli/workspace_cmd_test.go`:

```go
package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/workspace"
)

// wsCall is one command a workspace operation ran: its directory relative
// to the workspace root and its command line.
type wsCall struct{ Dir, Cmd string }

// workspaceTree writes a small npm workspace into a temp dir, makes it the
// working directory and returns its symlink-free path.
func workspaceTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"package.json":            `{"name": "acme", "private": true, "workspaces": ["packages/*"]}`,
		"package-lock.json":       `{"lockfileVersion": 3}`,
		"packages/a/package.json": `{"name": "a", "scripts": {"build": "tsc"}}`,
		"packages/b/package.json": `{"name": "b", "scripts": {"build": "tsc"}}`,
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, root)
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// withWorkspaceRunner records every command instead of running it; commands
// whose directory ends in failDir fail.
func withWorkspaceRunner(t *testing.T, root, failDir string) *[]wsCall {
	t.Helper()
	var (
		mu    sync.Mutex
		calls []wsCall
	)
	oldRunner, oldLook, oldExe := workspaceRunner, workspaceLookPath, workspaceExecutable
	workspaceRunner = func(_ context.Context, c workspace.Command, _, _ io.Writer) error {
		dir, err := filepath.EvalSymlinks(c.Dir)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, dir)
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, wsCall{filepath.ToSlash(rel), c.String()})
		if failDir != "" && filepath.Base(dir) == failDir {
			return errors.New("exit status 2")
		}
		return nil
	}
	workspaceLookPath = func(f string) (string, error) { return "/usr/bin/" + f, nil }
	workspaceExecutable = "/opt/xpm"
	t.Cleanup(func() { workspaceRunner, workspaceLookPath, workspaceExecutable = oldRunner, oldLook, oldExe })
	return &calls
}

func TestCmdWorkspacesAppliesIncludeExclude(t *testing.T) {
	workspaceTree(t)
	withConfig(t, config.Config{Workspace: config.WorkspaceConfig{Include: []string{"packages/*"}, Exclude: []string{"packages/b"}}})
	var code int
	out := captureStdout(t, func() { code = cmdWorkspaces(nil) })
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "packages/a") || strings.Contains(out, "packages/b") {
		t.Fatalf("output = %q, want packages/a only", out)
	}
}

func TestCmdRunWorkspaceReexecsInEachProject(t *testing.T) {
	root := workspaceTree(t)
	withConfig(t, config.Config{})
	calls := withWorkspaceRunner(t, root, "")
	var code int
	captureStdout(t, func() { code = cmdRunWorkspace("build") })
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	want := []wsCall{{"packages/a", "/opt/xpm run build"}, {"packages/b", "/opt/xpm run build"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %v, want %v", *calls, want)
	}
}

func TestCmdRunWorkspaceFailureExitsNonZero(t *testing.T) {
	root := workspaceTree(t)
	withConfig(t, config.Config{Workspace: config.WorkspaceConfig{Parallel: true}})
	calls := withWorkspaceRunner(t, root, "a")
	var code int
	errOut := captureStderr(t, func() {
		captureStdout(t, func() { code = cmdRunWorkspace("build") })
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if len(*calls) != 2 || !strings.Contains(errOut, "[a] /opt/xpm run build: exit status 2") {
		t.Fatalf("calls = %v, stderr = %q", *calls, errOut)
	}
}

func TestCmdInstallWorkspaceInstallsNodeRootOnce(t *testing.T) {
	root := workspaceTree(t)
	withConfig(t, config.Config{})
	calls := withWorkspaceRunner(t, root, "")
	var code int
	captureStdout(t, func() { code = cmdInstallWorkspace(false) })
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if want := []wsCall{{".", "npm install"}}; !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %v, want %v", *calls, want)
	}
}

func TestCmdInstallWorkspaceRejectsGlobal(t *testing.T) {
	root := workspaceTree(t)
	withConfig(t, config.Config{})
	calls := withWorkspaceRunner(t, root, "")
	var code int
	errOut := captureStderr(t, func() { code = cmdInstallWorkspace(true) })
	if code != 1 || len(*calls) != 0 || !strings.Contains(errOut, "--global cannot be combined with --workspace") {
		t.Fatalf("exit = %d, calls = %v, stderr = %q", code, *calls, errOut)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/workspace/ ./internal/cli/`
Expected: FAIL. The build fails with `undefined: Command`, `undefined: execute`, `undefined: InstallOptions`, `undefined: RunOptions` and `undefined: workspaceRunner`. `TestNoChdirInWorkspacePackage` would also fail on the old `install.go` and `run.go`.

- [ ] **Step 3: Add the command seam and executor**

Create `internal/workspace/exec.go`:

```go
package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// Command is one process to run in a project directory.
type Command struct {
	Dir  string   // working directory, set as cmd.Dir (xpm never changes its own)
	Name string   // executable
	Args []string // arguments
	Env  []string // extra KEY=VALUE pairs on top of the current environment
}

// String renders the command line for headers and error messages.
func (c Command) String() string {
	return strings.TrimSpace(strings.Join(append(append([]string{}, c.Env...), append([]string{c.Name}, c.Args...)...), " "))
}

// Runner runs c with its output sent to stdout and stderr. Tests inject a
// fake; nil means ExecRunner.
type Runner func(ctx context.Context, c Command, stdout, stderr io.Writer) error

// ExecRunner runs c with exec.CommandContext in c.Dir.
func ExecRunner(ctx context.Context, c Command, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	return cmd.Run()
}

// step is one labelled command of a workspace operation.
type step struct {
	label string // project name (or the workspace root's name)
	cmd   Command
	err   error // set instead of cmd when the step cannot run (e.g. tool missing)
}

// execOptions are the shared knobs of Install and Run.
type execOptions struct {
	parallel       bool
	run            Runner
	stdout, stderr io.Writer
}

// execute runs steps and returns their failures joined, in step order.
// Sequential runs stream each step's output after a "[label] $ cmd" header.
// Parallel runs (at most GOMAXPROCS at a time) buffer each step's output and
// print the header plus that output in one piece when the step finishes, so
// projects never interleave.
func execute(ctx context.Context, steps []step, o execOptions) error {
	if o.run == nil {
		o.run = ExecRunner
	}
	if o.stdout == nil {
		o.stdout = os.Stdout
	}
	if o.stderr == nil {
		o.stderr = os.Stderr
	}
	errs := make([]error, len(steps))
	runStep := func(i int, stdout, stderr io.Writer) {
		s := steps[i]
		if s.err != nil {
			errs[i] = fmt.Errorf("[%s] %w", s.label, s.err)
			return
		}
		if err := o.run(ctx, s.cmd, stdout, stderr); err != nil {
			errs[i] = fmt.Errorf("[%s] %s: %w", s.label, s.cmd, err)
		}
	}
	header := func(s step) string { return fmt.Sprintf("[%s] $ %s\n", s.label, s.cmd) }

	if !o.parallel {
		for i, s := range steps {
			if s.err == nil {
				_, _ = io.WriteString(o.stdout, header(s))
			}
			runStep(i, o.stdout, o.stderr)
		}
		return errors.Join(errs...)
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, runtime.GOMAXPROCS(0))
	)
	for i := range steps {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var out, errOut bytes.Buffer
			runStep(i, &out, &errOut)
			if steps[i].err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			_, _ = io.WriteString(o.stdout, header(steps[i]))
			_, _ = o.stdout.Write(out.Bytes())
			_, _ = o.stderr.Write(errOut.Bytes())
		}(i)
	}
	wg.Wait()
	return errors.Join(errs...)
}
```

- [ ] **Step 4: Replace Install and Run**

Replace `internal/workspace/install.go`:

```go
package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/crenspire/xpm/internal/pm"
)

// InstallOptions configures Install.
type InstallOptions struct {
	Parallel bool
	Runner   Runner                            // nil = ExecRunner
	LookPath func(file string) (string, error) // nil = exec.LookPath
	Stdout   io.Writer                         // nil = os.Stdout
	Stderr   io.Writer                         // nil = os.Stderr
}

// Install installs the dependencies of every workspace, in order. A workspace
// with a RootPM (npm/yarn/pnpm/bun, Cargo, Maven) is installed once at its
// root; every other project is installed in its own directory. Nothing is
// built. A project whose tool is not on PATH fails on its own; the others
// still run, and all failures are returned joined.
func Install(workspaces []Workspace, opts InstallOptions) error {
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	var steps []step
	for _, ws := range workspaces {
		if len(ws.Projects) == 0 {
			continue
		}
		if ws.RootPM != "" {
			label := ws.Ecosystem + " workspace"
			cmd, ok := installCommand(ws.RootPM, ws.Root)
			if !ok {
				_, _ = fmt.Fprintf(stderr, "[%s] skipped: no install command for %s\n", label, ws.RootPM)
				continue
			}
			steps = append(steps, checkTool(step{label: label, cmd: cmd}, opts.LookPath))
			continue
		}
		for _, p := range ws.Projects {
			cmd, ok := installCommand(p.PM, p.Path)
			if !ok {
				_, _ = fmt.Fprintf(stderr, "[%s] skipped: nothing to install for %s\n", p.Name, p.PM)
				continue
			}
			steps = append(steps, checkTool(step{label: p.Name, cmd: cmd}, opts.LookPath))
		}
	}
	if len(steps) == 0 {
		return errors.New("no workspace projects to install")
	}
	return execute(context.Background(), steps, execOptions{
		parallel: opts.Parallel, run: opts.Runner, stdout: opts.Stdout, stderr: stderr,
	})
}

// checkTool turns a step whose executable is not on PATH into a failed step.
func checkTool(s step, lookPath func(string) (string, error)) step {
	if _, err := lookPath(s.cmd.Name); err != nil {
		s.err = fmt.Errorf("%s is not installed (needed for: %s)", s.cmd.Name, s.cmd)
	}
	return s
}

// installCommand returns the command that downloads id's dependencies for
// the project or workspace root in dir without building anything. ok is
// false when there is nothing to run (a pip project without
// requirements.txt, or an unknown manager).
func installCommand(id pm.ID, dir string) (cmd Command, ok bool) {
	c := func(name string, args ...string) (Command, bool) {
		return Command{Dir: dir, Name: name, Args: args}, true
	}
	switch id {
	case pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun:
		meta, _ := pm.MetaFor(id)
		return c(meta.Binary, "install")
	case uvID:
		return c("uv", "sync")
	case pm.Poetry:
		return c("poetry", "install")
	case pm.Pipenv:
		return c("pipenv", "install")
	case pm.Pip:
		if !isFile(filepath.Join(dir, "requirements.txt")) {
			return Command{}, false
		}
		return c("pip", "install", "-r", "requirements.txt")
	case pm.Composer:
		return c("composer", "install")
	case pm.Cargo:
		return c("cargo", "fetch")
	case pm.GoMod:
		// GOWORK=off: download this module's own requirements even when a
		// go.work encloses it.
		cmd := Command{Dir: dir, Name: "go", Args: []string{"mod", "download"}, Env: []string{"GOWORK=off"}}
		return cmd, true
	case pm.Maven:
		return c("mvn", "-q", "dependency:resolve")
	case pm.Gradle:
		return c("gradle", "-q", "dependencies")
	}
	return Command{}, false
}
```

Replace `internal/workspace/run.go`:

```go
package workspace

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/crenspire/xpm/internal/scripts"
)

// RunOptions configures Run.
type RunOptions struct {
	Parallel   bool
	Prefer     []string  // scripts.prefer: which config file wins a name clash
	Executable string    // binary re-executed per project; "" = os.Executable()
	Runner     Runner    // nil = ExecRunner
	Stdout     io.Writer // nil = os.Stdout
	Stderr     io.Writer // nil = os.Stderr
}

// Run runs task in every project that defines it by re-executing xpm as
// `<exe> run <task>` with the project as working directory. Projects without
// the task are skipped with a note on stderr; it is an error when no project
// has it. A directory listed by several ecosystems runs once. Failures are
// returned joined, in project order.
func Run(workspaces []Workspace, task string, opts RunOptions) error {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	exe := opts.Executable
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return fmt.Errorf("cannot locate the xpm binary: %w", err)
		}
	}
	seen := map[string]bool{}
	var steps []step
	for _, ws := range workspaces {
		for _, p := range ws.Projects {
			if seen[p.Path] {
				continue
			}
			seen[p.Path] = true
			merged, err := scripts.LoadAllScripts(p.Path, opts.Prefer)
			if err != nil {
				steps = append(steps, step{label: p.Name, err: fmt.Errorf("loading tasks: %w", err)})
				continue
			}
			if _, found := merged.GetScript(task); !found {
				_, _ = fmt.Fprintf(stderr, "[%s] skipped: no task %q\n", p.Name, task)
				continue
			}
			steps = append(steps, step{label: p.Name, cmd: Command{Dir: p.Path, Name: exe, Args: []string{"run", task}}})
		}
	}
	if len(steps) == 0 {
		return fmt.Errorf("no workspace project defines task %q", task)
	}
	return execute(context.Background(), steps, execOptions{
		parallel: opts.Parallel, run: opts.Runner, stdout: opts.Stdout, stderr: stderr,
	})
}
```

- [ ] **Step 5: Replace the CLI commands**

Replace `internal/cli/workspace_cmd.go`. If `cmdGraphWorkspace` is still in this file, read the note under **Files** first.

```go
package cli

import (
	"fmt"
	"os"

	"github.com/crenspire/xpm/internal/workspace"
)

// Test seams for workspace commands: nil/empty means the real thing.
var (
	workspaceRunner     workspace.Runner
	workspaceLookPath   func(string) (string, error)
	workspaceExecutable string
)

// loadWorkspaces detects the workspaces under the current directory and
// applies workspace.include / workspace.exclude from the config.
func loadWorkspaces() ([]workspace.Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	all, err := workspace.DetectWorkspaces(cwd)
	if err != nil {
		return nil, fmt.Errorf("detecting workspaces: %w", err)
	}
	return workspace.Filter(all, cfg.Workspace.Include, cfg.Workspace.Exclude), nil
}

// cmdWorkspaces lists detected workspaces.
func cmdWorkspaces(_ []string) int {
	workspaces, err := loadWorkspaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println(workspace.FormatWorkspaces(workspaces))
	return 0
}

// cmdInstallWorkspace installs dependencies in all workspace projects.
func cmdInstallWorkspace(global bool) int {
	if global {
		fmt.Fprintln(os.Stderr, "error: --global cannot be combined with --workspace")
		return 1
	}
	workspaces, err := loadWorkspaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "No workspaces detected.")
		return 1
	}
	err = workspace.Install(workspaces, workspace.InstallOptions{
		Parallel: cfg.Workspace.Parallel,
		Runner:   workspaceRunner,
		LookPath: workspaceLookPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println("\n✓ All workspace installations completed")
	return 0
}

// cmdRunWorkspace runs a task across all workspace projects.
func cmdRunWorkspace(task string) int {
	workspaces, err := loadWorkspaces()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "No workspaces detected.")
		return 1
	}
	err = workspace.Run(workspaces, task, workspace.RunOptions{
		Parallel:   cfg.Workspace.Parallel,
		Prefer:     cfg.Scripts.Prefer,
		Executable: workspaceExecutable,
		Runner:     workspaceRunner,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println("\n✓ All workspace tasks completed")
	return 0
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test -count=1 -race ./internal/workspace/ ./internal/cli/`
Expected:
```
ok  	github.com/crenspire/xpm/internal/workspace
ok  	github.com/crenspire/xpm/internal/cli
```

Run: `grep -rn 'os.Chdir' internal/workspace --include='*.go' | grep -v _test.go`
Expected: no output.

- [ ] **Step 7: Full verification**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: all `ok`, no gofmt output, and `0 issues.`
Optional: `GOTOOLCHAIN=go1.22.12 go vet ./... && GOTOOLCHAIN=go1.22.12 go test ./internal/workspace/`. Expected: `ok`. On macOS 26, Go 1.22 test binaries that link the cli package abort with `dyld: missing LC_UUID`. This happens on an unmodified tree as well, so treat `go vet` as the 1.22 check for `internal/cli`.

- [ ] **Step 8: Commit**

```bash
git add internal/workspace/exec.go internal/workspace/install.go internal/workspace/run.go \
  internal/workspace/exec_test.go internal/workspace/install_test.go internal/workspace/run_test.go \
  internal/cli/workspace_cmd.go internal/cli/workspace_cmd_test.go
git commit -m "workspace: run and install via cmd.Dir (no os.Chdir), install node/cargo/maven roots once, run tasks by re-exec, buffered parallel output"
```

---

### Rulings (Tasks 12–13)

- Ruling: `DetectWorkspaces` returns one workspace per ecosystem, in the fixed order node, python, rust, go, java, php. The `"mixed"` merge is dropped. — Why: a merged workspace loses each ecosystem's `Root` and `RootPM`, which the Node, Cargo and Maven root installs need, and the listing already grouped output by ecosystem. — Cost if wrong: a caller that expected exactly one workspace sees several. Task 7 iterates `ws.Projects` for every workspace, so it is unaffected.
- Ruling: walks and wildcards skip `node_modules`, `vendor`, `venv`, `target`, `testdata`, `__pycache__` and every hidden directory (`.git`, `.venv`, ...). Depth is capped at 6 segments. A literal pattern segment such as `vendor/acme/*` still enters the directory. `build` and `dist` are **not** skipped: `packages/build` is a real package name in some monorepos, and npm's own workspace globbing does not skip them either. `target` is skipped because `cargo package` copies member manifests into `target/package/`. `testdata` is skipped because the Go toolchain ignores it and xpm's own graph fixtures contain `go.mod` files. — Cost if wrong: a project nested deeper than 6 levels, or inside a skipped name, is not listed.
- Ruling: a `!pattern` subtracts its matches from the union of the positive patterns, whatever its position in the list. This is not npm's ordered re-inclusion. A Cargo `exclude` entry also excludes everything below that path. — Cost if wrong: a re-include after a negation (`"!a/**", "a/keep"`) is ignored. This is rare.
- Ruling: the Node root manager comes from `pnpm-workspace.yaml` (pnpm), otherwise from pm's lock-file table order (package-lock.json, yarn.lock, pnpm-lock.yaml, bun.lock, bun.lockb), otherwise npm. Members inherit the root's manager and lock file. This also fixes the P3-deferred nondeterminism: the old code ranged over a map in `detectNodePM`, `detectPythonPM` and `installProject`. — Cost if wrong: a repo with two root lock files installs with npm when the user meant yarn. That is already xpm's single-project rule.
- Ruling: installs never build. Node and Cargo workspaces and Maven reactors install once at the root. Go uses `GOWORK=off go mod download` per module, so each module's own requirements are fetched even inside a go.work. Gradle uses `gradle -q dependencies` per subproject, which resolves its configurations. Maven uses `mvn -q dependency:resolve` at the root. — Cost if wrong: in Maven 3, a module that depends on an unbuilt sibling fails `dependency:resolve`. The user then runs `mvn install` themselves, as before. The old code ran `mvn install`, `cargo build` and `gradle build`, which build and test the project.
- Ruling: uv has no `pm.ID`, and internal/pm cannot be edited in P6. The workspace package uses a local `uvID pm.ID = "uv"`, used only to choose `uv sync`. — Cost if wrong: a P4 `pm.UV` constant later replaces one line.
- Ruling: pip projects without `requirements.txt` are skipped, with a stderr note. The alternative, `pip install -e .`, installs the project itself into whatever environment is active. — Cost if wrong: a pure-PEP-621 project needs a manual install.
- Ruling: `Run` skips projects that lack the task, with a note on stderr `[name] skipped: no task "x"`. It errors only when no project has the task. A directory detected by two ecosystems runs once. Failures are joined with `errors.Join` in project order, and `cmdRunWorkspace` returns 1. — Cost if wrong: a typo in a task name that matches some project is not caught for the other projects.
- Ruling: parallel execution is bounded to `GOMAXPROCS` processes. Each project's stdout and stderr are buffered separately and flushed under a mutex when the project finishes, in completion order. Sequential execution streams. — Cost if wrong: one project's stdout and stderr lose their relative order in parallel mode.
- Ruling: `cmdInstallWorkspace(true)` is an error (`--global cannot be combined with --workspace`, exit 1). The old code printed a warning and ignored the flag. — Cost if wrong: P7 may prefer a warning, which is a one-line change.
- Ruling: `Filter` matches the root project only with `.` or a pattern made only of `**`, not with `*`. When the filter keeps any member of a Node, Cargo or Maven workspace, the whole workspace is still installed once at its root, because package managers do not install a subset of a workspace. — Cost if wrong: `workspace.exclude` does not narrow root installs. This is documented here, and Task 14 should say so in the README.
- Ruling: `cmdWorkspaces` ignores its `args`. There are no flags, and the include/exclude settings come from config. — Cost if wrong: none.

### Conflicts with the header / other tasks

- **Header vs dispatch on `nolint:unused`:** the dispatch says to keep the markers, and the header ruling says none. I followed the header. Lint was verified at `0 issues.` with the test file present, and it flags a seam when the test file is removed. Recommendation: keep the header as written.
- **Task 7 (consumer of `DetectWorkspaces`):** the signature is unchanged. Two behaviours change. (a) There may be several workspaces, one per ecosystem. (b) The same directory can appear in two of them, for example `packages/c` holding both `package.json` and `composer.json`. Task 7's workspace graph should skip a `project.Path` it has already extracted, or rely on node dedupe in `Merge`. It must also not call `workspace.GroupByEcosystem`, which is removed.
- **Runner seam shape:** the dispatch suggested `func(ctx, dir, stdout, stderr, name, args...)`. I used `func(ctx, Command, stdout, stderr)` so that `Env` (`GOWORK=off`) fits. This is not a header-fixed interface.
- **Task 14 (README):** the workspace section should state the ecosystem order, the skipped directories, the install command table above, the root-install rule and its interaction with include/exclude, run-by-re-exec, and that `xpm install --workspace` is wired in P7.

---

## Section D — Docs (Task 14)

### Task 14: README — graph, lock, workspaces, cache, doctor

**Files:**
- Modify: `README.md` — ONLY: the Status table rows `Diagnostics`, `Dependency graph`, `Unified lockfile`, `Monorepos`, `Global dependency cache`; the `### Diagnose` subsection; a NEW subsection `### Dependency graph, lockfile and monorepos (experimental)` inserted directly after `### Diagnose`. Do not touch any other README line (P4 owns the rest, including the Configuration table).

**Interfaces:**
- Consumes: the final behaviour of Tasks 1–13 (read the code, not this plan, for exact flags and messages).

- [ ] **Step 1: Collect the facts.** Run, in the worktree:

```bash
go build -o /tmp/xpm-p6 ./cmd/xpm
grep -n 'fs\.\(Bool\|String\|Int\)' internal/cli/graph_cmd.go internal/cli/lock_cmd.go internal/cli/workspace_cmd.go
grep -n 'Status\|"added"\|"missing"\|"changed"' internal/lock/*.go | head -40
```

Then exercise the commands against temp projects (never in the repo root; never run real package managers):

```bash
T=$(mktemp -d) && cp internal/graph/testdata/npm/v3/* "$T"/ && cd "$T"
/tmp/xpm-p6 graph | head; /tmp/xpm-p6 graph --json | python3 -m json.tool >/dev/null && echo JSON-OK
/tmp/xpm-p6 graph --depth 1; /tmp/xpm-p6 lock; /tmp/xpm-p6 lock; /tmp/xpm-p6 lock --verify; echo exit=$?
touch yarn.lock; /tmp/xpm-p6 lock --verify; echo exit=$?   # expect "added", exit 1
/tmp/xpm-p6 cache; echo exit=$?                              # expect removal message, exit 1
cd - && rm -rf "$T"
```

(Adjust the fixture directory name to whatever Task 3 created.)

- [ ] **Step 2: Edit the Status rows.**
  - `Diagnostics | doctor`: keep Stable; nothing else changes unless Task 11 changed the command surface.
  - `Dependency graph | graph`: `🧪 Experimental: parses npm, pnpm, yarn, Cargo, Go, Poetry, Composer, Maven and Gradle files; runs build tools only with --exec` (list only ecosystems Tasks 3–6 actually parse).
  - `Unified lockfile | lock`: `🧪 Experimental: records lockfile hashes in xpm-lock.yaml; --verify detects changed, added and removed lockfiles`.
  - `Monorepos | workspaces`, `run --workspace`: `🧪 Experimental` plus one honest clause (e.g. `xpm install --workspace is not wired yet`).
  - Delete the `Global dependency cache | cache` row.

- [ ] **Step 3: Extend `### Diagnose`** with one sentence each, only if true in code after Task 11: a security audit whose tool fails is reported as unavailable (never as passing); lockfile drift is judged by content, not file times.

- [ ] **Step 4: Add the new subsection** after `### Diagnose`, in this shape (every flag and message must match the code; delete any line that does not):

````markdown
### Dependency graph, lockfile and monorepos (experimental)

```bash
xpm graph                  # dependency tree from the lockfiles in this directory; repeats are marked (*)
xpm graph react            # only the subtree under react
xpm graph --depth 2        # limit tree depth (default: graph.depth, 5)
xpm graph --json > g.json  # machine-readable; stdout carries only the graph, warnings go to stderr
xpm graph --svg > g.svg    # needs GraphViz `dot`
xpm graph --exec           # also run mvn / gradle / go mod graph for full Java and Go trees
xpm lock                   # write xpm-lock.yaml (hashes of every lockfile in this directory)
xpm lock --verify          # exit 1 if a lockfile changed, appeared or disappeared since `xpm lock`
xpm workspaces             # list monorepo projects (npm/yarn/pnpm, Cargo, go.work, Poetry, Maven, Gradle, Composer)
xpm run --workspace test   # run `test` in every project that defines it
```

`xpm graph` reads files only; it never runs a build tool unless you pass `--exec`. `xpm-lock.yaml` has no timestamps, so running `xpm lock` again on an unchanged project leaves the file untouched. Workspace commands honour `workspace.include` / `workspace.exclude` (glob lists matched against each project's path relative to the workspace root, `**` allowed) and `workspace.parallel`.

The dependency cache (`xpm cache`) was removed: npm, pip, Cargo, Go and the others already keep their own caches.
````

- [ ] **Step 5: Verify every claim.** For each bullet/line added, point to the code or the test that proves it (write the mapping into your report). Remove or reword anything you cannot prove. `grep -n 'xpm cache\|cache gc\|cache clean' README.md` must only match the removal sentence.

- [ ] **Step 6: Gates + commit.**

```bash
go build ./... && go vet ./... && go test ./... && test -z "$(gofmt -l .)"
git add README.md
git commit -m "docs(readme): graph, lock, workspaces and doctor match P6 behaviour; cache removed"
```
