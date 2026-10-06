package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FormatWorkspaces formats workspaces for CLI output, in the order given
// (DetectWorkspaces returns them in a fixed ecosystem order).
func FormatWorkspaces(workspaces []Workspace) string {
	if len(workspaces) == 0 {
		return "No workspaces detected."
	}
	var b strings.Builder
	b.WriteString("Detected Workspaces\n")
	b.WriteString(strings.Repeat("─", 30) + "\n")
	for _, ws := range workspaces {
		eco := ws.Ecosystem
		if eco == "" {
			eco = "unknown"
		}
		fmt.Fprintf(&b, "\n%s:\n", eco)
		for _, p := range ws.Projects {
			rel := relSlash(ws.Root, p.Path)
			if rel == "." {
				rel = filepath.Base(p.Path)
			}
			fmt.Fprintf(&b, "  %-30s → %s\n", rel, filepath.Base(p.Manifest))
		}
	}
	return b.String()
}
