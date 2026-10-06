package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/crenspire/xpm/internal/pm"
)

// uvID names uv, which internal/pm has no ID for; it is only used to pick
// the install command (`uv sync`).
const uvID pm.ID = "uv"

// pyproject is the subset of pyproject.toml workspace detection reads.
type pyproject struct {
	Project struct {
		Name string `toml:"name"`
	} `toml:"project"`
	Tool struct {
		Poetry struct {
			Name string `toml:"name"`
		} `toml:"poetry"`
		UV struct {
			Workspace struct {
				Members []string `toml:"members"`
				Exclude []string `toml:"exclude"`
			} `toml:"workspace"`
		} `toml:"uv"`
	} `toml:"tool"`
}

// DetectPythonWorkspace detects Python projects: uv workspace members
// ([tool.uv.workspace] members/exclude), Poetry projects anywhere below root
// (bounded walk), pyproject.toml projects in src/*, packages/*, apps/*,
// libs/*, and the root pyproject.toml itself.
func DetectPythonWorkspace(root string) (*Workspace, error) {
	var projects []Project
	add := func(dir string) {
		if p := createPythonProject(dir, filepath.Join(dir, "pyproject.toml")); p != nil {
			projects = append(projects, *p)
		}
	}

	var rootCfg pyproject
	if _, err := toml.DecodeFile(filepath.Join(root, "pyproject.toml"), &rootCfg); err == nil {
		uvws := rootCfg.Tool.UV.Workspace
		patterns := append([]string{}, uvws.Members...)
		for _, ex := range uvws.Exclude {
			patterns = append(patterns, "!"+ex)
		}
		for _, dir := range expandGlobs(root, patterns) {
			add(dir)
		}
		add(root)
	}

	walkDirs(root, func(dir string) bool {
		var cfg pyproject
		if _, err := toml.DecodeFile(filepath.Join(dir, "pyproject.toml"), &cfg); err != nil {
			return false
		}
		if cfg.Tool.Poetry.Name == "" {
			return false
		}
		add(dir)
		return true
	})

	for _, dir := range expandGlobs(root, []string{"src/*", "packages/*", "apps/*", "libs/*"}) {
		add(dir)
	}

	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "python"}, nil
}

// createPythonProject creates a Project from a directory's pyproject.toml,
// or returns nil when there is none or it does not parse.
func createPythonProject(projectDir, pyprojectPath string) *Project {
	var cfg pyproject
	if _, err := toml.DecodeFile(pyprojectPath, &cfg); err != nil {
		return nil
	}
	name := cfg.Project.Name
	if name == "" {
		name = cfg.Tool.Poetry.Name
	}
	if name == "" {
		name = filepath.Base(projectDir)
	}
	pmID, lockfile := detectPythonPM(projectDir)
	return &Project{
		Name:      name,
		Path:      projectDir,
		Ecosystem: "python",
		Manifest:  pyprojectPath,
		Lockfile:  lockfile,
		PM:        pmID,
	}
}

// detectPythonPM picks a Python project's manager in a fixed order:
// uv.lock, poetry.lock, Pipfile.lock, Pipfile, a [tool.poetry] table, pip.
// The lock file is returned when there is one.
func detectPythonPM(projectPath string) (pm.ID, string) {
	for _, lf := range []struct {
		name string
		id   pm.ID
	}{
		{"uv.lock", uvID},
		{"poetry.lock", pm.Poetry},
		{"Pipfile.lock", pm.Pipenv},
	} {
		if p := filepath.Join(projectPath, lf.name); isFile(p) {
			return lf.id, p
		}
	}
	if isFile(filepath.Join(projectPath, "Pipfile")) {
		return pm.Pipenv, ""
	}
	if data, err := os.ReadFile(filepath.Join(projectPath, "pyproject.toml")); err == nil &&
		strings.Contains(string(data), "[tool.poetry]") {
		return pm.Poetry, ""
	}
	return pm.Pip, ""
}
