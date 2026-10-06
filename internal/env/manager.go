// Package env provides runtime version management functionality.
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crenspire/xpm/internal/config"
)

// Manager manages runtime versions and environment.
type Manager struct {
	config       config.Config
	envPath      string
	runtimesPath string
	shimsPath    string
	activePath   string
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
	if envPath == "~" {
		envPath = homeDir
	} else if strings.HasPrefix(envPath, "~/") {
		envPath = filepath.Join(homeDir, envPath[2:])
	}

	m := &Manager{
		config:       cfg,
		envPath:      envPath,
		runtimesPath: filepath.Join(envPath, "runtimes"),
		shimsPath:    filepath.Join(envPath, "shims"),
		activePath:   filepath.Join(envPath, "active.json"),
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

// GetActivePath returns the path of the global active.json.
func (m *Manager) GetActivePath() string {
	return m.activePath
}

// EnsureDirs creates necessary directories.
func (m *Manager) EnsureDirs() error {
	for _, dir := range []string{m.envPath, m.runtimesPath, m.shimsPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}
