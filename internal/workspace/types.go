// Package workspace provides workspace/monorepo detection and management.
package workspace

import (
	"github.com/crenspire/xpm/internal/pm"
)

// Workspace represents a detected workspace/monorepo of one ecosystem.
type Workspace struct {
	Root      string
	Projects  []Project
	Ecosystem string // "node", "python", "rust", "go", "java", "php"
	// RootPM is set when the workspace is installed once, at Root, by this
	// package manager (npm/yarn/pnpm/bun workspaces, Cargo workspaces, Maven
	// reactors). Empty means each project is installed in its own directory.
	RootPM pm.ID
}

// Project represents a single project within a workspace.
type Project struct {
	Name      string
	Path      string
	Ecosystem string
	Manifest  string // path to manifest file
	Lockfile  string // path to lockfile (if exists)
	PM        pm.ID  // detected package manager
}
