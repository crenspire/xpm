package pm

import "fmt"

// PackageManagerNotFoundError indicates that a required package manager binary
// is not installed or not in the system PATH.
type PackageManagerNotFoundError struct {
	// Manager is the ID of the missing package manager.
	Manager ID
	// Binary is the expected binary name.
	Binary string
	// Hint is an optional installation hint.
	Hint string
}

// Error implements the error interface.
func (e *PackageManagerNotFoundError) Error() string {
	msg := fmt.Sprintf("package manager %s not found (binary: %s)", e.Manager, e.Binary)
	if e.Hint != "" {
		msg += fmt.Sprintf("\n  → Install hint: %s", e.Hint)
	}
	return msg
}

// Is implements error matching for errors.Is().
func (e *PackageManagerNotFoundError) Is(target error) bool {
	_, ok := target.(*PackageManagerNotFoundError)
	return ok
}

// PackageNotFoundError indicates that a package was not found in a registry.
type PackageNotFoundError struct {
	// Package is the name of the package that was not found.
	Package string
	// Manager is the package manager/registry that was searched.
	Manager ID
}

// Error implements the error interface.
func (e *PackageNotFoundError) Error() string {
	return fmt.Sprintf("package %q not found in %s", e.Package, e.Manager)
}

// Is implements error matching for errors.Is().
func (e *PackageNotFoundError) Is(target error) bool {
	_, ok := target.(*PackageNotFoundError)
	return ok
}

// InstallationFailedError indicates that a package installation failed.
type InstallationFailedError struct {
	// Package is the name of the package that failed to install.
	Package string
	// Manager is the package manager that attempted the installation.
	Manager ID
	// Cause is the underlying error that caused the failure.
	Cause error
	// Suggestion is an optional suggestion for resolving the issue.
	Suggestion string
}

// Error implements the error interface.
func (e *InstallationFailedError) Error() string {
	msg := fmt.Sprintf("failed to install %q via %s", e.Package, e.Manager)
	if e.Cause != nil {
		msg += fmt.Sprintf(": %v", e.Cause)
	}
	if e.Suggestion != "" {
		msg += fmt.Sprintf("\n  → Suggestion: %s", e.Suggestion)
	}
	return msg
}

// Unwrap returns the underlying error.
func (e *InstallationFailedError) Unwrap() error {
	return e.Cause
}

// Is implements error matching for errors.Is().
func (e *InstallationFailedError) Is(target error) bool {
	_, ok := target.(*InstallationFailedError)
	return ok
}

// SearchError indicates an error occurred while searching registries.
type SearchError struct {
	// Query is the search query that failed.
	Query string
	// Manager is the registry that failed (empty if multiple).
	Manager ID
	// Cause is the underlying error.
	Cause error
}

// Error implements the error interface.
func (e *SearchError) Error() string {
	if e.Manager != "" {
		return fmt.Sprintf("search for %q failed in %s: %v", e.Query, e.Manager, e.Cause)
	}
	return fmt.Sprintf("search for %q failed: %v", e.Query, e.Cause)
}

// Unwrap returns the underlying error.
func (e *SearchError) Unwrap() error {
	return e.Cause
}

// ConfigError indicates a configuration-related error.
type ConfigError struct {
	// Path is the config file path.
	Path string
	// Cause is the underlying error.
	Cause error
}

// Error implements the error interface.
func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error (%s): %v", e.Path, e.Cause)
}

// Unwrap returns the underlying error.
func (e *ConfigError) Unwrap() error {
	return e.Cause
}

// ValidationError indicates invalid input.
type ValidationError struct {
	// Field is the field or parameter that is invalid.
	Field string
	// Value is the invalid value.
	Value string
	// Reason explains why the value is invalid.
	Reason string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s %q: %s", e.Field, e.Value, e.Reason)
}

// NewPackageManagerNotFoundError creates a new PackageManagerNotFoundError.
func NewPackageManagerNotFoundError(id ID) *PackageManagerNotFoundError {
	meta, _ := MetaFor(id)
	return &PackageManagerNotFoundError{
		Manager: id,
		Binary:  meta.Binary,
		Hint:    InstallHint(id),
	}
}

// NewPackageNotFoundError creates a new PackageNotFoundError.
func NewPackageNotFoundError(pkg string, manager ID) *PackageNotFoundError {
	return &PackageNotFoundError{
		Package: pkg,
		Manager: manager,
	}
}

// NewInstallationFailedError creates a new InstallationFailedError.
func NewInstallationFailedError(pkg string, manager ID, cause error, suggestion string) *InstallationFailedError {
	return &InstallationFailedError{
		Package:    pkg,
		Manager:    manager,
		Cause:      cause,
		Suggestion: suggestion,
	}
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, value, reason string) *ValidationError {
	return &ValidationError{
		Field:  field,
		Value:  value,
		Reason: reason,
	}
}

