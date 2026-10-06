// Package config handles loading and managing xpm configuration.
//
// Configuration is stored in a JSON file at platform-specific locations:
//   - Linux/macOS: ~/.config/xpm/xpmrc.json
//   - Windows: %APPDATA%\xpm\xpmrc.json
//
// If no configuration file exists, sensible defaults are used.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// warnOut receives warnings about an unusable config file. Tests replace it.
var warnOut io.Writer = os.Stderr

// ScriptsConfig holds configuration for the `xpm run` command.
type ScriptsConfig struct {
	// Prefer specifies which script sources take precedence when the same
	// task name exists in multiple config files.
	// Values: "npm", "composer", "python", "cargo", "makefile"
	// Example: ["npm", "composer"] would prefer npm scripts over composer scripts.
	Prefer []string `json:"prefer"`
}

// LockConfig holds configuration for the `xpm lock` command.
type LockConfig struct {
	// AutoGenerate controls whether xpm automatically regenerates xpm-lock.yaml
	// after install operations that modify lockfiles.
	AutoGenerate bool `json:"autoGenerate"`

	// AutoVerify controls whether xpm automatically verifies lockfile integrity
	// before install operations.
	AutoVerify bool `json:"autoVerify"`
}

// DoctorConfig holds configuration for the `xpm doctor` command.
type DoctorConfig struct {
	// SkipSecurity skips the security audit section.
	SkipSecurity bool `json:"skipSecurity"`

	// SkipEnv skips the environment/runtime check section.
	SkipEnv bool `json:"skipEnv"`

	// SkipConflicts skips the conflict detection section.
	SkipConflicts bool `json:"skipConflicts"`

	// SkipDrift skips the dependency drift detection section.
	SkipDrift bool `json:"skipDrift"`
}

// CacheConfig holds configuration for the global dependency cache.
type CacheConfig struct {
	// Enabled controls whether caching is enabled.
	Enabled bool `json:"enabled"`

	// Path is the cache directory path. Defaults to ~/.xpm/cache.
	Path string `json:"path"`

	// MaxAgeDays is the maximum age in days for cached artifacts during GC.
	MaxAgeDays int `json:"maxAgeDays"`

	// MaxVersions is the maximum number of versions to keep per package during GC.
	MaxVersions int `json:"maxVersions"`
}

// GraphConfig holds configuration for the `xpm graph` command.
type GraphConfig struct {
	// ShowVersions controls whether versions are shown in tree output.
	ShowVersions bool `json:"showVersions"`

	// ShowEcosystem controls whether ecosystem labels are shown in tree output.
	ShowEcosystem bool `json:"showEcosystem"`

	// Depth limits the depth of the dependency tree (0 = unlimited).
	Depth int `json:"depth"`
}

// SearchUIConfig holds configuration for the interactive TUI search.
type SearchUIConfig struct {
	// Enabled controls whether the TUI search is enabled.
	Enabled bool `json:"enabled"`

	// DebounceMs is the debounce delay in milliseconds for search queries.
	DebounceMs int `json:"debounceMs"`

	// PageSize is the maximum number of results to display per page.
	PageSize int `json:"pageSize"`
}

// WorkspaceConfig holds configuration for workspace/monorepo operations.
type WorkspaceConfig struct {
	// Enabled controls whether workspace detection is enabled.
	Enabled bool `json:"enabled"`

	// Include specifies glob patterns for workspace directories to include.
	// If empty, all detected workspaces are included.
	Include []string `json:"include"`

	// Exclude specifies glob patterns for workspace directories to exclude.
	Exclude []string `json:"exclude"`

	// Parallel controls whether operations run in parallel across workspaces.
	Parallel bool `json:"parallel"`
}

// EnvConfig holds configuration for runtime version management.
type EnvConfig struct {
	// Enabled controls whether runtime version management is enabled.
	Enabled bool `json:"enabled"`

	// Path specifies the root directory for runtime installations.
	Path string `json:"path"`

	// Default specifies default versions for each runtime.
	Default map[string]string `json:"default"`
}

// TimeoutConfig holds how long registry lookups may take.
type TimeoutConfig struct {
	// Default is the timeout for every registry, in seconds.
	// 0 means the built-in 2.5 s deadline.
	Default int `json:"default"`

	// PerRegistry overrides Default per registry, in seconds.
	// Keys: "npm", "pypi" (or "pip"), "packagist" (or "composer"),
	// "crates" (or "cargo"), "maven".
	PerRegistry map[string]int `json:"perRegistry"`
}

