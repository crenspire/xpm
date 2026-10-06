package runtimes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// GoInstaller installs Go versions.
type GoInstaller struct{}

func init() {
	env.RegisterInstaller("go", &GoInstaller{})
}

// Name returns the runtime name.
func (g *GoInstaller) Name() string {
	return "go"
}

// ListRemote fetches available Go versions.
func (g *GoInstaller) ListRemote() ([]string, error) {
	resp, err := downloadClient.Get(goDLURL + "/?mode=json&include=all")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var releases []struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	var versions []string
	seen := make(map[string]bool)
	for _, release := range releases {
		// Remove 'go' prefix
		version := strings.TrimPrefix(release.Version, "go")
		if !seen[version] {
			versions = append(versions, version)
			seen[version] = true
		}
	}

	return versions, nil
}

// ValidateVersion validates a Go version string.
func (g *GoInstaller) ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	// Allow special aliases
	if version == "latest" {
		return nil
	}
	if !strings.HasPrefix(version, "1.") && !strings.HasPrefix(version, "0.") {
		return fmt.Errorf("invalid Go version format")
	}
	return nil
}

// GetLatestVersion returns the latest Go version.
func (g *GoInstaller) GetLatestVersion() (string, error) {
	versions, err := g.ListRemote()
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available")
	}
	// First version should be the latest
	return versions[0], nil
}

// Install downloads and installs a Go version.
func (g *GoInstaller) Install(version string, dest string) error {
	// Handle special aliases
	if version == "latest" {
		latest, err := g.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		fmt.Printf("Resolved 'latest' to %s\n", latest)
		version = latest
	}

	// Determine platform
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go arch names
	arch := goarch
	if goarch == "amd64" {
		arch = "amd64"
	} else if goarch == "arm64" {
		arch = "arm64"
	}

	filename := fmt.Sprintf("go%s.%s-%s.tar.gz", version, goos, arch)
	meta, err := fetchSmall(goDLURL + "/?mode=json&include=all")
	if err != nil {
		return fmt.Errorf("fetch Go release list: %w", err)
	}
	want, err := goChecksum(meta, filename)
	if err != nil {
		return err
	}
	fmt.Printf("Downloading %s/%s...\n", goDLURL, filename)
	archive, err := downloadVerified(goDLURL+"/"+filename, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)

	// Extract
	if err := extractTarGz(archive, dest); err != nil {
		return err
	}

	// Go extracts to go/, move contents up
	goDir := filepath.Join(dest, "go")
	if info, err := os.Stat(goDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(goDir)
		for _, entry := range entries {
			oldPath := filepath.Join(goDir, entry.Name())
			newPath := filepath.Join(dest, entry.Name())
			os.Rename(oldPath, newPath)
		}
		os.Remove(goDir)
	}

	return nil
}

// PostInstall performs post-installation setup.
func (g *GoInstaller) PostInstall(version, dest string) error {
	return nil
}

// BinaryPaths returns the paths to Go binaries.
func (g *GoInstaller) BinaryPaths(version, dest string) []string {
	return []string{"bin/go"}
}
