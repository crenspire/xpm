package env

import (
	"fmt"
	"os"
	"path/filepath"
)

// DetectLocalEnv reads .xpm-env file from the given directory or parent directories.
func DetectLocalEnv(dir string) (map[string]string, error) {
	envFile, err := FindXpmEnv(dir)
	if err != nil {
		return nil, err
	}

	return readEnvFile(envFile)
}

// FindXpmEnv walks up the directory tree to find .xpm-env file.
func FindXpmEnv(dir string) (string, error) {
	current := dir
	for {
		envFile := filepath.Join(current, ".xpm-env")
		if _, err := os.Stat(envFile); err == nil {
			return envFile, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", fmt.Errorf(".xpm-env not found")
}

// ActivateVersions activates the specified versions.
func ActivateVersions(manager *Manager, versions map[string]string) error {
	for runtime, version := range versions {
		if ValidateRuntimeName(runtime) != nil || ValidateVersionSpec(version) != nil {
			continue
		}
		// Verify version is installed
		versionPath := filepath.Join(manager.GetRuntimesPath(), runtime, version)
		if _, err := os.Stat(versionPath); err != nil {
			// Version not installed, skip
			continue
		}

		// Set as active (local)
		if err := manager.SetActiveVersion(runtime, version, false); err != nil {
			return fmt.Errorf("failed to activate %s@%s: %w", runtime, version, err)
		}
	}

	return nil
}

// ActivateFromLocalEnv detects and activates versions from .xpm-env.
func ActivateFromLocalEnv(manager *Manager) error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil // Can't get cwd, skip activation
	}

	versions, err := DetectLocalEnv(cwd)
	if err != nil {
		return nil // No .xpm-env found, that's okay
	}

	if len(versions) > 0 {
		return ActivateVersions(manager, versions)
	}

	return nil
}
