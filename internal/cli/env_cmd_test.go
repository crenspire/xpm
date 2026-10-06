package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/env"
)

// cliFake is a test runtime; the registry is global, so register it once.
type cliFake struct{}

var (
	cliFakeOnce    sync.Once
	cliFakeInstall func(ctx context.Context, req env.InstallRequest) error
)

func (cliFake) Name() string                                 { return "xpmclifake" }
func (cliFake) ListRemote(context.Context) ([]string, error) { return []string{"1.0.0", "1.1.0"}, nil }
func (cliFake) BinaryPaths() []string                        { return []string{"bin/clifakebin"} }
func (cliFake) Install(ctx context.Context, req env.InstallRequest) error {
	if cliFakeInstall != nil {
		return cliFakeInstall(ctx, req)
	}
	if err := os.MkdirAll(filepath.Join(req.Dest, "bin"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(req.Dest, "bin", "clifakebin"), []byte("#!/bin/sh\n"), 0o755)
}

// envTest isolates HOME and cwd, registers the fake runtime and makes
// shims point at a fake xpm path. It returns the project dir and env root.
func envTest(t *testing.T) (proj, root string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("xpm env is Unix-only")
	}
	isolatedHome(t)
	chdir(t, t.TempDir())
	proj, _ = os.Getwd() // resolved (/private/var vs /var on macOS)
	cliFakeOnce.Do(func() { env.RegisterInstaller("xpmclifake", cliFake{}) })
	old := newEnvManager
	exe := filepath.Join(t.TempDir(), "xpm")
	newEnvManager = func(c config.Config) (*env.Manager, error) {
		m, err := env.NewManager(c)
		if err == nil {
			m.SetExecutable(exe)
		}
		return m, err
	}
	t.Cleanup(func() { newEnvManager = old; cliFakeInstall = nil })
	home, _ := os.UserHomeDir()
	return proj, filepath.Join(home, ".xpm", "env")
}

func TestEnvDisabledOnWindows(t *testing.T) {
	old := envGOOS
	envGOOS = "windows"
	t.Cleanup(func() { envGOOS = old })
	var code int
	errOut := captureStderr(t, func() { code = cmdEnv([]string{"list"}) })
	if code != 1 || errOut != "xpm env is not supported on Windows yet: runtime downloads, shims and PATH setup are Unix-only (macOS, Linux).\n" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestParseUseArgs(t *testing.T) {
	cases := []struct {
		args   []string
		spec   string
		global bool
		ok     bool
	}{
		{[]string{"node@20"}, "node@20", false, true},
		{[]string{"--global", "node@20"}, "node@20", true, true},
		{[]string{"node@20", "-g"}, "node@20", true, true},
		{[]string{"node@20", "go@1.22"}, "", false, false},
		{[]string{"--force", "node@20"}, "", false, false},
		{nil, "", false, false},
	}
	for _, c := range cases {
		spec, global, err := parseUseArgs(c.args)
		if (err == nil) != c.ok || spec != c.spec || global != c.global {
			t.Errorf("%v: %q %v %v", c.args, spec, global, err)
		}
	}
}

func TestEnvInstallSetsGlobalDefaultAndShims(t *testing.T) {
	proj, root := envTest(t)
	var code int
	out := captureStdout(t, func() { code = cmdEnv([]string{"install", "xpmclifake@1"}) })
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, want := range []string{"Resolved xpmclifake@1 to 1.1.0", "Set xpmclifake@1.1.0 as the global default", "xpm env setup-path"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(proj, ".xpm-env")); err == nil {
		t.Fatal("install wrote .xpm-env")
	}
	if _, err := os.Readlink(filepath.Join(root, "shims", "clifakebin")); err != nil {
		t.Fatalf("shim not created: %v", err)
	}

	out = captureStdout(t, func() { code = cmdEnv([]string{"use", "xpmclifake@1.1.0"}) })
	if code != 0 || !strings.Contains(out, "Using xpmclifake@1.1.0 ("+filepath.Join(proj, ".xpm-env")+")") {
		t.Fatalf("use: exit %d\n%s", code, out)
	}
}

func TestEnvInstallCancelledExits130(t *testing.T) {
	_, root := envTest(t)
	cliFakeInstall = func(context.Context, env.InstallRequest) error { return context.Canceled }
	var code int
	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() { code = cmdEnv([]string{"install", "xpmclifake@1.0.0"}) })
	})
	if code != 130 || !strings.Contains(errOut, "Cancelled; nothing was installed.") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "runtimes", "xpmclifake"))
	for _, e := range entries {
		if e.Name() != ".lock" {
			t.Fatalf("left behind: %s", e.Name())
		}
	}
}

func TestEnvInstallGlobalDefaultFailureIsAWarning(t *testing.T) {
	_, root := envTest(t)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "active.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() { code = cmdEnv([]string{"install", "xpmclifake@1.0.0"}) })
	})
	if code != 0 || !strings.Contains(errOut, "warning: ") || !strings.Contains(errOut, "active.json") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if _, err := os.Readlink(filepath.Join(root, "shims", "clifakebin")); err != nil {
		t.Fatalf("not reshimmed: %v", err)
	}
}

func TestEnvCurrentListsSortedWithSourceAndWarns(t *testing.T) {
	proj, _ := envTest(t)
	_ = captureStdout(t, func() { cmdEnv([]string{"install", "xpmclifake@1.0.0"}) })
	m, err := newEnvManager(config.Load())
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	writeCurrent(m, &out, &errOut)
	if !strings.Contains(out.String(), "xpmclifake 1.0.0 ("+m.GetActivePath()+")\n") {
		t.Fatalf("stdout %q", out.String())
	}

	if err := os.WriteFile(filepath.Join(proj, ".xpm-env"), []byte("xpmclifake=9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	writeCurrent(m, &out, &errOut)
	if !strings.Contains(out.String(), "xpmclifake 9 (") || !strings.Contains(errOut.String(), "xpm env install xpmclifake@9") {
		t.Fatalf("stdout %q stderr %q", out.String(), errOut.String())
	}
}

func TestEnvDisabledByConfig(t *testing.T) {
	envTest(t)
	writeConfig(t, `{"env":{"enabled":false}}`)
	var code int
	errOut := captureStderr(t, func() { code = cmdEnv([]string{"list"}) })
	if code != 1 || !strings.Contains(errOut, "disabled") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}
