package env

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ANSI color codes for terminal output
const (
	colorReset = "\033[0m"
	colorBold  = "\033[1m"
	colorCyan  = "\033[36m"
	colorGreen = "\033[32m"
)

// VersionInfo contains information about an installed version.
type VersionInfo struct {
	Version string
	Path    string
	Active  bool
	Alias   string // "lts" or "latest" when installed through that alias
}

// ListInstalled returns installed versions per runtime, newest first.
func ListInstalled(m *Manager) (map[string][]VersionInfo, error) {
	result := make(map[string][]VersionInfo)
	entries, err := os.ReadDir(m.GetRuntimesPath())
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read runtimes directory: %w", err)
	}
	for _, entry := range entries {
		runtime := entry.Name()
		if !entry.IsDir() || ValidateRuntimeName(runtime) != nil {
			continue
		}
		versions, err := m.InstalledVersions(runtime)
		if err != nil || len(versions) == 0 {
			continue
		}
		active, _ := m.ActiveVersion(runtime)
		for _, v := range versions {
			dir := filepath.Join(m.GetRuntimesPath(), runtime, v)
			result[runtime] = append(result[runtime], VersionInfo{
				Version: v,
				Path:    dir,
				Active:  v == active.Version,
				Alias:   readMeta(dir).Alias,
			})
		}
	}
	return result, nil
}

// FormatInstalled formats the installed versions for display.
func FormatInstalled(installed map[string][]VersionInfo) string {
	if len(installed) == 0 {
		return "No runtimes installed.\n\nInstall one:\n  xpm env install node@20\n  xpm env install python@3.12\n  xpm env install go@latest\n\nRuntimes: node, go, python, java, rust, bun, deno, php\nSee available versions: xpm env ls-remote <runtime>\n"
	}

	runtimes := make([]string, 0, len(installed))
	for runtime := range installed {
		runtimes = append(runtimes, runtime)
	}
	sort.Strings(runtimes)

	var b strings.Builder
	for _, runtime := range runtimes {
		fmt.Fprintf(&b, "%s%s%s%s:\n", colorBold, colorCyan, runtime, colorReset)
		for _, v := range installed[runtime] {
			alias := ""
			if v.Alias != "" {
				alias = " (" + v.Alias + ")"
			}
			if v.Active {
				fmt.Fprintf(&b, "%s%s  → %s%s%s %s%s(active)%s\n", colorBold, colorGreen, v.Version, colorReset, alias, colorBold, colorGreen, colorReset)
			} else {
				fmt.Fprintf(&b, "  - %s%s\n", v.Version, alias)
			}
		}
	}
	return b.String()
}
