package scripts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/crenspire/xpm/internal/pm"
)

// LoadAllScripts loads scripts from all detected config files in the directory
// and merges them with the specified precedence.
func LoadAllScripts(dir string, prefer []string) (*MergedScripts, error) {
	var allScripts []ScriptDefinition
	var sources []string

	// Load from package.json
	if scripts, err := loadPackageJSON(filepath.Join(dir, "package.json")); err == nil && len(scripts) > 0 {
		allScripts = append(allScripts, scripts...)
		sources = append(sources, "package.json")
	}

	// Load from composer.json
	if scripts, err := loadComposerJSON(filepath.Join(dir, "composer.json")); err == nil && len(scripts) > 0 {
		allScripts = append(allScripts, scripts...)
		sources = append(sources, "composer.json")
	}

	// Load from pyproject.toml
	if scripts, err := loadPyprojectTOML(filepath.Join(dir, "pyproject.toml")); err == nil && len(scripts) > 0 {
		allScripts = append(allScripts, scripts...)
		sources = append(sources, "pyproject.toml")
	}

	// Load from Cargo.toml
	if scripts, err := loadCargoTOML(filepath.Join(dir, "Cargo.toml")); err == nil && len(scripts) > 0 {
		allScripts = append(allScripts, scripts...)
		sources = append(sources, "Cargo.toml")
	}

	if len(allScripts) == 0 {
		return &MergedScripts{
			Scripts: make(map[string]ScriptDefinition),
			Sources: sources,
		}, nil
	}

	// Merge scripts with precedence
	merged := mergeScripts(allScripts, prefer)

	return &MergedScripts{
		Scripts: merged,
		Sources: sources,
	}, nil
}

// loadPackageJSON loads scripts from a package.json file.
func loadPackageJSON(path string) ([]ScriptDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	if len(pkg.Scripts) == 0 {
		return nil, nil
	}

	// Detect which Node.js package manager to use based on lock files
	manager := detectNodePM(filepath.Dir(path))

	scripts := make([]ScriptDefinition, 0, len(pkg.Scripts))
	for name, cmd := range pkg.Scripts {
		scripts = append(scripts, ScriptDefinition{
			Name:    name,
			Command: cmd,
			Source:  SourceNPM,
			PM:      manager,
			Path:    "package.json",
		})
	}

	return scripts, nil
}

// detectNodePM determines which Node.js package manager to use based on lock files.
func detectNodePM(dir string) pm.ID {
	if _, err := os.Stat(filepath.Join(dir, "bun.lockb")); err == nil {
		return pm.Bun
	}
	if _, err := os.Stat(filepath.Join(dir, "pnpm-lock.yaml")); err == nil {
		return pm.Pnpm
	}
	if _, err := os.Stat(filepath.Join(dir, "yarn.lock")); err == nil {
		return pm.Yarn
	}
	return pm.Npm
}

