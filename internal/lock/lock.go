// Package lock provides unified lockfile generation and verification.
//
// This package scans project directories for lockfiles from all supported
// ecosystems, extracts metadata, and produces a unified xpm-lock.yaml file
// for CI/CD, reproducible builds, and integrity verification.
package lock

import (
	"time"
)

// LockInfo holds metadata about a single lockfile.
type LockInfo struct {
	// Ecosystem identifies the package ecosystem (e.g., "node", "python", "rust").
	Ecosystem string `yaml:"ecosystem,omitempty"`

	// Manager specifies the package manager used (e.g., "npm", "yarn", "pip").
	Manager string `yaml:"manager,omitempty"`

	// File is the path to the lockfile relative to project root.
	File string `yaml:"file"`

	// Hash is the SHA256 hash of the lockfile contents.
	Hash string `yaml:"hash"`

	// Modified is the last modification time of the lockfile.
	Modified time.Time `yaml:"modified"`

	// PackageCnt is the number of packages/dependencies in the lockfile.
	PackageCnt int `yaml:"packages"`
}

// UnifiedLock represents the complete xpm-lock.yaml structure.
type UnifiedLock struct {
	// Version is the schema version of the unified lock format.
	Version int `yaml:"version"`

	// GeneratedAt is the timestamp when this lock file was generated.
	GeneratedAt time.Time `yaml:"generatedAt"`

	// Locks contains lockfile metadata keyed by ecosystem name.
	Locks map[string]*LockInfo `yaml:"locks"`
}

// LockfileName is the name of the unified lock file.
const LockfileName = "xpm-lock.yaml"

// CurrentVersion is the current schema version.
const CurrentVersion = 1

// NewUnifiedLock creates a new UnifiedLock with default values.
func NewUnifiedLock() *UnifiedLock {
	return &UnifiedLock{
		Version:     CurrentVersion,
		GeneratedAt: time.Now().UTC(),
		Locks:       make(map[string]*LockInfo),
	}
}

// AddLock adds a lockfile info to the unified lock.
func (u *UnifiedLock) AddLock(key string, info *LockInfo) {
	u.Locks[key] = info
}

// GetLock retrieves lockfile info by key.
func (u *UnifiedLock) GetLock(key string) (*LockInfo, bool) {
	info, ok := u.Locks[key]
	return info, ok
}

// Count returns the number of lockfiles tracked.
func (u *UnifiedLock) Count() int {
	return len(u.Locks)
}

// IsEmpty returns true if no lockfiles are tracked.
func (u *UnifiedLock) IsEmpty() bool {
	return len(u.Locks) == 0
}

// TotalPackages returns the sum of packages across all lockfiles.
func (u *UnifiedLock) TotalPackages() int {
	total := 0
	for _, info := range u.Locks {
		total += info.PackageCnt
	}
	return total
}

// VerificationResult holds the result of verifying a lockfile.
type VerificationResult struct {
	// Key is the ecosystem/lockfile identifier.
	Key string

	// File is the lockfile path.
	File string

	// Status indicates the verification outcome.
	Status VerificationStatus

	// ExpectedHash is the hash from upm-lock.yaml.
	ExpectedHash string

	// ActualHash is the computed hash of the current file.
	ActualHash string

	// Error contains any error encountered during verification.
	Error error
}

// VerificationStatus represents the outcome of a lockfile verification.
type VerificationStatus int

const (
	// StatusUnchanged indicates the lockfile hash matches.
	StatusUnchanged VerificationStatus = iota

	// StatusChanged indicates the lockfile hash differs.
	StatusChanged

	// StatusMissing indicates the lockfile no longer exists.
	StatusMissing

	// StatusError indicates an error occurred during verification.
	StatusError
)

// String returns a human-readable status string.
func (s VerificationStatus) String() string {
	switch s {
	case StatusUnchanged:
		return "unchanged"
	case StatusChanged:
		return "changed"
	case StatusMissing:
		return "missing"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}
