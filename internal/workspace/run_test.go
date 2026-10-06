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
	want := []string{"packages/a: /opt/xpm run -- build", "packages/c: /opt/xpm run -- build"}
	if got := rec.lines(root); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
	if got := stderr.String(); got != "[b] skipped: no task \"build\"\n" {
		t.Errorf("stderr = %q", got)
	}
	if !strings.HasPrefix(stdout.String(), "[a] $ /opt/xpm run -- build\na-1\na-2\n") {
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
	if err == nil || !strings.Contains(err.Error(), "[a] /opt/xpm run -- test: exit status 1") {
		t.Fatalf("err = %v", err)
	}
	if len(rec.cmds) != 2 {
		t.Errorf("ran %d projects, want 2 (a failing does not stop b)", len(rec.cmds))
	}
}
