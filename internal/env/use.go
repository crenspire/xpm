package env

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UseVersion switches to a specific version of a runtime.
func UseVersion(manager *Manager, runtime, version string, global bool) error {
	// Try to resolve version if it's a partial version (e.g., "20" -> "20.11.0")
	// First check if the exact version is installed
	versionPath := filepath.Join(manager.GetRuntimesPath(), runtime, version)
	if _, err := os.Stat(versionPath); err != nil {
		// Try to find a matching installed version
		runtimePath := filepath.Join(manager.GetRuntimesPath(), runtime)
		entries, err := os.ReadDir(runtimePath)
		if err == nil {
			// Look for versions that start with the requested version
			var candidates []string
			for _, entry := range entries {
				if entry.IsDir() && strings.HasPrefix(entry.Name(), version) {
					candidates = append(candidates, entry.Name())
				}
			}
			if len(candidates) > 0 {
				// Use the first (should be sorted, but pick the most specific match)
				// Prefer exact match, then longest match
				var bestMatch string
				for _, candidate := range candidates {
					if candidate == version {
						bestMatch = candidate
						break
					}
					if bestMatch == "" || len(candidate) > len(bestMatch) {
						bestMatch = candidate
					}
				}
				if bestMatch != "" {
					version = bestMatch
					versionPath = filepath.Join(runtimePath, version)
				}
			}
		}

		// If still not found, return error
		if _, err := os.Stat(versionPath); err != nil {
			return fmt.Errorf("version %s@%s is not installed. Run 'xpm env install %s@%s' first", runtime, version, runtime, version)
		}
	}

	// Set active version
	if err := manager.SetActiveVersion(runtime, version, global); err != nil {
		return err
	}

	scope := "local"
	if global {
		scope = "global"
	}

	fmt.Printf("Switched to %s@%s (%s)\n", runtime, version, scope)

	// Update shims
	if err := CreateShims(manager); err != nil {
		return fmt.Errorf("failed to update shims: %w", err)
	}

	// Check if shims are in PATH
	if !CheckPATH(manager) {
		shimsPath := manager.GetShimsPath()
		fmt.Printf("\n⚠️  WARNING: Shims directory is not in your PATH!\n")
		fmt.Printf("   The activated runtime (%s@%s) will not be used until PATH is configured.\n\n", runtime, version)
		fmt.Printf("   Quick fix (current session only):\n")
		fmt.Printf("     export PATH=\"%s:$PATH\"\n\n", shimsPath)
		fmt.Printf("   Permanent fix (recommended):\n")
		fmt.Printf("     xpm env setup-path\n")
		fmt.Printf("     # Then restart your shell or run: source ~/.zshrc\n\n")
		fmt.Printf("   Shims directory: %s\n", shimsPath)
		fmt.Printf("   Current PHP: %s (system version, not from xpm)\n\n", getSystemPHPVersion())
	}

	return nil
}

// getSystemPHPVersion returns the version of PHP found in system PATH.
func getSystemPHPVersion() string {
	cmd := exec.Command("php", "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return "unknown"
}
