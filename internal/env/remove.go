package env

import (
	"fmt"
	"os"
)

// RemoveVersion removes an installed version.
func RemoveVersion(manager *Manager, runtime, version string) error {
	versionPath, err := manager.versionDir(runtime, version)
	if err != nil {
		return err
	}

	// Check if version exists
	if _, err := os.Stat(versionPath); err != nil {
		return fmt.Errorf("version %s@%s is not installed", runtime, version)
	}

	// Check if it's currently active
	activeVersion, _ := manager.GetActiveVersion(runtime)
	if version == activeVersion {
		return fmt.Errorf("cannot remove active version %s@%s. Switch to another version first", runtime, version)
	}

	// Remove directory
	if err := os.RemoveAll(versionPath); err != nil {
		return fmt.Errorf("failed to remove version: %w", err)
	}

	fmt.Printf("Removed %s@%s\n", runtime, version)

	return nil
}
