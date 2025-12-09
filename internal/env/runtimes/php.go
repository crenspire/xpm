package runtimes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// PHPInstaller installs PHP versions.
type PHPInstaller struct{}

func init() {
	env.RegisterInstaller("php", &PHPInstaller{})
}

// Name returns the runtime name.
func (p *PHPInstaller) Name() string {
	return "php"
}

// ListRemote fetches available PHP versions.
func (p *PHPInstaller) ListRemote() ([]string, error) {
	// PHP.net releases API
	resp, err := http.Get("https://www.php.net/releases/index.php?json&max=100")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var releases map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		// Fallback to common versions
		return []string{
			"8.3.0", "8.2.14", "8.2.13", "8.1.27", "8.1.26",
			"8.0.30", "8.0.29", "7.4.33",
		}, nil
	}

	var versions []string
	for version := range releases {
		versions = append(versions, version)
	}

	// Sort versions semantically (newest first)
	sortVersions(versions)

	return versions, nil
}

// sortVersions sorts version strings in descending order (newest first).
func sortVersions(versions []string) {
	// Simple bubble sort for semantic versions (newest first)
	// This is a simplified version - a proper implementation would use a semantic version library
	for i := 0; i < len(versions)-1; i++ {
		for j := i + 1; j < len(versions); j++ {
			if compareVersions(versions[i], versions[j]) < 0 {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}
}

// compareVersions compares two version strings.
// Returns: >0 if v1 > v2, <0 if v1 < v2, 0 if v1 == v2
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}
	
	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &num1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &num2)
		}
		
		if num1 > num2 {
			return 1
		}
		if num1 < num2 {
			return -1
		}
	}
	
	return 0
}

// ValidateVersion validates a PHP version string.
func (p *PHPInstaller) ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}
	// Allow special aliases
	if version == "latest" {
		return nil
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid version format")
	}
	return nil
}

// GetLatestVersion returns the latest PHP version.
func (p *PHPInstaller) GetLatestVersion() (string, error) {
	versions, err := p.ListRemote()
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available")
	}
	// Versions are already sorted by ListRemote (newest first)
	return versions[0], nil
}

// Install downloads and installs a PHP version.
func (p *PHPInstaller) Install(version string, dest string) error {
	// Handle special aliases
	if version == "latest" {
		latest, err := p.GetLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		fmt.Printf("Resolved 'latest' to %s\n", latest)
		version = latest
	}

	// Try phpbrew first if available
	if phpbrewExists() {
		return p.installViaPhpbrew(version, dest)
	}

	// Try Homebrew on macOS
	if runtime.GOOS == "darwin" && homebrewExists() {
		return p.installViaHomebrew(version, dest)
	}

	// Try to download prebuilt binaries (macOS only for now)
	if runtime.GOOS == "darwin" {
		return p.installPrebuiltMacOS(version, dest)
	}

	// For other platforms, provide helpful error
	return fmt.Errorf("PHP installation on %s requires phpbrew or manual compilation.\n\nInstall phpbrew: curl -L -O https://github.com/phpbrew/phpbrew/releases/latest/download/phpbrew.phar && chmod +x phpbrew.phar && sudo mv phpbrew.phar /usr/local/bin/phpbrew\n\nThen run: phpbrew install %s", runtime.GOOS, version)
}

// phpbrewExists checks if phpbrew is available.
func phpbrewExists() bool {
	_, err := exec.LookPath("phpbrew")
	return err == nil
}

// installViaPhpbrew uses phpbrew if available.
func (p *PHPInstaller) installViaPhpbrew(version string, dest string) error {
	// Use phpbrew to install
	cmd := exec.Command("phpbrew", "install", version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("phpbrew install failed: %w", err)
	}

	// Get phpbrew root
	phpbrewRoot := os.Getenv("PHPBREW_ROOT")
	if phpbrewRoot == "" {
		home, _ := os.UserHomeDir()
		phpbrewRoot = filepath.Join(home, ".phpbrew")
	}

	phpbrewVersionPath := filepath.Join(phpbrewRoot, "php", "php-"+version)
	if _, err := os.Stat(phpbrewVersionPath); err != nil {
		return fmt.Errorf("phpbrew installation not found at %s", phpbrewVersionPath)
	}

	// Copy to our location
	return copyDirectory(phpbrewVersionPath, dest)
}

