package runtimes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// DenoInstaller installs Deno versions.
type DenoInstaller struct{}

func init() {
	env.RegisterInstaller("deno", &DenoInstaller{})
}

// Name returns the runtime name.
func (d *DenoInstaller) Name() string {
	return "deno"
}

// ListRemote fetches available Deno versions from GitHub releases.
func (d *DenoInstaller) ListRemote() ([]string, error) {
	resp, err := http.Get("https://api.github.com/repos/denoland/deno/releases")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var releases []struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	var versions []string
	for _, release := range releases {
		// Remove 'v' prefix
		version := strings.TrimPrefix(release.TagName, "v")
		versions = append(versions, version)
	}

	return versions, nil
}

// ValidateVersion validates a Deno version string.
func (d *DenoInstaller) ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	return nil
}

// Install downloads and installs a Deno version.
func (d *DenoInstaller) Install(version string, dest string) error {
	// Determine platform
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go arch to Deno arch names
	arch := goarch
	if goarch == "amd64" {
		arch = "x86_64"
	} else if goarch == "arm64" {
		arch = "aarch64"
	}

	// Map Go OS to Deno OS names
	osName := goos
	if goos == "darwin" {
		osName = "apple-darwin"
	} else if goos == "linux" {
		osName = "unknown-linux-gnu"
	} else if goos == "windows" {
		osName = "pc-windows-msvc"
	}

	filename := fmt.Sprintf("deno-%s-%s.zip", arch, osName)
	url := fmt.Sprintf("https://github.com/denoland/deno/releases/download/v%s/%s", version, filename)

	fmt.Printf("Downloading from %s...\n", url)

	// Download
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "deno-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Extract zip
	if err := extractZip(tmpFile.Name(), dest); err != nil {
		return err
	}

	// Deno extracts to deno-<arch>-<os>/, move binary up
	entries, _ := os.ReadDir(dest)
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "deno-") {
			denoDir := filepath.Join(dest, entry.Name())
			denoBinary := filepath.Join(denoDir, "deno")
			if goos == "windows" {
				denoBinary = filepath.Join(denoDir, "deno.exe")
			}
			if _, err := os.Stat(denoBinary); err == nil {
				// Move binary to bin/
				binDir := filepath.Join(dest, "bin")
				os.MkdirAll(binDir, 0755)
				target := filepath.Join(binDir, filepath.Base(denoBinary))
				os.Rename(denoBinary, target)
				os.RemoveAll(denoDir)
			}
			break
		}
	}

	return nil
}

// PostInstall performs post-installation setup.
func (d *DenoInstaller) PostInstall(version, dest string) error {
	// Make deno executable
	denoPath := filepath.Join(dest, "bin", "deno")
	if runtime.GOOS == "windows" {
		denoPath = filepath.Join(dest, "bin", "deno.exe")
	}
	if info, err := os.Stat(denoPath); err == nil {
		os.Chmod(denoPath, info.Mode()|0111)
	}
	return nil
}

// BinaryPaths returns the paths to Deno binaries.
func (d *DenoInstaller) BinaryPaths(version, dest string) []string {
	if runtime.GOOS == "windows" {
		return []string{"bin\\deno.exe"}
	}
	return []string{"bin/deno"}
}
