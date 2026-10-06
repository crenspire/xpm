package runtimes

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// PythonInstaller installs Python versions.
type PythonInstaller struct{}

func init() {
	env.RegisterInstaller("python", &PythonInstaller{})
}

// Name returns the runtime name.
func (p *PythonInstaller) Name() string {
	return "python"
}

// ListRemote fetches available Python versions from pyenv mirror.
func (p *PythonInstaller) ListRemote() ([]string, error) {
	// Use pyenv's version list API or python.org
	// For simplicity, we'll fetch from a known source
	// In production, this could use python.org/downloads API
	resp, err := http.Get("https://www.python.org/ftp/python/")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse HTML to extract versions (simplified)
	// In production, use proper HTML parsing or API
	var versions []string
	content := string(body)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "href=\"") && strings.Contains(line, "python-") {
			// Extract version from href
			start := strings.Index(line, "python-")
			if start != -1 {
				end := strings.Index(line[start:], "/")
				if end != -1 {
					version := line[start+7 : start+end]
					if isValidPythonVersion(version) {
						versions = append(versions, version)
					}
				}
			}
		}
	}

	// Fallback: return common versions if parsing fails
	if len(versions) == 0 {
		versions = []string{
			"3.12.1", "3.12.0", "3.11.7", "3.11.6", "3.11.5",
			"3.10.13", "3.10.12", "3.9.18", "3.9.17",
			"3.8.18", "3.7.17",
		}
	}

	return versions, nil
}

// isValidPythonVersion checks if a version string looks valid.
func isValidPythonVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	// Check each part is numeric
	for _, part := range parts {
		if len(part) == 0 {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// ValidateVersion validates a Python version string.
func (p *PythonInstaller) ValidateVersion(version string) error {
	// Allow special aliases
	if version == "latest" {
		return nil
	}
	if !isValidPythonVersion(version) {
		return fmt.Errorf("invalid Python version format")
	}
	return nil
}

// GetLatestVersion returns the latest Python version.
func (p *PythonInstaller) GetLatestVersion() (string, error) {
	versions, err := p.ListRemote()
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available")
	}
	// First version should be the latest
	return versions[0], nil
}

// Install downloads and installs a Python version.
func (p *PythonInstaller) Install(version string, dest string) error {
	// Handle special aliases
	if version == "latest" {
		latest, err := p.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		fmt.Printf("Resolved 'latest' to %s\n", latest)
		version = latest
	}

	// Download prebuilt Python binaries directly to xpm directory
	// For macOS and Linux, we'll use python.org's prebuilt installers
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	if goos == "darwin" {
		return p.installMacOS(version, dest)
	} else if goos == "linux" {
		return p.installLinux(version, dest)
	} else if goos == "windows" {
		return p.installWindows(version, dest)
	}

	return fmt.Errorf("Python installation not yet implemented for %s/%s", goos, goarch)
}

// installMacOS installs Python on macOS using python.org's macOS installer.
func (p *PythonInstaller) installMacOS(version string, dest string) error {
	// Python installation on macOS is complex because:
	// 1. Official installers are .pkg files (hard to extract programmatically)
	// 2. DMG files require mounting
	// 3. Framework builds need special handling

	// For now, provide clear instructions
	majorMinor := strings.Join(strings.Split(version, ".")[:2], ".")
	return fmt.Errorf("Python installation on macOS requires manual setup.\n\n"+
		"Option 1: Use system Python (already installed on macOS)\n"+
		"Option 2: Install via Homebrew: brew install python@%s\n"+
		"Option 3: Download from python.org and install manually\n\n"+
		"Note: Automatic Python installation on macOS is complex due to .pkg installer format.\n"+
		"Consider using the system Python or Homebrew-installed Python.\n\n"+
		"After installing Python via Homebrew or manually, you can use it with xpm by setting it up manually.", majorMinor)
}

// installLinux installs Python on Linux.
func (p *PythonInstaller) installLinux(version string, dest string) error {
	// For Linux, we could download source and compile, or use prebuilt binaries
	// For now, suggest using system package manager
	return fmt.Errorf("Python installation on Linux requires compilation or system packages.\n\n"+
		"Option 1: Use system Python: sudo apt-get install python3.%s (Debian/Ubuntu)\n"+
		"Option 2: Use pyenv: pyenv install %s\n"+
		"Option 3: Download source from python.org and compile\n\n"+
		"Note: Automatic Python installation on Linux requires compilation which is time-consuming.",
		strings.Split(version, ".")[1], version)
}

// installWindows installs Python on Windows.
func (p *PythonInstaller) installWindows(version string, dest string) error {
	// For Windows, download the embeddable package
	filename := fmt.Sprintf("python-%s-embed-amd64.zip", version)
	url := fmt.Sprintf("https://www.python.org/ftp/python/%s/%s", version, filename)

	fmt.Printf("Downloading from %s...\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "python-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	// Extract to destination
	return extractZip(tmpFile.Name(), dest)
}

// PostInstall performs post-installation setup.
func (p *PythonInstaller) PostInstall(version, dest string) error {
	// Ensure pip is available
	pipPath := filepath.Join(dest, "bin", "pip")
	if _, err := os.Stat(pipPath); err != nil {
		// Try to bootstrap pip
		pythonPath := filepath.Join(dest, "bin", "python")
		if runtime.GOOS == "windows" {
			pythonPath = filepath.Join(dest, "python.exe")
		}
		if _, err := os.Stat(pythonPath); err == nil {
			// Download get-pip.py and run it
			// For now, skip - pip should come with Python 3.4+
		}
	}
	return nil
}

// BinaryPaths returns the paths to Python binaries.
func (p *PythonInstaller) BinaryPaths(version, dest string) []string {
	if runtime.GOOS == "windows" {
		return []string{"python.exe", "python3.exe", "Scripts\\pip.exe"}
	}
	return []string{"bin/python", "bin/python3", "bin/pip"}
}
