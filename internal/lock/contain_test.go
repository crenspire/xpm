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

func TestVerifyRejectsSymlinkedDirectoryEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	outside := t.TempDir()
	writeFile(t, outside, "package-lock.json", npmLock)
	dir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "sub")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, LockfileName, "version: 2\n"+
		"locks:\n"+
		"  sub/package-lock.json:\n"+
		"    file: sub/package-lock.json\n"+
		"    hash: "+hashBytes([]byte(npmLock))+"\n")

	results, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != StatusError || !errors.Is(results[0].Error, errOutsideRoot) {
		t.Fatalf("results = %+v, want one StatusError outside the project", results)
	}
	if results[0].ActualHash != "" {
		t.Errorf("ActualHash = %q; the file must never be read", results[0].ActualHash)
	}
}

func TestContainedPathFailsClosedOnUnresolvableSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	// A symlink loop: resolving it fails with an error other than "not exist".
	if err := os.Symlink("go.sum", filepath.Join(root, "go.sum")); err != nil {
		t.Fatal(err)
	}
	if full, err := containedPath(root, "go.sum"); err == nil {
		t.Fatalf("containedPath(symlink loop) = %q, nil; want an error", full)
	}
	writeFile(t, root, LockfileName, "version: 2\n"+
		"locks:\n"+
		"  go.sum:\n"+
		"    file: go.sum\n"+
		"    hash: 00\n")
	results, err := Verify(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != StatusError {
		t.Fatalf("results = %+v, want one StatusError", results)
	}
}

func TestContainedPathDanglingSymlinkIsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	if err := os.Symlink("nowhere.sum", filepath.Join(root, "go.sum")); err != nil {
		t.Fatal(err)
	}
	full, err := containedPath(root, "go.sum")
	if err != nil || fileExists(full) {
		t.Fatalf("containedPath(dangling symlink) = %q, %v; want a missing path, nil", full, err)
	}
}

func TestContainedPathReturnsTheResolvedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	writeFile(t, root, "real.sum", "x")
	if err := os.Symlink("real.sum", filepath.Join(root, "go.sum")); err != nil {
		t.Fatal(err)
	}
	full, err := containedPath(root, "go.sum")
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	// The checked path is the one opened: no symlink is followed again.
	if want := filepath.Join(realRoot, "real.sum"); full != want {
		t.Errorf("containedPath = %q, want %q", full, want)
	}
}
