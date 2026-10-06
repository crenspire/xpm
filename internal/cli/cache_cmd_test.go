package cli

import (
	"os"
	"strings"
	"testing"
)

func TestCmdCacheReportsRemoval(t *testing.T) {
	for _, args := range [][]string{nil, {"clean"}, {"gc"}, {"tree"}} {
		var code int
		var out string
		errOut := captureStderr(t, func() {
			out = captureStdout(t, func() { code = cmdCache(args) })
		})
		if code != 1 {
			t.Errorf("cmdCache(%v) = %d, want 1", args, code)
		}
		if out != "" {
			t.Errorf("cmdCache(%v) wrote to stdout: %q", args, out)
		}
		if !strings.HasPrefix(errOut, "xpm cache has been removed: package managers keep their own caches.\n") {
			t.Errorf("cmdCache(%v) stderr = %q", args, errOut)
		}
		if !strings.Contains(errOut, "go clean -modcache") {
			t.Errorf("cmdCache(%v) stderr lacks examples: %q", args, errOut)
		}
	}
}

func TestRunCacheAliasesReportRemoval(t *testing.T) {
	isolatedHome(t)
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	for _, cmd := range []string{"cache", "cc", "cg"} {
		os.Args = []string{"xpm", cmd}
		var code int
		errOut := captureStderr(t, func() {
			_ = captureStdout(t, func() { code = Run() })
		})
		if code != 1 || !strings.Contains(errOut, "xpm cache has been removed") {
			t.Errorf("Run(%q) = %d, stderr %q; want 1 and the removal message", cmd, code, errOut)
		}
	}
}
