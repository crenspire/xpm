package env

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fakeRT is the test-only runtime name; the real installers live in
// internal/env/runtimes, which this package cannot import.
const fakeRT = "xpmfake"

// fakeInstaller records calls and installs tiny executable files.
type fakeInstaller struct {
	mu        sync.Mutex
	remote    []string
	remoteErr error
	lts       string // non-empty: also implements LTSResolver via fakeLTS
	installed []string
	install   func(ctx context.Context, req InstallRequest) error // nil: write the binaries
	listCalls int
}

func (f *fakeInstaller) Name() string { return fakeRT }

func (f *fakeInstaller) ListRemote(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return f.remote, f.remoteErr
}

func (f *fakeInstaller) Install(ctx context.Context, req InstallRequest) error {
	f.mu.Lock()
	f.installed = append(f.installed, req.Version)
	fn := f.install
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return writeFakeBinaries(req.Dest)
}

func (f *fakeInstaller) BinaryPaths() []string { return []string{"bin/fakebin", "bin/fakebin2"} }

// fakeLTS adds LatestLTS to fakeInstaller.
type fakeLTS struct{ *fakeInstaller }

func (f fakeLTS) LatestLTS(context.Context) (string, error) {
	if f.lts == "" {
		return "", errors.New("no lts")
	}
	return f.lts, nil
}

// fakeResolver adds Resolve to fakeInstaller.
type fakeResolver struct {
	*fakeInstaller
	resolve func(spec string) (string, error)
}

func (f fakeResolver) Resolve(_ context.Context, spec string) (string, error) {
	return f.resolve(spec)
}

func writeFakeBinaries(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		return err
	}
	for _, b := range []string{"fakebin", "fakebin2"} {
		if err := os.WriteFile(filepath.Join(dir, "bin", b), []byte("#!/bin/sh\n"), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// useFake registers inst as the xpmfake runtime for one test.
func useFake(t *testing.T, inst RuntimeInstaller) {
	t.Helper()
	RegisterInstaller(fakeRT, inst)
	t.Cleanup(func() { unregisterInstaller(fakeRT) })
}

// installFake creates an installed xpmfake version without InstallRuntime.
func installFake(t *testing.T, m *Manager, version, alias string) string {
	t.Helper()
	dir := filepath.Join(m.GetRuntimesPath(), fakeRT, version)
	if err := writeFakeBinaries(dir); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(dir, versionMeta{Version: version, Alias: alias}); err != nil {
		t.Fatal(err)
	}
	return dir
}
