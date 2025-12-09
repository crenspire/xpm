package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/pm"
)

// formatError formats an error for user-friendly display.
// It handles custom error types and provides actionable suggestions.
func formatError(err error) string {
	if err == nil {
		return ""
	}

	var pmNotFound *pm.PackageManagerNotFoundError
	if errors.As(err, &pmNotFound) {
		return formatPackageManagerNotFound(pmNotFound)
	}

	var pkgNotFound *pm.PackageNotFoundError
	if errors.As(err, &pkgNotFound) {
		return formatPackageNotFound(pkgNotFound)
	}

	var installFailed *pm.InstallationFailedError
	if errors.As(err, &installFailed) {
		return formatInstallationFailed(installFailed)
	}

	var validationErr *pm.ValidationError
	if errors.As(err, &validationErr) {
		return formatValidationError(validationErr)
	}

	// Default formatting
	return fmt.Sprintf("Error: %v", err)
}

// formatPackageManagerNotFound formats a package manager not found error.
func formatPackageManagerNotFound(e *pm.PackageManagerNotFoundError) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n❌ Package manager not found: %s\n", e.Manager))
	sb.WriteString(fmt.Sprintf("   Binary '%s' is not in your PATH.\n\n", e.Binary))

	if e.Hint != "" {
		sb.WriteString("📦 To install:\n")
		sb.WriteString(fmt.Sprintf("   %s\n\n", e.Hint))
	}

	sb.WriteString("💡 After installation, ensure the binary is in your PATH and try again.\n")
	sb.WriteString("   Run 'xpm doctor' to verify your environment.\n")

	return sb.String()
}

// formatPackageNotFound formats a package not found error.
func formatPackageNotFound(e *pm.PackageNotFoundError) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n❌ Package not found: %s\n", e.Package))
	sb.WriteString(fmt.Sprintf("   The package '%s' was not found in %s.\n\n", e.Package, e.Manager))

	sb.WriteString("💡 Suggestions:\n")
	sb.WriteString("   • Check the package name for typos\n")
	sb.WriteString("   • Use 'xpm which <package>' to search all ecosystems\n")
	sb.WriteString("   • Visit the registry website to verify the package exists\n")

	return sb.String()
}

// formatInstallationFailed formats an installation failure error.
func formatInstallationFailed(e *pm.InstallationFailedError) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n❌ Installation failed: %s\n", e.Package))
	sb.WriteString(fmt.Sprintf("   Failed to install '%s' via %s.\n\n", e.Package, e.Manager))

	if e.Cause != nil {
		sb.WriteString(fmt.Sprintf("   Reason: %v\n\n", e.Cause))
	}

	if e.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("💡 %s\n", e.Suggestion))
	} else {
		sb.WriteString("💡 Suggestions:\n")
		sb.WriteString("   • Check your network connection\n")
		sb.WriteString("   • Verify you have write permissions\n")
		sb.WriteString("   • Try running with verbose mode: xpm -v install <package>\n")
	}

	return sb.String()
}

// formatValidationError formats a validation error.
func formatValidationError(e *pm.ValidationError) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n❌ Invalid input: %s\n", e.Field))
	sb.WriteString(fmt.Sprintf("   Value '%s' is not valid: %s\n\n", e.Value, e.Reason))

	sb.WriteString("💡 Please check your input and try again.\n")

	return sb.String()
}

// printError prints a formatted error to stderr.
func printError(err error) {
	fmt.Fprintln(os.Stderr, formatError(err))
}

// printErrorf prints a formatted error message to stderr.
func printErrorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "\n❌ Error: "+format+"\n", args...)
}

// printWarning prints a warning message to stderr.
func printWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "\n⚠️  Warning: "+format+"\n", args...)
}

// printSuccess prints a success message to stdout.
func printSuccess(format string, args ...interface{}) {
	fmt.Printf("\n✅ "+format+"\n", args...)
}

// printInfo prints an informational message to stdout.
func printInfo(format string, args ...interface{}) {
	fmt.Printf("ℹ️  "+format+"\n", args...)
}

// suggestSimilarPackages suggests similar package names if available.
// This is a placeholder for future fuzzy matching functionality.
func suggestSimilarPackages(pkg string, results []string) {
	if len(results) == 0 {
		return
	}

	fmt.Println("\n💡 Did you mean one of these?")
	for i, name := range results {
		if i >= 5 {
			break
		}
		fmt.Printf("   • %s\n", name)
	}
}

// formatNetworkError provides helpful suggestions for network errors.
func formatNetworkError(err error, registry string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n❌ Network error while contacting %s\n", registry))
	sb.WriteString(fmt.Sprintf("   %v\n\n", err))

	sb.WriteString("💡 Suggestions:\n")
	sb.WriteString("   • Check your internet connection\n")
	sb.WriteString("   • Verify the registry is not blocked by firewall\n")
	sb.WriteString("   • Try again in a few moments\n")
	sb.WriteString("   • Check if proxy settings are required\n")

	return sb.String()
}

