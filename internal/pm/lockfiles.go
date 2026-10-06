package pm

import (
	"os"
	"path/filepath"
	"strings"
)

// Ecosystem represents a group of package managers that share a registry.
type Ecosystem string

// Ecosystem identifiers.
const (
	EcosystemNode   Ecosystem = "node"
	EcosystemPython Ecosystem = "python"
)

// LockFileMapping maps lock file names to their package managers.
var LockFileMapping = map[Ecosystem]map[string]ID{
	EcosystemNode: {
		"package-lock.json": Npm,
		"yarn.lock":         Yarn,
		"pnpm-lock.yaml":    Pnpm,
		"bun.lockb":         Bun,
	},
	EcosystemPython: {
		"poetry.lock":      Poetry,
		"Pipfile.lock":     Pipenv,
		"requirements.txt": Pip, // Not a true lock file, but indicates pip usage
	},
}

// EcosystemForManager returns the ecosystem that a package manager belongs to.
func EcosystemForManager(id ID) Ecosystem {
	switch id {
	case Npm, Yarn, Pnpm, Bun:
		return EcosystemNode
	case Pip, Poetry, Pipenv:
		return EcosystemPython
	default:
		return ""
	}
}

// ManagersInEcosystem returns all package managers in a given ecosystem.
func ManagersInEcosystem(eco Ecosystem) []ID {
	switch eco {
	case EcosystemNode:
		return []ID{Npm, Yarn, Pnpm, Bun}
	case EcosystemPython:
		return []ID{Pip, Poetry, Pipenv}
	default:
		return nil
	}
}

// DetectLockFiles scans a directory for lock files and returns detected package managers
// grouped by ecosystem. Only returns ecosystems where at least one lock file was found.
// Validates the directory path to prevent path traversal attacks.
//
// Edge cases:
//   - Path with "..": returns empty map (path traversal detected)
//   - Invalid path: uses cleaned path, returns empty map if still invalid
//   - Non-existent directory: returns empty map (no lock files found)
//   - Multiple lock files in same ecosystem: returns all detected managers
//
// Security: This function validates paths to prevent directory traversal attacks.
// It resolves paths to absolute form and validates they don't contain ".." sequences.
func DetectLockFiles(dir string) map[Ecosystem][]ID {
	result := make(map[Ecosystem][]ID)

	// Validate and clean the directory path
	cleanDir := filepath.Clean(dir)
	if strings.Contains(cleanDir, "..") {
		// Path traversal detected, return empty result
		return result
	}

	// Resolve to absolute path to prevent relative path issues
	absDir, err := filepath.Abs(cleanDir)
	if err != nil {
		// If we can't resolve, use cleaned path
		absDir = cleanDir
	}

	for eco, lockFiles := range LockFileMapping {
		var detected []ID
		for file, manager := range lockFiles {
			// Use filepath.Join which is safe, and validate the result
			lockPath := filepath.Join(absDir, file)
			// Ensure the resolved path is still within the intended directory
			if !strings.HasPrefix(lockPath, absDir) {
				continue
			}
			if _, err := os.Stat(lockPath); err == nil {
				detected = append(detected, manager)
			}
		}
		if len(detected) > 0 {
			result[eco] = detected
		}
	}

	return result
}

// DetectLockFilesForEcosystem returns the package managers with lock files in the given
// ecosystem. Returns nil if no lock files are found for that ecosystem.
// Validates the directory path to prevent path traversal attacks.
func DetectLockFilesForEcosystem(dir string, eco Ecosystem) []ID {
	lockFiles, ok := LockFileMapping[eco]
	if !ok {
		return nil
	}

	// Validate and clean the directory path
	cleanDir := filepath.Clean(dir)
	if strings.Contains(cleanDir, "..") {
		return nil
	}

	// Resolve to absolute path
	absDir, err := filepath.Abs(cleanDir)
	if err != nil {
		absDir = cleanDir
	}

	var detected []ID
	for file, manager := range lockFiles {
		lockPath := filepath.Join(absDir, file)
		// Ensure the resolved path is still within the intended directory
		if !strings.HasPrefix(lockPath, absDir) {
			continue
		}
		if _, err := os.Stat(lockPath); err == nil {
			detected = append(detected, manager)
		}
	}
	return detected
}

// HasLockFile checks if any lock file exists for the given ecosystem in the directory.
func HasLockFile(dir string, eco Ecosystem) bool {
	return len(DetectLockFilesForEcosystem(dir, eco)) > 0
}

// GetLockFileName returns the lock file name for a given package manager.
func GetLockFileName(id ID) string {
	for _, lockFiles := range LockFileMapping {
		for file, manager := range lockFiles {
			if manager == id {
				return file
			}
		}
	}
	return ""
}
