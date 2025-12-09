// Package env provides runtime version management functionality.
package env

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/config"
)

// Manager manages runtime versions and environment.
type Manager struct {
	config      config.Config
	envPath     string
	runtimesPath string
	shimsPath   string
	activePath  string
	defaultsPath string
}

// NewManager creates a new environment manager.
func NewManager(cfg config.Config) (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	envPath := cfg.Env.Path
	if envPath == "" {
		envPath = "~/.xpm/env"
	}
	// Expand ~ to home directory
	if envPath[0] == '~' {
		envPath = filepath.Join(homeDir, envPath[2:])
	}

	m := &Manager{
		config:       cfg,
		envPath:      envPath,
		runtimesPath: filepath.Join(envPath, "runtimes"),
		shimsPath:    filepath.Join(envPath, "shims"),
		activePath:   filepath.Join(envPath, "active.json"),
		defaultsPath: filepath.Join(envPath, "defaults.json"),
	}

	if err := m.EnsureDirs(); err != nil {
		return nil, err
	}

	return m, nil
}

// GetEnvPath returns the environment root path.
func (m *Manager) GetEnvPath() string {
	return m.envPath
}

// GetRuntimesPath returns the runtimes directory path.
func (m *Manager) GetRuntimesPath() string {
	return m.runtimesPath
}

// GetShimsPath returns the shims directory path.
func (m *Manager) GetShimsPath() string {
	return m.shimsPath
}

// EnsureDirs creates necessary directories.
func (m *Manager) EnsureDirs() error {
	dirs := []string{
		m.envPath,
		m.runtimesPath,
		m.shimsPath,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// GetActiveVersion returns the active version for a runtime.
// If the active version is an alias (lts, latest), it resolves to the actual installed version.
func (m *Manager) GetActiveVersion(runtime string) (string, error) {
	var activeVersion string
	
	// First check local .xpm-env
	if version := m.getLocalVersion(runtime); version != "" {
		activeVersion = version
	} else {
		// Then check global active.json
		active, err := m.loadActiveVersions()
		if err == nil {
			if version, ok := active[runtime]; ok {
				activeVersion = version
			}
		}
		
		// Finally check defaults
		if activeVersion == "" {
			defaults, err := m.loadDefaults()
			if err == nil {
				if version, ok := defaults[runtime]; ok {
					activeVersion = version
				}
			}
		}
	}
	
	if activeVersion == "" {
		return "", fmt.Errorf("no active version found for %s", runtime)
	}
	
	// If the active version is an alias (lts, latest), resolve it to the actual installed version
	if activeVersion == "lts" || activeVersion == "latest" {
		resolved, err := m.resolveAliasToVersion(runtime, activeVersion)
		if err == nil {
			return resolved, nil
		}
		// If resolution fails, return the alias anyway (might be used elsewhere)
		return activeVersion, nil
	}
	
	return activeVersion, nil
}

// resolveAliasToVersion finds the installed version that has the given alias.
func (m *Manager) resolveAliasToVersion(runtime, alias string) (string, error) {
	runtimePath := filepath.Join(m.GetRuntimesPath(), runtime)
	entries, err := os.ReadDir(runtimePath)
	if err != nil {
		return "", err
	}
	
	// Look for a version directory that has this alias in its metadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		version := entry.Name()
		// Skip alias directories themselves
		if version == "latest" || version == "lts" || version == "" {
			continue
		}
		
		versionPath := filepath.Join(runtimePath, version)
		versionAlias := getVersionAlias(versionPath)
		
		if versionAlias == alias {
			return version, nil
		}
	}
	
	return "", fmt.Errorf("no installed version found with alias %s", alias)
}

// SetActiveVersion sets the active version for a runtime.
func (m *Manager) SetActiveVersion(runtime, version string, global bool) error {
	if global {
		return m.setGlobalVersion(runtime, version)
	}
	return m.setLocalVersion(runtime, version)
}

// setGlobalVersion sets the global active version.
func (m *Manager) setGlobalVersion(runtime, version string) error {
	active, err := m.loadActiveVersions()
	if err != nil {
		active = make(map[string]string)
	}

	active[runtime] = version

	data, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal active versions: %w", err)
	}

	if err := os.WriteFile(m.activePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write active.json: %w", err)
	}

	return nil
}

// setLocalVersion writes to .xpm-env in current directory.
func (m *Manager) setLocalVersion(runtime, version string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	envFile := filepath.Join(cwd, ".xpm-env")
	versions, err := readEnvFile(envFile)
	if err != nil {
		versions = make(map[string]string)
	}

	versions[runtime] = version

	return writeEnvFile(envFile, versions)
}

// getLocalVersion reads version from .xpm-env.
func (m *Manager) getLocalVersion(runtime string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	envFile, err := FindXpmEnv(cwd)
	if err != nil {
		return ""
	}

	versions, err := readEnvFile(envFile)
	if err != nil {
		return ""
	}

	return versions[runtime]
}

// loadActiveVersions loads global active versions.
func (m *Manager) loadActiveVersions() (map[string]string, error) {
	data, err := os.ReadFile(m.activePath)
	if err != nil {
		return nil, err
	}

	var active map[string]string
	if err := json.Unmarshal(data, &active); err != nil {
		return nil, err
	}

	return active, nil
}

// loadDefaults loads default versions.
func (m *Manager) loadDefaults() (map[string]string, error) {
	data, err := os.ReadFile(m.defaultsPath)
	if err != nil {
		return nil, err
	}

	var defaults map[string]string
	if err := json.Unmarshal(data, &defaults); err != nil {
		return nil, err
	}

	return defaults, nil
}

// readEnvFile reads a .xpm-env file.
func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return parseEnvFile(string(data))
}

// writeEnvFile writes a .xpm-env file.
func writeEnvFile(path string, versions map[string]string) error {
	var content string
	for runtime, version := range versions {
		content += fmt.Sprintf("%s=%s\n", runtime, version)
	}

	return os.WriteFile(path, []byte(content), 0644)
}

// parseEnvFile parses .xpm-env file content.
func parseEnvFile(content string) (map[string]string, error) {
	result := make(map[string]string)

	lines := splitLines(content)
	for _, line := range lines {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}

		parts := splitKeyValue(line, "=")
		if len(parts) != 2 {
			continue
		}

		runtime := trimSpace(parts[0])
		version := trimSpace(parts[1])
		if runtime != "" && version != "" {
			result[runtime] = version
		}
	}

	return result, nil
}

// Helper functions for parsing
func splitLines(s string) []string {
	var lines []string
	var current string
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func splitKeyValue(s, sep string) []string {
	for i := 0; i < len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return []string{s[:i], s[i+len(sep):]}
		}
	}
	return []string{s}
}

