package lock

import (
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

// SupportedLockfiles lists all lockfiles that can be detected, in the order
// they are reported.
var SupportedLockfiles = []LockfileSpec{
	// Node.js ecosystem
	{File: "package-lock.json", Ecosystem: "node", Manager: "npm"},
	{File: "yarn.lock", Ecosystem: "node", Manager: "yarn"},
	{File: "pnpm-lock.yaml", Ecosystem: "node", Manager: "pnpm"},
	{File: "bun.lock", Ecosystem: "node", Manager: "bun"},
	{File: "bun.lockb", Ecosystem: "node", Manager: "bun"},

	// PHP ecosystem
	{File: "composer.lock", Ecosystem: "composer", Manager: "composer"},

	// Python ecosystem
	{File: "poetry.lock", Ecosystem: "python", Manager: "poetry"},
	{File: "uv.lock", Ecosystem: "python", Manager: "uv"},
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

	// RelPath is the slash-separated path relative to the scanned root; it is
	// the file's key in xpm-lock.yaml.
	RelPath string
}

// DetectAll returns the supported lockfiles present in dir (the project root
// only; subdirectories are not scanned), in SupportedLockfiles order.
// A lockfile that is a symlink resolving outside dir is not detected.
func DetectAll(dir string) []DetectedLockfile {
	var detected []DetectedLockfile
	for _, spec := range SupportedLockfiles {
		full, err := containedPath(dir, spec.File)
		if err != nil || !fileExists(full) {
			continue
		}
		detected = append(detected, DetectedLockfile{
			Spec:    spec,
			Path:    filepath.Join(dir, spec.File),
			RelPath: spec.File,
		})
	}
	return detected
}

// ListSupportedFiles returns the names of all supported lockfiles.
func ListSupportedFiles() []string {
	files := make([]string, len(SupportedLockfiles))
	for i, spec := range SupportedLockfiles {
		files[i] = spec.File
	}
	return files
}