// loadComposerJSON loads scripts from a composer.json file.
func loadComposerJSON(path string) ([]ScriptDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pkg struct {
		Scripts map[string]interface{} `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	if len(pkg.Scripts) == 0 {
		return nil, nil
	}

	scripts := make([]ScriptDefinition, 0, len(pkg.Scripts))
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
			continue
		}

		scripts = append(scripts, ScriptDefinition{
			Name:    name,
			Command: cmdStr,
			Source:  SourceComposer,
			PM:      pm.Composer,
			Path:    "composer.json",
		})
	}

	return scripts, nil
}

// pyprojectTOML represents the structure of pyproject.toml for script loading.
type pyprojectTOML struct {
	Tool struct {
		UPM struct {
			Scripts map[string]string `toml:"scripts"`
		} `toml:"upm"`
		Poetry struct {
			Scripts map[string]string `toml:"scripts"`
		} `toml:"poetry"`
	} `toml:"tool"`
	Project struct {
		Scripts map[string]string `toml:"scripts"`
	} `toml:"project"`
}

// loadPyprojectTOML loads scripts from a pyproject.toml file.
// It looks for scripts in [tool.upm.scripts], [tool.poetry.scripts], and [project.scripts].
func loadPyprojectTOML(path string) ([]ScriptDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config pyprojectTOML
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, err
	}

	scripts := make([]ScriptDefinition, 0)

	// Prefer [tool.upm.scripts] section
	for name, cmd := range config.Tool.UPM.Scripts {
		scripts = append(scripts, ScriptDefinition{
			Name:    name,
			Command: cmd,
			Source:  SourcePython,
			PM:      pm.Pip,
			Path:    "pyproject.toml",
		})
	}

	// If no upm scripts, try [tool.poetry.scripts]
	if len(scripts) == 0 {
		for name, cmd := range config.Tool.Poetry.Scripts {
			scripts = append(scripts, ScriptDefinition{
				Name:    name,
				Command: cmd,
				Source:  SourcePython,
				PM:      pm.Poetry,
				Path:    "pyproject.toml",
			})
		}
	}

	// If still no scripts, try [project.scripts]
	if len(scripts) == 0 {
		for name, cmd := range config.Project.Scripts {
			scripts = append(scripts, ScriptDefinition{
				Name:    name,
				Command: cmd,
				Source:  SourcePython,
				PM:      pm.Pip,
				Path:    "pyproject.toml",
			})
		}
	}

	return scripts, nil
}

// cargoTOML represents the structure of Cargo.toml for script loading.
type cargoTOML struct {
	Package struct {
		Metadata struct {
			UPM struct {
				Scripts map[string]string `toml:"scripts"`
			} `toml:"upm"`
		} `toml:"metadata"`
	} `toml:"package"`
}

// loadCargoTOML loads scripts from a Cargo.toml file.
// It looks for scripts in [package.metadata.upm.scripts].
func loadCargoTOML(path string) ([]ScriptDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config cargoTOML
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, err
	}

	scripts := make([]ScriptDefinition, 0)
	for name, cmd := range config.Package.Metadata.UPM.Scripts {
		scripts = append(scripts, ScriptDefinition{
			Name:    name,
			Command: cmd,
			Source:  SourceCargo,
			PM:      pm.Cargo,
			Path:    "Cargo.toml",
		})
	}

	return scripts, nil
}

// mergeScripts merges scripts from multiple sources using precedence rules.
// If the same script name exists in multiple sources, the one with higher precedence wins.
// Precedence order:
// 1. Scripts from sources matching prefer list (in order)
// 2. Alphabetical by source name
func mergeScripts(scripts []ScriptDefinition, prefer []string) map[string]ScriptDefinition {
	merged := make(map[string]ScriptDefinition)

	// Create a priority map for sources
	priority := make(map[Source]int)
	for i, p := range prefer {
		// Map prefer strings to Source types
		switch strings.ToLower(p) {
		case "npm", "yarn", "pnpm", "bun", "node":
			priority[SourceNPM] = i
		case "composer", "php":
			priority[SourceComposer] = i
		case "python", "pip", "poetry", "pipenv":
			priority[SourcePython] = i
		case "cargo", "rust":
			priority[SourceCargo] = i
		case "make", "makefile":
			priority[SourceMakefile] = i
		}
	}

	// Sort scripts by priority (lower is better)
	sortedScripts := make([]ScriptDefinition, len(scripts))
	copy(sortedScripts, scripts)

	sort.SliceStable(sortedScripts, func(i, j int) bool {
		pi, okI := priority[sortedScripts[i].Source]
		pj, okJ := priority[sortedScripts[j].Source]

		// If both have priority, compare by priority
		if okI && okJ {
			return pi < pj
		}
		// If only one has priority, it comes first
		if okI {
			return true
		}
		if okJ {
			return false
		}
		// Neither has priority, sort alphabetically by source
		return string(sortedScripts[i].Source) < string(sortedScripts[j].Source)
	})

	// Add scripts to merged map (first one wins due to sorting)
	for _, script := range sortedScripts {
		if _, exists := merged[script.Name]; !exists {
			merged[script.Name] = script
		}
	}

	return merged
}


