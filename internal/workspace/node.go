package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectNodeWorkspace detects npm/yarn/bun workspaces (package.json
// "workspaces", as an array or as {"packages": [...]}) and pnpm workspaces
// (pnpm-workspace.yaml "packages"). Every member needs its own package.json.
func DetectNodeWorkspace(root string) (*Workspace, error) {
	patterns, rootPM, err := nodeWorkspacePatterns(root)
	if err != nil || len(patterns) == 0 {
		return nil, err
	}
	lockfile := nodeLockfile(root, rootPM)
	var projects []Project
	for _, dir := range expandGlobs(root, patterns) {
		manifest := filepath.Join(dir, "package.json")
		data, err := os.ReadFile(manifest)
		if err != nil {
			continue
		}
		var pkg struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &pkg) != nil {
			continue
		}
		name := pkg.Name
		if name == "" {
			name = filepath.Base(dir)
		}
		projects = append(projects, Project{
			Name:      name,
			Path:      dir,
			Ecosystem: "node",
			Manifest:  manifest,
			Lockfile:  lockfile,
			PM:        rootPM,
		})
	}
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "node", RootPM: rootPM}, nil
}

// nodeWorkspacePatterns returns the member globs and the package manager
// that owns the workspace root. pnpm-workspace.yaml wins over package.json.
func nodeWorkspacePatterns(root string) ([]string, pm.ID, error) {
	if data, err := os.ReadFile(filepath.Join(root, "pnpm-workspace.yaml")); err == nil {
		var cfg struct {
			Packages []string `yaml:"packages"`
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, "", fmt.Errorf("pnpm-workspace.yaml: %w", err)
		}
		return cfg.Packages, pm.Pnpm, nil
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, "", nil
	}
	var pkg struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, "", fmt.Errorf("package.json: %w", err)
	}
	if len(pkg.Workspaces) == 0 {
		return nil, "", nil
	}
	var patterns []string
	if json.Unmarshal(pkg.Workspaces, &patterns) != nil {
		var obj struct {
			Packages []string `json:"packages"`
		}
		if err := json.Unmarshal(pkg.Workspaces, &obj); err != nil {
			return nil, "", fmt.Errorf("package.json workspaces: %w", err)
		}
		patterns = obj.Packages
	}
	return patterns, rootNodePM(root), nil
}

// rootNodePM picks the root's package manager from its lock files in pm's
// fixed order (package-lock.json, yarn.lock, pnpm-lock.yaml, bun.lock,
// bun.lockb); npm when there is none.
func rootNodePM(root string) pm.ID {
	if ids := pm.DetectLockFilesForEcosystem(root, pm.EcosystemNode); len(ids) > 0 {
		return ids[0]
	}
	return pm.Npm
}

// nodeLockfile returns the root lock file written by id, or "".
func nodeLockfile(root string, id pm.ID) string {
	names := map[pm.ID][]string{
		pm.Npm:  {"package-lock.json"},
		pm.Yarn: {"yarn.lock"},
		pm.Pnpm: {"pnpm-lock.yaml"},
		pm.Bun:  {"bun.lock", "bun.lockb"},
	}[id]
	for _, n := range names {
		if p := filepath.Join(root, n); isFile(p) {
			return p
		}
	}
	return ""
}
