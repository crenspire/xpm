package runtimes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// NodeInstaller installs Node.js versions.
type NodeInstaller struct{}

func init() {
	env.RegisterInstaller("node", &NodeInstaller{})
}

// Name returns the runtime name.
func (n *NodeInstaller) Name() string {
	return "node"
}

// NodeRelease represents a Node.js release from the API.
type NodeRelease struct {
	Version string      `json:"version"`
	LTS     interface{} `json:"lts"` // Can be bool or string
}

// ListRemote fetches available Node.js versions.
func (n *NodeInstaller) ListRemote(_ context.Context) ([]string, error) {
	resp, err := http.Get("https://nodejs.org/dist/index.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var releases []NodeRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	var versions []string
	for _, release := range releases {
		// Remove 'v' prefix
		version := strings.TrimPrefix(release.Version, "v")
		versions = append(versions, version)
	}

	return versions, nil
}

// GetLatestVersion returns the latest current version.
func (n *NodeInstaller) GetLatestVersion() (string, error) {
	versions, err := n.ListRemote(context.Background())
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available")
	}
	// First version is the latest
	return versions[0], nil
}

// LatestLTS implements env.LTSResolver.
func (n *NodeInstaller) LatestLTS(_ context.Context) (string, error) {
	return n.GetLTSVersion()
}

// GetLTSVersion returns the latest LTS version.
func (n *NodeInstaller) GetLTSVersion() (string, error) {
	resp, err := http.Get("https://nodejs.org/dist/index.json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var releases []NodeRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", err
	}

	// Find the first LTS version (they're ordered newest first)
	for _, release := range releases {
		// LTS can be a boolean true or a string (LTS name)
		if release.LTS != nil {
			// Check if it's a truthy value
			ltsValue := false
			switch v := release.LTS.(type) {
			case bool:
				ltsValue = v
			case string:
				ltsValue = v != "" && v != "false"
			}
			if ltsValue {
				return strings.TrimPrefix(release.Version, "v"), nil
			}
		}
	}

	return "", fmt.Errorf("no LTS version found")
}

// resolveVersion resolves a version string to a full semantic version.
// If only major or major.minor is provided, it finds the latest matching version.
func (n *NodeInstaller) resolveVersion(version string) (string, error) {
	parts := strings.Split(version, ".")
	if len(parts) >= 3 {
		// Already a full version
		return version, nil
	}

	// Fetch available versions
	versions, err := n.ListRemote(context.Background())
	if err != nil {
		return "", err
	}

	// Find the latest version matching the prefix
	var candidates []string
	for _, v := range versions {
		if strings.HasPrefix(v, version) {
			candidates = append(candidates, v)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no version found matching %s", version)
	}

	// Sort and return the latest (first in list, as ListRemote returns newest first)
	return candidates[0], nil
}

// ValidateVersion validates a Node.js version string.
func (n *NodeInstaller) ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	// Allow special aliases
	if version == "lts" || version == "latest" {
		return nil
	}
	// Basic validation - version should start with a number
	if version[0] < '0' || version[0] > '9' {
		return fmt.Errorf("invalid version format")
	}
	return nil
}

// Install adapts the v1 installer to the v2 interface; the installer
// rewrite replaces it.
func (n *NodeInstaller) Install(_ context.Context, req env.InstallRequest) error {
	if err := n.install(req.Version, req.Dest); err != nil {
		return err
	}
	return n.PostInstall(req.Version, req.Dest)
}

// Install downloads and installs a Node.js version.
// Returns the resolved version and any alias used.
func (n *NodeInstaller) install(version string, dest string) error {
	return n.InstallWithAlias(version, dest, "")
}

