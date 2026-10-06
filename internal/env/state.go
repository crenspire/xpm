package env

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoVersion means no .xpm-env and no active.json entry names the runtime.
var ErrNoVersion = errors.New("no version configured")

// ErrNotInstalled means a version is configured but none installed matches it.
var ErrNotInstalled = errors.New("not installed")

// Active is the version of a runtime in effect for the current directory.
type Active struct {
	Version string // exact installed version (or the raw value when not installed)
	Source  string // the .xpm-env or active.json that set it
	Global  bool   // true when Source is active.json
}

const envFileName = ".xpm-env"

// ActiveVersion resolves the runtime's version for the current directory:
// the nearest .xpm-env (walking up from cwd) that has a key for it, else
// active.json. The value may be exact, partial ("20"), "latest" or "lts";
// it is matched against installed versions. Shims and the CLI share this.
func (m *Manager) ActiveVersion(runtime string) (Active, error) {
	raw, source, global, err := m.configuredVersion(runtime)
	if err != nil {
		return Active{}, err
	}
	a := Active{Version: raw, Source: source, Global: global}
	if ValidateVersionSpec(raw) != nil {
		return Active{}, fmt.Errorf("invalid %s version %q in %s", runtime, raw, source)
	}
	exact, ok := m.resolveInstalled(runtime, raw)
	if !ok {
		return a, fmt.Errorf("%s@%s is %w (set in %s)", runtime, raw, ErrNotInstalled, source)
	}
	a.Version = exact
	return a, nil
}

// configuredVersion finds the raw configured value and where it came from.
func (m *Manager) configuredVersion(runtime string) (raw, source string, global bool, err error) {
	if cwd, werr := os.Getwd(); werr == nil {
		for dir := cwd; ; {
			path := filepath.Join(dir, envFileName)
			if data, rerr := os.ReadFile(path); rerr == nil {
				if v, ok := parseEnvFile(string(data))[runtime]; ok {
					return v, path, false, nil
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	active, err := m.loadActiveVersions()
	if err != nil {
		return "", "", false, err
	}
	if v, ok := active[runtime]; ok {
		return v, m.activePath, true, nil
	}
	return "", "", false, fmt.Errorf("%s: %w", runtime, ErrNoVersion)
}

// InstalledVersions lists the installed versions of runtime, newest first.
// Hidden entries (".tmp-*", ".lock") and names that are not versions
// (pre-P5 "latest"/"lts" alias dirs) are ignored.
func (m *Manager) InstalledVersions(runtime string) ([]string, error) {
	if err := ValidateRuntimeName(runtime); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(m.runtimesPath, runtime))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") || ValidateVersionSpec(name) != nil {
			continue
		}
		if _, ok := ParseVersion(name); !ok {
			continue
		}
		versions = append(versions, name)
	}
	SortVersionsDesc(versions)
	return versions, nil
}

// resolveInstalled matches a configured value against installed versions:
// exact name, "lts" (highest whose metadata alias is lts), "latest"
// (highest), or a component-wise prefix (highest match, stable first).
func (m *Manager) resolveInstalled(runtime, spec string) (string, bool) {
	installed, err := m.InstalledVersions(runtime)
	if err != nil || len(installed) == 0 {
		return "", false
	}
	for _, v := range installed {
		if v == spec {
			return v, true
		}
	}
	if spec == "lts" {
		for _, v := range installed { // newest first
			if readMeta(filepath.Join(m.runtimesPath, runtime, v)).Alias == "lts" {
				return v, true
			}
		}
		return "", false
	}
	if v, ok := HighestMatch(spec, installed, false); ok {
		return v, true
	}
	return HighestMatch(spec, installed, true)
}

// versionMeta is <versionDir>/.xpm-meta.json.
type versionMeta struct {
	Version string `json:"version"`
	Alias   string `json:"alias"`
}

const metaFileName = ".xpm-meta.json"

func readMeta(dir string) versionMeta {
	var meta versionMeta
	data, err := os.ReadFile(filepath.Join(dir, metaFileName))
	if err == nil {
		_ = json.Unmarshal(data, &meta) // a damaged file only loses the alias
	}
	return meta
}

func writeMeta(dir string, meta versionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, metaFileName), append(data, '\n'), 0o644)
}

// loadActiveVersions reads active.json; a missing file is an empty map.
func (m *Manager) loadActiveVersions() (map[string]string, error) {
	active := map[string]string{}
	data, err := os.ReadFile(m.activePath)
	if errors.Is(err, os.ErrNotExist) {
		return active, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &active); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", m.activePath, err)
	}
	return active, nil
}

// GlobalVersion returns the raw active.json entry for runtime ("" if none).
func (m *Manager) GlobalVersion(runtime string) (string, error) {
	active, err := m.loadActiveVersions()
	if err != nil {
		return "", err
	}
	return active[runtime], nil
}

// SetGlobalVersion writes runtime=version into active.json atomically
// (sorted keys, trailing newline).
func (m *Manager) SetGlobalVersion(runtime, version string) error {
	active, err := m.loadActiveVersions()
	if err != nil {
		return err
	}
	active[runtime] = version
	data, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(m.activePath, append(data, '\n'), 0o644)
}

// SetLocalVersion writes runtime=version into dir/.xpm-env atomically,
// keeping comments, blank lines and key order.
func SetLocalVersion(dir, runtime, version string) (string, error) {
	path := filepath.Join(dir, envFileName)
	perm := os.FileMode(0o644)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if fi, serr := os.Stat(path); serr == nil {
		perm = fi.Mode().Perm()
	}
	return path, writeFileAtomic(path, []byte(updateEnvContent(string(data), runtime, version)), perm)
}

// parseEnvFile parses `key=value` lines; '#' starts a comment line. The
// first occurrence of a key wins.
func parseEnvFile(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			continue
		}
		if _, seen := result[key]; !seen {
			result[key] = value
		}
	}
	return result
}

// updateEnvContent replaces the value of the first `key=` line (keeping its
// key text and indentation) or appends `key=value`.
func updateEnvContent(content, key, value string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		k, _, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		eq := strings.Index(line, "=")
		cr := ""
		if strings.HasSuffix(line, "\r") {
			cr = "\r"
		}
		lines[i] = line[:eq+1] + value + cr
		return strings.Join(lines, "\n")
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + key + "=" + value + "\n"
}

// writeFileAtomic writes data to a temp file in path's directory and renames
// it over path, so readers see the old or the new file, never a mix.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved // write through a symlink instead of replacing it
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	if werr == nil {
		werr = tmp.Sync()
	}
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, perm)
	}
	if werr == nil {
		werr = os.Rename(name, path)
	}
	if werr != nil {
		_ = os.Remove(name)
		return werr
	}
	return nil
}
