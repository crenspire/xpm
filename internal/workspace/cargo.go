package workspace

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectCargoWorkspace detects a Cargo workspace: [workspace] members (globs)
// minus [workspace] exclude (paths or globs), plus the root package when the
// root Cargo.toml also has a [package].
func DetectCargoWorkspace(root string) (*Workspace, error) {
	rootManifest := filepath.Join(root, "Cargo.toml")
	if !isFile(rootManifest) {
		return nil, nil
	}
	var cfg struct {
		Package struct {
			Name string `toml:"name"`
		} `toml:"package"`
		Workspace struct {
			Members []string `toml:"members"`
			Exclude []string `toml:"exclude"`
		} `toml:"workspace"`
	}
	if _, err := toml.DecodeFile(rootManifest, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Workspace.Members) == 0 {
		return nil, nil
	}
	lockfile := ""
	if p := filepath.Join(root, "Cargo.lock"); isFile(p) {
		lockfile = p
	}
	patterns := append([]string{}, cfg.Workspace.Members...)
	for _, ex := range cfg.Workspace.Exclude {
		patterns = append(patterns, "!"+ex)
	}
	var projects []Project
	if cfg.Package.Name != "" {
		projects = append(projects, Project{Name: cfg.Package.Name, Path: root, Ecosystem: "rust",
			Manifest: rootManifest, Lockfile: lockfile, PM: pm.Cargo})
	}
	for _, dir := range expandGlobs(root, patterns) {
		manifest := filepath.Join(dir, "Cargo.toml")
		var pkg struct {
			Package struct {
				Name string `toml:"name"`
			} `toml:"package"`
		}
		if _, err := toml.DecodeFile(manifest, &pkg); err != nil {
			continue
		}
		name := pkg.Package.Name
		if name == "" {
			name = filepath.Base(dir)
		}
		projects = append(projects, Project{Name: name, Path: dir, Ecosystem: "rust",
			Manifest: manifest, Lockfile: lockfile, PM: pm.Cargo})
	}
	if len(projects) == 0 {
		return nil, nil
	}
	return &Workspace{Root: root, Projects: projects, Ecosystem: "rust", RootPM: pm.Cargo}, nil
}
