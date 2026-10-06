// Package scripts provides parsers for extracting runnable scripts from
// various package manager configuration files (package.json, composer.json, pyproject.toml).
package scripts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/crenspire/xpm/internal/pm"
)

// Script represents a runnable script from a project configuration.
type Script struct {
	Name    string // Script name (e.g., "dev", "build", "test")
	Command string // The command to run
	Manager pm.ID  // The package manager that can run this script
}

// ProjectScripts holds the scripts found in a project and the associated package manager.
type ProjectScripts struct {
	Manager pm.ID    // The detected package manager
	Scripts []Script // List of available scripts
	File    string   // The config file where scripts were found
}

// DetectAndParse finds the project configuration file and parses scripts from it.
// It checks for package.json, composer.json, and pyproject.toml in order.
func DetectAndParse(dir string) (*ProjectScripts, error) {
	// Check for package.json (npm/yarn/pnpm/bun)
	if scripts, err := parsePackageJSON(filepath.Join(dir, "package.json")); err == nil {
		return scripts, nil
	}

	// Check for composer.json (PHP)
	if scripts, err := parseComposerJSON(filepath.Join(dir, "composer.json")); err == nil {
		return scripts, nil
	}

	// Check for pyproject.toml (Python)
	if scripts, err := parsePyprojectTOML(filepath.Join(dir, "pyproject.toml")); err == nil {
		return scripts, nil
	}

	return nil, fmt.Errorf("no supported configuration file found in %s", dir)
}

// parsePackageJSON parses scripts from a package.json file.
func parsePackageJSON(path string) (*ProjectScripts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("invalid package.json: %w", err)
	}

	if len(pkg.Scripts) == 0 {
		return nil, fmt.Errorf("no scripts found in package.json")
	}

	// Detect which Node.js package manager to use
	manager := detectNodePackageManager(filepath.Dir(path))

	scripts := make([]Script, 0, len(pkg.Scripts))
	for name, cmd := range pkg.Scripts {
		scripts = append(scripts, Script{
			Name:    name,
			Command: cmd,
			Manager: manager,
		})
	}

	// Sort scripts by name for consistent output
	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].Name < scripts[j].Name
	})

	return &ProjectScripts{
		Manager: manager,
		Scripts: scripts,
		File:    "package.json",
	}, nil
}

// detectNodePackageManager determines which Node.js package manager to use
// based on lock files present in the directory.
func detectNodePackageManager(dir string) pm.ID {
	// Check for lock files in order of preference
	if _, err := os.Stat(filepath.Join(dir, "bun.lockb")); err == nil {
		return pm.Bun
	}
	if _, err := os.Stat(filepath.Join(dir, "pnpm-lock.yaml")); err == nil {
		return pm.Pnpm
	}
	if _, err := os.Stat(filepath.Join(dir, "yarn.lock")); err == nil {
		return pm.Yarn
	}
	// Default to npm
	return pm.Npm
}

// parseComposerJSON parses scripts from a composer.json file.
func parseComposerJSON(path string) (*ProjectScripts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pkg struct {
		Scripts map[string]interface{} `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("invalid composer.json: %w", err)
	}

	if len(pkg.Scripts) == 0 {
		return nil, fmt.Errorf("no scripts found in composer.json")
	}

	scripts := make([]Script, 0, len(pkg.Scripts))
	for name, cmd := range pkg.Scripts {
		cmdStr := ""
		switch v := cmd.(type) {
		case string:
			cmdStr = v
		case []interface{}:
			// Composer scripts can be arrays of commands
			parts := make([]string, 0, len(v))
			for _, part := range v {
				if s, ok := part.(string); ok {
					parts = append(parts, s)
				}
			}
			cmdStr = strings.Join(parts, " && ")
		default:
			continue // Skip unsupported script formats
		}

		scripts = append(scripts, Script{
			Name:    name,
			Command: cmdStr,
			Manager: pm.Composer,
		})
	}

	// Sort scripts by name for consistent output
	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].Name < scripts[j].Name
	})

	return &ProjectScripts{
		Manager: pm.Composer,
		Scripts: scripts,
		File:    "composer.json",
	}, nil
}

// parsePyprojectTOML parses scripts from a pyproject.toml file.
// It looks for scripts in [project.scripts] and [tool.poetry.scripts] sections.
func parsePyprojectTOML(path string) (*ProjectScripts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Simple TOML parsing for scripts sections
	scripts := make([]Script, 0)
	content := string(data)

	// Look for [project.scripts] or [tool.poetry.scripts] sections
	scriptSections := []string{"[project.scripts]", "[tool.poetry.scripts]"}

	for _, section := range scriptSections {
		idx := strings.Index(content, section)
		if idx == -1 {
			continue
		}

		// Extract the section content until next section or end of file
		sectionContent := content[idx+len(section):]
		nextSection := strings.Index(sectionContent, "\n[")
		if nextSection != -1 {
			sectionContent = sectionContent[:nextSection]
		}

		// Parse key = "value" entries
		lines := strings.Split(sectionContent, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			name := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Remove quotes from value
			value = strings.Trim(value, "\"'")

			scripts = append(scripts, Script{
				Name:    name,
				Command: value,
				Manager: pm.Pip,
			})
		}
	}

	if len(scripts) == 0 {
		return nil, fmt.Errorf("no scripts found in pyproject.toml")
	}

	// Sort scripts by name
	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].Name < scripts[j].Name
	})

	return &ProjectScripts{
		Manager: pm.Pip,
		Scripts: scripts,
		File:    "pyproject.toml",
	}, nil
}

// GetScript finds a script by name in the project scripts.
func (ps *ProjectScripts) GetScript(name string) (*Script, bool) {
	for _, s := range ps.Scripts {
		if s.Name == name {
			return &s, true
		}
	}
	return nil, false
}

// ListScripts returns a formatted list of available scripts.
func (ps *ProjectScripts) ListScripts() string {
	if len(ps.Scripts) == 0 {
		return "No scripts available"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Scripts from %s:\n\n", ps.File))

	// Find the longest script name for alignment
	maxLen := 0
	for _, s := range ps.Scripts {
		if len(s.Name) > maxLen {
			maxLen = len(s.Name)
		}
	}

	for _, s := range ps.Scripts {
		// Truncate command if too long
		cmd := s.Command
		if len(cmd) > 50 {
			cmd = cmd[:47] + "..."
		}
		sb.WriteString(fmt.Sprintf("  %-*s  %s\n", maxLen, s.Name, cmd))
	}

	return sb.String()
}
