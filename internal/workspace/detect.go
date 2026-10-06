package workspace

import (
	"fmt"
	"path/filepath"
	"sort"
)

// detectors run in this order, which is also the order of the returned
// workspaces: node, python, rust, go, java, php.
var detectors = []func(root string) (*Workspace, error){
	DetectNodeWorkspace,
	DetectPythonWorkspace,
	DetectCargoWorkspace,
	DetectGoWorkspace,
	DetectJavaWorkspace,
	DetectComposerWorkspace,
}

// DetectWorkspaces detects the workspaces rooted at root: at most one per
// ecosystem, in a fixed ecosystem order, each with its projects sorted by
// path relative to root and de-duplicated. A detector that fails (for
// example on a malformed manifest) contributes nothing.
func DetectWorkspaces(root string) ([]Workspace, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("invalid root path: %w", err)
	}
	var out []Workspace
	for _, detect := range detectors {
		ws, err := detect(absRoot)
		if err != nil || ws == nil {
			continue
		}
		finalize(ws)
		if len(ws.Projects) > 0 {
			out = append(out, *ws)
		}
	}
	return out, nil
}

// finalize sorts a workspace's projects by relative slash path and drops
// repeated project directories (first one wins).
func finalize(ws *Workspace) {
	sort.SliceStable(ws.Projects, func(i, j int) bool {
		return relSlash(ws.Root, ws.Projects[i].Path) < relSlash(ws.Root, ws.Projects[j].Path)
	})
	seen := map[string]bool{}
	kept := ws.Projects[:0]
	for _, p := range ws.Projects {
		if seen[p.Path] {
			continue
		}
		seen[p.Path] = true
		kept = append(kept, p)
	}
	ws.Projects = kept
}
