package env

import (
	"fmt"
	"sync"
)

// RuntimeInstaller is the interface that all runtime installers must implement.
type RuntimeInstaller interface {
	// Name returns the runtime name (e.g., "node", "python").
	Name() string

	// ListRemote returns a list of available versions from the remote source.
	ListRemote() ([]string, error)

	// Install downloads and installs a specific version to the destination directory.
	Install(version string, dest string) error

	// PostInstall performs any post-installation setup (e.g., creating symlinks).
	PostInstall(version, dest string) error

	// BinaryPaths returns the paths to binaries relative to the installation directory.
	// Example: ["bin/node", "bin/npm"] for Node.js
	BinaryPaths(version, dest string) []string

	// ValidateVersion checks if a version string is valid for this runtime.
	ValidateVersion(version string) error
}

var (
	installersMu sync.RWMutex
	installers   = make(map[string]RuntimeInstaller)
)

// RegisterInstaller registers a runtime installer.
func RegisterInstaller(name string, installer RuntimeInstaller) {
	installersMu.Lock()
	defer installersMu.Unlock()
	installers[name] = installer
}

// GetInstaller returns the installer for the given runtime name.
func GetInstaller(name string) (RuntimeInstaller, error) {
	installersMu.RLock()
	defer installersMu.RUnlock()

	installer, ok := installers[name]
	if !ok {
		// Provide helpful suggestions for common mistakes
		suggestion := suggestRuntime(name)
		if suggestion != "" {
			return nil, fmt.Errorf("no installer found for runtime: %s\n\nDid you mean: %s?\n\nNote: %s is a package manager, not a runtime. Install the runtime instead:\n  xpm env install %s@<version>", name, suggestion, name, suggestion)
		}
		
		// List available runtimes
		var available []string
		for n := range installers {
			available = append(available, n)
		}
		return nil, fmt.Errorf("no installer found for runtime: %s\n\nAvailable runtimes: %v", name, available)
	}

	return installer, nil
}

// suggestRuntime suggests a runtime name for common package manager names.
func suggestRuntime(name string) string {
	aliases := map[string]string{
		"npm":   "node",
		"yarn":  "node",
		"pnpm":  "node",
		"bun":   "bun", // bun is both a runtime and package manager
		"pip":   "python",
		"pip3":  "python",
		"poetry": "python",
		"php":   "php",
		"composer": "php",
		"go":    "go",
		"golang": "go",
		"java":  "java",
		"jdk":   "java",
		"rust":  "rust",
		"cargo": "rust",
		"deno":  "deno",
	}
	
	if suggested, ok := aliases[name]; ok {
		return suggested
	}
	return ""
}

// ListRuntimes returns all registered runtime names.
func ListRuntimes() []string {
	installersMu.RLock()
	defer installersMu.RUnlock()

	var names []string
	for name := range installers {
		names = append(names, name)
	}

	return names
}

