package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// DetectWorkspaces detects all workspaces in the given root directory.
func DetectWorkspaces(root string) ([]Workspace, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("invalid root path: %w", err)
	}

	workspaces := detectAllEcosystems(absRoot)
	if len(workspaces) == 0 {
		return nil, nil
	}

	// Validate and filter workspaces
	var validWorkspaces []Workspace
	for _, ws := range workspaces {
		if err := validateWorkspace(ws); err != nil {
			// Log warning but continue
			continue
		}
		validWorkspaces = append(validWorkspaces, ws)
	}

	return mergeWorkspaces(validWorkspaces), nil
}

// detectAllEcosystems runs all ecosystem detectors.
func detectAllEcosystems(root string) []Workspace {
	var workspaces []Workspace

	// Node.js
	if ws, err := DetectNodeWorkspace(root); err == nil && ws != nil && len(ws.Projects) > 0 {
		workspaces = append(workspaces, *ws)
	}

	// Python
	if ws, err := DetectPythonWorkspace(root); err == nil && ws != nil && len(ws.Projects) > 0 {
		workspaces = append(workspaces, *ws)
	}

	// Rust
	if ws, err := DetectCargoWorkspace(root); err == nil && ws != nil && len(ws.Projects) > 0 {
		workspaces = append(workspaces, *ws)
	}

	// Go
	if ws, err := DetectGoWorkspace(root); err == nil && ws != nil && len(ws.Projects) > 0 {
		workspaces = append(workspaces, *ws)
	}

	// Java
	if ws, err := DetectJavaWorkspace(root); err == nil && ws != nil && len(ws.Projects) > 0 {
		workspaces = append(workspaces, *ws)
	}

	// PHP
	if ws, err := DetectComposerWorkspace(root); err == nil && ws != nil && len(ws.Projects) > 0 {
		workspaces = append(workspaces, *ws)
	}

	return workspaces
}

// mergeWorkspaces combines workspaces from different ecosystems.
// If multiple ecosystems are detected, creates a "mixed" workspace.
func mergeWorkspaces(workspaces []Workspace) []Workspace {
	if len(workspaces) == 0 {
		return nil
	}

	if len(workspaces) == 1 {
		return workspaces
	}

	// Multiple ecosystems detected - merge into a single "mixed" workspace
	merged := Workspace{
		Root:      workspaces[0].Root,
		Ecosystem: "mixed",
		Projects:  []Project{},
	}

	for _, ws := range workspaces {
		merged.Projects = append(merged.Projects, ws.Projects...)
	}

	return []Workspace{merged}
}

// validateWorkspace checks for circular or invalid workspace definitions.
func validateWorkspace(w Workspace) error {
	if len(w.Projects) == 0 {
		return fmt.Errorf("workspace has no projects")
	}

	// Check for duplicate project paths
	seen := make(map[string]bool)
	for _, p := range w.Projects {
		if seen[p.Path] {
			return fmt.Errorf("duplicate project path: %s", p.Path)
		}
		seen[p.Path] = true

		// Validate project path exists
		if _, err := os.Stat(p.Path); err != nil {
			return fmt.Errorf("project path does not exist: %s", p.Path)
		}
	}

	return nil
}

