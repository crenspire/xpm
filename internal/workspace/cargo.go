package workspace

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/crenspire/xpm/internal/pm"
)

// DetectCargoWorkspace detects Rust Cargo workspaces.
func DetectCargoWorkspace(root string) (*Workspace, error) {
	cargoTomlPath := filepath.Join(root, "Cargo.toml")
	if _, err := os.Stat(cargoTomlPath); err != nil {
		return nil, nil // Not a Cargo workspace
	}

	var config struct {
		Workspace struct {
			Members []string `toml:"members"`
		} `toml:"workspace"`
	}

	if _, err := toml.DecodeFile(cargoTomlPath, &config); err != nil {
		return nil, err
	}

	if len(config.Workspace.Members) == 0 {
		return nil, nil // No workspace members
	}

	var projects []Project
	cargoLockPath := filepath.Join(root, "Cargo.lock")

	for _, member := range config.Workspace.Members {
		// Resolve member path (can be glob pattern)
		memberPattern := filepath.Join(root, member)
		matches, err := filepath.Glob(memberPattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || !info.IsDir() {
				continue
			}

			// Check for Cargo.toml in member directory
			memberCargoToml := filepath.Join(match, "Cargo.toml")
			if _, err := os.Stat(memberCargoToml); err != nil {
				continue
			}

			// Read Cargo.toml to get package name
			var pkgConfig struct {
				Package struct {
					Name string `toml:"name"`
				} `toml:"package"`
			}

			if _, err := toml.DecodeFile(memberCargoToml, &pkgConfig); err != nil {
				continue
			}

			name := pkgConfig.Package.Name
			if name == "" {
				name = filepath.Base(match)
			}

			projects = append(projects, Project{
				Name:      name,
				Path:      match,
				Ecosystem: "rust",
				Manifest:  memberCargoToml,
				Lockfile:  cargoLockPath, // Cargo workspaces share a single lockfile
				PM:        pm.Cargo,
			})
		}
	}

	if len(projects) == 0 {
		return nil, nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "rust",
	}, nil
}

