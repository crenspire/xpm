package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/logx"
)

// InstallRuntime installs a specific version of a runtime.
func InstallRuntime(manager *Manager, runtime, version string) error {
	return InstallRuntimeWithAlias(manager, runtime, version, "")
}

// InstallRuntimeWithAlias installs a specific version of a runtime and stores the alias used.
func InstallRuntimeWithAlias(manager *Manager, runtime, version, alias string) error {
	installer, err := GetInstaller(runtime)
	if err != nil {
		return err
	}

	// Validate version
	if err := installer.ValidateVersion(version); err != nil {
		return fmt.Errorf("invalid version %s for %s: %w", version, runtime, err)
	}

	// Resolve aliases and partial versions BEFORE creating dest directory
	// This ensures we install to the correct version directory
	resolvedVersion := version
	detectedAlias := alias
	
	// If no alias was provided but version is an alias, detect it
	if alias == "" {
		if version == "latest" || version == "lts" {
			detectedAlias = version
		}
	}
	
	// Resolve the version (handles aliases and partial versions)
	// We need to call a method that resolves without installing
	// For now, we'll let Install handle resolution, but we need to track the resolved version
	// Actually, let's resolve it here for Node.js specifically
	if runtime == "node" {
		if version == "latest" {
			if nodeInstaller, ok := installer.(interface{ GetLatestVersion() (string, error) }); ok {
				latest, err := nodeInstaller.GetLatestVersion()
				if err == nil {
					resolvedVersion = latest
					if detectedAlias == "" {
						detectedAlias = "latest"
					}
				}
			}
		} else if version == "lts" {
			if nodeInstaller, ok := installer.(interface{ GetLTSVersion() (string, error) }); ok {
				lts, err := nodeInstaller.GetLTSVersion()
				if err == nil {
					resolvedVersion = lts
					if detectedAlias == "" {
						detectedAlias = "lts"
					}
				}
			}
		}
	} else {
		// For other runtimes, try to resolve "latest"
		if version == "latest" {
			if latestGetter, ok := installer.(interface{ GetLatestVersion() (string, error) }); ok {
				latest, err := latestGetter.GetLatestVersion()
				if err == nil {
					resolvedVersion = latest
					if detectedAlias == "" {
						detectedAlias = "latest"
					}
				}
			}
		}
	}

	// Use resolved version for destination
	dest := filepath.Join(manager.GetRuntimesPath(), runtime, resolvedVersion)
	if _, err := os.Stat(dest); err == nil {
		// Verify installation
		binaryPaths := installer.BinaryPaths(resolvedVersion, dest)
		if err := verifyInstallation(dest, binaryPaths); err == nil {
			fmt.Printf("%s@%s is already installed at %s\n", runtime, resolvedVersion, dest)
			// Still save alias if provided
			if detectedAlias != "" {
				saveVersionAlias(dest, detectedAlias)
			}
			return nil
		}
		// Installation is corrupted, remove and reinstall
		fmt.Printf("Corrupted installation detected, reinstalling...\n")
		if err := os.RemoveAll(dest); err != nil {
			logx.Info("failed to remove corrupted installation at %s: %v", dest, err)
			// Continue anyway, installation will overwrite
		}
	}

	if resolvedVersion != version {
		fmt.Printf("Installing %s@%s (resolved from %s)...\n", runtime, resolvedVersion, version)
	} else {
		fmt.Printf("Installing %s@%s...\n", runtime, version)
	}

	// Create destination directory
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Download and install (pass original version, Install will handle resolution)
	if err := installer.Install(version, dest); err != nil {
		// Clean up on failure
		if rmErr := os.RemoveAll(dest); rmErr != nil {
			logx.Info("failed to clean up failed installation at %s: %v", dest, rmErr)
		}
		return fmt.Errorf("installation failed: %w", err)
	}
	
	// Save alias metadata if detected (after successful installation)
	if detectedAlias != "" {
		if err := saveVersionAlias(dest, detectedAlias); err != nil {
			logx.Info("failed to save alias metadata: %v", err)
		}
	}

	// Post-install setup
	if err := installer.PostInstall(version, dest); err != nil {
		logx.Info("post-install warning: %v", err)
		// Don't fail on post-install errors, but log them
	}

	// Verify installation (use resolved version)
	binaryPaths := installer.BinaryPaths(resolvedVersion, dest)
	if err := verifyInstallation(dest, binaryPaths); err != nil {
		if rmErr := os.RemoveAll(dest); rmErr != nil {
			logx.Info("failed to clean up after verification failure at %s: %v", dest, rmErr)
		}
		return fmt.Errorf("verification failed: %v\n\nTo retry installation:\n  xpm env install %s@%s\n\nIf the issue persists, try:\n  xpm env ls-remote %s  # to see available versions\n  xpm env install %s@<different-version>", err, runtime, resolvedVersion, runtime, runtime)
	}

	fmt.Printf("✓ Installed %s@%s at %s\n", runtime, version, dest)

	// Automatically activate the installed version (local)
	// Use the resolved version, not the original (which might be an alias)
	if err := UseVersion(manager, runtime, resolvedVersion, false); err != nil {
		logx.Info("failed to auto-activate %s@%s: %v", runtime, resolvedVersion, err)
		// Don't fail installation if activation fails
	}

	// Update shims
	if err := CreateShims(manager); err != nil {
		logx.Info("failed to update shims: %v", err)
	}

	return nil
}

// verifyInstallation checks that all expected binaries exist.
func verifyInstallation(dest string, binaryPaths []string) error {
	for _, relPath := range binaryPaths {
		fullPath := filepath.Join(dest, relPath)
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("binary not found: %s", relPath)
		}
		if info.Mode()&0111 == 0 {
			// Make executable
			os.Chmod(fullPath, 0755)
		}
	}
	return nil
}

