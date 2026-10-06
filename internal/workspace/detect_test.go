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
