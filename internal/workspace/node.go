package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/pm"
	"gopkg.in/yaml.v3"
)

// DetectNodeWorkspace detects Node.js workspaces (npm/yarn/pnpm/bun).
func DetectNodeWorkspace(root string) (*Workspace, error) {
	// Check for pnpm-workspace.yaml first
	if ws := detectPnpmWorkspace(root); ws != nil {
		return ws, nil
	}

	// Check for package.json with workspaces field
	packageJSONPath := filepath.Join(root, "package.json")
	if _, err := os.Stat(packageJSONPath); err != nil {
		return nil, nil // Not a Node workspace
	}

	var pkgJSON struct {
		Workspaces interface{} `json:"workspaces"`
	}

	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, &pkgJSON); err != nil {
		return nil, err
	}

	if pkgJSON.Workspaces == nil {
		return nil, nil // No workspaces field
	}

	// Parse workspaces field (can be array or object with packages array)
	var workspacePatterns []string

	switch v := pkgJSON.Workspaces.(type) {
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok {
				workspacePatterns = append(workspacePatterns, str)
			}
		}
	case map[string]interface{}:
		if packages, ok := v["packages"].([]interface{}); ok {
			for _, item := range packages {
				if str, ok := item.(string); ok {
					workspacePatterns = append(workspacePatterns, str)
				}
			}
		}
	}

	if len(workspacePatterns) == 0 {
		return nil, nil
	}

	// Resolve glob patterns
	var projects []Project
	for _, pattern := range workspacePatterns {
		// Convert to absolute glob pattern
		globPattern := filepath.Join(root, pattern)
		matches, err := filepath.Glob(globPattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			// Check if it's a directory with package.json
			info, err := os.Stat(match)
			if err != nil || !info.IsDir() {
				continue
			}

			pkgPath := filepath.Join(match, "package.json")
			if _, err := os.Stat(pkgPath); err != nil {
				continue
			}

			// Read package.json to get name
			pkgData, err := os.ReadFile(pkgPath)
			if err != nil {
				continue
			}

			var pkg struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(pkgData, &pkg); err != nil {
				continue
			}

			name := pkg.Name
			if name == "" {
				name = filepath.Base(match)
			}

			// Detect package manager and lockfile
			pmID, lockfile := detectNodePM(match)

			projects = append(projects, Project{
				Name:      name,
				Path:      match,
				Ecosystem: "node",
				Manifest:  pkgPath,
				Lockfile:  lockfile,
				PM:        pmID,
			})
		}
	}

	if len(projects) == 0 {
		return nil, nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "node",
	}, nil
}

// detectPnpmWorkspace detects pnpm workspaces via pnpm-workspace.yaml.
func detectPnpmWorkspace(root string) *Workspace {
	workspaceYAML := filepath.Join(root, "pnpm-workspace.yaml")
	if _, err := os.Stat(workspaceYAML); err != nil {
		return nil
	}

	data, err := os.ReadFile(workspaceYAML)
	if err != nil {
		return nil
	}

	var config struct {
		Packages []string `yaml:"packages"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil
	}

	if len(config.Packages) == 0 {
		return nil
	}

	var projects []Project
	for _, pattern := range config.Packages {
		globPattern := filepath.Join(root, pattern)
		matches, err := filepath.Glob(globPattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || !info.IsDir() {
				continue
			}

			pkgPath := filepath.Join(match, "package.json")
			if _, err := os.Stat(pkgPath); err != nil {
				continue
			}

			pkgData, err := os.ReadFile(pkgPath)
			if err != nil {
				continue
			}

			var pkg struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(pkgData, &pkg); err != nil {
				continue
			}

			name := pkg.Name
			if name == "" {
				name = filepath.Base(match)
			}

			_, lockfile := detectNodePM(match)

			projects = append(projects, Project{
				Name:      name,
				Path:      match,
				Ecosystem: "node",
				Manifest:  pkgPath,
				Lockfile:  lockfile,
				PM:        pm.Pnpm, // pnpm workspace
			})
		}
	}

	if len(projects) == 0 {
		return nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "node",
	}
}

// detectNodePM detects the package manager and lockfile for a Node project.
func detectNodePM(projectPath string) (pm.ID, string) {
	// Check for lockfiles in order of preference
	lockfiles := map[string]pm.ID{
		"pnpm-lock.yaml": pm.Pnpm,
		"yarn.lock":      pm.Yarn,
		"package-lock.json": pm.Npm,
		"bun.lockb":      pm.Bun,
	}

	for lockfile, pmID := range lockfiles {
		lockPath := filepath.Join(projectPath, lockfile)
		if _, err := os.Stat(lockPath); err == nil {
			return pmID, lockPath
		}
	}

	// Check root for lockfiles
	root := filepath.Dir(projectPath)
	for lockfile, pmID := range lockfiles {
		lockPath := filepath.Join(root, lockfile)
		if _, err := os.Stat(lockPath); err == nil {
			return pmID, lockPath
		}
	}

	// Default to npm if no lockfile found
	return pm.Npm, ""
}

