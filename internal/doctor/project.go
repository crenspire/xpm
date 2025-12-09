package doctor

import (
	"os"
	"path/filepath"
)

// ProjectFileInfo holds information about a detected project file.
type ProjectFileInfo struct {
	Name     string
	Path     string
	Exists   bool
	Type     FileType
	Ecosystem string
}

// FileType categorizes project files.
type FileType int

const (
	FileTypeDependency FileType = iota
	FileTypeLock
	FileTypeBuild
)

// ProjectFileSpec defines a project file to detect.
type ProjectFileSpec struct {
	Name       string
	Type       FileType
	Ecosystem  string
	LockFile   string // Associated lock file (for dependency files)
	DepFile    string // Associated dependency file (for lock files)
}

// projectFileSpecs defines all project files to check.
var projectFileSpecs = []ProjectFileSpec{
	// Node.js
	{Name: "package.json", Type: FileTypeDependency, Ecosystem: "node", LockFile: "package-lock.json"},
	{Name: "package-lock.json", Type: FileTypeLock, Ecosystem: "node", DepFile: "package.json"},
	{Name: "yarn.lock", Type: FileTypeLock, Ecosystem: "node", DepFile: "package.json"},
	{Name: "pnpm-lock.yaml", Type: FileTypeLock, Ecosystem: "node", DepFile: "package.json"},
	{Name: "bun.lockb", Type: FileTypeLock, Ecosystem: "node", DepFile: "package.json"},

	// PHP
	{Name: "composer.json", Type: FileTypeDependency, Ecosystem: "php", LockFile: "composer.lock"},
	{Name: "composer.lock", Type: FileTypeLock, Ecosystem: "php", DepFile: "composer.json"},

	// Python
	{Name: "pyproject.toml", Type: FileTypeDependency, Ecosystem: "python", LockFile: "poetry.lock"},
	{Name: "requirements.txt", Type: FileTypeDependency, Ecosystem: "python", LockFile: "requirements.lock"},
	{Name: "poetry.lock", Type: FileTypeLock, Ecosystem: "python", DepFile: "pyproject.toml"},
	{Name: "Pipfile", Type: FileTypeDependency, Ecosystem: "python", LockFile: "Pipfile.lock"},
	{Name: "Pipfile.lock", Type: FileTypeLock, Ecosystem: "python", DepFile: "Pipfile"},

	// Rust
	{Name: "Cargo.toml", Type: FileTypeDependency, Ecosystem: "rust", LockFile: "Cargo.lock"},
	{Name: "Cargo.lock", Type: FileTypeLock, Ecosystem: "rust", DepFile: "Cargo.toml"},

	// Go
	{Name: "go.mod", Type: FileTypeDependency, Ecosystem: "go", LockFile: "go.sum"},
	{Name: "go.sum", Type: FileTypeLock, Ecosystem: "go", DepFile: "go.mod"},

	// Java
	{Name: "pom.xml", Type: FileTypeBuild, Ecosystem: "maven"},
	{Name: "build.gradle", Type: FileTypeBuild, Ecosystem: "gradle"},
	{Name: "build.gradle.kts", Type: FileTypeBuild, Ecosystem: "gradle"},
	{Name: "gradle.lockfile", Type: FileTypeLock, Ecosystem: "gradle"},
}

// ProjectScanResult holds the results of scanning a project directory.
type ProjectScanResult struct {
	Files           []ProjectFileInfo
	MissingLockFiles []string
	Ecosystems       map[string]bool
}

// ScanProject scans a directory for project files.
func ScanProject(dir string) ProjectScanResult {
	result := ProjectScanResult{
		Files:      make([]ProjectFileInfo, 0),
		Ecosystems: make(map[string]bool),
	}

	// Check each project file
	depFiles := make(map[string]bool)
	lockFiles := make(map[string]bool)

	for _, spec := range projectFileSpecs {
		path := filepath.Join(dir, spec.Name)
		exists := fileExists(path)

		info := ProjectFileInfo{
			Name:      spec.Name,
			Path:      path,
			Exists:    exists,
			Type:      spec.Type,
			Ecosystem: spec.Ecosystem,
		}

		if exists {
			result.Ecosystems[spec.Ecosystem] = true

			if spec.Type == FileTypeDependency {
				depFiles[spec.Name] = true
			} else if spec.Type == FileTypeLock {
				lockFiles[spec.Name] = true
			}
		}

		result.Files = append(result.Files, info)
	}

	// Check for missing lock files
	for _, spec := range projectFileSpecs {
		if spec.Type == FileTypeDependency && spec.LockFile != "" {
			if depFiles[spec.Name] && !lockFiles[spec.LockFile] {
				// Special case for Node.js: any lock file is acceptable
				if spec.Ecosystem == "node" {
					hasNodeLock := lockFiles["package-lock.json"] ||
						lockFiles["yarn.lock"] ||
						lockFiles["pnpm-lock.yaml"] ||
						lockFiles["bun.lockb"]
					if !hasNodeLock {
						result.MissingLockFiles = append(result.MissingLockFiles, spec.LockFile)
					}
				} else {
					result.MissingLockFiles = append(result.MissingLockFiles, spec.LockFile)
				}
			}
		}
	}

	return result
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// PrintProjectReport prints the project file scan results.
func PrintProjectReport(result ProjectScanResult) {
	Section("Project Files")

	hasAnyFiles := false

	// Print dependency files
	for _, info := range result.Files {
		if info.Exists && info.Type == FileTypeDependency {
			hasAnyFiles = true
			StatusLine(true, info.Name, "")
		}
	}

	// Print lock files
	for _, info := range result.Files {
		if info.Exists && info.Type == FileTypeLock {
			hasAnyFiles = true
			StatusLine(true, info.Name, "")
		}
	}

	// Print build files
	for _, info := range result.Files {
		if info.Exists && info.Type == FileTypeBuild {
			hasAnyFiles = true
			StatusLine(true, info.Name, "")
		}
	}

	// Print missing lock files
	for _, name := range result.MissingLockFiles {
		Bad(name + " missing")
	}

	if !hasAnyFiles {
		Info("No project files detected")
	}
}

// GetDetectedEcosystems returns a list of detected ecosystems.
func GetDetectedEcosystems(result ProjectScanResult) []string {
	ecosystems := make([]string, 0, len(result.Ecosystems))
	for eco := range result.Ecosystems {
		ecosystems = append(ecosystems, eco)
	}
	return ecosystems
}

// HasFile checks if a specific file exists in the scan result.
func HasFile(result ProjectScanResult, name string) bool {
	for _, info := range result.Files {
		if info.Name == name {
			return info.Exists
		}
	}
	return false
}

// GetFilesForEcosystem returns all files for a specific ecosystem.
func GetFilesForEcosystem(result ProjectScanResult, ecosystem string) []ProjectFileInfo {
	var files []ProjectFileInfo
	for _, info := range result.Files {
		if info.Ecosystem == ecosystem && info.Exists {
			files = append(files, info)
		}
	}
	return files
}

