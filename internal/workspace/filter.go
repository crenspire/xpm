package workspace

// Filter keeps the projects whose path relative to their workspace root
// (slash-separated, "." for the root itself) matches an include pattern and
// no exclude pattern. Patterns use path.Match syntax per segment plus "**"
// for any number of segments. An empty include list keeps every project;
// exclude wins over include. Workspaces left without projects are dropped.
// The input is not modified.
func Filter(workspaces []Workspace, include, exclude []string) []Workspace {
	if len(include) == 0 && len(exclude) == 0 {
		return workspaces
	}
	var out []Workspace
	for _, ws := range workspaces {
		var kept []Project
		for _, p := range ws.Projects {
			rel := relSlash(ws.Root, p.Path)
			if (len(include) == 0 || anyMatch(include, rel)) && !anyMatch(exclude, rel) {
				kept = append(kept, p)
			}
		}
		if len(kept) > 0 {
			ws.Projects = kept
			out = append(out, ws)
		}
	}
	return out
}

func anyMatch(patterns []string, rel string) bool {
	for _, p := range patterns {
		if matchPath(p, rel) {
			return true
		}
	}
	return false
}