// homebrewExists checks if Homebrew is available.
func homebrewExists() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// installViaHomebrew uses Homebrew to install PHP.
func (p *PHPInstaller) installViaHomebrew(version string, dest string) error {
	// Extract major version (e.g., 8.3.0 -> 8.3)
	parts := strings.Split(version, ".")
	majorMinor := strings.Join(parts[:2], ".")
	
	// Use shivammathur/php tap which provides multiple PHP versions
	fmt.Printf("Installing PHP %s via Homebrew...\n", version)
	
	// First, ensure the tap is added
	tapCmd := exec.Command("brew", "tap", "shivammathur/php")
	tapCmd.Stdout = os.Stdout
	tapCmd.Stderr = os.Stderr
	tapCmd.Run() // Ignore errors, tap might already exist
	
	// Install PHP version
	installCmd := exec.Command("brew", "install", fmt.Sprintf("shivammathur/php/php@%s", majorMinor))
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("homebrew install failed: %w\n\nTry running manually: brew install shivammathur/php/php@%s", err, majorMinor)
	}
	
	// Find the installed PHP location
	// Try the specific versioned path first
	brewPrefixCmd := exec.Command("brew", "--prefix", fmt.Sprintf("shivammathur/php/php@%s", majorMinor))
	brewPrefix, err := brewPrefixCmd.Output()
	if err != nil {
		// Fallback: try the standard php@X.Y path (without the tap prefix)
		brewPrefixCmd = exec.Command("brew", "--prefix", fmt.Sprintf("php@%s", majorMinor))
		brewPrefix, err = brewPrefixCmd.Output()
		if err != nil {
			// Final fallback: try standard Homebrew PHP location
			brewPrefixCmd = exec.Command("brew", "--prefix", "php")
			brewPrefix, err = brewPrefixCmd.Output()
			if err != nil {
				return fmt.Errorf("failed to find installed PHP location: %w\n\nTry running: brew --prefix shivammathur/php/php@%s", err, majorMinor)
			}
		}
	}
	
	phpPath := strings.TrimSpace(string(brewPrefix))
	
	// Resolve symlinks - Homebrew often uses symlinks for versioned packages
	resolvedPath, err := filepath.EvalSymlinks(phpPath)
	if err == nil && resolvedPath != phpPath {
		fmt.Printf("Resolved symlink %s -> %s\n", phpPath, resolvedPath)
		phpPath = resolvedPath
	}
	
	// Debug: List what's in the source directory
	entries, _ := os.ReadDir(phpPath)
	var found []string
	for _, entry := range entries {
		found = append(found, entry.Name())
	}
	fmt.Printf("Source PHP directory contents: %v\n", found)
	
	// Verify PHP is installed - check multiple possible locations
	possibleBins := []string{
		filepath.Join(phpPath, "bin", "php"),
		filepath.Join(phpPath, "sbin", "php-fpm"), // Some installations have php-fpm in sbin
		filepath.Join(phpPath, "php"),             // Sometimes PHP is at root
	}
	
	var phpBin string
	for _, bin := range possibleBins {
		if _, err := os.Stat(bin); err == nil {
			phpBin = bin
			fmt.Printf("Found PHP binary at: %s\n", phpBin)
			break
		}
	}
	
	if phpBin == "" {
		// List what's actually in the directory for debugging
		return fmt.Errorf("PHP binary not found at %s\n\nFound in directory: %v\n\nTry checking: ls -la %s", phpPath, found, phpPath)
	}
	
	// Ensure destination directory exists and is empty
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	// Copy PHP installation to our destination
	fmt.Printf("Copying PHP from %s to %s...\n", phpPath, dest)
	if err := copyDirectory(phpPath, dest); err != nil {
		return fmt.Errorf("failed to copy PHP installation: %w\n\nSource: %s\nDestination: %s", err, phpPath, dest)
	}
	
	// Debug: List what was actually copied
	copiedEntries, _ := os.ReadDir(dest)
	fmt.Printf("Copied %d items to destination\n", len(copiedEntries))
	
	// Verify the copy succeeded by checking if destination has any contents
	destEntries, err := os.ReadDir(dest)
	if err != nil {
		return fmt.Errorf("failed to read destination directory after copy: %w", err)
	}
	
	if len(destEntries) == 0 {
		return fmt.Errorf("copy appeared to succeed but destination directory is empty.\n\nSource: %s\nDestination: %s\n\nThis might indicate a permissions issue or the copyDirectory function failed silently.", phpPath, dest)
	}
	
	// Verify the binary exists in the destination after copying
	// Check multiple possible locations
	possibleDestBins := []string{
		filepath.Join(dest, "bin", "php"),
		filepath.Join(dest, "sbin", "php-fpm"),
		filepath.Join(dest, "php"), // Sometimes PHP is at root
	}
	
	var foundBin string
	for _, binPath := range possibleDestBins {
		if _, err := os.Stat(binPath); err == nil {
			foundBin = binPath
			break
		}
	}
	
	if foundBin == "" {
		// List what's actually in the destination for debugging
		var found []string
		for _, entry := range destEntries {
			found = append(found, entry.Name())
		}
		
		// Also check if bin/ directory exists and what's in it
		binDir := filepath.Join(dest, "bin")
		var binContents []string
		if binEntries, err := os.ReadDir(binDir); err == nil {
			for _, entry := range binEntries {
				binContents = append(binContents, entry.Name())
			}
		}
		
		return fmt.Errorf("PHP binary not found in destination after copy.\n\nExpected at one of: %v\n\nFound in destination root: %v\nFound in bin/: %v\n\nSource path was: %s\n\nThis might indicate the Homebrew PHP structure is different than expected.", possibleDestBins, found, binContents, phpPath)
	}
	
	fmt.Printf("Successfully copied PHP binary to: %s\n", foundBin)
	return nil
}

