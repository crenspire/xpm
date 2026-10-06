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
