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

// BunInstaller installs Bun versions.
type BunInstaller struct{}

func init() {
	env.RegisterInstaller("bun", &BunInstaller{})
}

// Name returns the runtime name.
func (b *BunInstaller) Name() string {
	return "bun"
}

// ListRemote fetches available Bun versions from GitHub releases.
func (b *BunInstaller) ListRemote() ([]string, error) {
	resp, err := http.Get("https://api.github.com/repos/oven-sh/bun/releases")
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

// ValidateVersion validates a Bun version string.
func (b *BunInstaller) ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	return nil
}

// Install downloads and installs a Bun version.
func (b *BunInstaller) Install(version string, dest string) error {
	// Determine platform
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go arch to Bun arch names
	arch := goarch
	if goarch == "amd64" {
		arch = "x64"
	} else if goarch == "arm64" {
		arch = "aarch64"
	}

	// Map Go OS to Bun OS names
	osName := goos
	if goos == "darwin" {
		osName = "darwin"
	} else if goos == "linux" {
		osName = "linux"
	}

	var ext string
	if goos == "windows" {
		ext = ".exe"
	}

	filename := fmt.Sprintf("bun-%s-%s.%s%s", osName, arch, "zip", ext)
	if goos != "windows" {
		filename = fmt.Sprintf("bun-%s-%s.zip", osName, arch)
	}

	url := fmt.Sprintf("https://github.com/oven-sh/bun/releases/download/bun-v%s/%s", version, filename)

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
	tmpFile, err := os.CreateTemp("", "bun-*.tmp")
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

	// Bun extracts to bun-<os>-<arch>/, move binary up
	entries, _ := os.ReadDir(dest)
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "bun-") {
			bunDir := filepath.Join(dest, entry.Name())
			bunBinary := filepath.Join(bunDir, "bun")
			if goos == "windows" {
				bunBinary = filepath.Join(bunDir, "bun.exe")
			}
			if _, err := os.Stat(bunBinary); err == nil {
				// Move binary to bin/
				binDir := filepath.Join(dest, "bin")
				os.MkdirAll(binDir, 0755)
				target := filepath.Join(binDir, filepath.Base(bunBinary))
				os.Rename(bunBinary, target)
				os.RemoveAll(bunDir)
			}
			break
		}
	}

	return nil
}

// PostInstall performs post-installation setup.
func (b *BunInstaller) PostInstall(version, dest string) error {
	// Make bun executable
	bunPath := filepath.Join(dest, "bin", "bun")
	if runtime.GOOS == "windows" {
		bunPath = filepath.Join(dest, "bin", "bun.exe")
	}
	if info, err := os.Stat(bunPath); err == nil {
		os.Chmod(bunPath, info.Mode()|0111)
	}
	return nil
}

// BinaryPaths returns the paths to Bun binaries.
func (b *BunInstaller) BinaryPaths(version, dest string) []string {
	if runtime.GOOS == "windows" {
		return []string{"bin\\bun.exe"}
	}
	return []string{"bin/bun"}
}

