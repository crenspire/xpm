package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/crenspire/xpm/internal/pm"
)

// DetectPythonWorkspace detects Python multi-project layouts.
func DetectPythonWorkspace(root string) (*Workspace, error) {
	var projects []Project

	// Strategy 1: Check for Poetry multi-package layout
	// Look for pyproject.toml files in subdirectories
	if pyProjects := detectPoetryWorkspace(root); len(pyProjects) > 0 {
		projects = append(projects, pyProjects...)
	}

	// Strategy 2: Scan for common patterns
	patterns := []string{
		"src/*/pyproject.toml",
		"packages/*/pyproject.toml",
		"apps/*/pyproject.toml",
		"libs/*/pyproject.toml",
	}

	for _, pattern := range patterns {
		globPattern := filepath.Join(root, pattern)
		matches, err := filepath.Glob(globPattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			projectDir := filepath.Dir(match)
			if isProjectInList(projects, projectDir) {
				continue
			}

			project := createPythonProject(projectDir, match)
			if project != nil {
				projects = append(projects, *project)
			}
		}
	}

	// Strategy 3: Check root pyproject.toml for multiple [project] definitions
	// (less common, but possible)
	if rootProject := detectRootPythonWorkspace(root); rootProject != nil {
		projects = append(projects, *rootProject)
	}

	if len(projects) == 0 {
		return nil, nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "python",
	}, nil
}

// detectPoetryWorkspace detects Poetry multi-package workspaces.
func detectPoetryWorkspace(root string) []Project {
	var projects []Project

	// Look for pyproject.toml files in subdirectories
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip root
		if path == root {
			return nil
		}

		// Only check directories
		if !info.IsDir() {
			return nil
		}

		// Check for pyproject.toml
		pyprojectPath := filepath.Join(path, "pyproject.toml")
		if _, err := os.Stat(pyprojectPath); err != nil {
			return nil
		}

		// Check if it's a Poetry project
		var config struct {
			Tool struct {
				Poetry struct {
					Name string `toml:"name"`
				} `toml:"poetry"`
			} `toml:"tool"`
		}

		if _, err := toml.DecodeFile(pyprojectPath, &config); err != nil {
			return nil
		}

		if config.Tool.Poetry.Name == "" {
			return nil
		}

		project := createPythonProject(path, pyprojectPath)
		if project != nil {
			projects = append(projects, *project)
		}

		return filepath.SkipDir // Don't recurse into subdirectories
	})

	return projects
}

// detectRootPythonWorkspace checks if root has a multi-project pyproject.toml.
func detectRootPythonWorkspace(root string) *Project {
	pyprojectPath := filepath.Join(root, "pyproject.toml")
	if _, err := os.Stat(pyprojectPath); err != nil {
		return nil
	}

	// This is a simplified check - full multi-project detection would require
	// more complex parsing. For now, if root has pyproject.toml, treat as single project.
	return createPythonProject(root, pyprojectPath)
}

// createPythonProject creates a Project from a Python project directory.
func createPythonProject(projectDir, pyprojectPath string) *Project {
	// Read pyproject.toml to get project name
	var config struct {
		Project struct {
			Name string `toml:"name"`
		} `toml:"project"`
		Tool struct {
			Poetry struct {
				Name string `toml:"name"`
			} `toml:"poetry"`
		} `toml:"tool"`
	}

	if _, err := toml.DecodeFile(pyprojectPath, &config); err != nil {
		return nil
	}

	name := config.Project.Name
	if name == "" {
		name = config.Tool.Poetry.Name
	}
	if name == "" {
		name = filepath.Base(projectDir)
	}

	// Detect package manager and lockfile
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

// detectPythonPM detects the package manager and lockfile for a Python project.
func detectPythonPM(projectPath string) (pm.ID, string) {
	// Check for lockfiles
	lockfiles := map[string]pm.ID{
		"poetry.lock": pm.Poetry,
		"Pipfile.lock": pm.Pipenv,
		"requirements.lock": pm.Pip,
	}

	for lockfile, pmID := range lockfiles {
		lockPath := filepath.Join(projectPath, lockfile)
		if _, err := os.Stat(lockPath); err == nil {
			return pmID, lockPath
		}
	}

	// Check for Pipfile (Pipenv)
	pipfilePath := filepath.Join(projectPath, "Pipfile")
	if _, err := os.Stat(pipfilePath); err == nil {
		return pm.Pipenv, ""
	}

	// Check for pyproject.toml with Poetry section
	pyprojectPath := filepath.Join(projectPath, "pyproject.toml")
	if data, err := os.ReadFile(pyprojectPath); err == nil {
		content := string(data)
		if strings.Contains(content, "[tool.poetry]") {
			return pm.Poetry, ""
		}
	}

	// Default to pip
	return pm.Pip, ""
}

// isProjectInList checks if a project path is already in the list.
func isProjectInList(projects []Project, path string) bool {
	for _, p := range projects {
		if p.Path == path {
			return true
		}
	}
	return false
}

