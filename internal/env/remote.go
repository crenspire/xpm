package env

import (
	"fmt"
	"sort"
	"strings"
)

// ListRemote returns available remote versions for a runtime.
func ListRemote(manager *Manager, runtime string) ([]string, error) {
	installer, err := GetInstaller(runtime)
	if err != nil {
		return nil, err
	}

	versions, err := installer.ListRemote()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote versions: %w", err)
	}

	// Sort versions (simple string sort)
	sort.Strings(versions)

	return versions, nil
}

// FormatRemote formats remote versions for display.
func FormatRemote(runtime string, versions []string) string {
	if len(versions) == 0 {
		return fmt.Sprintf("No versions available for %s.", runtime)
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Available %s versions:\n", runtime))

	// Show last 20 versions (most recent)
	start := 0
	if len(versions) > 20 {
		start = len(versions) - 20
		output.WriteString(fmt.Sprintf("(showing last 20 of %d versions)\n\n", len(versions)))
	}

	for i := len(versions) - 1; i >= start; i-- {
		output.WriteString(fmt.Sprintf("  %s\n", versions[i]))
	}

	return output.String()
}

