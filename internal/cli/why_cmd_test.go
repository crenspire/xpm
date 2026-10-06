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

func TestWhyLimitIsPerVersion(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	code, stdout, _ := runWhy(t, "ms", "--limit", "1")
	if code != 0 || !strings.Contains(stdout, "ms@2.0.0 (node)") || !strings.Contains(stdout, "ms@2.1.3 (node)") {
		t.Fatalf("exit %d stdout %q", code, stdout)
	}
}

const diamondLock = `{
  "name": "demo", "version": "1.0.0", "lockfileVersion": 3,
  "packages": {
    "": {"name": "demo", "version": "1.0.0", "dependencies": {"a": "1.0.0", "b": "1.0.0"}},
    "node_modules/a": {"version": "1.0.0", "dependencies": {"c": "1.0.0"}},
    "node_modules/b": {"version": "1.0.0", "dependencies": {"c": "1.0.0"}},
    "node_modules/c": {"version": "1.0.0"}
  }
}`

func TestWhyTruncated(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": diamondLock})
	code, stdout, _ := runWhy(t, "--limit", "1", "c")
	want := "c@1.0.0 (node)\n  demo@1.0.0 > a@1.0.0 > c@1.0.0\n  … more paths (use --limit 0 to show all)\n"
	if code != 0 || stdout != want {
		t.Fatalf("exit %d stdout %q", code, stdout)
	}
	if code, stdout, _ := runWhy(t, "--limit", "0", "c"); code != 0 || strings.Contains(stdout, "more paths") || strings.Count(stdout, " > ") != 4 {
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
		Package string
		Targets []struct {
			ID        string
			Paths     [][]string
			Truncated bool
		}
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	if out.Package != "debug" || len(out.Targets) != 1 || out.Targets[0].ID != "node:debug@2.6.9" || out.Targets[0].Truncated ||
		len(out.Targets[0].Paths) != 1 || strings.Join(out.Targets[0].Paths[0], ",") != "node:demo@1.0.0,node:debug@2.6.9" {
		t.Errorf("out = %+v", out)
	}
}
