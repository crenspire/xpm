package env

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ANSI color codes for terminal output
const (
	colorReset      = "\033[0m"
	colorBold       = "\033[1m"
	colorCyan       = "\033[36m"
	colorYellow     = "\033[33m"
	colorGreen      = "\033[32m"
	colorBrightCyan = "\033[96m"
)

// VersionInfo contains information about an installed version.
type VersionInfo struct {
	Version string
	Path    string
	Active  bool
	Alias   string // The alias used to install (e.g., "lts", "latest"), empty if installed with explicit version
}

// ListInstalled returns all installed versions grouped by runtime.
func ListInstalled(manager *Manager) (map[string][]VersionInfo, error) {
	result := make(map[string][]VersionInfo)

	runtimesPath := manager.GetRuntimesPath()
	entries, err := os.ReadDir(runtimesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to read runtimes directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		runtime := entry.Name()
		runtimePath := filepath.Join(runtimesPath, runtime)

		versions, err := listVersionsForRuntime(runtimePath, runtime, manager)
		if err != nil {
			continue
		}

		if len(versions) > 0 {
			result[runtime] = versions
		}
	}

	return result, nil
}

// listVersionsForRuntime lists versions for a specific runtime.
func listVersionsForRuntime(runtimePath, runtime string, manager *Manager) ([]VersionInfo, error) {
	entries, err := os.ReadDir(runtimePath)
	if err != nil {
		return nil, err
	}

	var versions []VersionInfo
	activeVersion, _ := manager.GetActiveVersion(runtime)

	// Get installer to validate versions
	installer, err := GetInstaller(runtime)
	if err != nil {
		// If no installer found, still list directories but filter invalid ones
		installer = nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		version := entry.Name()

		// Filter out special aliases and invalid version strings
		if version == "latest" || version == "lts" || version == "" {
			continue
		}

		// Validate version format if installer is available
		if installer != nil {
			if err := installer.ValidateVersion(version); err != nil {
				// Skip invalid version directories (like "latest", "lts", etc.)
				continue
			}
		} else {
			// Basic validation: version should start with a number
			if len(version) == 0 || (version[0] < '0' || version[0] > '9') {
				continue
			}
		}

		versionPath := filepath.Join(runtimePath, version)

		// Verify the installation actually exists and has binaries
		if !isValidInstallation(versionPath, runtime, installer) {
			continue
		}

		// Check for alias metadata
		alias := getVersionAlias(versionPath)

		info := VersionInfo{
			Version: version,
			Path:    versionPath,
			Active:  version == activeVersion,
			Alias:   alias,
		}

		versions = append(versions, info)
	}

	// Sort versions (simple string sort for now)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})

	return versions, nil
}

// isValidInstallation checks if a version directory contains a valid installation.
func isValidInstallation(versionPath, runtime string, installer RuntimeInstaller) bool {
	// Check if directory exists and is not empty
	entries, err := os.ReadDir(versionPath)
	if err != nil || len(entries) == 0 {
		return false
	}

	// If we have an installer, check for expected binaries
	if installer != nil {
		binaryPaths := installer.BinaryPaths("", versionPath) // version not needed for path checking
		for _, binPath := range binaryPaths {
			fullPath := filepath.Join(versionPath, binPath)
			if _, err := os.Stat(fullPath); err == nil {
				return true // At least one expected binary exists
			}
		}
		// If no binaries found but directory exists, might be incomplete installation
		// Return true anyway to show it (user can clean it up)
		return true
	}

	return true
}

// getVersionAlias reads the alias metadata from a version directory.
func getVersionAlias(versionPath string) string {
	metaPath := filepath.Join(versionPath, ".xpm-meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}

	var meta struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}

	return meta.Alias
}

// saveVersionAlias saves the alias metadata to a version directory.
func saveVersionAlias(versionPath, alias string) error {
	metaPath := filepath.Join(versionPath, ".xpm-meta.json")
	meta := struct {
		Alias string `json:"alias"`
	}{
		Alias: alias,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0644)
}

// FormatInstalled formats the installed versions for display.
func FormatInstalled(installed map[string][]VersionInfo) string {
	if len(installed) == 0 {
		return "No runtimes installed.\n\nTo install a runtime, run:\n  xpm env install <runtime>@<version>\n\nExamples:\n  xpm env install node@20.11.0\n  xpm env install python@3.12\n  xpm env install go@1.21\n\nAvailable runtimes: node, python, php, go, java, rust, bun, deno\n\nSee 'xpm env ls-remote <runtime>' to list available versions."
	}

	var output strings.Builder

	// Sort runtimes for consistent output
	runtimes := make([]string, 0, len(installed))
	for runtime := range installed {
		runtimes = append(runtimes, runtime)
	}
	sort.Strings(runtimes)

	for _, runtime := range runtimes {
		versions := installed[runtime]
		// Runtime heading: bold + cyan
		output.WriteString(fmt.Sprintf("%s%s%s%s:\n", colorBold, colorCyan, runtime, colorReset))

		for _, v := range versions {
			marker := "  - "
			if v.Active {
				marker = "  → "
			}

			// Active versions: green + bold
			if v.Active {
				output.WriteString(fmt.Sprintf("%s%s%s%s%s", colorBold, colorGreen, marker, v.Version, colorReset))
			} else {
				output.WriteString(fmt.Sprintf("%s%s", marker, v.Version))
			}

			// Show alias if available
			if v.Alias != "" {
				output.WriteString(fmt.Sprintf(" (%s)", v.Alias))
			}

			// Show active status
			if v.Active {
				output.WriteString(fmt.Sprintf(" %s%s(active)%s", colorBold, colorGreen, colorReset))
			}
			output.WriteString("\n")
		}
	}

	return output.String()
}
