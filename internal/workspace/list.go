package workspace

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// FormatWorkspaces formats workspaces for CLI output.
func FormatWorkspaces(workspaces []Workspace) string {
	if len(workspaces) == 0 {
		return "No workspaces detected."
	}

	var output strings.Builder
	output.WriteString("Detected Workspaces\n")
	output.WriteString(strings.Repeat("─", 30) + "\n")

	// Group by ecosystem
	grouped := GroupByEcosystem(workspaces)

	// Sort ecosystems for consistent output
	ecosystems := make([]string, 0, len(grouped))
	for eco := range grouped {
		ecosystems = append(ecosystems, eco)
	}
	sort.Strings(ecosystems)

	for _, eco := range ecosystems {
		wsList := grouped[eco]
		output.WriteString(fmt.Sprintf("\n%s:\n", eco))
		for _, ws := range wsList {
			for _, project := range ws.Projects {
				relPath, _ := filepath.Rel(ws.Root, project.Path)
				if relPath == "." {
					relPath = filepath.Base(project.Path)
				}
				manifestName := filepath.Base(project.Manifest)
				output.WriteString(fmt.Sprintf("  %-30s → %s\n", relPath, manifestName))
			}
		}
	}

	return output.String()
}

// GroupByEcosystem groups workspaces by ecosystem.
func GroupByEcosystem(workspaces []Workspace) map[string][]Workspace {
	grouped := make(map[string][]Workspace)

	for _, ws := range workspaces {
		eco := ws.Ecosystem
		if eco == "" {
			eco = "unknown"
		}
		grouped[eco] = append(grouped[eco], ws)
	}

	return grouped
}