// installPrebuiltMacOS downloads prebuilt PHP binaries for macOS.
func (p *PHPInstaller) installPrebuiltMacOS(version string, dest string) error {
	// For macOS without Homebrew, provide instructions
	return fmt.Errorf("PHP installation on macOS requires phpbrew or Homebrew.\n\nOption 1 - Install phpbrew:\n  curl -L -O https://github.com/phpbrew/phpbrew/releases/latest/download/phpbrew.phar\n  chmod +x phpbrew.phar\n  sudo mv phpbrew.phar /usr/local/bin/phpbrew\n  phpbrew install %s\n\nOption 2 - Install Homebrew (if not installed):\n  /bin/bash -c \"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"\n  brew tap shivammathur/php\n  brew install shivammathur/php/php@%s\n\nThen run: xpm env install php@%s", version, strings.Split(version, ".")[0], version)
}

// PostInstall performs post-installation setup.
func (p *PHPInstaller) PostInstall(version, dest string) error {
	// On macOS, create wrapper scripts instead of modifying binaries
	// This avoids breaking the binaries with install_name_tool
	if runtime.GOOS == "darwin" {
		return p.createMacOSWrappers(dest)
	}
	return nil
}

// createMacOSWrappers creates wrapper scripts for PHP binaries on macOS.
// Instead of modifying binaries, we create wrappers that set DYLD_LIBRARY_PATH.
func (p *PHPInstaller) createMacOSWrappers(dest string) error {
	fmt.Printf("Creating wrapper scripts for PHP binaries...\n")
	
	// Get Homebrew prefix
	brewPrefixCmd := exec.Command("brew", "--prefix")
	brewPrefixOutput, err := brewPrefixCmd.Output()
	if err != nil {
		fmt.Printf("Warning: brew not found, skipping wrapper creation\n")
		return nil
	}
	brewPrefix := strings.TrimSpace(string(brewPrefixOutput))
	
	// Find all binaries that need wrappers
	var binaries []string
	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			binaries = append(binaries, path)
		}
		return nil
	}
	
	filepath.Walk(filepath.Join(dest, "bin"), walkFunc)
	filepath.Walk(filepath.Join(dest, "sbin"), walkFunc)
	
	// Build DYLD_LIBRARY_PATH with all Homebrew library directories
	// Start with common packages
	libDirs := []string{
		filepath.Join(brewPrefix, "lib"),
		filepath.Join(brewPrefix, "opt", "tidy-html5", "lib"),
		filepath.Join(brewPrefix, "opt", "freetds", "lib"),
		filepath.Join(brewPrefix, "opt", "openldap", "lib"),
		filepath.Join(brewPrefix, "opt", "gettext", "lib"),
		filepath.Join(brewPrefix, "opt", "openssl@3", "lib"),
		filepath.Join(brewPrefix, "opt", "openssl@1.1", "lib"),
		filepath.Join(brewPrefix, "opt", "libiconv", "lib"),
		filepath.Join(brewPrefix, "opt", "libxml2", "lib"),
		filepath.Join(brewPrefix, "opt", "curl", "lib"),
		filepath.Join(brewPrefix, "opt", "zlib", "lib"),
		filepath.Join(brewPrefix, "opt", "oniguruma", "lib"),
		filepath.Join(brewPrefix, "opt", "libzip", "lib"),
		filepath.Join(brewPrefix, "opt", "icu4c", "lib"),
		filepath.Join(brewPrefix, "opt", "pcre2", "lib"),
		filepath.Join(brewPrefix, "opt", "sqlite", "lib"),
		filepath.Join(brewPrefix, "opt", "gd", "lib"),
		filepath.Join(brewPrefix, "opt", "gmp", "lib"),
		filepath.Join(brewPrefix, "opt", "unixodbc", "lib"),
		filepath.Join(brewPrefix, "opt", "libpq", "lib"),
		filepath.Join(brewPrefix, "opt", "net-snmp", "lib"),
		filepath.Join(brewPrefix, "opt", "libsodium", "lib"),
		filepath.Join(brewPrefix, "opt", "argon2", "lib"),
		filepath.Join(brewPrefix, "opt", "krb5", "lib"),
	}
	
	// Dynamically discover additional Homebrew packages with lib directories
	optDir := filepath.Join(brewPrefix, "opt")
	fmt.Printf("Discovering Homebrew packages in %s...\n", optDir)
	if entries, err := os.ReadDir(optDir); err == nil {
		discoveredCount := 0
		for _, entry := range entries {
			if entry.IsDir() {
				libPath := filepath.Join(optDir, entry.Name(), "lib")
				if _, err := os.Stat(libPath); err == nil {
					// Check if not already in list
					alreadyInList := false
					for _, existing := range libDirs {
						if existing == libPath {
							alreadyInList = true
							break
						}
					}
					if !alreadyInList {
						libDirs = append(libDirs, libPath)
						discoveredCount++
					}
				}
			}
		}
		if discoveredCount > 0 {
			fmt.Printf("Discovered %d additional Homebrew packages with libraries\n", discoveredCount)
		}
	} else {
		fmt.Printf("Warning: could not read %s: %v\n", optDir, err)
	}
	
	// Filter to only existing directories
	var existingLibDirs []string
	for _, dir := range libDirs {
		if _, err := os.Stat(dir); err == nil {
			existingLibDirs = append(existingLibDirs, dir)
		}
	}
	
	dyldLibPath := strings.Join(existingLibDirs, ":")
	
	// Always use wrapper scripts - don't modify binaries
	// Modifying binaries with install_name_tool can break them
	wrappedCount := 0
	for _, binPath := range binaries {
		// Create backup of original binary
		backupPath := binPath + ".orig"
		
		// Check if .orig already exists (from previous installation)
		if _, err := os.Stat(backupPath); err != nil {
			// Backup doesn't exist, create it
			if err := os.Rename(binPath, backupPath); err != nil {
				fmt.Printf("Warning: failed to backup %s: %v\n", binPath, err)
				continue
			}
		}
		
		// Create wrapper script
		wrapperScript := fmt.Sprintf(`#!/bin/bash
export DYLD_LIBRARY_PATH="%s:$DYLD_LIBRARY_PATH"
export DYLD_FALLBACK_LIBRARY_PATH="%s:$DYLD_FALLBACK_LIBRARY_PATH"
exec "%s" "$@"
`, dyldLibPath, dyldLibPath, backupPath)
		
		if err := os.WriteFile(binPath, []byte(wrapperScript), 0755); err != nil {
			fmt.Printf("Warning: failed to create wrapper for %s: %v\n", binPath, err)
			continue
		}
		wrappedCount++
	}
	
	if wrappedCount > 0 {
		fmt.Printf("✓ Fixed library paths in %d binaries\n", wrappedCount)
	}
	
	return nil
}

