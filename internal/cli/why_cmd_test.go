package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func runWhy(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() { code = cmdWhy(args) })
	})
	return code, stdout, stderr
}

func TestWhyPrintsPaths(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	code, stdout, stderr := runWhy(t, "ms")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
	want := "ms@2.0.0 (node)\n  demo@1.0.0 > debug@2.6.9 > ms@2.0.0\n" +
		"ms@2.1.3 (node)\n  demo@1.0.0 > ms@2.1.3\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	// Flags may follow the package.
	if code, out2, _ := runWhy(t, "ms", "--limit", "5"); code != 0 || out2 != want {
		t.Errorf("flags after positional: exit %d out %q", code, out2)
	}
}

func TestWhyTruncated(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	code, stdout, _ := runWhy(t, "--limit", "1", "ms")
	if code != 0 || !strings.HasSuffix(stdout, "  … more paths (use --limit 0 to show all)\n") {
		t.Fatalf("exit %d stdout %q", code, stdout)
	}
	if code, stdout, _ := runWhy(t, "--limit", "0", "ms"); code != 0 || strings.Contains(stdout, "more paths") {
		t.Fatalf("exit %d stdout %q", code, stdout)
	}
}

func TestWhyNotFound(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	code, stdout, stderr := runWhy(t, "nope")
	if code != 1 || stdout != "" || !strings.Contains(stderr, `error: package "nope" not found in the dependency graph`) {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestWhyUsageErrors(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	for _, args := range [][]string{{}, {"a", "b"}, {"ms", "--limit", "-1"}, {"ms", "--limit", "x"}, {"ms", "--bogus"}} {
		if code, stdout, _ := runWhy(t, args...); code != 2 || stdout != "" {
			t.Errorf("%v: exit %d stdout %q", args, code, stdout)
		}
	}
}

func TestWhyJSON(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	code, stdout, stderr := runWhy(t, "--json", "debug")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
	var out struct {
		Package   string
		Paths     [][]string
		Truncated bool
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	if out.Package != "debug" || out.Truncated || len(out.Paths) != 1 ||
		strings.Join(out.Paths[0], ",") != "node:demo@1.0.0,node:debug@2.6.9" {
		t.Errorf("out = %+v", out)
	}
}
