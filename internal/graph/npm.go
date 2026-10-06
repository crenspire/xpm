package graph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// npmManifest is the subset of package.json the Node parsers need.
type npmManifest struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
}

// parseNpmManifest parses package.json bytes; nil or empty input yields an empty manifest.
func parseNpmManifest(data []byte) (npmManifest, error) {
	var m npmManifest
	if len(data) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("package.json: %w", err)
	}
	return m, nil
}

// npmLockfile covers package-lock.json / npm-shrinkwrap.json v1, v2 and v3.
type npmLockfile struct {
	Name            string                `json:"name"`
	Version         string                `json:"version"`
	LockfileVersion int                   `json:"lockfileVersion"`
	Packages        map[string]npmPackage `json:"packages"`
	Dependencies    map[string]npmV1Dep   `json:"dependencies"`
}

// npmPackage is one entry of the v2/v3 "packages" map.
type npmPackage struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Resolved             string            `json:"resolved"`
	Integrity            string            `json:"integrity"`
	Link                 bool              `json:"link"`
	Dev                  bool              `json:"dev"`
	Optional             bool              `json:"optional"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
}

// npmV1Dep is one entry of the v1 nested "dependencies" tree.
type npmV1Dep struct {
	Version      string              `json:"version"`
	Resolved     string              `json:"resolved"`
	Integrity    string              `json:"integrity"`
	Dev          bool                `json:"dev"`
	Optional     bool                `json:"optional"`
	Requires     map[string]string   `json:"requires"`
	Dependencies map[string]npmV1Dep `json:"dependencies"`
}

// parseNpmLock builds the graph of a package-lock.json (v1, v2 or v3).
// manifest is package.json; it is only consulted when the lockfile has no
// root entry (v1). The single root is the project (named by the lockfile,
// else package.json, else fallback), with edges to its direct dependencies
// and to any workspace packages.
func parseNpmLock(data, manifest []byte, fallback string) (*DepGraph, error) {
	var lock npmLockfile
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("package-lock.json: %w", err)
	}
	pkgs := lock.Packages
	if len(pkgs) == 0 && len(lock.Dependencies) > 0 {
		pkgs = map[string]npmPackage{}
		flattenNpmV1(lock.Dependencies, "", pkgs)
	}
	if _, ok := pkgs[""]; !ok {
		m, err := parseNpmManifest(manifest)
		if err != nil {
			return nil, err
		}
		if pkgs == nil {
			pkgs = map[string]npmPackage{}
		}
		name, version := lock.Name, lock.Version
		if name == "" {
			name, version = m.Name, m.Version
		}
		pkgs[""] = npmPackage{
			Name:                 name,
			Version:              version,
			Dependencies:         m.Dependencies,
			DevDependencies:      m.DevDependencies,
			OptionalDependencies: m.OptionalDependencies,
			PeerDependencies:     m.PeerDependencies,
		}
	}
	return buildNpmGraph(pkgs, fallback), nil
}

// flattenNpmV1 converts the v1 nested tree into v2-style node_modules paths.
func flattenNpmV1(deps map[string]npmV1Dep, parent string, out map[string]npmPackage) {
	for name, d := range deps {
		p := "node_modules/" + name
		if parent != "" {
			p = parent + "/node_modules/" + name
		}
		out[p] = npmPackage{
			Version:      d.Version,
			Resolved:     d.Resolved,
			Integrity:    d.Integrity,
			Dev:          d.Dev,
			Optional:     d.Optional,
			Dependencies: d.Requires,
		}
		flattenNpmV1(d.Dependencies, p, out)
	}
}

// npmPackageName is the package name at a "packages" path: the explicit name
// field, else everything after the last "node_modules/" (keeps "@scope/name"),
// else the last path element (workspace folders).
func npmPackageName(path string, p npmPackage) string {
	if p.Name != "" {
		return p.Name
	}
	if i := strings.LastIndex(path, "node_modules/"); i >= 0 {
		return path[i+len("node_modules/"):]
	}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// npmParentPath is the folder whose node_modules Node searches next.
func npmParentPath(path string) string {
	if i := strings.LastIndex(path, "/node_modules/"); i >= 0 {
		return path[:i]
	}
	return ""
}

// npmResolve finds the path a require of dep from the package at path loads,
// following Node's lookup (own node_modules, then each ancestor's, then the
// root's) and workspace links. ok is false when nothing is installed.
func npmResolve(pkgs map[string]npmPackage, from, dep string) (string, bool) {
	p := from
	for {
		cand := "node_modules/" + dep
		if p != "" {
			cand = p + "/node_modules/" + dep
		}
		if e, ok := pkgs[cand]; ok {
			if e.Link {
				if _, ok := pkgs[e.Resolved]; ok {
					return e.Resolved, true
				}
				return "", false
			}
			return cand, true
		}
		if p == "" {
			return "", false
		}
		p = npmParentPath(p)
	}
}

func buildNpmGraph(pkgs map[string]npmPackage, fallback string) *DepGraph {
	g := NewGraph()
	project := addProject(g, "node", pkgs[""].Name, pkgs[""].Version, fallback)
	paths := make([]string, 0, len(pkgs))
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	ids := map[string]string{} // path -> node ID
	for _, p := range paths {
		e := pkgs[p]
		if p == "" || e.Link {
			continue
		}
		name := npmPackageName(p, e)
		if name == "" || (e.Version == "" && strings.Contains(p, "node_modules/")) {
			continue
		}
		n := NewDepNode("node", name, e.Version)
		if g.GetNode(n.ID) == nil {
			if e.Resolved != "" {
				n.WithMetadata("resolved", e.Resolved)
			}
			if e.Integrity != "" {
				n.WithMetadata("integrity", e.Integrity)
			}
			g.AddNode(n)
		}
		ids[p] = n.ID
	}

	edgesFrom := func(from string, e npmPackage, includeDev bool) []string {
		deps := map[string]string{}
		sections := []map[string]string{e.PeerDependencies, e.Dependencies, e.OptionalDependencies}
		if includeDev {
			sections = append(sections, e.DevDependencies)
		}
		for _, sec := range sections {
			for name, rng := range sec {
				deps[name] = rng
			}
		}
		var out []string
		for _, kv := range sortedPairs(deps) {
			if target, ok := npmResolve(pkgs, from, kv[0]); ok {
				if id, ok := ids[target]; ok {
					out = append(out, id)
				}
			}
		}
		return out
	}

	for _, to := range edgesFrom("", pkgs[""], true) {
		g.AddEdge(NewEdge(project, to))
	}
	for _, p := range paths {
		id, ok := ids[p]
		if !ok {
			continue
		}
		workspace := !strings.Contains(p, "node_modules/")
		if workspace {
			g.AddEdge(NewEdge(project, id))
		}
		for _, to := range edgesFrom(p, pkgs[p], workspace) {
			g.AddEdge(NewEdge(id, to))
		}
	}
	return g
}

// sortedPairs returns the map's entries sorted by key.
func sortedPairs(m map[string]string) [][2]string {
	out := make([][2]string, 0, len(m))
	for k, v := range m {
		out = append(out, [2]string{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}
