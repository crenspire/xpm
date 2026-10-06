package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectComposerWorkspace detects PHP packages: "path" repositories in the
// root composer.json (their url may be a glob, as Composer allows) and
// composer.json projects in packages/*, modules/*, src/*.
func DetectComposerWorkspace(root string) (*Workspace, error) {
	var projects []Project
	add := func(dir string) {
		if p := createComposerProject(dir, filepath.Join(dir, "composer.json")); p != nil {
			projects = append(projects, *p)
		}
	}
	for _, dir := range composerPathRepos(root) {
		add(dir)
	}
	for _, dir := range expandGlobs(root, []string{"packages/*", "modules/*", "src/*"}) {
		add(dir)
	}
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "php"}, nil
}

// composerPathRepos returns the directories named by the root composer.json's
// {"type": "path", "url": ...} repositories.
func composerPathRepos(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return nil
	}
	var cfg struct {
		Repositories json.RawMessage `json:"repositories"`
	}
	if json.Unmarshal(data, &cfg) != nil || len(cfg.Repositories) == 0 {
		return nil
	}
	type repo struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	// "repositories" is a list or an object keyed by name.
	var list []repo
	if json.Unmarshal(cfg.Repositories, &list) != nil {
		var byName map[string]repo
		if json.Unmarshal(cfg.Repositories, &byName) != nil {
			return nil
		}
		for _, r := range byName {
			list = append(list, r)
		}
	}
	var dirs []string
	for _, r := range list {
		if r.Type != "path" || r.URL == "" {
			continue
		}
		if filepath.IsAbs(r.URL) {
			if isDir(r.URL) {
				dirs = append(dirs, filepath.Clean(r.URL))
			}
			continue
		}
		dirs = append(dirs, expandGlobs(root, []string{r.URL})...)
	}
	return dirs
}

// createComposerProject creates a Project from a composer.json, or returns
// nil when there is none or it does not parse.
func createComposerProject(projectDir, composerJSONPath string) *Project {
	data, err := os.ReadFile(composerJSONPath)
	if err != nil {
		return nil
	}
	var cfg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	name := cfg.Name
	if name == "" {
		name = filepath.Base(projectDir)
	}
	lockfile := ""
	if p := filepath.Join(projectDir, "composer.lock"); isFile(p) {
		lockfile = p
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
