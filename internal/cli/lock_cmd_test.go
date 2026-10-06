package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lockCmdNpmLock = "{\"lockfileVersion\":3,\"packages\":{\"\":{},\"node_modules/a\":{\"version\":\"1.0.0\"}}}\n"

// lockCmdProject creates a temp project with the given files, chdirs into it
// and returns its path.
func lockCmdProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	return dir
}

func TestCmdLockGeneratesThenReportsUpToDate(t *testing.T) {
	dir := lockCmdProject(t, map[string]string{"package-lock.json": lockCmdNpmLock})

	var code int
	out := captureStdout(t, func() { code = cmdLock(nil) })
	if code != 0 {
		t.Fatalf("first cmdLock = %d, output:\n%s", code, out)
	}
	if !strings.HasPrefix(out, "Generated xpm-lock.yaml\n") {
		t.Fatalf("first run output:\n%s\nwant it to start with %q", out, "Generated xpm-lock.yaml")
	}
	if !strings.Contains(out, "  package-lock.json (npm): 1 packages\n") {
		t.Errorf("first run output lacks the package-lock.json line:\n%s", out)
	}
	before, err := os.ReadFile(filepath.Join(dir, "xpm-lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	out = captureStdout(t, func() { code = cmdLock(nil) })
	if code != 0 {
		t.Fatalf("second cmdLock = %d, output:\n%s", code, out)
	}
	if !strings.HasPrefix(out, "xpm-lock.yaml is up to date\n") {
		t.Fatalf("second run output:\n%s\nwant it to start with %q", out, "xpm-lock.yaml is up to date")
	}
	after, err := os.ReadFile(filepath.Join(dir, "xpm-lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("xpm-lock.yaml changed on an unchanged project:\n%s\n---\n%s", before, after)
	}
}

func TestCmdLockVerifyReportsAddedChangedMissingInPathOrder(t *testing.T) {
	dir := lockCmdProject(t, map[string]string{
		"package-lock.json": lockCmdNpmLock,
		"go.sum":            "example.com/m v1.0.0 h1:abc=\n",
		"poetry.lock":       "[[package]]\nname = \"a\"\nversion = \"1\"\n",
	})
	_ = captureStdout(t, func() { _ = cmdLock(nil) })

	var code int
	out := captureStdout(t, func() { code = cmdLock([]string{"--verify"}) })
	if code != 0 {
		t.Fatalf("verify on fresh lock = %d, output:\n%s", code, out)
	}
	if !strings.Contains(out, "Verification PASSED.") {
		t.Fatalf("output:\n%s\nwant PASSED", out)
	}

	if err := os.WriteFile(filepath.Join(dir, "poetry.lock"), []byte("[[package]]\nname = \"b\"\nversion = \"2\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "go.sum")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "yarn.lock"), []byte("a@^1.0.0:\n  version \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out = captureStdout(t, func() { code = cmdLock([]string{"--verify"}) })
	if code != 1 {
		t.Fatalf("verify after edits = %d, want 1; output:\n%s", code, out)
	}
	want := "✘ go.sum missing\n" +
		"✔ package-lock.json unchanged\n" +
		"✘ poetry.lock changed\n" +
		"✘ yarn.lock added (not in xpm-lock.yaml)\n" +
		"\n" +
		"Verification FAILED.\n" +
		"Run 'xpm lock' to update xpm-lock.yaml.\n"
	if out != want {
		t.Fatalf("verify output:\n%s\nwant:\n%s", out, want)
	}
}

func TestCmdLockVerifyWithoutLockFileFails(t *testing.T) {
	lockCmdProject(t, map[string]string{"package-lock.json": lockCmdNpmLock})
	var code int
	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() { code = cmdLock([]string{"--verify"}) })
	})
	if code != 1 {
		t.Fatalf("cmdLock --verify = %d, want 1", code)
	}
	if !strings.Contains(errOut, "xpm-lock.yaml not found") {
		t.Fatalf("stderr = %q, want a not-found message", errOut)
	}
}

func TestCmdLockVerifyReportsUnsafePathAsError(t *testing.T) {
	lockCmdProject(t, map[string]string{
		"xpm-lock.yaml": "version: 2\nlocks:\n  ../escape.lock:\n    file: ../escape.lock\n    hash: 00\n",
	})
	var code int
	out := captureStdout(t, func() { code = cmdLock([]string{"--verify"}) })
	if code != 1 {
		t.Fatalf("cmdLock --verify = %d, want 1; output:\n%s", code, out)
	}
	if !strings.Contains(out, "✘ ../escape.lock error: ") || !strings.Contains(out, "outside the project") {
		t.Fatalf("output:\n%s\nwant an error line for ../escape.lock", out)
	}
}

func TestCmdLockWarningsGoToStderr(t *testing.T) {
	lockCmdProject(t, map[string]string{"package-lock.json": "{ not json"})
	var code int
	var out string
	errOut := captureStderr(t, func() {
		out = captureStdout(t, func() { code = cmdLock(nil) })
	})
	if code != 0 {
		t.Fatalf("cmdLock = %d, want 0", code)
	}
	if !strings.Contains(errOut, "warning: package-lock.json:") {
		t.Errorf("stderr = %q, want a package-lock.json warning", errOut)
	}
	if strings.Contains(out, "warning") {
		t.Errorf("stdout carries a warning:\n%s", out)
	}
}

func TestCmdLockWarnsAboutStaleLockWhenNoLockfilesRemain(t *testing.T) {
	dir := lockCmdProject(t, map[string]string{
		"xpm-lock.yaml": "version: 2\nlocks:\n  package-lock.json:\n    file: package-lock.json\n    hash: 00\n",
	})
	var code int
	var out string
	errOut := captureStderr(t, func() {
		out = captureStdout(t, func() { code = cmdLock(nil) })
	})
	if code != 0 {
		t.Fatalf("cmdLock = %d, want 0", code)
	}
	if !strings.Contains(out, "No lockfiles found") {
		t.Errorf("stdout = %q, want the no-lockfiles message", out)
	}
	if !strings.Contains(errOut, "warning: xpm-lock.yaml is stale") {
		t.Errorf("stderr = %q, want a stale xpm-lock.yaml warning", errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "xpm-lock.yaml")); err != nil {
		t.Errorf("xpm-lock.yaml was removed: %v", err)
	}
}

func TestCmdLockNoLockfilesAndNoLockIsQuiet(t *testing.T) {
	lockCmdProject(t, nil)
	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() { _ = cmdLock(nil) })
	})
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing", errOut)
	}
}
