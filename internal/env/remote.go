package env

import (
	"context"
	"fmt"
	"strings"
)

// ListRemote returns the runtime's available versions, newest first.
func ListRemote(ctx context.Context, runtime string) ([]string, error) {
	installer, err := GetInstaller(runtime)
	if err != nil {
		return nil, err
	}
	versions, err := installer.ListRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote versions: %w", err)
	}
	SortVersionsDesc(versions)
	return versions, nil
}

// FormatRemote formats remote versions (newest first) for display.
func FormatRemote(runtime string, versions []string) string {
	if len(versions) == 0 {
		return fmt.Sprintf("No versions available for %s.\n", runtime)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Available %s versions:\n", runtime)
	shown := versions
	if len(shown) > 20 {
		shown = shown[:20]
		fmt.Fprintf(&b, "(showing the newest 20 of %d versions)\n\n", len(versions))
	}
	for _, v := range shown {
		fmt.Fprintf(&b, "  %s\n", v)
	}
	return b.String()
}
