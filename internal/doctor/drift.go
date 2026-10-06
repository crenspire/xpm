package doctor

import (
	"path/filepath"
	"strings"
)

// DriftInfo is the result of comparing a dependency file with its lockfile.
type DriftInfo struct {
	DepFile   string
	LockFile  string
	Ecosystem string
	Status    DriftStatus
	Detail    string // what differs, why it could not be compared, or the parse error
}

// DriftStatus categorizes drift status.
type DriftStatus int

const (
	// DriftStatusOK: contents were compared and agree.
	DriftStatusOK DriftStatus = iota
	// DriftStatusOutdated: contents were compared and the lockfile does not
	// match the dependency file.
	DriftStatusOutdated
	// DriftStatusUnknown: this lockfile format is not compared; nothing is
	// claimed either way.
	DriftStatusUnknown
	// DriftStatusInvalid: the dependency file or lockfile could not be parsed.
	DriftStatusInvalid
)

// nodeLockfiles lists Node lockfiles in the order drift checks prefer them.
var nodeLockfiles = []string{"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb"}

// pythonProjectLockfiles lists lockfiles that may accompany pyproject.toml.
var pythonProjectLockfiles = []string{"poetry.lock", "uv.lock", "pdm.lock"}

// CheckDrift compares each dependency file in dir with its lockfile by
// content (file modification times are never used). A dependency file whose
// lockfile is absent produces no entry: ScanProject reports missing
// lockfiles, so each one is counted once.
func CheckDrift(dir string) []DriftInfo {
	var results []DriftInfo
	exists := func(name string) bool { return fileExists(filepath.Join(dir, name)) }
	first := func(names []string) string {
		for _, n := range names {
			if exists(n) {
				return n
			}
		}
		return ""
	}

	if exists("package.json") {
		if lf := first(nodeLockfiles); lf != "" {
			results = append(results, checkNodeDrift(dir, lf))
		}
	}
	if exists("composer.json") && exists("composer.lock") {
		results = append(results, parseOnly("composer.json", "composer.lock", "php", parsesAsJSON(filepath.Join(dir, "composer.lock"))))
	}
	if exists("pyproject.toml") {
		if lf := first(pythonProjectLockfiles); lf != "" {
			results = append(results, parseOnly("pyproject.toml", lf, "python", parsesAsTOML(filepath.Join(dir, lf))))
		}
	}
	if exists("Pipfile") && exists("Pipfile.lock") {
		results = append(results, parseOnly("Pipfile", "Pipfile.lock", "python", parsesAsJSON(filepath.Join(dir, "Pipfile.lock"))))
	}
	if exists("Cargo.toml") && exists("Cargo.lock") {
		results = append(results, checkCargoDrift(dir))
	}
	if exists("go.mod") && exists("go.sum") {
		results = append(results, checkGoDrift(dir))
	}
	return results
}

// checkNodeDrift compares package.json's dependency names with the root
// entry of package-lock.json (v2/v3) or importers["."] of pnpm-lock.yaml.
// Other Node lockfiles are not compared.
func checkNodeDrift(dir, lockFile string) DriftInfo {
	d := DriftInfo{DepFile: "package.json", LockFile: lockFile, Ecosystem: "node"}
	if lockFile != "package-lock.json" && lockFile != "pnpm-lock.yaml" {
		d.Status = DriftStatusUnknown
		d.Detail = "contents not compared"
		return d
	}
	manifest, err := readPackageJSON(dir)
	if err != nil {
		return invalid(d, err)
	}

	var declared, locked nameSet
	if lockFile == "package-lock.json" {
		root, ok, err := npmLockRoot(dir)
		if err != nil {
			return invalid(d, err)
		}
		if !ok {
			d.Status = DriftStatusUnknown
			d.Detail = "lockfileVersion 1 has no root entry; contents not compared"
			return d
		}
		declared, locked = manifest.names(true), root.names(true)
	} else {
		locked, err = pnpmImporterNames(dir)
		if err != nil {
			return invalid(d, err)
		}
		declared = manifest.names(false)
	}
	return compareNames(d, declared, locked, true)
}

// checkCargoDrift requires every dependency named in Cargo.toml to be a
// package in Cargo.lock.
func checkCargoDrift(dir string) DriftInfo {
	d := DriftInfo{DepFile: "Cargo.toml", LockFile: "Cargo.lock", Ecosystem: "rust"}
	declared, err := cargoManifestNames(dir)
	if err != nil {
		return invalid(d, err)
	}
	locked, err := cargoLockNames(dir)
	if err != nil {
		return invalid(d, err)
	}
	return compareNames(d, declared, locked, false)
}

