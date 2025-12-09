package doctor

import (
	"os"
	"path/filepath"
	"time"
)

// DriftInfo represents dependency drift between a dependency file and its lockfile.
type DriftInfo struct {
	DepFile       string
	LockFile      string
	DepModTime    time.Time
	LockModTime   time.Time
	Status        DriftStatus
	Ecosystem     string
}

// DriftStatus categorizes drift status.
type DriftStatus int

const (
	// DriftStatusOK means lockfile is up-to-date with dependency file.
	DriftStatusOK DriftStatus = iota
	// DriftStatusOutdated means dependency file is newer than lockfile.
	DriftStatusOutdated
	// DriftStatusMissing means lockfile is missing.
	DriftStatusMissing
	// DriftStatusNoDepFile means dependency file doesn't exist.
	DriftStatusNoDepFile
)

// DriftPair defines a dependency file and its expected lockfile.
type DriftPair struct {
	DepFile   string
	LockFile  string
	Ecosystem string
}

// driftPairs defines all dependency/lockfile pairs to check.
var driftPairs = []DriftPair{
	// Node.js - check against primary lock files
	{DepFile: "package.json", LockFile: "package-lock.json", Ecosystem: "node"},
	{DepFile: "package.json", LockFile: "yarn.lock", Ecosystem: "node"},
	{DepFile: "package.json", LockFile: "pnpm-lock.yaml", Ecosystem: "node"},
	{DepFile: "package.json", LockFile: "bun.lockb", Ecosystem: "node"},

	// PHP
	{DepFile: "composer.json", LockFile: "composer.lock", Ecosystem: "php"},

	// Python
	{DepFile: "pyproject.toml", LockFile: "poetry.lock", Ecosystem: "python"},
	{DepFile: "requirements.txt", LockFile: "requirements.lock", Ecosystem: "python"},
	{DepFile: "Pipfile", LockFile: "Pipfile.lock", Ecosystem: "python"},

	// Rust
	{DepFile: "Cargo.toml", LockFile: "Cargo.lock", Ecosystem: "rust"},

	// Go
	{DepFile: "go.mod", LockFile: "go.sum", Ecosystem: "go"},
}

// CheckDrift checks for dependency drift in a directory.
func CheckDrift(dir string) []DriftInfo {
	var results []DriftInfo
	checkedPairs := make(map[string]bool)

	for _, pair := range driftPairs {
		// Avoid duplicate checks for the same dependency file
		if checkedPairs[pair.DepFile] {
			continue
		}

		depPath := filepath.Join(dir, pair.DepFile)
		lockPath := filepath.Join(dir, pair.LockFile)

		// Skip if dependency file doesn't exist
		if !fileExists(depPath) {
			continue
		}

		// Check if lockfile exists
		if !fileExists(lockPath) {
			// For Node.js, check if any lock file exists
			if pair.Ecosystem == "node" {
				if hasAnyNodeLockFile(dir) {
					// Skip this pair, another lock file exists
					continue
				}
			}

			results = append(results, DriftInfo{
				DepFile:   pair.DepFile,
				LockFile:  pair.LockFile,
				Status:    DriftStatusMissing,
				Ecosystem: pair.Ecosystem,
			})
			checkedPairs[pair.DepFile] = true
			continue
		}

		// Both files exist, compare timestamps
		depModTime := getModTime(depPath)
		lockModTime := getModTime(lockPath)

		status := DriftStatusOK
		if depModTime.After(lockModTime) {
			status = DriftStatusOutdated
		}

		results = append(results, DriftInfo{
			DepFile:     pair.DepFile,
			LockFile:    pair.LockFile,
			DepModTime:  depModTime,
			LockModTime: lockModTime,
			Status:      status,
			Ecosystem:   pair.Ecosystem,
		})
		checkedPairs[pair.DepFile] = true
	}

	return results
}

