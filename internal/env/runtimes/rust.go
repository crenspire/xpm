package runtimes

import (
	"fmt"
	"io"
	"net/http"
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
func (r *RustInstaller) ListRemote() ([]string, error) {
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

// Install downloads and installs a Rust version.
func (r *RustInstaller) Install(version string, dest string) error {
	// Use rustup if available
	if rustupExists() {
		return r.installViaRustup(version, dest)
	}

	// Otherwise, download and run rustup installer
	return r.installRustup(version, dest)
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

// installRustup downloads and runs rustup installer.
func (r *RustInstaller) installRustup(version string, dest string) error {
	// Download rustup-init
	var filename string
	if runtime.GOOS == "windows" {
		filename = "rustup-init.exe"
	} else {
		filename = "rustup-init.sh"
	}

	url := fmt.Sprintf("https://static.rust-lang.org/rustup/dist/%s-%s/%s", runtime.GOOS, runtime.GOARCH, filename)
	fmt.Printf("Downloading rustup from %s...\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "rustup-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Make executable and run
	os.Chmod(tmpFile.Name(), 0755)
	cmd := exec.Command(tmpFile.Name(), "-y", "--default-toolchain", version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rustup installation failed: %w", err)
	}

	// Now use rustup to install
	return r.installViaRustup(version, dest)
}

// PostInstall performs post-installation setup.
func (r *RustInstaller) PostInstall(version, dest string) error {
	return nil
}

// BinaryPaths returns the paths to Rust binaries.
func (r *RustInstaller) BinaryPaths(version, dest string) []string {
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