// checkGoDrift requires every go.mod requirement (path and version) to have
// a go.sum entry.
func checkGoDrift(dir string) DriftInfo {
	d := DriftInfo{DepFile: "go.mod", LockFile: "go.sum", Ecosystem: "go"}
	declared, err := goRequirements(dir)
	if err != nil {
		return invalid(d, err)
	}
	locked, err := goSumEntries(dir)
	if err != nil {
		return invalid(d, err)
	}
	return compareNames(d, declared, locked, false)
}

// compareNames sets d's status from the declared and locked name sets. With
// both, names locked but no longer declared also count as drift.
func compareNames(d DriftInfo, declared, locked nameSet, both bool) DriftInfo {
	var parts []string
	if missing := declared.minus(locked); len(missing) > 0 {
		parts = append(parts, "in "+d.DepFile+" but not "+d.LockFile+": "+strings.Join(missing, ", "))
	}
	if both {
		if extra := locked.minus(declared); len(extra) > 0 {
			parts = append(parts, "in "+d.LockFile+" but not "+d.DepFile+": "+strings.Join(extra, ", "))
		}
	}
	if len(parts) == 0 {
		d.Status = DriftStatusOK
		return d
	}
	d.Status = DriftStatusOutdated
	d.Detail = strings.Join(parts, "; ")
	return d
}

// parseOnly builds the result for lockfiles that are only checked for
// well-formedness.
func parseOnly(depFile, lockFile, ecosystem string, parseErr error) DriftInfo {
	d := DriftInfo{DepFile: depFile, LockFile: lockFile, Ecosystem: ecosystem}
	if parseErr != nil {
		return invalid(d, parseErr)
	}
	d.Status = DriftStatusUnknown
	d.Detail = "lockfile parses; contents not compared"
	return d
}

func invalid(d DriftInfo, err error) DriftInfo {
	d.Status = DriftStatusInvalid
	d.Detail = err.Error()
	return d
}

// PrintDriftReport prints the drift check results.
func PrintDriftReport(results []DriftInfo) {
	Section("Dependency Drift")

	if len(results) == 0 {
		Info("No dependency files to check")
		return
	}

	for _, info := range results {
		switch info.Status {
		case DriftStatusOK:
			Good(info.DepFile + " and " + info.LockFile + " agree")
		case DriftStatusOutdated:
			Bad(info.LockFile + " is out of date with " + info.DepFile + " (" + info.Detail + ")")
		case DriftStatusInvalid:
			Bad(info.DepFile + " / " + info.LockFile + " could not be read: " + info.Detail)
		default:
			Info(info.DepFile + " / " + info.LockFile + ": " + info.Detail)
		}
	}
}

// CountDriftIssues counts compared-and-OK, outdated, and unreadable pairs.
// Pairs whose contents are not compared are not counted.
func CountDriftIssues(results []DriftInfo) (ok, outdated, invalidCount int) {
	for _, info := range results {
		switch info.Status {
		case DriftStatusOK:
			ok++
		case DriftStatusOutdated:
			outdated++
		case DriftStatusInvalid:
			invalidCount++
		}
	}
	return
}

// GetDriftSuggestions returns suggestions for drift issues.
func GetDriftSuggestions(results []DriftInfo) []string {
	var suggestions []string
	seen := make(map[string]bool)

	for _, info := range results {
		if info.Status != DriftStatusOutdated && info.Status != DriftStatusInvalid {
			continue
		}
		suggestion := getSyncCommand(info)
		if suggestion != "" && !seen[suggestion] {
			suggestions = append(suggestions, suggestion)
			seen[suggestion] = true
		}
	}

	return suggestions
}

// getSyncCommand returns the command to sync a lockfile.
func getSyncCommand(info DriftInfo) string {
	switch info.LockFile {
	case "package-lock.json":
		return "Run `npm install` to sync package-lock.json"
	case "pnpm-lock.yaml":
		return "Run `pnpm install` to sync pnpm-lock.yaml"
	case "yarn.lock":
		return "Run `yarn install` to sync yarn.lock"
	case "bun.lock", "bun.lockb":
		return "Run `bun install` to sync " + info.LockFile
	case "composer.lock":
		return "Run `composer update --lock` to sync composer.lock"
	case "poetry.lock":
		return "Run `poetry lock` to sync poetry.lock"
	case "uv.lock":
		return "Run `uv lock` to sync uv.lock"
	case "pdm.lock":
		return "Run `pdm lock` to sync pdm.lock"
	case "Pipfile.lock":
		return "Run `pipenv lock` to sync Pipfile.lock"
	case "Cargo.lock":
		return "Run `cargo update --workspace` to sync Cargo.lock"
	case "go.sum":
		return "Run `go mod tidy` to sync go.sum"
	}
	return ""
}

// HasDriftIssues reports whether any pair is outdated or unreadable.
func HasDriftIssues(results []DriftInfo) bool {
	for _, info := range results {
		if info.Status == DriftStatusOutdated || info.Status == DriftStatusInvalid {
			return true
		}
	}
	return false
}
