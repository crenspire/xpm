package lock

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// ParseLockfile extracts metadata from a detected lockfile.
func ParseLockfile(d DetectedLockfile) (*LockInfo, error) {
	hash, err := computeFileHash(d.Path)
	if err != nil {
		return nil, err
	}

	modified, err := getFileModTime(d.Path)
	if err != nil {
		return nil, err
	}

	pkgCount := countPackages(d)

	return &LockInfo{
		Ecosystem:  d.Spec.Ecosystem,
		Manager:    d.Spec.Manager,
		File:       d.RelPath,
		Hash:       hash,
		Modified:   modified,
		PackageCnt: pkgCount,
	}, nil
}

// computeFileHash computes the SHA256 hash of a file.
func computeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// getFileModTime returns the modification time of a file.
func getFileModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime().UTC(), nil
}

// countPackages counts packages in a lockfile based on its type.
func countPackages(d DetectedLockfile) int {
	switch d.Spec.File {
	case "package-lock.json":
		return countPackageLockJSON(d.Path)
	case "yarn.lock":
		return countYarnLock(d.Path)
	case "pnpm-lock.yaml":
		return countPnpmLock(d.Path)
	case "bun.lockb":
		return countBunLock(d.Path)
	case "composer.lock":
		return countComposerLock(d.Path)
	case "poetry.lock":
		return countPoetryLock(d.Path)
	case "requirements.lock":
		return countRequirementsLock(d.Path)
	case "Pipfile.lock":
		return countPipfileLock(d.Path)
	case "Cargo.lock":
		return countCargoLock(d.Path)
	case "go.sum":
		return countGoSum(d.Path)
	case "gradle.lockfile":
		return countGradleLock(d.Path)
	default:
		return 0
	}
}

// countPackageLockJSON counts packages in package-lock.json.
func countPackageLockJSON(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var lock struct {
		// npm v2+ format with "packages" field
		Packages map[string]interface{} `json:"packages"`
		// npm v1 format with "dependencies" field
		Dependencies map[string]interface{} `json:"dependencies"`
	}

	if err := json.Unmarshal(data, &lock); err != nil {
		return 0
	}

	// Prefer "packages" (npm v2+), excluding the root package ""
	if len(lock.Packages) > 0 {
		count := len(lock.Packages)
		if _, hasRoot := lock.Packages[""]; hasRoot {
			count--
		}
		return count
	}

	// Fall back to "dependencies" (npm v1)
	return len(lock.Dependencies)
}

// countYarnLock counts packages in yarn.lock.
// Counts lines starting with a package name (no leading whitespace, ends with :).
func countYarnLock(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	// Pattern matches package entries like:
	// "package@version":
	// package@version:
	pkgPattern := regexp.MustCompile(`^[^\s#].*:$`)

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if pkgPattern.MatchString(line) {
			count++
		}
	}

	return count
}

// countPnpmLock counts packages in pnpm-lock.yaml.
// Counts entries under "packages:" section.
func countPnpmLock(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	inPackages := false
	count := 0
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "packages:") {
			inPackages = true
			continue
		}

		// New top-level section starts
		if inPackages && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			break
		}

		// Count indented package entries (2 spaces + path)
		if inPackages && strings.HasPrefix(line, "  /") || strings.HasPrefix(line, "  '") {
			count++
		}
	}

	return count
}

// countBunLock counts packages in bun.lockb.
// Binary format - we can only estimate based on file size.
func countBunLock(path string) int {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	// Rough estimate: ~100 bytes per package entry on average
	// Use int64 for size calculations to prevent overflow
	size := info.Size()
	if size <= 0 {
		return 0
	}
	// Check for potential overflow before division
	const maxInt = int64(^uint(0) >> 1) // Maximum value for int
	if size/100 > maxInt {
		return int(maxInt)
	}
	return int(size / 100)
}

// countComposerLock counts packages in composer.lock.
func countComposerLock(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var lock struct {
		Packages    []interface{} `json:"packages"`
		PackagesDev []interface{} `json:"packages-dev"`
	}

	if err := json.Unmarshal(data, &lock); err != nil {
		return 0
	}

	return len(lock.Packages) + len(lock.PackagesDev)
}

// countPoetryLock counts packages in poetry.lock.
// Counts [[package]] entries.
func countPoetryLock(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[[package]]" {
			count++
		}
	}

	return count
}

// countRequirementsLock counts packages in requirements.lock.
// Counts non-empty, non-comment lines.
func countRequirementsLock(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "-") {
			count++
		}
	}

	return count
}

// countPipfileLock counts packages in Pipfile.lock.
func countPipfileLock(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var lock struct {
		Default map[string]interface{} `json:"default"`
		Develop map[string]interface{} `json:"develop"`
	}

	if err := json.Unmarshal(data, &lock); err != nil {
		return 0
	}

	return len(lock.Default) + len(lock.Develop)
}

// countCargoLock counts packages in Cargo.lock.
// Counts [[package]] entries.
func countCargoLock(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[[package]]" {
			count++
		}
	}

	return count
}

// countGoSum counts unique modules in go.sum.
// Each module can have two entries (one for go.mod, one for the module itself).
// Improved parsing to handle all go.sum format edge cases and deduplicate properly.
func countGoSum(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	modules := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			// Skip empty lines and comments
			continue
		}
		// Format: module version hash [hash]
		// or: module version/go.mod hash [hash]
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		
		module := parts[0]
		version := parts[1]
		
		// Handle /go.mod suffix - this indicates a go.mod checksum entry
		// We want to count the module, not the go.mod entry separately
		if strings.HasSuffix(version, "/go.mod") {
			version = strings.TrimSuffix(version, "/go.mod")
		}
		
		// Create unique key for module@version
		// This deduplicates entries for the same module version
		key := module + "@" + version
		modules[key] = true
	}

	return len(modules)
}

// countGradleLock counts dependencies in gradle.lockfile.
// Counts non-empty, non-comment lines.
func countGradleLock(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines, comments, and the "empty=" line
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "empty=") {
			count++
		}
	}

	return count
}

// ComputeHash is a public wrapper for computing file hashes.
func ComputeHash(path string) (string, error) {
	return computeFileHash(path)
}

