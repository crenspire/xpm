package env

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolveSpec(t *testing.T) {
	ctx := context.Background()
	base := &fakeInstaller{remote: []string{"20.9.0", "20.11.1", "200.1.0", "21.0.0-rc.1", "18.19.0", "2.5rc1"}}
	cases := []struct {
		spec, want, err string
	}{
		{spec: "20", want: "20.11.1"},
		{spec: "latest", want: "200.1.0"},
		{spec: "21", err: "no xpmfake version matches 21 (see: xpm env ls-remote xpmfake)"},
		{spec: "2.5rc1", want: "2.5rc1"},
		{spec: "2.5", err: "no xpmfake version matches 2.5"},
		{spec: "lts", err: "xpmfake has no lts alias"},
		{spec: "../x", err: "invalid version"},
	}
	for _, c := range cases {
		got, err := ResolveSpec(ctx, base, c.spec)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: err = %v, want %q", c.spec, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.spec, got, err, c.want)
		}
	}

	offline := &fakeInstaller{remoteErr: errors.New("network must not be used")}
	if got, err := ResolveSpec(ctx, offline, "1.2.3"); err != nil || got != "1.2.3" {
		t.Errorf("exact spec: got %q, %v", got, err)
	}
	if offline.listCalls != 0 {
		t.Error("an exact spec must not list remote versions")
	}

	lts := fakeLTS{&fakeInstaller{lts: "20.11.1"}}
	if got, err := ResolveSpec(ctx, lts, "lts"); err != nil || got != "20.11.1" {
		t.Errorf("lts: got %q, %v", got, err)
	}

	res := fakeResolver{&fakeInstaller{}, func(spec string) (string, error) { return "9.9-" + spec, nil }}
	if got, err := ResolveSpec(ctx, res, "1"); err != nil || got != "9.9-1" {
		t.Errorf("Resolver: got %q, %v", got, err)
	}
	evil := fakeResolver{&fakeInstaller{}, func(string) (string, error) { return "../../etc", nil }}
	if _, err := ResolveSpec(ctx, evil, "1"); err == nil {
		t.Error("a path-like resolved version must be rejected")
	}
}

func noTmpLeft(t *testing.T, m *Manager) {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(m.GetRuntimesPath(), fakeRT))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("staging dir left behind: %s", e.Name())
		}
	}
}

func TestInstallRuntimeInstallsAtomicallyAndSetsGlobalDefault(t *testing.T) {
	m := isolate(t)
	var out bytes.Buffer
	m.SetOutput(&out)
	f := &fakeInstaller{remote: []string{"1.2.0", "1.10.0"}}
	var stagedIn string
	f.install = func(_ context.Context, req InstallRequest) error {
		stagedIn = req.Dest
		if req.Root != m.GetEnvPath() {
			t.Errorf("Root = %q", req.Root)
		}
		if entries, _ := os.ReadDir(req.Dest); len(entries) != 0 {
			t.Error("Dest must be an empty dir")
		}
		return writeFakeBinaries(req.Dest)
	}
	useFake(t, f)

	exact, err := InstallRuntime(context.Background(), m, fakeRT, "latest")
	if err != nil || exact != "1.10.0" {
		t.Fatalf("InstallRuntime = %q, %v", exact, err)
	}
	if !strings.HasPrefix(filepath.Base(stagedIn), ".tmp-1.10.0-") || filepath.Dir(stagedIn) != filepath.Join(m.GetRuntimesPath(), fakeRT) {
		t.Fatalf("staged in %s", stagedIn)
	}
	dir := filepath.Join(m.GetRuntimesPath(), fakeRT, "1.10.0")
	if meta := readMeta(dir); meta.Version != "1.10.0" || meta.Alias != "latest" {
		t.Fatalf("meta = %+v", meta)
	}
	noTmpLeft(t, m)
	if g, _ := m.GlobalVersion(fakeRT); g != "1.10.0" {
		t.Fatalf("global = %q", g)
	}
	if _, err := os.Stat(".xpm-env"); err == nil {
		t.Fatal("install wrote .xpm-env")
	}
	for _, want := range []string{"Resolved xpmfake@latest to 1.10.0", "Set xpmfake@1.10.0 as the global default"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q lacks %q", out.String(), want)
		}
	}

	out.Reset()
	if _, err := InstallRuntime(context.Background(), m, fakeRT, "1.10.0"); err != nil {
		t.Fatal(err)
	}
	if len(f.installed) != 1 || !strings.Contains(out.String(), "already installed") {
		t.Fatalf("second install ran the installer again: %v / %q", f.installed, out.String())
	}

	out.Reset()
	if _, err := InstallRuntime(context.Background(), m, fakeRT, "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if g, _ := m.GlobalVersion(fakeRT); g != "1.10.0" {
		t.Fatalf("an existing global default was replaced: %q", g)
	}
	if !strings.Contains(out.String(), "Use it here: xpm env use xpmfake@1.2.0") {
		t.Fatalf("output %q", out.String())
	}
}