// fixMacOSLibraryPaths fixes dynamic library paths in PHP binaries on macOS.
// DEPRECATED: This modifies binaries which can break them. Use createMacOSWrappers instead.
func (p *PHPInstaller) fixMacOSLibraryPaths(dest string) error {
	fmt.Printf("Fixing dynamic library paths for PHP binaries...\n")
	
	// Get Homebrew prefix
	brewPrefixCmd := exec.Command("brew", "--prefix")
	brewPrefixOutput, err := brewPrefixCmd.Output()
	if err != nil {
		// If brew is not available, skip fixing (might not be Homebrew install)
		fmt.Printf("Warning: brew not found, skipping library path fixes\n")
		return nil
	}
	brewPrefix := strings.TrimSpace(string(brewPrefixOutput))
	fmt.Printf("Homebrew prefix: %s\n", brewPrefix)
	
	// Find all binaries that need fixing
	var binaries []string
	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		// Check if it's a regular file (not a symlink) and executable
		if !info.IsDir() && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			binaries = append(binaries, path)
		}
		return nil
	}
	
	// Walk bin/ and sbin/ directories
	filepath.Walk(filepath.Join(dest, "bin"), walkFunc)
	filepath.Walk(filepath.Join(dest, "sbin"), walkFunc)
	
	fmt.Printf("Found %d binaries to fix\n", len(binaries))
	
	// Fix each binary
	fixedCount := 0
	for _, binPath := range binaries {
		if fixed, err := p.fixBinaryLibraryPaths(binPath, brewPrefix); err != nil {
			// Log but don't fail - some binaries might not need fixing
			fmt.Printf("Warning: failed to fix library paths in %s: %v\n", binPath, err)
		} else if fixed > 0 {
			fixedCount++
			fmt.Printf("Fixed %d library paths in %s\n", fixed, filepath.Base(binPath))
		}
		// Also fix library dependencies (not just rpaths)
		if fixed, err := p.fixLibraryDependencies(binPath, brewPrefix); err != nil {
			fmt.Printf("Warning: failed to fix library dependencies in %s: %v\n", binPath, err)
		} else if fixed > 0 {
			fmt.Printf("Fixed %d library dependencies in %s\n", fixed, filepath.Base(binPath))
		}
	}
	
	if fixedCount > 0 {
		fmt.Printf("✓ Fixed library paths in %d binaries\n", fixedCount)
	}
	
	return nil
}

