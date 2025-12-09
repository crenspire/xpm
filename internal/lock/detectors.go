package lock

import (
	"os"
	"path/filepath"
)

// LockfileSpec defines a lockfile to detect.
type LockfileSpec struct {
	// File is the lockfile name.
	File string

	// Ecosystem is the ecosystem identifier (node, python, rust, etc.).
	Ecosystem string

	// Manager is the package manager name (npm, yarn, pip, etc.).
	Manager string
}

// SupportedLockfiles lists all lockfiles that can be detected.
var SupportedLockfiles = []LockfileSpec{
	// Node.js ecosystem
	{File: "package-lock.json", Ecosystem: "node", Manager: "npm"},
	{File: "yarn.lock", Ecosystem: "node", Manager: "yarn"},
	{File: "pnpm-lock.yaml", Ecosystem: "node", Manager: "pnpm"},
	{File: "bun.lockb", Ecosystem: "node", Manager: "bun"},

	// PHP ecosystem
	{File: "composer.lock", Ecosystem: "composer", Manager: "composer"},

	// Python ecosystem
	{File: "poetry.lock", Ecosystem: "python", Manager: "poetry"},
	{File: "requirements.lock", Ecosystem: "python", Manager: "pip"},
	{File: "Pipfile.lock", Ecosystem: "python", Manager: "pipenv"},

	// Rust ecosystem
	{File: "Cargo.lock", Ecosystem: "rust", Manager: "cargo"},

	// Go ecosystem
	{File: "go.sum", Ecosystem: "go", Manager: "go"},

	// Java ecosystem
	{File: "gradle.lockfile", Ecosystem: "gradle", Manager: "gradle"},
}

// DetectedLockfile holds information about a detected lockfile.
type DetectedLockfile struct {
	// Spec is the lockfile specification.
	Spec LockfileSpec

	// Path is the full path to the lockfile.
	Path string

	// RelPath is the path relative to the scan directory.
	RelPath string
}

// DetectAll scans a directory for all supported lockfiles.
// Returns a slice of detected lockfiles.
func DetectAll(dir string) ([]DetectedLockfile, error) {
	var detected []DetectedLockfile

	for _, spec := range SupportedLockfiles {
		path := filepath.Join(dir, spec.File)
		if fileExists(path) {
			detected = append(detected, DetectedLockfile{
				Spec:    spec,
				Path:    path,
				RelPath: spec.File,
			})
		}
	}

	return detected, nil
}

// DetectByEcosystem scans a directory for lockfiles of a specific ecosystem.
func DetectByEcosystem(dir string, ecosystem string) ([]DetectedLockfile, error) {
	var detected []DetectedLockfile

	for _, spec := range SupportedLockfiles {
		if spec.Ecosystem != ecosystem {
			continue
		}
		path := filepath.Join(dir, spec.File)
		if fileExists(path) {
			detected = append(detected, DetectedLockfile{
				Spec:    spec,
				Path:    path,
				RelPath: spec.File,
			})
		}
	}

	return detected, nil
}

// HasLockfiles checks if any supported lockfiles exist in the directory.
func HasLockfiles(dir string) bool {
	for _, spec := range SupportedLockfiles {
		path := filepath.Join(dir, spec.File)
		if fileExists(path) {
			return true
		}
	}
	return false
}

// GetEcosystemKey returns a unique key for a detected lockfile.
// For ecosystems with multiple possible managers (like Node), uses the ecosystem name.
// For single-manager ecosystems, uses the ecosystem name.
func GetEcosystemKey(d DetectedLockfile) string {
	return d.Spec.Ecosystem
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// ListSupportedEcosystems returns a list of all supported ecosystems.
func ListSupportedEcosystems() []string {
	seen := make(map[string]bool)
	var ecosystems []string

	for _, spec := range SupportedLockfiles {
		if !seen[spec.Ecosystem] {
			seen[spec.Ecosystem] = true
			ecosystems = append(ecosystems, spec.Ecosystem)
		}
	}

	return ecosystems
}

// ListSupportedFiles returns a list of all supported lockfile names.
func ListSupportedFiles() []string {
	files := make([]string, len(SupportedLockfiles))
	for i, spec := range SupportedLockfiles {
		files[i] = spec.File
	}
	return files
}

