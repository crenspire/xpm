package scripts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/crenspire/xpm/internal/pm"
)

// Source identifies where a script was loaded from.
type Source string

// Script source identifiers.
const (
	SourceNPM      Source = "npm"
	SourceComposer Source = "composer"
	SourcePython   Source = "python"
	SourceCargo    Source = "cargo"
	SourceMakefile Source = "makefile"
)

// ScriptDefinition represents a runnable script from any ecosystem.
type ScriptDefinition struct {
	Name    string // Script name (e.g., "dev", "build", "test")
	Command string // The command to run
	Source  Source // Where the script came from (npm, composer, python, cargo)
	PM      pm.ID  // The package manager to use for execution
	Path    string // The file from which it was loaded
}

// MergedScripts holds scripts from all detected sources, merged with precedence.
type MergedScripts struct {
	Scripts map[string]ScriptDefinition // Merged scripts by name
	Sources []string                    // List of files scripts were loaded from
}

// GetScript finds a script by name.
func (m *MergedScripts) GetScript(name string) (*ScriptDefinition, bool) {
	if script, ok := m.Scripts[name]; ok {
		return &script, true
	}
	return nil, false
}

// List returns all script names sorted alphabetically.
func (m *MergedScripts) List() []string {
	names := make([]string, 0, len(m.Scripts))
	for name := range m.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// FormatList returns a formatted string listing all available scripts.
func (m *MergedScripts) FormatList() string {
	if len(m.Scripts) == 0 {
		return "No tasks available"
	}

	var sb strings.Builder
	sb.WriteString("Available tasks:\n\n")

	names := m.List()

	// Find the longest name for alignment
	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}

	for _, name := range names {
		script := m.Scripts[name]
		source := formatSource(script)
		sb.WriteString(fmt.Sprintf("  %-*s  %s\n", maxLen, name, source))
	}

	if len(m.Sources) > 0 {
		sb.WriteString(fmt.Sprintf("\nLoaded from: %s\n", strings.Join(m.Sources, ", ")))
	}

	return sb.String()
}

// formatSource returns a human-readable source description.
func formatSource(script ScriptDefinition) string {
	switch script.Source {
	case SourceNPM:
		return fmt.Sprintf("(package.json via %s)", script.PM)
	case SourceComposer:
		return "(composer.json)"
	case SourcePython:
		return "(pyproject.toml)"
	case SourceCargo:
		return "(Cargo.toml)"
	case SourceMakefile:
		return "(Makefile)"
	default:
		return fmt.Sprintf("(%s)", script.Path)
	}
}

// IsEmpty returns true if no scripts were loaded.
func (m *MergedScripts) IsEmpty() bool {
	return len(m.Scripts) == 0
}

// Count returns the number of scripts.
func (m *MergedScripts) Count() int {
	return len(m.Scripts)
}