func TestInstallRuntimeFailuresLeaveNothing(t *testing.T) {
	cases := map[string]func(ctx context.Context, req InstallRequest) error{
		"installer error": func(_ context.Context, req InstallRequest) error {
			_ = os.WriteFile(filepath.Join(req.Dest, "partial"), []byte("x"), 0o644)
			return errors.New("boom")
		},
		"missing binary": func(_ context.Context, req InstallRequest) error {
			return os.WriteFile(filepath.Join(req.Dest, "README"), []byte("x"), 0o644)
		},
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			m := isolate(t)
			useFake(t, &fakeInstaller{install: fn})
			if _, err := InstallRuntime(context.Background(), m, fakeRT, "1.0.0"); err == nil {
				t.Fatal("want an error")
			}
			if _, err := os.Stat(filepath.Join(m.GetRuntimesPath(), fakeRT, "1.0.0")); err == nil {
				t.Fatal("a partial version dir exists")
			}
			noTmpLeft(t, m)
			if g, _ := m.GlobalVersion(fakeRT); g != "" {
				t.Fatalf("failed install set global %q", g)
			}
		})
	}
}

func TestInstallRuntimeCancelledLeavesNothing(t *testing.T) {
	m := isolate(t)
	ctx, cancel := context.WithCancel(context.Background())
	useFake(t, &fakeInstaller{install: func(ctx context.Context, req InstallRequest) error {
		if err := writeFakeBinaries(req.Dest); err != nil {
			return err
		}
		cancel() // Ctrl-C arrives while the installer runs
		return ctx.Err()
	}})
	_, err := InstallRuntime(ctx, m, fakeRT, "1.0.0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(m.GetRuntimesPath(), fakeRT, "1.0.0")); err == nil {
		t.Fatal("cancelled install left a version dir")
	}
	noTmpLeft(t, m)
}

func TestInstallRuntimeCancelledInstallerErrorIsCancel(t *testing.T) {
	m := isolate(t)
	ctx, cancel := context.WithCancel(context.Background())
	useFake(t, &fakeInstaller{install: func(context.Context, InstallRequest) error {
		cancel() // Ctrl-C kills rustup/brew; they fail with their own error
		return errors.New("signal: interrupt")
	}})
	_, err := InstallRuntime(ctx, m, fakeRT, "1.0.0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	noTmpLeft(t, m)
}

func TestInstallRuntimePostInstallFailureKeepsTheVersion(t *testing.T) {
	m := isolate(t)
	useFake(t, &fakeInstaller{})
	if err := os.WriteFile(m.GetActivePath(), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	exact, err := InstallRuntime(context.Background(), m, fakeRT, "1.0.0")
	var post *PostInstallError
	if !errors.As(err, &post) || exact != "1.0.0" {
		t.Fatalf("got %q, %v; want 1.0.0 and a PostInstallError", exact, err)
	}
	if verifyInstallation(filepath.Join(m.GetRuntimesPath(), fakeRT, "1.0.0"), []string{"bin/fakebin"}) != nil {
		t.Fatal("version not installed")
	}
}

func TestInstallRuntimeReplacesCorruptAndStaleDirs(t *testing.T) {
	m := isolate(t)
	rtDir := filepath.Join(m.GetRuntimesPath(), fakeRT)
	for _, d := range []string{"1.0.0/lib", ".tmp-1.0.0-old"} {
		if err := os.MkdirAll(filepath.Join(rtDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeInstaller{}
	useFake(t, f)
	if _, err := InstallRuntime(context.Background(), m, fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if len(f.installed) != 1 {
		t.Fatal("a dir without its binaries must be reinstalled")
	}
	if _, err := os.Stat(filepath.Join(rtDir, "1.0.0", "lib")); err == nil {
		t.Fatal("the corrupt install was not replaced")
	}
	noTmpLeft(t, m)
}

func TestInstallRuntimeWaitsForConcurrentInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock is Unix-only")
	}
	m := isolate(t)
	out := newSignalWriter("Waiting for another xpm process installing xpmfake...")
	m.SetOutput(out)
	f := &fakeInstaller{}
	useFake(t, f)
	rtDir := filepath.Join(m.GetRuntimesPath(), fakeRT)
	if err := os.MkdirAll(rtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// "Another process" holds the lock and finishes the same version.
	unlock, err := lockFile(context.Background(), filepath.Join(rtDir, ".lock"), func() {})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := InstallRuntime(context.Background(), m, fakeRT, "1.0.0")
		done <- err
	}()
	out.wait(t)
	installFake(t, m, "1.0.0", "")
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("install never got the lock")
	}
	if len(f.installed) != 0 {
		t.Fatal("installed again after waiting; must re-check under the lock")
	}
	for _, want := range []string{"Waiting for another xpm process installing xpmfake...", "xpmfake@1.0.0 is already installed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q lacks %q", out.String(), want)
		}
	}
}

func TestLockFileWaitsAndHonoursCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock is Unix-only")
	}
	path := filepath.Join(t.TempDir(), ".lock")
	unlock, err := lockFile(context.Background(), path, func() { t.Error("first lock must not wait") })
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	waited := false
	if _, err := lockFile(ctx, path, func() { waited = true }); !errors.Is(err, context.DeadlineExceeded) || !waited {
		t.Fatalf("second lock: err=%v waited=%v", err, waited)
	}

	got := make(chan error, 1)
	go func() {
		u, err := lockFile(context.Background(), path, func() {})
		if err == nil {
			u()
		}
		got <- err
	}()
	time.Sleep(150 * time.Millisecond)
	unlock()
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiter never got the lock")
	}
}
