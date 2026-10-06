package env

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("xpm env shims are Unix-only")
	}
}

func TestShimName(t *testing.T) {
	skipOnWindows(t)
	useFake(t, &fakeInstaller{})
	cases := map[string]bool{
		"/usr/local/xpm/shims/fakebin": true,
		"fakebin2":                     true,
		"fakebin.exe":                  true,
		"xpm":                          false,
		"/usr/local/bin/xpm":           false,
		"unknown":                      false,
		"":                             false,
	}
	for argv0, want := range cases {
		if _, ok := ShimName(argv0); ok != want {
			t.Errorf("ShimName(%q) ok = %v, want %v", argv0, ok, want)
		}
	}
}

func TestCreateShimsLinksEveryBinaryAndPrunes(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	useFake(t, &fakeInstaller{})
	installFake(t, m, "1.0.0", "")
	shims := m.GetShimsPath()
	for _, junk := range []string{"node", "node.go", "stale.tmp-abc"} {
		if err := os.WriteFile(filepath.Join(shims, junk), []byte("old compiled shim"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(t.TempDir(), "xpm")
	m.SetExecutable(exe)
	if err := CreateShims(m); err != nil {
		t.Fatal(err)
	}
	if err := CreateShims(m); err != nil { // idempotent
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(shims)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "fakebin,fakebin2" {
		t.Fatalf("shims = %v", names)
	}
	if target, _ := os.Readlink(filepath.Join(shims, "fakebin")); target != exe {
		t.Fatalf("fakebin -> %q, want %q", target, exe)
	}

	upgraded := filepath.Join(t.TempDir(), "xpm-new")
	m.SetExecutable(upgraded)
	if err := CreateShims(m); err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(filepath.Join(shims, "fakebin2")); target != upgraded {
		t.Fatalf("not re-pointed: %q", target)
	}
}

// shimRun sets up HOME-based config (default env root ~/.xpm/env), a fake
// xpm executable and a recording execFn.
type shimRun struct {
	m      *Manager
	stderr bytes.Buffer
	path   string
	argv   []string
	env    []string
	calls  int
}

func newShimRun(t *testing.T) *shimRun {
	t.Helper()
	skipOnWindows(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(shimDepthVar, "")
	chdir(t, t.TempDir())
	useFake(t, &fakeInstaller{})
	exe := filepath.Join(t.TempDir(), "xpm")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe, oldExec, oldErr := osExecutable, execFn, shimStderr
	t.Cleanup(func() { osExecutable, execFn, shimStderr = oldExe, oldExec, oldErr })
	osExecutable = func() (string, error) { return exe, nil }

	r := &shimRun{}
	shimStderr = &r.stderr
	execFn = func(path string, argv, env []string) error {
		r.calls++
		r.path, r.argv, r.env = path, argv, env
		return nil
	}
	m, err := NewManager(configWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	m.SetOutput(&bytes.Buffer{})
	r.m = m
	return r
}

func (r *shimRun) envVar(key string) string {
	for _, kv := range r.env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			return v
		}
	}
	return ""
}

func TestRunShimExecsTheActiveVersion(t *testing.T) {
	r := newShimRun(t)
	dir := installFake(t, r.m, "1.2.3", "")
	if err := r.m.SetGlobalVersion(fakeRT, "1.2"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin")

	if code := RunShim("fakebin", []string{"-v", "x y"}); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr.String())
	}
	bin := filepath.Join(dir, "bin", "fakebin")
	if r.path != bin || strings.Join(r.argv, "|") != bin+"|-v|x y" {
		t.Fatalf("exec %q %q", r.path, r.argv)
	}
	if got := r.envVar("PATH"); got != filepath.Join(dir, "bin")+string(os.PathListSeparator)+"/usr/bin" {
		t.Fatalf("PATH = %q", got)
	}
	if got := r.envVar(shimDepthVar); got != "1" {
		t.Fatalf("depth = %q", got)
	}
}

func TestRunShimConfiguredButNotInstalled(t *testing.T) {
	r := newShimRun(t)
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sys := t.TempDir()
	if err := os.WriteFile(filepath.Join(sys, "fakebin"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", sys)
	if code := RunShim("fakebin", nil); code != 127 || r.calls != 0 {
		t.Fatalf("exit %d calls %d; an explicit pin must not fall back", code, r.calls)
	}
	cwd, _ := os.Getwd()
	want := "xpm: xpmfake@9.9.9 is not installed (set in " + filepath.Join(cwd, ".xpm-env") + ")\nRun: xpm env install xpmfake@9.9.9\n"
	if r.stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", r.stderr.String(), want)
	}
}

func TestRunShimInvalidEntryFailsClosed(t *testing.T) {
	r := newShimRun(t)
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=../../evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := RunShim("fakebin", nil); code != 1 || r.calls != 0 {
		t.Fatalf("exit %d calls %d", code, r.calls)
	}
	if !strings.HasPrefix(r.stderr.String(), `xpm: invalid xpmfake version "../../evil" in `) {
		t.Fatalf("stderr = %q", r.stderr.String())
	}
}

func TestRunShimFallsBackToSystemBinary(t *testing.T) {
	r := newShimRun(t)
	exe, _ := osExecutable()
	selfDir := t.TempDir() // a PATH dir whose fakebin is a link back to xpm
	if err := os.Symlink(exe, filepath.Join(selfDir, "fakebin")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(r.m.GetShimsPath(), "fakebin")); err != nil {
		t.Fatal(err)
	}
	sys := t.TempDir()
	sysBin := filepath.Join(sys, "fakebin")
	if err := os.WriteFile(sysBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sep := string(os.PathListSeparator)
	t.Setenv("PATH", r.m.GetShimsPath()+sep+selfDir+sep+sys)

	if code := RunShim("fakebin", []string{"a"}); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr.String())
	}
	if r.path != sysBin || r.envVar("PATH") != os.Getenv("PATH") {
		t.Fatalf("exec %q with PATH %q", r.path, r.envVar("PATH"))
	}
}

func TestRunShimNoVersionNoSystemBinary(t *testing.T) {
	r := newShimRun(t)
	t.Setenv("PATH", t.TempDir())
	if code := RunShim("fakebin", nil); code != 127 {
		t.Fatalf("exit %d", code)
	}
	want := "xpm: no xpmfake version is configured and no system fakebin was found on PATH\nInstall one: xpm env install xpmfake@<version>\n"
	if r.stderr.String() != want {
		t.Fatalf("stderr = %q", r.stderr.String())
	}
}

func TestRunShimDisabledConfigUsesSystem(t *testing.T) {
	r := newShimRun(t)
	installFake(t, r.m, "1.0.0", "")
	if err := r.m.SetGlobalVersion(fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	cfgDir := filepath.Join(home, ".config", "xpm")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "xpmrc.json"), []byte(`{"env":{"enabled":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sys := t.TempDir()
	if err := os.WriteFile(filepath.Join(sys, "fakebin"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", sys)
	if code := RunShim("fakebin", nil); code != 0 || r.path != filepath.Join(sys, "fakebin") {
		t.Fatalf("exit %d exec %q", code, r.path)
	}
}

func TestRunShimRecursionGuard(t *testing.T) {
	r := newShimRun(t)
	installFake(t, r.m, "1.0.0", "")
	if err := r.m.SetGlobalVersion(fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(shimDepthVar, "4")
	if code := RunShim("fakebin", nil); code != 1 || r.calls != 0 {
		t.Fatalf("exit %d calls %d", code, r.calls)
	}
	if r.stderr.String() != "xpm: shim recursion detected for fakebin\n" {
		t.Fatalf("stderr = %q", r.stderr.String())
	}
}

func TestRunShimBinaryMissingFromVersion(t *testing.T) {
	r := newShimRun(t)
	dir := installFake(t, r.m, "1.0.0", "")
	if err := os.Remove(filepath.Join(dir, "bin", "fakebin2")); err != nil {
		t.Fatal(err)
	}
	if err := r.m.SetGlobalVersion(fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if code := RunShim("fakebin2", nil); code != 127 || r.stderr.String() != "xpm: fakebin2 is not provided by xpmfake@1.0.0\n" {
		t.Fatalf("exit %d stderr %q", code, r.stderr.String())
	}
}