// InstallWithAlias downloads and installs a Node.js version, storing the alias.
func (n *NodeInstaller) InstallWithAlias(version string, dest string, alias string) error {
	// Handle special aliases
	if version == "latest" {
		latest, err := n.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		fmt.Printf("Resolved 'latest' to %s\n", latest)
		alias = "latest"
		version = latest
	} else if version == "lts" {
		lts, err := n.GetLTSVersion()
		if err != nil {
			return fmt.Errorf("failed to get LTS version: %w", err)
		}
		fmt.Printf("Resolved 'lts' to %s\n", lts)
		alias = "lts"
		version = lts
	} else {
		// Resolve version if only major or major.minor is provided
		resolvedVersion, err := n.resolveVersion(version)
		if err != nil {
			return fmt.Errorf("failed to resolve version %s: %w", version, err)
		}
		if resolvedVersion != version {
			fmt.Printf("Resolved %s to %s\n", version, resolvedVersion)
			version = resolvedVersion
		}
	}

	// Determine platform and architecture
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	var ext string
	if goos == "windows" {
		ext = "zip"
	} else {
		ext = "tar.gz"
	}

	// Map Go arch to Node.js arch names
	arch := goarch
	if goarch == "amd64" {
		arch = "x64"
	} else if goarch == "arm64" {
		arch = "arm64"
	}

	// Map Go OS to Node.js OS names
	nodeos := goos
	if goos == "darwin" {
		nodeos = "darwin"
	}

	filename := fmt.Sprintf("node-v%s-%s-%s.%s", version, nodeos, arch, ext)
	base := fmt.Sprintf("%s/v%s", nodeDistURL, version)
	sums, err := fetchSmall(context.TODO(), base+"/SHASUMS256.txt")
	if err != nil {
		return fmt.Errorf("fetch Node.js checksums: %w", err)
	}
	want, err := checksumFromSums(string(sums), filename)
	if err != nil {
		return err
	}
	fmt.Printf("Downloading %s/%s...\n", base, filename)
	archive, err := downloadVerified(context.TODO(), base+"/"+filename, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)

	// Extract to a temporary directory first
	tmpExtractDir, err := os.MkdirTemp("", "node-extract-*")
	if err != nil {
		return fmt.Errorf("failed to create temp extract dir: %w", err)
	}
	defer os.RemoveAll(tmpExtractDir)

	// Extract
	if ext == "zip" {
		if err := extractZip(archive, tmpExtractDir); err != nil {
			return err
		}
	} else {
		if err := extractTarGz(archive, tmpExtractDir); err != nil {
			return err
		}
	}

	// Node.js extracts to node-v<version>-<os>-<arch>/, find and copy contents
	expectedDir := fmt.Sprintf("node-v%s-%s-%s", version, nodeos, arch)
	extractedPath := filepath.Join(tmpExtractDir, expectedDir)

	// Check if the directory exists
	if info, err := os.Stat(extractedPath); err == nil && info.IsDir() {
		// Debug: Check what's in bin/ before copying
		binDir := filepath.Join(extractedPath, "bin")
		if entries, err := os.ReadDir(binDir); err == nil {
			fmt.Printf("Found in source bin/: ")
			for _, entry := range entries {
				entryPath := filepath.Join(binDir, entry.Name())
				linkInfo, _ := os.Lstat(entryPath)
				if linkInfo.Mode()&os.ModeSymlink != 0 {
					target, _ := os.Readlink(entryPath)
					fmt.Printf("%s -> %s, ", entry.Name(), target)
				} else {
					fmt.Printf("%s, ", entry.Name())
				}
			}
			fmt.Println()
		}

		// Copy contents from extracted directory to destination
		if err := copyDirectory(extractedPath, dest); err != nil {
			return fmt.Errorf("failed to copy extracted files: %w", err)
		}
	} else {
		// If the expected directory doesn't exist, check if files are directly in tmpExtractDir
		// This shouldn't happen with Node.js archives, but handle it gracefully
		entries, err := os.ReadDir(tmpExtractDir)
		if err != nil {
			return fmt.Errorf("failed to read extract directory: %w", err)
		}
		// Look for the expected directory in case it's nested differently
		found := false
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "node-v") {
				extractedPath = filepath.Join(tmpExtractDir, entry.Name())
				if err := copyDirectory(extractedPath, dest); err != nil {
					return fmt.Errorf("failed to copy extracted files: %w", err)
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("could not find extracted Node.js directory in %s", tmpExtractDir)
		}
	}

	// Verify that key binaries were copied
	expectedBinaries := []string{"bin/node", "bin/npm", "bin/npx"}
	var missing []string
	for _, bin := range expectedBinaries {
		binPath := filepath.Join(dest, bin)
		if _, err := os.Stat(binPath); err != nil {
			missing = append(missing, bin)
		}
	}

	if len(missing) > 0 {
		// List what actually exists in bin/ with details
		binDir := filepath.Join(dest, "bin")
		var existing []string
		var existingDetails []string
		if entries, err := os.ReadDir(binDir); err == nil {
			for _, entry := range entries {
				existing = append(existing, entry.Name())
				entryPath := filepath.Join(binDir, entry.Name())
				if linkInfo, err := os.Lstat(entryPath); err == nil {
					if linkInfo.Mode()&os.ModeSymlink != 0 {
						target, _ := os.Readlink(entryPath)
						existingDetails = append(existingDetails, fmt.Sprintf("%s -> %s", entry.Name(), target))
					} else {
						existingDetails = append(existingDetails, entry.Name())
					}
				}
			}
		}

		// Try to manually create npm/npx if they're missing but the target exists
		// npm typically points to ../lib/node_modules/npm/bin/npm-cli.js
		if len(missing) > 0 {
			for _, missingBin := range missing {
				binName := filepath.Base(missingBin)
				// Check common npm/npx locations
				possibleTargets := []string{
					filepath.Join(dest, "lib", "node_modules", "npm", "bin", binName+"-cli.js"),
					filepath.Join(dest, "lib", "node_modules", "npm", "bin", binName+".js"),
				}
				for _, target := range possibleTargets {
					if _, err := os.Stat(target); err == nil {
						// Create the symlink
						binPath := filepath.Join(dest, "bin", binName)
						relTarget, err := filepath.Rel(filepath.Join(dest, "bin"), target)
						if err == nil {
							os.Remove(binPath) // Remove if exists
							if err := os.Symlink(relTarget, binPath); err == nil {
								// Successfully created, remove from missing
								missing = removeString(missing, missingBin)
								fmt.Printf("Created missing symlink: %s -> %s\n", binPath, relTarget)
								break
							}
						}
					}
				}
			}
		}

		// Check again after attempting to fix
		if len(missing) > 0 {
			var suggestions []string
			suggestions = append(suggestions, "\nTroubleshooting:")
			suggestions = append(suggestions, fmt.Sprintf("Found in bin/: %v", existingDetails))
			suggestions = append(suggestions, fmt.Sprintf("Missing: %v", missing))
			suggestions = append(suggestions, fmt.Sprintf("Installation directory: %s", dest))
			suggestions = append(suggestions, "This may be a Node.js archive structure issue. Try a different version.")

			return fmt.Errorf("installation incomplete: missing binaries %v.\n%s",
				missing, strings.Join(suggestions, "\n"))
		}
	}

	return nil
}

// PostInstall performs post-installation setup.
func (n *NodeInstaller) PostInstall(version, dest string) error {
	// Ensure binaries are executable
	binaryPaths := n.BinaryPaths()
	for _, relPath := range binaryPaths {
		fullPath := filepath.Join(dest, relPath)
		if info, err := os.Stat(fullPath); err == nil {
			// Make executable if not already
			if info.Mode()&0111 == 0 {
				os.Chmod(fullPath, 0755)
			}
		}
	}
	return nil
}

// BinaryPaths returns the paths to Node.js binaries.
func (n *NodeInstaller) BinaryPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{"node.exe", "npm.cmd", "npx.cmd"}
	}
	return []string{"bin/node", "bin/npm", "bin/npx"}
}

// removeString removes a string from a slice.
func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
