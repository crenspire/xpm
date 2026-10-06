package runtimes

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// RustInstaller installs Rust versions.
type RustInstaller struct{}

func init() {
	env.RegisterInstaller("rust", &RustInstaller{})
}

// Name returns the runtime name.
func (r *RustInstaller) Name() string {
	return "rust"
}

// ListRemote fetches available Rust versions.
func (r *RustInstaller) ListRemote(_ context.Context) ([]string, error) {
	// Use rustup to list available toolchains
	if rustupExists() {
		cmd := exec.Command("rustup", "toolchain", "list", "--available")
		output, err := cmd.Output()
		if err == nil {
			// Parse output
			var versions []string
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.Contains(line, "installed") {
					// Extract version (e.g., "stable-2024-01-01-x86_64-unknown-linux-gnu" -> "stable-2024-01-01")
					parts := strings.Fields(line)
					if len(parts) > 0 {
						version := parts[0]
						// Extract just the version part
						if idx := strings.Index(version, "-"); idx > 0 {
							version = version[:idx]
						}
						versions = append(versions, version)
					}
				}
			}
			if len(versions) > 0 {
				return versions, nil
			}
		}
	}

	// Fallback to common versions
	return []string{
		"stable", "beta", "nightly",
		"1.75.0", "1.74.0", "1.73.0", "1.72.0",
	}, nil
}

// ValidateVersion validates a Rust version string.
func (r *RustInstaller) ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	return nil
}

// Install adapts the v1 installer to the v2 interface; the installer
// rewrite replaces it.
func (r *RustInstaller) Install(_ context.Context, req env.InstallRequest) error {
	if err := r.install(req.Version, req.Dest); err != nil {
		return err
	}
	return r.PostInstall(req.Version, req.Dest)
}

// Install installs a Rust toolchain via the user's rustup. xpm deliberately
// does not bootstrap rustup itself: that meant running an unverified download
// that also rewrote ~/.cargo and shell profiles.
func (r *RustInstaller) install(version string, dest string) error {
	if !rustupExists() {
		return fmt.Errorf("rust needs rustup: install it from https://rustup.rs, then re-run `xpm env install rust@%s`", version)
	}
	return r.installViaRustup(version, dest)
}

// installViaRustup uses rustup to install a toolchain.
func (r *RustInstaller) installViaRustup(version string, dest string) error {
	// Install toolchain
	cmd := exec.Command("rustup", "toolchain", "install", version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rustup install failed: %w", err)
	}

	// Get rustup home
	rustupHome := os.Getenv("RUSTUP_HOME")
	if rustupHome == "" {
		home, _ := os.UserHomeDir()
		rustupHome = filepath.Join(home, ".rustup")
	}

	// Find installed toolchain
	toolchainPath := filepath.Join(rustupHome, "toolchains", version+"-*")
	matches, err := filepath.Glob(toolchainPath)
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("installed toolchain not found")
	}

	// Copy to our location
	return copyDirectory(matches[0], dest)
}

// PostInstall performs post-installation setup.
func (r *RustInstaller) PostInstall(version, dest string) error {
	return nil
}

// BinaryPaths returns the paths to Rust binaries.
func (r *RustInstaller) BinaryPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{"bin\\rustc.exe", "bin\\cargo.exe"}
	}
	return []string{"bin/rustc", "bin/cargo"}
}

// rustupExists checks if rustup is available.
func rustupExists() bool {
	_, err := exec.LookPath("rustup")
	return err == nil
}