// hasAnyNodeLockFile checks if any Node.js lock file exists.
func hasAnyNodeLockFile(dir string) bool {
	lockFiles := []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lockb"}
	for _, lf := range lockFiles {
		if fileExists(filepath.Join(dir, lf)) {
			return true
		}
	}
	return false
}

// getModTime returns the modification time of a file.
func getModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// PrintDriftReport prints the drift check results.
func PrintDriftReport(results []DriftInfo) {
	Section("Dependency Drift")

	if len(results) == 0 {
		Info("No dependency files to check")
		return
	}

	hasIssues := false
	for _, info := range results {
		switch info.Status {
		case DriftStatusOK:
			Good(info.DepFile + " and " + info.LockFile + " aligned")
		case DriftStatusOutdated:
			hasIssues = true
			Bad(info.DepFile + " is newer than " + info.LockFile)
		case DriftStatusMissing:
			hasIssues = true
			Bad(info.LockFile + " is missing")
		}
	}

	if !hasIssues {
		// Already printed OK statuses
	}
}

// CountDriftIssues counts drift issues.
func CountDriftIssues(results []DriftInfo) (ok, outdated, missing int) {
	for _, info := range results {
		switch info.Status {
		case DriftStatusOK:
			ok++
		case DriftStatusOutdated:
			outdated++
		case DriftStatusMissing:
			missing++
		}
	}
	return
}

// GetDriftSuggestions returns suggestions for drift issues.
func GetDriftSuggestions(results []DriftInfo) []string {
	var suggestions []string
	seen := make(map[string]bool)

	for _, info := range results {
		if info.Status == DriftStatusOutdated {
			suggestion := getSyncCommand(info)
			if suggestion != "" && !seen[suggestion] {
				suggestions = append(suggestions, suggestion)
				seen[suggestion] = true
			}
		} else if info.Status == DriftStatusMissing {
			suggestion := getGenerateLockCommand(info)
			if suggestion != "" && !seen[suggestion] {
				suggestions = append(suggestions, suggestion)
				seen[suggestion] = true
			}
		}
	}

	return suggestions
}

// getSyncCommand returns the command to sync a lockfile.
func getSyncCommand(info DriftInfo) string {
	switch info.Ecosystem {
	case "node":
		switch info.LockFile {
		case "package-lock.json":
			return "Run `npm install` to sync lockfile"
		case "yarn.lock":
			return "Run `yarn install` to sync lockfile"
		case "pnpm-lock.yaml":
			return "Run `pnpm install` to sync lockfile"
		case "bun.lockb":
			return "Run `bun install` to sync lockfile"
		}
	case "php":
		return "Run `composer update` to sync lockfile"
	case "python":
		if info.LockFile == "poetry.lock" {
			return "Run `poetry lock` to sync lockfile"
		} else if info.LockFile == "Pipfile.lock" {
			return "Run `pipenv lock` to sync lockfile"
		}
		return "Run `pip freeze > requirements.lock` to sync lockfile"
	case "rust":
		return "Run `cargo update` to sync lockfile"
	case "go":
		return "Run `go mod tidy` to sync go.sum"
	}
	return ""
}

// getGenerateLockCommand returns the command to generate a missing lockfile.
func getGenerateLockCommand(info DriftInfo) string {
	switch info.Ecosystem {
	case "node":
		return "Run `npm install` to generate lockfile"
	case "php":
		return "Run `composer install` to generate composer.lock"
	case "python":
		if info.LockFile == "poetry.lock" {
			return "Run `poetry lock` to generate poetry.lock"
		} else if info.LockFile == "Pipfile.lock" {
			return "Run `pipenv lock` to generate Pipfile.lock"
		}
	case "rust":
		return "Run `cargo build` to generate Cargo.lock"
	case "go":
		return "Run `go mod tidy` to generate go.sum"
	}
	return ""
}

// HasDriftIssues checks if there are any drift issues.
func HasDriftIssues(results []DriftInfo) bool {
	for _, info := range results {
		if info.Status != DriftStatusOK {
			return true
		}
	}
	return false
}