// fixBinaryLibraryPaths fixes library paths in a single binary.
// We need to replace @loader_path rpaths with absolute paths.
// Returns the number of paths fixed and any error.
func (p *PHPInstaller) fixBinaryLibraryPaths(binPath, brewPrefix string) (int, error) {
	// Get existing rpaths
	otoolCmd := exec.Command("otool", "-l", binPath)
	otoolOutput, err := otoolCmd.Output()
	if err != nil {
		return 0, nil // Not a binary or otool failed, skip
	}
	
	fixedCount := 0
	lines := strings.Split(string(otoolOutput), "\n")
	
	// Parse otool output to find @loader_path rpaths
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		
		// Look for LC_RPATH command
		if strings.Contains(line, "LC_RPATH") {
			// The path should be 2 lines after LC_RPATH (after cmdsize line)
			if i+2 < len(lines) {
				pathLine := strings.TrimSpace(lines[i+2])
				if strings.HasPrefix(pathLine, "path ") {
					rpath := strings.TrimSpace(strings.TrimPrefix(pathLine, "path "))
					// Remove "(offset 12)" if present
					if idx := strings.Index(rpath, " ("); idx >= 0 {
						rpath = rpath[:idx]
					}
					
					// If it's an @loader_path rpath, replace it
					if strings.HasPrefix(rpath, "@loader_path") {
						// Extract the relative path (e.g., "../../../../opt/openssl@3/lib")
						relPath := strings.TrimPrefix(rpath, "@loader_path")
						if strings.HasPrefix(relPath, "/") {
							relPath = relPath[1:] // Remove leading /
						}
						
						// Try to resolve to absolute path
						var absPath string
						if strings.Contains(relPath, "/opt/") {
							optIndex := strings.Index(relPath, "/opt/")
							if optIndex >= 0 {
								optPath := relPath[optIndex+1:] // Remove leading ../
								possiblePath := filepath.Join(brewPrefix, optPath)
								if _, err := os.Stat(possiblePath); err == nil {
									absPath = possiblePath
								}
							}
						} else if relPath == "" || relPath == "." {
							// @loader_path or @loader_path/ -> use bin directory's parent
							absPath = filepath.Join(brewPrefix, "lib")
						}
						
						if absPath != "" {
							// Delete the old rpath and add the new one
							delCmd := exec.Command("install_name_tool", "-delete_rpath", rpath, binPath)
							delCmd.Run() // Ignore errors
							
							addCmd := exec.Command("install_name_tool", "-add_rpath", absPath, binPath)
							if err := addCmd.Run(); err == nil {
								fixedCount++
							}
						}
					}
				}
			}
		}
	}
	
	// Also ensure common Homebrew library directories are in rpath
	commonRpaths := []string{
		filepath.Join(brewPrefix, "lib"),
		filepath.Join(brewPrefix, "opt", "tidy-html5", "lib"),
		filepath.Join(brewPrefix, "opt", "freetds", "lib"),
		filepath.Join(brewPrefix, "opt", "openldap", "lib"),
		filepath.Join(brewPrefix, "opt", "gettext", "lib"),
		filepath.Join(brewPrefix, "opt", "openssl@3", "lib"),
		filepath.Join(brewPrefix, "opt", "openssl@1.1", "lib"),
		filepath.Join(brewPrefix, "opt", "libiconv", "lib"),
		filepath.Join(brewPrefix, "opt", "libxml2", "lib"),
		filepath.Join(brewPrefix, "opt", "curl", "lib"),
		filepath.Join(brewPrefix, "opt", "zlib", "lib"),
		filepath.Join(brewPrefix, "opt", "oniguruma", "lib"),
		filepath.Join(brewPrefix, "opt", "libzip", "lib"),
		filepath.Join(brewPrefix, "opt", "icu4c", "lib"),
		filepath.Join(brewPrefix, "opt", "pcre2", "lib"),
		filepath.Join(brewPrefix, "opt", "sqlite", "lib"),
		filepath.Join(brewPrefix, "opt", "gd", "lib"),
		filepath.Join(brewPrefix, "opt", "gmp", "lib"),
		filepath.Join(brewPrefix, "opt", "unixodbc", "lib"),
		filepath.Join(brewPrefix, "opt", "libpq", "lib"),
		filepath.Join(brewPrefix, "opt", "net-snmp", "lib"),
		filepath.Join(brewPrefix, "opt", "libsodium", "lib"),
		filepath.Join(brewPrefix, "opt", "argon2", "lib"),
	}
	
	for _, rpath := range commonRpaths {
		// Check if rpath exists
		if _, err := os.Stat(rpath); err != nil {
			continue // Skip if directory doesn't exist
		}
		
		// Add rpath using install_name_tool (will fail silently if already exists)
		installNameCmd := exec.Command("install_name_tool", "-add_rpath", rpath, binPath)
		if err := installNameCmd.Run(); err == nil {
			fixedCount++
		}
	}
	
	return fixedCount, nil
}

