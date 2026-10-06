package doctor

import (
	"os"
	"path/filepath"
)

// ProjectFileInfo holds information about a detected project file.
type ProjectFileInfo struct {
	Name      string
	Path      string
	Exists    bool
	Type      FileType
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
	Name      string
	Type      FileType
	Ecosystem string
}

// projectFileSpecs defines all project files to check.
var projectFileSpecs = []ProjectFileSpec{
	// Node.js
	{Name: "package.json", Type: FileTypeDependency, Ecosystem: "node"},
	{Name: "package-lock.json", Type: FileTypeLock, Ecosystem: "node"},
	{Name: "yarn.lock", Type: FileTypeLock, Ecosystem: "node"},
	{Name: "pnpm-lock.yaml", Type: FileTypeLock, Ecosystem: "node"},
	{Name: "bun.lock", Type: FileTypeLock, Ecosystem: "node"},
	{Name: "bun.lockb", Type: FileTypeLock, Ecosystem: "node"},

	// PHP
	{Name: "composer.json", Type: FileTypeDependency, Ecosystem: "php"},
	{Name: "composer.lock", Type: FileTypeLock, Ecosystem: "php"},

	// Python
	{Name: "pyproject.toml", Type: FileTypeDependency, Ecosystem: "python"},
	{Name: "requirements.txt", Type: FileTypeDependency, Ecosystem: "python"},
	{Name: "poetry.lock", Type: FileTypeLock, Ecosystem: "python"},
	{Name: "uv.lock", Type: FileTypeLock, Ecosystem: "python"},
	{Name: "pdm.lock", Type: FileTypeLock, Ecosystem: "python"},
	{Name: "Pipfile", Type: FileTypeDependency, Ecosystem: "python"},
	{Name: "Pipfile.lock", Type: FileTypeLock, Ecosystem: "python"},

	// Rust
	{Name: "Cargo.toml", Type: FileTypeDependency, Ecosystem: "rust"},
	{Name: "Cargo.lock", Type: FileTypeLock, Ecosystem: "rust"},

	// Go
	{Name: "go.mod", Type: FileTypeDependency, Ecosystem: "go"},
	{Name: "go.sum", Type: FileTypeLock, Ecosystem: "go"},

	// Java
	{Name: "pom.xml", Type: FileTypeBuild, Ecosystem: "maven"},
	{Name: "build.gradle", Type: FileTypeBuild, Ecosystem: "gradle"},
	{Name: "build.gradle.kts", Type: FileTypeBuild, Ecosystem: "gradle"},
	{Name: "gradle.lockfile", Type: FileTypeLock, Ecosystem: "gradle"},
}

// ProjectScanResult holds the results of scanning a project directory.
type ProjectScanResult struct {
	Files            []ProjectFileInfo
	MissingLockFiles []string
	Ecosystems       map[string]bool
}

// ScanProject scans dir for project files and lists the lockfiles that a
// present dependency file needs but that are absent.
func ScanProject(dir string) ProjectScanResult {
	result := ProjectScanResult{
		Files:      make([]ProjectFileInfo, 0, len(projectFileSpecs)),
		Ecosystems: make(map[string]bool),
	}
	present := make(map[string]bool)

	for _, spec := range projectFileSpecs {
		path := filepath.Join(dir, spec.Name)
		exists := fileExists(path)
		if exists {
			present[spec.Name] = true
			result.Ecosystems[spec.Ecosystem] = true
		}
		result.Files = append(result.Files, ProjectFileInfo{
			Name:      spec.Name,
			Path:      path,
			Exists:    exists,
			Type:      spec.Type,
			Ecosystem: spec.Ecosystem,
		})
	}

	for _, spec := range projectFileSpecs {
		if spec.Type != FileTypeDependency || !present[spec.Name] {
			continue
		}
		if lf := missingLockfile(dir, spec.Name, present); lf != "" {
			result.MissingLockFiles = append(result.MissingLockFiles, lf)
		}
	}

	return result
}

// missingLockfile returns the lockfile that depFile (present in dir) needs
// but that is absent, or "" when it needs none or has one:
//   - package.json: any Node lockfile will do (else package-lock.json);
//   - pyproject.toml: poetry.lock only for a [tool.poetry] project; PEP 621
//     projects may use uv.lock, pdm.lock, or no lockfile at all;
//   - requirements.txt: never (pip has no lockfile);
//   - go.mod: go.sum only when go.mod has requirements whose sums it must hold.
func missingLockfile(dir, depFile string, present map[string]bool) string {
	switch depFile {
	case "package.json":
		for _, lf := range nodeLockfiles {
			if present[lf] {
				return ""
			}
		}
		return "package-lock.json"
	case "composer.json":
		if !present["composer.lock"] {
			return "composer.lock"
		}
	case "pyproject.toml":
		if !present["poetry.lock"] && isPoetryProject(dir) {
			return "poetry.lock"
		}
	case "Pipfile":
		if !present["Pipfile.lock"] {
			return "Pipfile.lock"
		}
	case "Cargo.toml":
		if !present["Cargo.lock"] {
			return "Cargo.lock"
		}
	case "go.mod":
		if !present["go.sum"] {
			if reqs, err := goRequirements(dir); err == nil && len(reqs) > 0 {
				return "go.sum"
			}
		}
	}
	return ""
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
