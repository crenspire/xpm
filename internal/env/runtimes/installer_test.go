package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

// installInto runs inst.Install into a fresh empty dir and checks that
// every BinaryPaths entry exists and is executable.
func installInto(t *testing.T, inst env.RuntimeInstaller, version string) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "stage")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := inst.Install(context.Background(), env.InstallRequest{Version: version, Dest: dest, Root: root}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	for _, rel := range inst.BinaryPaths() {
		fi, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s missing: %v", rel, err)
		}
		if fi.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s is not executable", rel)
		}
	}
	return dest
}

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("archives with symlinks and exec bits: Unix-only")
	}
}
