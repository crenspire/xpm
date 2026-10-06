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
