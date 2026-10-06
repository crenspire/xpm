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
	EcosystemJava   Ecosystem = "java"
)

// ProjectFile ties a file in a project directory to the package manager it
// implies.
type ProjectFile struct {
	Name      string
	Ecosystem Ecosystem
	Manager   ID
}

// lockFiles are checked in this order, which is also the order results are
// returned in: detection is deterministic.
var lockFiles = []ProjectFile{
	{"package-lock.json", EcosystemNode, Npm},
	{"yarn.lock", EcosystemNode, Yarn},
	{"pnpm-lock.yaml", EcosystemNode, Pnpm},
	{"bun.lock", EcosystemNode, Bun},  // bun >= 1.2 (text)
	{"bun.lockb", EcosystemNode, Bun}, // bun < 1.2 (binary)
	{"poetry.lock", EcosystemPython, Poetry},
	{"Pipfile.lock", EcosystemPython, Pipenv},
	{"Pipfile", EcosystemPython, Pipenv},
	{"requirements.txt", EcosystemPython, Pip}, // not a lock file, but implies pip
}

// buildFiles name a Java project's build tool. They lock nothing, but they
// narrow Maven-vs-Gradle the way lock files narrow npm-vs-yarn.
var buildFiles = []ProjectFile{
	{"pom.xml", EcosystemJava, Maven},
	{"build.gradle", EcosystemJava, Gradle},
	{"build.gradle.kts", EcosystemJava, Gradle},
}

// EcosystemForManager returns the ecosystem that a package manager belongs to.
func EcosystemForManager(id ID) Ecosystem {
	switch id {
	case Npm, Yarn, Pnpm, Bun:
		return EcosystemNode
	case Pip, Poetry, Pipenv:
		return EcosystemPython
	case Maven, Gradle:
		return EcosystemJava
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
	case EcosystemJava:
		return []ID{Maven, Gradle}
	default:
		return nil
	}
}

// safeDir cleans dir and makes it absolute. It reports false for paths
// containing "..", which are refused.
func safeDir(dir string) (string, bool) {
	clean := filepath.Clean(dir)
	if strings.Contains(clean, "..") {
		return "", false
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return clean, true
	}
	return abs, true
}

// present returns the entries of files that exist in dir, in table order,
// keeping only the first file per manager.
func present(dir string, files []ProjectFile) []ProjectFile {
	abs, ok := safeDir(dir)
	if !ok {
		return nil
	}
	var out []ProjectFile
	seen := map[ID]bool{}
	for _, f := range files {
		if seen[f.Manager] {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, f.Name)); err == nil {
			out = append(out, f)
			seen[f.Manager] = true
		}
	}
	return out
}

// ProjectManagers returns, per ecosystem, the managers that dir's lock files
// and Java build files point at, in table order, with the file that implied
// each. Ecosystems with no such files are absent.
func ProjectManagers(dir string) map[Ecosystem][]ProjectFile {
	out := map[Ecosystem][]ProjectFile{}
	for _, f := range append(present(dir, lockFiles), present(dir, buildFiles)...) {
		out[f.Ecosystem] = append(out[f.Ecosystem], f)
	}
	return out
}

// DetectLockFiles returns the managers implied by lock files in dir, grouped
// by ecosystem (node and python only), in a fixed order. Paths containing
// ".." return an empty map.
func DetectLockFiles(dir string) map[Ecosystem][]ID {
	result := make(map[Ecosystem][]ID)
	for _, f := range present(dir, lockFiles) {
		result[f.Ecosystem] = append(result[f.Ecosystem], f.Manager)
	}
	return result
}

// DetectLockFilesForEcosystem returns the managers with lock files in the
// given ecosystem, or nil.
func DetectLockFilesForEcosystem(dir string, eco Ecosystem) []ID {
	return DetectLockFiles(dir)[eco]
}

// HasLockFile checks if any lock file exists for the given ecosystem in the directory.
func HasLockFile(dir string, eco Ecosystem) bool {
	return len(DetectLockFilesForEcosystem(dir, eco)) > 0
}
