package runtimes

import (
	"context"
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

// JavaInstaller installs Java (JDK) versions.
type JavaInstaller struct{}

func init() {
	env.RegisterInstaller("java", &JavaInstaller{})
}

// Name returns the runtime name.
func (j *JavaInstaller) Name() string {
	return "java"
}

// ListRemote fetches available Java versions from Adoptium API.
func (j *JavaInstaller) ListRemote(_ context.Context) ([]string, error) {
	// Use Adoptium API
	url := "https://api.adoptium.net/v3/info/available_releases"
	resp, err := http.Get(url)
	if err != nil {
		// Fallback to common versions
		return []string{
			"21", "20", "19", "17", "11", "8",
		}, nil
	}
	defer resp.Body.Close()

	var data struct {
		Releases []int `json:"available_releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		// Fallback
		return []string{"21", "20", "19", "17", "11", "8"}, nil
	}

	var versions []string
	for _, release := range data.Releases {
		versions = append(versions, fmt.Sprintf("%d", release))
	}

	return versions, nil
}

// ValidateVersion validates a Java version string.
func (j *JavaInstaller) ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	// Java versions are typically single numbers (8, 11, 17, etc.) or semantic versions
	if version[0] < '0' || version[0] > '9' {
		return fmt.Errorf("invalid version format")
	}
	return nil
}

// Install adapts the v1 installer to the v2 interface; the installer
// rewrite replaces it.
func (j *JavaInstaller) Install(_ context.Context, req env.InstallRequest) error {
	if err := j.install(req.Version, req.Dest); err != nil {
		return err
	}
	return j.PostInstall(req.Version, req.Dest)
}

// Install downloads and installs a Java version.
func (j *JavaInstaller) install(version string, dest string) error {
	// Determine platform
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go arch to Java arch names
	arch := goarch
	if goarch == "amd64" {
		arch = "x64"
	} else if goarch == "arm64" {
		arch = "aarch64"
	}

	// Map Go OS to Java OS names
	osName := goos
	if goos == "darwin" {
		osName = "mac"
	}

	// Use Adoptium API to get download URL
	apiURL := fmt.Sprintf("https://api.adoptium.net/v3/binary/latest/%s/ga/%s/%s/jdk/hotspot/normal/adoptium", version, osName, arch)
	if goos == "darwin" && goarch == "arm64" {
		apiURL = fmt.Sprintf("https://api.adoptium.net/v3/binary/latest/%s/ga/%s/%s/jdk/hotspot/normal/adoptium", version, "mac", "aarch64")
	}

	fmt.Printf("Fetching download URL from Adoptium API...\n")

	// Get download link
	resp, err := http.Get(apiURL + "?redirect=true")
	if err != nil {
		return fmt.Errorf("failed to get download URL: %w", err)
	}
	defer resp.Body.Close()

	// Follow redirect to get actual download URL
	downloadURL := resp.Request.URL.String()
	if downloadURL == "" {
		return fmt.Errorf("no download URL found")
	}

	fmt.Printf("Downloading from %s...\n", downloadURL)

	// Download
	downloadResp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", downloadResp.StatusCode)
	}

	// Determine file extension
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "java-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, downloadResp.Body); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Extract
	if ext == "zip" {
		if err := extractZip(tmpFile.Name(), dest); err != nil {
			return err
		}
	} else {
		if err := extractTarGz(tmpFile.Name(), dest); err != nil {
			return err
		}
	}

	// JDK extracts to jdk-<version>/, move contents up
	entries, _ := os.ReadDir(dest)
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "jdk-") {
			jdkDir := filepath.Join(dest, entry.Name())
			subEntries, _ := os.ReadDir(jdkDir)
			for _, subEntry := range subEntries {
				oldPath := filepath.Join(jdkDir, subEntry.Name())
				newPath := filepath.Join(dest, subEntry.Name())
				os.Rename(oldPath, newPath)
			}
			os.Remove(jdkDir)
			break
		}
	}

	return nil
}

// PostInstall performs post-installation setup.
func (j *JavaInstaller) PostInstall(version, dest string) error {
	return nil
}

// BinaryPaths returns the paths to Java binaries.
func (j *JavaInstaller) BinaryPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{"bin\\java.exe", "bin\\javac.exe", "bin\\keytool.exe"}
	}
	return []string{"bin/java", "bin/javac", "bin/keytool"}
}
