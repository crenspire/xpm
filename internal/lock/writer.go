package lock

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WriteUnifiedLock writes the unified lock to a file.
func WriteUnifiedLock(dir string, lock *UnifiedLock) error {
	path := filepath.Join(dir, LockfileName)

	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("failed to marshal lock: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", LockfileName, err)
	}

	return nil
}

// ReadUnifiedLock reads the unified lock from a file.
func ReadUnifiedLock(dir string) (*UnifiedLock, error) {
	path := filepath.Join(dir, LockfileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s not found - run 'xpm lock' to generate it", LockfileName)
		}
		return nil, fmt.Errorf("failed to read %s: %w", LockfileName, err)
	}

	var lock UnifiedLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", LockfileName, err)
	}

	return &lock, nil
}

// UnifiedLockExists checks if the unified lock file exists.
func UnifiedLockExists(dir string) bool {
	path := filepath.Join(dir, LockfileName)
	_, err := os.Stat(path)
	return err == nil
}

// Generate scans a directory and generates a unified lock.
func Generate(dir string) (*UnifiedLock, error) {
	detected, err := DetectAll(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to detect lockfiles: %w", err)
	}

	lock := NewUnifiedLock()

	for _, d := range detected {
		info, err := ParseLockfile(d)
		if err != nil {
			// Log warning but continue with other lockfiles
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v\n", d.RelPath, err)
			continue
		}

		key := GetEcosystemKey(d)
		lock.AddLock(key, info)
	}

	return lock, nil
}

// Verify compares the current lockfile state against the unified lock.
func Verify(dir string) ([]VerificationResult, error) {
	savedLock, err := ReadUnifiedLock(dir)
	if err != nil {
		return nil, err
	}

	var results []VerificationResult

	for key, savedInfo := range savedLock.Locks {
		result := VerificationResult{
			Key:          key,
			File:         savedInfo.File,
			ExpectedHash: savedInfo.Hash,
		}

		path := filepath.Join(dir, savedInfo.File)
		if !fileExists(path) {
			result.Status = StatusMissing
			results = append(results, result)
			continue
		}

		currentHash, err := ComputeHash(path)
		if err != nil {
			result.Status = StatusError
			result.Error = err
			results = append(results, result)
			continue
		}

		result.ActualHash = currentHash
		if currentHash == savedInfo.Hash {
			result.Status = StatusUnchanged
		} else {
			result.Status = StatusChanged
		}

		results = append(results, result)
	}

	return results, nil
}

// VerificationPassed returns true if all verifications passed.
func VerificationPassed(results []VerificationResult) bool {
	for _, r := range results {
		if r.Status != StatusUnchanged {
			return false
		}
	}
	return true
}

// FormatVerificationResults formats verification results for display.
func FormatVerificationResults(results []VerificationResult) string {
	var output string

	for _, r := range results {
		switch r.Status {
		case StatusUnchanged:
			output += fmt.Sprintf("✔ %s unchanged\n", r.File)
		case StatusChanged:
			output += fmt.Sprintf("✘ %s changed\n", r.File)
		case StatusMissing:
			output += fmt.Sprintf("✘ %s missing\n", r.File)
		case StatusError:
			output += fmt.Sprintf("✘ %s error: %v\n", r.File, r.Error)
		}
	}

	return output
}
