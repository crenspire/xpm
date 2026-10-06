package env

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeRT is the test-only runtime name; the real installers live in
// internal/env/runtimes, which this package cannot import.
const fakeRT = "xpmfake"

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
