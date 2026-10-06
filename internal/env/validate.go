package env

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	runtimeNameRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,31}$`)
	versionSpecRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$`)
)

// ValidateRuntimeName accepts lowercase identifiers such as "node" or "python".
func ValidateRuntimeName(name string) error {
	if !runtimeNameRe.MatchString(name) {
		return fmt.Errorf("invalid runtime name %q", name)
	}
	return nil
}

// ValidateVersionSpec accepts versions and aliases ("20.11.0", "lts",
// "1.22rc1", "21.0.2+13") and rejects anything that could act as a path:
// separators, a leading dot or dash, "..", whitespace, or empty strings.
func ValidateVersionSpec(v string) error {
	if !versionSpecRe.MatchString(v) || strings.Contains(v, "..") {
		return fmt.Errorf("invalid version %q", v)
	}
	return nil
}

// versionDir returns <runtimes>/<runtime>/<version>, guaranteeing that it is
// strictly inside the runtimes root.
func (m *Manager) versionDir(runtime, version string) (string, error) {
	if err := ValidateRuntimeName(runtime); err != nil {
		return "", err
	}
	if err := ValidateVersionSpec(version); err != nil {
		return "", err
	}
	root := filepath.Clean(m.GetRuntimesPath())
	dir := filepath.Join(root, runtime, version)
	if !strings.HasPrefix(dir, root+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing path outside %s: %s", root, dir)
	}
	return dir, nil
}
