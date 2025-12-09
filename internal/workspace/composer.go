package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectComposerWorkspace detects PHP Composer multi-package layouts.
func DetectComposerWorkspace(root string) (*Workspace, error) {
	var projects []Project

	// Strategy 1: Check root composer.json for repositories with local paths
	rootComposerPath := filepath.Join(root, "composer.json")
	if _, err := os.Stat(rootComposerPath); err == nil {
		if localProjects := detectComposerRepositories(root, rootComposerPath); len(localProjects) > 0 {
			projects = append(projects, localProjects...)
		}
	}

	// Strategy 2: Scan for common patterns
	patterns := []string{
		"packages/*/composer.json",
		"modules/*/composer.json",
		"src/*/composer.json",
	}

	for _, pattern := range patterns {
		globPattern := filepath.Join(root, pattern)
		matches, err := filepath.Glob(globPattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			projectDir := filepath.Dir(match)
			if isComposerProjectInList(projects, projectDir) {
				continue
			}

			project := createComposerProject(projectDir, match)
			if project != nil {
				projects = append(projects, *project)
			}
		}
	}

	if len(projects) == 0 {
		return nil, nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "php",
	}, nil
}

// detectComposerRepositories detects local repositories in composer.json.
func detectComposerRepositories(root, composerPath string) []Project {
	var projects []Project

	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil
	}

	var config struct {
		Repositories []struct {
			Type string                 `json:"type"`
			URL  string                 `json:"url"`
			Path string                 `json:"path"`
			Options map[string]interface{} `json:"options"`
		} `json:"repositories"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}

	for _, repo := range config.Repositories {
		// Check for path-based repositories
		var repoPath string
		if repo.Path != "" {
			repoPath = repo.Path
		} else if repo.Type == "path" && repo.URL != "" {
			repoPath = repo.URL
		} else if repo.Options != nil {
			if path, ok := repo.Options["path"].(string); ok {
				repoPath = path
			}
		}

		if repoPath == "" {
			continue
		}

		// Resolve path relative to root
		projectPath := filepath.Join(root, repoPath)
		if !filepath.IsAbs(repoPath) {
			projectPath = filepath.Join(root, repoPath)
		}

		composerJSONPath := filepath.Join(projectPath, "composer.json")
		if _, err := os.Stat(composerJSONPath); err != nil {
			continue
		}

		project := createComposerProject(projectPath, composerJSONPath)
		if project != nil {
			projects = append(projects, *project)
		}
	}

	return projects
}

// createComposerProject creates a Project from a Composer project directory.
func createComposerProject(projectDir, composerJSONPath string) *Project {
	// Read composer.json to get package name
	data, err := os.ReadFile(composerJSONPath)
	if err != nil {
		return nil
	}

	var config struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}

	name := config.Name
	if name == "" {
		name = filepath.Base(projectDir)
	}

	// Check for composer.lock
	composerLockPath := filepath.Join(projectDir, "composer.lock")
	lockfile := ""
	if _, err := os.Stat(composerLockPath); err == nil {
		lockfile = composerLockPath
	}

	return &Project{
		Name:      name,
		Path:      projectDir,
		Ecosystem: "php",
		Manifest:  composerJSONPath,
		Lockfile:  lockfile,
		PM:        pm.Composer,
	}
}

// isComposerProjectInList checks if a project path is already in the list.
func isComposerProjectInList(projects []Project, path string) bool {
	for _, p := range projects {
		if p.Path == path {
			return true
		}
	}
	return false
}