// Config holds the user's configuration preferences for xpm.
type Config struct {
	// Prefer specifies package manager IDs to prioritize in search results.
	// Package managers listed here appear first in interactive selection prompts.
	// Example: ["npm", "pip"] would show npm results before pip results.
	Prefer []string `json:"prefer"`

	// Search controls which package ecosystems are searched.
	// Keys are package manager IDs (e.g., "npm", "pip", "cargo").
	// Set a value to false to disable searching that ecosystem.
	Search map[string]bool `json:"search"`

	// AutoInstallPM controls whether xpm offers to install missing package managers.
	// When true, xpm will prompt to install package managers that aren't found.
	AutoInstallPM bool `json:"autoInstallPM"`

	// Interactive controls whether xpm uses interactive prompts.
	// Set to false for CI/CD environments or scripted usage.
	// In non-interactive mode, xpm automatically picks the first/preferred option.
	Interactive bool `json:"interactive"`

	// Scripts holds configuration for the `xpm run` command.
	Scripts ScriptsConfig `json:"scripts"`

	// Lock holds configuration for the `xpm lock` command.
	Lock LockConfig `json:"lock"`

	// Doctor holds configuration for the `xpm doctor` command.
	Doctor DoctorConfig `json:"doctor"`

	// Cache holds configuration for the global dependency cache.
	Cache CacheConfig `json:"cache"`

	// Graph holds configuration for the `xpm graph` command.
	Graph GraphConfig `json:"graph"`

	// SearchUI holds configuration for the interactive TUI search.
	SearchUI SearchUIConfig `json:"searchUI"`

	// Workspace holds configuration for workspace/monorepo operations.
	Workspace WorkspaceConfig `json:"workspace"`
	Env       EnvConfig       `json:"env"`

	// Timeout holds configuration for HTTP timeouts.
	Timeout TimeoutConfig `json:"timeout"`
}

// defaultConfig returns the default configuration with all features enabled.
func defaultConfig() Config {
	return Config{
		Prefer: []string{},
		Search: map[string]bool{
			"npm":      true,
			"pip":      true,
			"composer": true,
			"cargo":    true,
			"gomod":    true,
			"maven":    true,
			"gradle":   true,
		},
		AutoInstallPM: true,
		Interactive:   true,
		Scripts: ScriptsConfig{
			Prefer: []string{},
		},
		Lock: LockConfig{
			AutoGenerate: true,
			AutoVerify:   false,
		},
		Doctor: DoctorConfig{
			SkipSecurity:  false,
			SkipEnv:       false,
			SkipConflicts: false,
			SkipDrift:     false,
		},
		Cache: CacheConfig{
			Enabled:     true,
			Path:        "~/.xpm/cache",
			MaxAgeDays:  60,
			MaxVersions: 5,
		},
		Graph: GraphConfig{
			ShowVersions:  true,
			ShowEcosystem: true,
			Depth:         5,
		},
		SearchUI: SearchUIConfig{
			Enabled:    true,
			DebounceMs: 200,
			PageSize:   20,
		},
		Workspace: WorkspaceConfig{
			Enabled:  true,
			Include:  []string{},
			Exclude:  []string{"**/test/**", "**/node_modules/**"},
			Parallel: true,
		},
		Env: EnvConfig{
			Enabled: true,
			Path:    "~/.xpm/env",
			Default: make(map[string]string),
		},
		Timeout: TimeoutConfig{
			Default:     0, // 0 = built-in 2.5 s deadline
			PerRegistry: make(map[string]int),
		},
	}
}

// configPath returns the platform-specific path to the configuration file.
// Returns an empty string if the path cannot be determined.
func configPath() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("APPDATA")
		if base == "" {
			return ""
		}
		return filepath.Join(base, "xpm", "xpmrc.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "xpm", "xpmrc.json")
}

// Load reads the configuration file and returns the Config.
// Fields missing from the file keep their default values.
// If the file doesn't exist, defaults are returned; if it is invalid,
// a warning is printed to stderr and defaults are returned.
func Load() Config {
	return loadFrom(configPath())
}

func loadFrom(path string) Config {
	c := defaultConfig()
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	// Unmarshal on top of the defaults: absent keys keep their default,
	// and map fields (Search) merge key-by-key.
	if err := json.Unmarshal(data, &c); err != nil {
		_, _ = fmt.Fprintf(warnOut, "xpm: ignoring invalid config %s: %v\n", path, err)
		return defaultConfig()
	}
	// An explicit null replaces a map with nil; treat it as "use the defaults".
	if c.Search == nil {
		c.Search = defaultConfig().Search
	}
	return c
}