// fixLibraryDependencies fixes @loader_path references in library dependencies.
func (p *PHPInstaller) fixLibraryDependencies(binPath, brewPrefix string) (int, error) {
	// Get library dependencies using otool
	otoolCmd := exec.Command("otool", "-L", binPath)
	otoolOutput, err := otoolCmd.Output()
	if err != nil {
		return 0, nil // Not a binary or otool failed, skip
	}
	
	fixedCount := 0
	lines := strings.Split(string(otoolOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, binPath) {
			continue // Skip empty lines and the binary itself
		}
		
		// Look for lines with @loader_path
		if !strings.Contains(line, "@loader_path") {
			continue
		}
		
		// Extract the library path (first field)
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		oldLibPath := parts[0]
		
		if !strings.HasPrefix(oldLibPath, "@loader_path/") {
			continue
		}
		
		// Extract the relative path (e.g., "../../../../opt/tidy-html5/lib/libtidy.58.dylib")
		relPath := strings.TrimPrefix(oldLibPath, "@loader_path/")
		libName := filepath.Base(relPath)
		
		// Try to find the library in Homebrew
		var newLibPath string
		
		// Method 1: Extract path after /opt/ or /lib/
		if strings.Contains(relPath, "/opt/") {
			optIndex := strings.Index(relPath, "/opt/")
			if optIndex >= 0 {
				optPath := relPath[optIndex+1:] // Remove leading ../
				possiblePath := filepath.Join(brewPrefix, optPath)
				if _, err := os.Stat(possiblePath); err == nil {
					newLibPath = possiblePath
				}
			}
		} else if strings.Contains(relPath, "/lib/") {
			libIndex := strings.Index(relPath, "/lib/")
			if libIndex >= 0 {
				libPath := relPath[libIndex+1:] // Remove leading ../
				possiblePath := filepath.Join(brewPrefix, libPath)
				if _, err := os.Stat(possiblePath); err == nil {
					newLibPath = possiblePath
				}
			}
		}
		
		// Method 2: Try to extract package name and use brew --prefix
		if newLibPath == "" && strings.Contains(relPath, "/opt/") {
			optIndex := strings.Index(relPath, "/opt/")
			if optIndex >= 0 {
				afterOpt := relPath[optIndex+5:] // Skip "/opt/"
				parts := strings.Split(afterOpt, "/")
				if len(parts) > 0 {
					packageName := parts[0]
					brewPkgCmd := exec.Command("brew", "--prefix", packageName)
					if pkgPrefix, err := brewPkgCmd.Output(); err == nil {
						pkgPrefixPath := strings.TrimSpace(string(pkgPrefix))
						libRelativePath := strings.Join(parts[1:], "/")
						possiblePath := filepath.Join(pkgPrefixPath, libRelativePath)
						if _, err := os.Stat(possiblePath); err == nil {
							newLibPath = possiblePath
						}
					}
				}
			}
		}
		
		// Method 3: Search common Homebrew library locations
		if newLibPath == "" {
			searchPaths := []string{
				filepath.Join(brewPrefix, "lib", libName),
			}
			
			commonPackages := []string{
				"openssl@3", "openssl@1.1", "libiconv", "libxml2", "curl", "zlib",
				"tidy-html5", "oniguruma", "libzip", "icu4c", "freetype", "jpeg",
				"libpng", "gmp", "libsodium", "argon2", "freetds", "openldap", "gettext",
			}
			
			for _, pkg := range commonPackages {
				searchPaths = append(searchPaths, filepath.Join(brewPrefix, "opt", pkg, "lib", libName))
			}
			
			for _, searchPath := range searchPaths {
				if _, err := os.Stat(searchPath); err == nil {
					newLibPath = searchPath
					break
				}
			}
		}
		
		// If we found the library, update the path
		if newLibPath != "" {
			installNameCmd := exec.Command("install_name_tool", "-change", oldLibPath, newLibPath, binPath)
			if err := installNameCmd.Run(); err != nil {
				return fixedCount, fmt.Errorf("failed to update library path %s -> %s: %w", oldLibPath, newLibPath, err)
			}
			fixedCount++
		}
	}
	
	return fixedCount, nil
}

