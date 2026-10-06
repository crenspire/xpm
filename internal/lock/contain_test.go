package lock

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckLocal(t *testing.T) {
	ok := []string{"package-lock.json", "sub/yarn.lock", "./go.sum"}
	bad := []string{
		"",
		".",
		"/etc/passwd",
		"../outside.lock",
		"sub/../../outside.lock",
		"..",
		`..\outside.lock`,
		`sub\yarn.lock`,
		"C:/Windows/win.ini",
		"c:go.sum",
		"go.sum\x00",
	}
	for _, p := range ok {
		if err := checkLocal(p); err != nil {
			t.Errorf("checkLocal(%q) = %v, want nil", p, err)
		}
	}
	for _, p := range bad {
		err := checkLocal(p)
		if err == nil {
			t.Errorf("checkLocal(%q) = nil, want error", p)
			continue
		}
		if !errors.Is(err, errOutsideRoot) {
			t.Errorf("checkLocal(%q) = %v, want errOutsideRoot", p, err)
		}
	}
}

func TestContainedPathRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("s3cret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(root, "go.sum")); err != nil {
		t.Fatal(err)
	}
	if _, err := containedPath(root, "go.sum"); !errors.Is(err, errOutsideRoot) {
		t.Fatalf("containedPath(symlink to outside) = %v, want errOutsideRoot", err)
	}
	// Detection skips it too, so `xpm lock` never hashes a file outside the project.
	if got := DetectAll(root); len(got) != 0 {
		t.Fatalf("DetectAll = %+v, want nothing", got)
	}
}

func TestContainedPathAllowsSymlinkInside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.sum"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.sum", filepath.Join(root, "go.sum")); err != nil {
		t.Fatal(err)
	}
	if _, err := containedPath(root, "go.sum"); err != nil {
		t.Fatalf("containedPath(symlink inside root) = %v, want nil", err)
	}
}
