package pm

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Validation patterns for package names in different ecosystems.
var (
	// npmPackagePattern matches valid npm package names.
	// npm allows: lowercase, digits, dots, dashes, underscores, and scopes (@scope/name).
	npmPackagePattern = regexp.MustCompile(`^(@[a-z0-9-~][a-z0-9-._~]*/)?[a-z0-9-~][a-z0-9-._~]*$`)

	// pipPackagePattern matches valid PyPI package names.
	// PyPI allows: letters, digits, dots, dashes, underscores.
	pipPackagePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)

	// composerPackagePattern matches valid Composer package names.
	// Composer uses vendor/package format.
	composerPackagePattern = regexp.MustCompile(`^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9]([_.-]?[a-z0-9]+)*$`)

	// cargoPackagePattern matches valid crate names.
	// Cargo allows: letters, digits, underscores, dashes (but must start with letter).
	cargoPackagePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

	// goModulePattern matches valid Go module paths.
	goModulePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)
)

// PackageNameLimits defines length limits for package names.
const (
	MinPackageNameLength = 1
	MaxPackageNameLength = 214 // npm limit
)

// ValidatePackageName validates a package name for a specific package manager.
// Returns nil if valid, or a ValidationError if invalid.
func ValidatePackageName(pkg string, manager ID) error {
	// Check for empty name
	if pkg == "" {
		return NewValidationError("package name", "", "cannot be empty")
	}

	if err := rejectLeadingDash("package name", pkg); err != nil {
		return err
	}

	// Check length
	if len(pkg) < MinPackageNameLength {
		return NewValidationError("package name", pkg, "too short")
	}
	if len(pkg) > MaxPackageNameLength {
		return NewValidationError("package name", pkg, "too long (max 214 characters)")
	}

	// Check for dangerous characters that could be used for command injection
	if containsDangerousChars(pkg) {
		return NewValidationError("package name", pkg, "contains potentially dangerous characters")
	}

	// Manager-specific validation
	switch manager {
	case Npm, Yarn, Pnpm, Bun:
		if !npmPackagePattern.MatchString(pkg) {
			return NewValidationError("package name", pkg, "invalid npm package name format")
		}
	case Pip:
		if !pipPackagePattern.MatchString(pkg) {
			return NewValidationError("package name", pkg, "invalid PyPI package name format")
		}
	case Composer:
		// Install-time names are full vendor/package names; partial names
		// only reach the search, which uses ValidateGenericPackageName.
		if !composerPackagePattern.MatchString(pkg) {
			return NewValidationError("package name", pkg, "invalid Composer package name (want vendor/package, lowercase)")
		}
	case Cargo:
		if !cargoPackagePattern.MatchString(pkg) {
			return NewValidationError("package name", pkg, "invalid crate name format")
		}
	case GoMod:
		if !goModulePattern.MatchString(pkg) {
			return NewValidationError("package name", pkg, "invalid Go module path format")
		}
	case Maven, Gradle:
		if strings.Contains(pkg, " ") {
			return NewValidationError("package name", pkg, "cannot contain spaces")
		}
	default:
		// Generic validation for unknown managers
		if strings.Contains(pkg, " ") {
			return NewValidationError("package name", pkg, "cannot contain spaces")
		}
	}

	return nil
}

// ValidateGenericPackageName performs basic validation suitable for searching.
// This is more lenient than manager-specific validation.
func ValidateGenericPackageName(pkg string) error {
	if pkg == "" {
		return NewValidationError("package name", "", "cannot be empty")
	}

	if err := rejectLeadingDash("package name", pkg); err != nil {
		return err
	}

	if len(pkg) > MaxPackageNameLength {
		return NewValidationError("package name", pkg, "too long")
	}

	if containsDangerousChars(pkg) {
		return NewValidationError("package name", pkg, "contains invalid characters")
	}

	// Check for excessive whitespace
	if strings.TrimSpace(pkg) != pkg {
		return NewValidationError("package name", pkg, "cannot have leading/trailing whitespace")
	}

	return nil
}

// rejectLeadingDash refuses values a package manager would parse as an
// option (`-g`, `--registry=...`, `--working-dir=/etc`).
func rejectLeadingDash(field, value string) error {
	if strings.HasPrefix(value, "-") {
		return NewValidationError(field, value, "cannot start with '-'")
	}
	return nil
}

// containsDangerousChars checks for characters that could be used for injection.
func containsDangerousChars(s string) bool {
	dangerous := []rune{';', '&', '|', '`', '$', '(', ')', '{', '}', '<', '>', '\n', '\r', '\t', '\x00'}
	for _, r := range s {
		for _, d := range dangerous {
			if r == d {
				return true
			}
		}
	}
	return false
}

// SanitizePackageName sanitizes a package name by removing dangerous characters.
func SanitizePackageName(pkg string) string {
	var result strings.Builder
	for _, r := range pkg {
		if !containsDangerousChar(r) && unicode.IsPrint(r) {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

// containsDangerousChar checks if a single rune is dangerous.
func containsDangerousChar(r rune) bool {
	dangerous := []rune{';', '&', '|', '`', '$', '(', ')', '{', '}', '<', '>', '\n', '\r', '\t', '\x00'}
	for _, d := range dangerous {
		if r == d {
			return true
		}
	}
	return false
}

// ValidateConfig validates configuration values.
func ValidateConfig(prefer []string, search map[string]bool) []error {
	var errs []error

	// Validate prefer list
	validIDs := map[string]bool{
		"npm": true, "yarn": true, "pnpm": true, "bun": true,
		"pip": true, "composer": true, "cargo": true,
		"gomod": true, "maven": true, "gradle": true,
	}

	for _, p := range prefer {
		if !validIDs[p] {
			errs = append(errs, NewValidationError("prefer", p, "unknown package manager"))
		}
	}

	// Validate search map keys
	for k := range search {
		if !validIDs[k] {
			errs = append(errs, NewValidationError("search", k, "unknown package manager"))
		}
	}

	return errs
}

// NormalizePackageName normalizes a package name for comparison.
func NormalizePackageName(pkg string) string {
	// Convert to lowercase
	pkg = strings.ToLower(pkg)
	// Replace underscores with dashes (common in npm/pip)
	pkg = strings.ReplaceAll(pkg, "_", "-")
	return pkg
}

// ValidateVersion validates a version string format.
// This performs basic validation to ensure the version string is reasonable.
// More specific validation should be done by package managers.
//
// Edge cases:
//   - Empty string: returns error
//   - Very long versions (>100 chars): returns error
//   - Versions with dangerous characters: returns error
//
// Returns nil if valid, or a ValidationError if invalid.
func ValidateVersion(version string) error {
	if version == "" {
		return NewValidationError("version", "", "cannot be empty")
	}

	if err := rejectLeadingDash("version", version); err != nil {
		return err
	}

	// Check length (very long versions are suspicious)
	const maxVersionLength = 100
	if len(version) > maxVersionLength {
		return NewValidationError("version", version, fmt.Sprintf("too long (max %d characters)", maxVersionLength))
	}

	// Check for dangerous characters that could be used for injection
	if containsDangerousChars(version) {
		return NewValidationError("version", version, "contains potentially dangerous characters")
	}

	// Basic format check: should contain at least one alphanumeric character
	hasAlphaNum := false
	for _, r := range version {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			hasAlphaNum = true
			break
		}
	}
	if !hasAlphaNum {
		return NewValidationError("version", version, "must contain at least one alphanumeric character")
	}

	return nil
}
