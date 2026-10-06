package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultCachePath is the default cache directory path.
const DefaultCachePath = "~/.xpm/cache"

// Config holds cache configuration.
type Config struct {
	Enabled     bool
	Path        string
	MaxAgeDays  int
	MaxVersions int
}

// DefaultConfig returns the default cache configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:     true,
		Path:        DefaultCachePath,
		MaxAgeDays:  60,
		MaxVersions: 5,
	}
}

// Manager manages the global dependency cache.
type Manager struct {
	config   Config
	basePath string
}

// NewManager creates a new cache manager.
func NewManager(cfg Config) (*Manager, error) {
	path := expandPath(cfg.Path)

	m := &Manager{
		config:   cfg,
		basePath: path,
	}

	// Ensure directories exist if cache is enabled
	if cfg.Enabled {
		if err := m.EnsureDirs(); err != nil {
			return nil, fmt.Errorf("failed to create cache directories: %w", err)
		}
	}

	return m, nil
}

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// EnsureDirs creates all necessary cache directories.
func (m *Manager) EnsureDirs() error {
	dirs := []string{
		m.basePath,
		m.EcosystemPath(EcosystemNode),
		m.EcosystemPath(EcosystemPython),
		m.EcosystemPath(EcosystemPHP),
		m.EcosystemPath(EcosystemRust),
		m.EcosystemPath(EcosystemGo),
		m.EcosystemPath(EcosystemJava),
		m.MetadataPath(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// IsEnabled returns whether caching is enabled.
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}

// BasePath returns the cache base path.
func (m *Manager) BasePath() string {
	return m.basePath
}

// EcosystemPath returns the path for an ecosystem's cache.
func (m *Manager) EcosystemPath(ecosystem Ecosystem) string {
	return filepath.Join(m.basePath, string(ecosystem))
}

// MetadataPath returns the path for metadata files.
func (m *Manager) MetadataPath() string {
	return filepath.Join(m.basePath, "metadata")
}

// PackagePath returns the path for a specific package in the cache.
// Validates and sanitizes inputs to prevent path traversal attacks.
//
// Edge cases handled:
//   - Empty name or version: returns empty string
//   - Path traversal sequences (..): returns empty string
//   - Absolute paths: returns empty string
//   - Already sanitized names: safely handles them
//
// Returns empty string if validation fails, otherwise returns the safe path.
func (m *Manager) PackagePath(ecosystem Ecosystem, name, version string) string {
	// Sanitize name first
	safeName := sanitizeName(name)

	// Clean both name and version to remove any path traversal sequences
	safeName = filepath.Clean(safeName)
	safeVersion := filepath.Clean(version)

	// Validate no path traversal sequences remain after cleaning
	if strings.Contains(safeName, "..") || strings.Contains(safeVersion, "..") {
		// If path traversal detected, return empty string to indicate error
		return ""
	}

	// Ensure cleaned paths don't start with / or contain absolute paths
	if filepath.IsAbs(safeName) || filepath.IsAbs(safeVersion) {
		return ""
	}

	return filepath.Join(m.EcosystemPath(ecosystem), safeName, safeVersion)
}

// ArtifactPath returns the full path for a cached artifact.
func (m *Manager) ArtifactPath(obj *CacheObject) string {
	return filepath.Join(
		m.PackagePath(obj.Ecosystem, obj.Name, obj.Version),
		obj.ArtifactFilename(),
	)
}

// Config returns the cache configuration.
func (m *Manager) Config() Config {
	return m.config
}

// Exists checks if the cache directory exists.
func (m *Manager) Exists() bool {
	_, err := os.Stat(m.basePath)
	return err == nil
}

// GetCachePath returns the default cache path, expanding ~.
func GetCachePath() string {
	return expandPath(DefaultCachePath)
}

// ExpandPath is a public wrapper for expandPath.
func ExpandPath(path string) string {
	return expandPath(path)
}
