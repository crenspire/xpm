package cache

import (
	"path/filepath"
)

// InjectionConfig holds environment variables and flags for cache injection.
type InjectionConfig struct {
	// EnvVars are environment variables to set.
	EnvVars map[string]string

	// Flags are command-line flags to add.
	Flags []string
}

// NewInjectionConfig creates an empty injection config.
func NewInjectionConfig() *InjectionConfig {
	return &InjectionConfig{
		EnvVars: make(map[string]string),
		Flags:   []string{},
	}
}

// InjectNode returns injection config for Node.js package managers.
func (m *Manager) InjectNode(pm string) *InjectionConfig {
	cfg := NewInjectionConfig()
	if !m.IsEnabled() {
		return cfg
	}

	nodeCachePath := m.EcosystemPath(EcosystemNode)

	switch pm {
	case "npm":
		// npm uses --cache flag or npm_config_cache env
		cfg.EnvVars["npm_config_cache"] = nodeCachePath
		cfg.Flags = append(cfg.Flags, "--cache", nodeCachePath)

	case "yarn":
		// Yarn uses YARN_CACHE_FOLDER env
		cfg.EnvVars["YARN_CACHE_FOLDER"] = nodeCachePath

	case "pnpm":
		// pnpm uses --store-dir flag or PNPM_HOME
		storePath := filepath.Join(nodeCachePath, "pnpm-store")
		cfg.EnvVars["PNPM_HOME"] = storePath
		cfg.Flags = append(cfg.Flags, "--store-dir", storePath)

	case "bun":
		// Bun uses BUN_INSTALL_CACHE_DIR env
		cfg.EnvVars["BUN_INSTALL_CACHE_DIR"] = nodeCachePath
	}

	return cfg
}

// InjectPython returns injection config for Python package managers.
func (m *Manager) InjectPython(pm string) *InjectionConfig {
	cfg := NewInjectionConfig()
	if !m.IsEnabled() {
		return cfg
	}

	pythonCachePath := m.EcosystemPath(EcosystemPython)

	switch pm {
	case "pip":
		// pip uses --cache-dir flag or PIP_CACHE_DIR env
		cfg.EnvVars["PIP_CACHE_DIR"] = pythonCachePath
		cfg.Flags = append(cfg.Flags, "--cache-dir", pythonCachePath)

	case "poetry":
		// Poetry uses POETRY_CACHE_DIR env
		cfg.EnvVars["POETRY_CACHE_DIR"] = pythonCachePath

	case "pipenv":
		// Pipenv uses PIPENV_CACHE_DIR env
		cfg.EnvVars["PIPENV_CACHE_DIR"] = pythonCachePath
	}

	return cfg
}

// InjectComposer returns injection config for Composer.
func (m *Manager) InjectComposer() *InjectionConfig {
	cfg := NewInjectionConfig()
	if !m.IsEnabled() {
		return cfg
	}

	phpCachePath := m.EcosystemPath(EcosystemPHP)

	// Composer uses COMPOSER_CACHE_DIR env
	cfg.EnvVars["COMPOSER_CACHE_DIR"] = phpCachePath

	return cfg
}

// InjectCargo returns injection config for Cargo.
func (m *Manager) InjectCargo() *InjectionConfig {
	cfg := NewInjectionConfig()
	if !m.IsEnabled() {
		return cfg
	}

	rustCachePath := m.EcosystemPath(EcosystemRust)

	// Cargo uses CARGO_HOME env which contains registry/cache
	cargoHome := filepath.Join(rustCachePath, "cargo-home")
	cfg.EnvVars["CARGO_HOME"] = cargoHome

	return cfg
}

// InjectGo returns injection config for Go modules.
func (m *Manager) InjectGo() *InjectionConfig {
	cfg := NewInjectionConfig()
	if !m.IsEnabled() {
		return cfg
	}

	goCachePath := m.EcosystemPath(EcosystemGo)

	// Go uses GOMODCACHE env for module cache
	cfg.EnvVars["GOMODCACHE"] = goCachePath

	return cfg
}

// InjectMaven returns injection config for Maven.
func (m *Manager) InjectMaven() *InjectionConfig {
	cfg := NewInjectionConfig()
	if !m.IsEnabled() {
		return cfg
	}

	javaCachePath := m.EcosystemPath(EcosystemJava)
	mavenRepo := filepath.Join(javaCachePath, "maven-repo")

	// Maven uses -Dmaven.repo.local flag
	cfg.Flags = append(cfg.Flags, "-Dmaven.repo.local="+mavenRepo)

	return cfg
}

// InjectGradle returns injection config for Gradle.
func (m *Manager) InjectGradle() *InjectionConfig {
	cfg := NewInjectionConfig()
	if !m.IsEnabled() {
		return cfg
	}

	javaCachePath := m.EcosystemPath(EcosystemJava)
	gradleHome := filepath.Join(javaCachePath, "gradle-home")

	// Gradle uses GRADLE_USER_HOME env
	cfg.EnvVars["GRADLE_USER_HOME"] = gradleHome

	return cfg
}

// InjectForEcosystem returns injection config based on ecosystem.
func (m *Manager) InjectForEcosystem(ecosystem Ecosystem, pm string) *InjectionConfig {
	switch ecosystem {
	case EcosystemNode:
		return m.InjectNode(pm)
	case EcosystemPython:
		return m.InjectPython(pm)
	case EcosystemPHP:
		return m.InjectComposer()
	case EcosystemRust:
		return m.InjectCargo()
	case EcosystemGo:
		return m.InjectGo()
	case EcosystemJava:
		if pm == "maven" || pm == "mvn" {
			return m.InjectMaven()
		}
		return m.InjectGradle()
	default:
		return NewInjectionConfig()
	}
}

// GetEnvSlice returns environment variables as a slice of KEY=VALUE strings.
func (c *InjectionConfig) GetEnvSlice() []string {
	var envs []string
	for k, v := range c.EnvVars {
		envs = append(envs, k+"="+v)
	}
	return envs
}

// Merge combines another InjectionConfig into this one.
func (c *InjectionConfig) Merge(other *InjectionConfig) {
	for k, v := range other.EnvVars {
		c.EnvVars[k] = v
	}
	c.Flags = append(c.Flags, other.Flags...)
}
