package runtimes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRustInstallWithoutRustupFailsSafely(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no rustup anywhere
	dest := filepath.Join(t.TempDir(), "rust", "1.75.0")

	err := (&RustInstaller{}).Install("1.75.0", dest)
	if err == nil || !strings.Contains(err.Error(), "rustup.rs") {
		t.Fatalf("want an error pointing at https://rustup.rs, got %v", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatal("dest must not be created")
	}
}
