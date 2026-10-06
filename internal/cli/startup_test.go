package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir is t.Chdir for Go < 1.24.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestRunHelpWritesNoFiles(t *testing.T) {
	home := isolatedHome(t)

	// A project with .xpm-env in the root and an installed matching version:
	// exactly the situation in which the old auto-activation rewrote files.
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".xpm-env"), []byte("node=20.11.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".xpm", "env", "runtimes", "node", "20.11.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(proj, "packages", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, sub)

	oldArgs := os.Args
	os.Args = []string{"xpm", "help"}
	t.Cleanup(func() { os.Args = oldArgs })

	if code := Run(); code != 0 {
		t.Fatalf("xpm help exited %d", code)
	}
	if _, err := os.Stat(filepath.Join(sub, ".xpm-env")); err == nil {
		t.Fatal("xpm help created .xpm-env in the current directory")
	}
	if _, err := os.Stat(filepath.Join(home, ".xpm", "env", "active.json")); err == nil {
		t.Fatal("xpm help wrote active.json")
	}
}
