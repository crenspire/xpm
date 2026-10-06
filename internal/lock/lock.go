// Package lock provides unified lockfile generation and verification.
//
// It scans a project's root directory for the lockfiles of all supported
// ecosystems and records each one's SHA-256 hash and package count in
// xpm-lock.yaml, so CI can detect a lockfile that changed, disappeared, or
// appeared since the file was generated.
//
// The file carries no timestamps: regenerating it for an unchanged project
// produces byte-identical output and is not rewritten.
package lock

// LockInfo holds metadata about a single lockfile.
type LockInfo struct {
	// Ecosystem identifies the package ecosystem (e.g. "node", "python").
	Ecosystem string `yaml:"ecosystem,omitempty"`

	// Manager is the package manager that writes the file (e.g. "npm", "uv").
	Manager string `yaml:"manager,omitempty"`

	// File is the lockfile path relative to the project root, with forward
	// slashes. It is always equal to the entry's key in UnifiedLock.Locks.
	File string `yaml:"file"`

	// Hash is the lowercase hex SHA-256 of the file's bytes.
	Hash string `yaml:"hash"`

	// Packages is the number of packages the lockfile resolves (0 when the
	// format cannot be counted, e.g. binary bun.lockb).
	Packages int `yaml:"packages"`
}

// UnifiedLock represents the complete xpm-lock.yaml structure.
type UnifiedLock struct {
	// Version is the schema version (CurrentVersion when written by xpm).
	Version int `yaml:"version"`

	// Locks maps each lockfile's slash-separated path relative to the project
	// root to its metadata.
	Locks map[string]*LockInfo `yaml:"locks"`
}

// LockfileName is the name of the unified lock file.
const LockfileName = "xpm-lock.yaml"

// CurrentVersion is the schema version written by this xpm. Version 1 files
// (keyed by ecosystem, with generatedAt/modified timestamps) are still read.
const CurrentVersion = 2

// NewUnifiedLock creates an empty UnifiedLock at the current schema version.
func NewUnifiedLock() *UnifiedLock {
	return &UnifiedLock{
		Version: CurrentVersion,
		Locks:   make(map[string]*LockInfo),
	}
}

// Count returns the number of lockfiles tracked.
func (u *UnifiedLock) Count() int {
	return len(u.Locks)
}

// IsEmpty reports whether no lockfiles are tracked.
func (u *UnifiedLock) IsEmpty() bool {
	return len(u.Locks) == 0
}

// TotalPackages returns the sum of packages across all lockfiles.
func (u *UnifiedLock) TotalPackages() int {
	total := 0
	for _, info := range u.Locks {
		total += info.Packages
	}
	return total
}

// VerificationResult holds the result of verifying one lockfile.
type VerificationResult struct {
	// Key is the lockfile path relative to the project root (slash-separated).
	Key string

	// File is the lockfile path as recorded (equal to Key for v2 files).
	File string

	// Status is the verification outcome.
	Status VerificationStatus

	// ExpectedHash is the hash recorded in xpm-lock.yaml ("" for StatusAdded).
	ExpectedHash string

	// ActualHash is the hash of the file on disk ("" when not computed).
	ActualHash string

	// Error explains StatusError results.
	Error error
}

// VerificationStatus represents the outcome of a lockfile verification.
type VerificationStatus int

const (
	// StatusUnchanged: the file's hash matches xpm-lock.yaml.
	StatusUnchanged VerificationStatus = iota

	// StatusChanged: the file's hash differs from xpm-lock.yaml.
	StatusChanged

	// StatusMissing: recorded in xpm-lock.yaml but no longer on disk.
	StatusMissing

	// StatusError: the entry could not be checked (unsafe path, read error).
	StatusError

	// StatusAdded: a supported lockfile on disk that xpm-lock.yaml does not record.
	StatusAdded
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
	case StatusAdded:
		return "added"
	default:
		return "unknown"
	}
}

// VerificationPassed reports whether every result is StatusUnchanged.
func VerificationPassed(results []VerificationResult) bool {
	for _, r := range results {
		if r.Status != StatusUnchanged {
			return false
		}
	}
	return true
}
