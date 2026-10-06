package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultConfig verifies the default configuration values.
func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	// Check default values
	if len(cfg.Prefer) != 0 {
		t.Errorf("default Prefer should be empty, got %v", cfg.Prefer)
	}

	if !cfg.AutoInstallPM {
		t.Error("default AutoInstallPM should be true")
	}

	if !cfg.Interactive {
		t.Error("default Interactive should be true")
	}

	// Check all search ecosystems are enabled
	expectedSearch := map[string]bool{
		"npm":      true,
		"pip":      true,
		"composer": true,
		"cargo":    true,
		"gomod":    true,
		"maven":    true,
		"gradle":   true,
	}

	for k, v := range expectedSearch {
		if cfg.Search[k] != v {
			t.Errorf("default Search[%s] = %v, want %v", k, cfg.Search[k], v)
		}
	}
}

// TestLoadNoFile verifies Load returns defaults when config file doesn't exist.
func TestLoadNoFile(t *testing.T) {
	// Load should return defaults when file doesn't exist
	cfg := Load()

	// Should have default values
	if !cfg.AutoInstallPM {
		t.Error("Load() with no file should return default AutoInstallPM = true")
	}

	if !cfg.Interactive {
		t.Error("Load() with no file should return default Interactive = true")
	}
}

// TestLoadValidJSON verifies Load correctly parses valid JSON config.
func TestLoadValidJSON(t *testing.T) {
	// Create a temporary config directory and file
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "upm")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create temp config dir: %v", err)
	}

	configFile := filepath.Join(configDir, "upmrc.json")
	config := Config{
		Prefer:        []string{"npm", "pip"},
		Search:        map[string]bool{"npm": true, "pip": false},
		AutoInstallPM: false,
		Interactive:   false,
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Note: This test can't fully test Load() because it uses the actual config path
	// In production, you'd use dependency injection or environment variables
}

// TestLoadInvalidJSON verifies Load returns defaults for invalid JSON.
func TestLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "upm")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create temp config dir: %v", err)
	}

	configFile := filepath.Join(configDir, "upmrc.json")
	if err := os.WriteFile(configFile, []byte("not valid json{{{"), 0644); err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	// Load() should return defaults for invalid JSON
	// Note: This test can't fully verify since Load() uses fixed paths
}

// TestConfigPath verifies configPath returns a non-empty string on most systems.
func TestConfigPath(t *testing.T) {
	path := configPath()
	// On most systems, this should return a valid path
	// It might be empty if HOME is not set
	if path != "" {
		// Verify it ends with the expected filename
		if filepath.Base(path) != "xpmrc.json" {
			t.Errorf("configPath() should end with xpmrc.json, got %s", path)
		}
	}
}

// writeConfig writes content to a temp xpmrc.json and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xpmrc.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadFromPartialConfigKeepsDefaults(t *testing.T) {
	c := loadFrom(writeConfig(t, `{"prefer":["yarn"],"search":{"maven":false}}`))

	if len(c.Prefer) != 1 || c.Prefer[0] != "yarn" {
		t.Errorf("Prefer = %v, want [yarn]", c.Prefer)
	}
	if c.Search["maven"] {
		t.Error("Search[maven] should be false (set in file)")
	}
	if !c.Search["npm"] || !c.Search["pip"] {
		t.Errorf("unset search keys must keep default true, got %v", c.Search)
	}
	if !c.Interactive || !c.AutoInstallPM || !c.SearchUI.Enabled || !c.Env.Enabled || !c.Cache.Enabled {
		t.Errorf("unset booleans must keep defaults, got %+v", c)
	}
	if c.Graph.Depth != 5 || c.Timeout.Default != 4 {
		t.Errorf("unset numbers must keep defaults, got depth=%d timeout=%d", c.Graph.Depth, c.Timeout.Default)
	}
}

func TestLoadFromExplicitFalseOverridesDefault(t *testing.T) {
	c := loadFrom(writeConfig(t, `{"interactive":false}`))
	if c.Interactive {
		t.Error("explicit interactive:false must win over default")
	}
	if !c.AutoInstallPM {
		t.Error("autoInstallPM must keep its default")
	}
}

func TestLoadFromInvalidJSONReturnsDefaults(t *testing.T) {
	c := loadFrom(writeConfig(t, `not valid json{{{`))
	if !c.Interactive || !c.Search["npm"] {
		t.Errorf("invalid JSON must yield defaults, got %+v", c)
	}
}

func TestLoadFromMissingFileReturnsDefaults(t *testing.T) {
	c := loadFrom(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if !c.Interactive {
		t.Error("missing file must yield defaults")
	}
}

// TestConfigJSONRoundTrip verifies config can be serialized and deserialized.
func TestConfigJSONRoundTrip(t *testing.T) {
	original := Config{
		Prefer: []string{"cargo", "pip"},
		Search: map[string]bool{
			"npm":      true,
			"pip":      false,
			"composer": true,
		},
		AutoInstallPM: false,
		Interactive:   true,
	}

	// Serialize
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Deserialize
	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify
	if len(loaded.Prefer) != len(original.Prefer) {
		t.Errorf("Prefer length mismatch: %d vs %d", len(loaded.Prefer), len(original.Prefer))
	}

	for i, v := range original.Prefer {
		if loaded.Prefer[i] != v {
			t.Errorf("Prefer[%d] = %s, want %s", i, loaded.Prefer[i], v)
		}
	}

	if loaded.AutoInstallPM != original.AutoInstallPM {
		t.Errorf("AutoInstallPM = %v, want %v", loaded.AutoInstallPM, original.AutoInstallPM)
	}

	if loaded.Interactive != original.Interactive {
		t.Errorf("Interactive = %v, want %v", loaded.Interactive, original.Interactive)
	}

	for k, v := range original.Search {
		if loaded.Search[k] != v {
			t.Errorf("Search[%s] = %v, want %v", k, loaded.Search[k], v)
		}
	}
}

// TestEmptyConfig verifies handling of empty JSON object.
func TestEmptyConfig(t *testing.T) {
	var c Config
	if err := json.Unmarshal([]byte("{}"), &c); err != nil {
		t.Fatalf("Failed to unmarshal empty object: %v", err)
	}

	// Empty config should have zero values
	if len(c.Prefer) != 0 {
		t.Errorf("empty config Prefer should be nil/empty, got %v", c.Prefer)
	}

	if c.Search != nil && len(c.Search) != 0 {
		t.Errorf("empty config Search should be nil/empty, got %v", c.Search)
	}
}