// fixLoaderPathsToRpath changes @loader_path references to absolute paths.
// We use absolute paths instead of @rpath to avoid issues with library name matching.
func (p *PHPInstaller) fixLoaderPathsToRpath(binPath, brewPrefix string, rpaths []string) error {
	// Get library dependencies
	otoolCmd := exec.Command("otool", "-L", binPath)
	otoolOutput, err := otoolCmd.Output()
	if err != nil {
		return nil // Not a binary, skip
	}
	
	fixedCount := 0
	lines := strings.Split(string(otoolOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, binPath) {
			continue
		}
		
		// Look for @loader_path references
		if !strings.Contains(line, "@loader_path") {
			continue
		}
		
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		oldLibPath := parts[0]
		
		if !strings.HasPrefix(oldLibPath, "@loader_path/") {
			continue
		}
		
		// Extract relative path
		relPath := strings.TrimPrefix(oldLibPath, "@loader_path/")
		
		// Try to find the actual library and use absolute path
		var newLibPath string
		if strings.Contains(relPath, "/opt/") {
			optIndex := strings.Index(relPath, "/opt/")
			if optIndex >= 0 {
				optPath := relPath[optIndex+1:]
				absPath := filepath.Join(brewPrefix, optPath)
				if _, err := os.Stat(absPath); err == nil {
					newLibPath = absPath
				}
			}
		} else if strings.Contains(relPath, "/lib/") {
			// Try /lib/ path
			libIndex := strings.Index(relPath, "/lib/")
			if libIndex >= 0 {
				libPath := relPath[libIndex+1:]
				absPath := filepath.Join(brewPrefix, libPath)
				if _, err := os.Stat(absPath); err == nil {
					newLibPath = absPath
				}
			}
		}
		
		// If not found, try searching in rpaths
		if newLibPath == "" {
			libName := filepath.Base(relPath)
			for _, rpath := range rpaths {
				possiblePath := filepath.Join(rpath, libName)
				if _, err := os.Stat(possiblePath); err == nil {
					newLibPath = possiblePath
					break
				}
			}
		}
		
		// Change the library path
		if newLibPath != "" {
			changeCmd := exec.Command("install_name_tool", "-change", oldLibPath, newLibPath, binPath)
			if err := changeCmd.Run(); err == nil {
				fixedCount++
			}
		}
	}
	
	// Also fix @loader_path rpaths to absolute paths
	otoolLCmd := exec.Command("otool", "-l", binPath)
	otoolLOutput, err := otoolLCmd.Output()
	if err == nil {
		lcLines := strings.Split(string(otoolLOutput), "\n")
		for i, line := range lcLines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "LC_RPATH") {
				if i+2 < len(lcLines) {
					pathLine := strings.TrimSpace(lcLines[i+2])
					if strings.HasPrefix(pathLine, "path ") {
						rpath := strings.TrimSpace(strings.TrimPrefix(pathLine, "path "))
						if idx := strings.Index(rpath, " ("); idx >= 0 {
							rpath = rpath[:idx]
						}
						if strings.HasPrefix(rpath, "@loader_path") {
							// Delete the @loader_path rpath
							delCmd := exec.Command("install_name_tool", "-delete_rpath", rpath, binPath)
							delCmd.Run() // Ignore errors
						}
					}
				}
			}
		}
	}
	
	// Add all rpaths as absolute paths
	for _, rpath := range rpaths {
		addCmd := exec.Command("install_name_tool", "-add_rpath", rpath, binPath)
		addCmd.Run() // Ignore errors if rpath already exists
	}
	
	return nil
}

// BinaryPaths returns the paths to PHP binaries.
func (p *PHPInstaller) BinaryPaths(version, dest string) []string {
	if runtime.GOOS == "windows" {
		return []string{"php.exe"}
	}
	return []string{"bin/php"}
}

