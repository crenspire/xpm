package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("APPDATA", dir)
	cfg := Load()

	// Should have default values
	if !cfg.AutoInstallPM {
		t.Error("Load() with no file should return default AutoInstallPM = true")
	}

	if !cfg.Interactive {
		t.Error("Load() with no file should return default Interactive = true")
	}
}

// TestPathFollowsEnv verifies Path() is derived from the environment variables
// the platform uses, so tests can isolate the config with a temp dir.
func TestPathFollowsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("APPDATA", dir)
	want := filepath.Join(dir, ".config", "xpm", "xpmrc.json")
	if runtime.GOOS == "windows" {
		want = filepath.Join(dir, "xpm", "xpmrc.json")
	}
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
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
	if !c.Interactive || !c.AutoInstallPM || !c.SearchUI.Enabled || !c.Env.Enabled {
		t.Errorf("unset booleans must keep defaults, got %+v", c)
	}
	if c.Graph.Depth != 5 || c.Timeout.Default != 0 {
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

// captureWarnings sends config warnings to a buffer for one test.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := warnOut
	warnOut = &buf
	t.Cleanup(func() { warnOut = old })
	return &buf
}

func TestLoadFromInvalidJSONReturnsDefaultsAndWarns(t *testing.T) {
	warnings := captureWarnings(t)
	path := writeConfig(t, `not valid json{{{`)
	c := loadFrom(path)
	if !c.Interactive || !c.Search["npm"] {
		t.Errorf("invalid JSON must yield defaults, got %+v", c)
	}
	if !strings.Contains(warnings.String(), "xpm: ignoring invalid config "+path) {
		t.Errorf("warning = %q, want it to name %s", warnings.String(), path)
	}
}

func TestLoadFromNullSearchKeepsDefaults(t *testing.T) {
	warnings := captureWarnings(t)
	c := loadFrom(writeConfig(t, `{"search": null}`))
	if c.Search == nil || !c.Search["npm"] || !c.Search["maven"] {
		t.Fatalf(`"search": null must keep the default registries, got %v`, c.Search)
	}
	if warnings.Len() != 0 {
		t.Errorf("valid JSON produced a warning: %q", warnings.String())
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

func TestOldKeysStillLoad(t *testing.T) {
	warnings := captureWarnings(t)
	c := loadFrom(writeConfig(t, `{"lock":{"autoGenerate":false},"workspace":{"enabled":false,"parallel":false},"env":{"default":{"node":"20"}},"prefer":["npm"]}`))
	if warnings.Len() != 0 {
		t.Errorf("unexpected warning: %q", warnings.String())
	}
	if len(c.Prefer) != 1 || c.Prefer[0] != "npm" {
		t.Errorf("Prefer = %v, want [npm]", c.Prefer)
	}
	if c.Workspace.Parallel {
		t.Error("Workspace.Parallel should be false (set in file)")
	}
}

// TestRemovedCacheKeyStillLoads proves config files written while the cache
// command existed keep loading: unknown keys are ignored without a warning.
func TestRemovedCacheKeyStillLoads(t *testing.T) {
	warnings := captureWarnings(t)
	c := loadFrom(writeConfig(t, `{"cache": {"enabled": true, "path": "~/.xpm/cache"}, "prefer": ["npm"]}`))
	if warnings.Len() != 0 {
		t.Errorf("unexpected warning: %q", warnings.String())
	}
	if len(c.Prefer) != 1 || c.Prefer[0] != "npm" {
		t.Errorf("Prefer = %v, want [npm]", c.Prefer)
	}
}
