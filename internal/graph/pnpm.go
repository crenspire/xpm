package graph

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// pnpmImporter is one project's direct dependencies (importers["."] or, in
// single-project v5/v6 lockfiles, the top level).
type pnpmImporter struct {
	Dependencies         map[string]pnpmImporterDep `yaml:"dependencies"`
	DevDependencies      map[string]pnpmImporterDep `yaml:"devDependencies"`
	OptionalDependencies map[string]pnpmImporterDep `yaml:"optionalDependencies"`
}

// pnpmImporterDep accepts both the v6+ {specifier, version} map and the
// v5 plain version string.
type pnpmImporterDep struct {
	Version string
}

func (d *pnpmImporterDep) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		d.Version = n.Value
		return nil
	}
	var v struct {
		Version string `yaml:"version"`
	}
	if err := n.Decode(&v); err != nil {
		return err
	}
	d.Version = v.Version
	return nil
}

type pnpmPackage struct {
	Dependencies         map[string]string `yaml:"dependencies"`
	OptionalDependencies map[string]string `yaml:"optionalDependencies"`
	Resolution           struct {
		Integrity string `yaml:"integrity"`
		Tarball   string `yaml:"tarball"`
	} `yaml:"resolution"`
}

type pnpmLockfile struct {
	LockfileVersion string                  `yaml:"lockfileVersion"`
	Importers       map[string]pnpmImporter `yaml:"importers"`
	pnpmImporter    `yaml:",inline"`
	Packages        map[string]pnpmPackage `yaml:"packages"`
	Snapshots       map[string]pnpmPackage `yaml:"snapshots"`
}

// pnpmVersion strips the peer suffix from a version: "18.2.0(react@18.2.0)"
// (v6+) or, in v5 lockfiles, "18.2.0_react@18.2.0".
func pnpmVersion(v string, v5 bool) string {
	if i := strings.IndexByte(v, '('); i >= 0 {
		v = v[:i]
	}
	if v5 {
		if i := strings.IndexByte(v, '_'); i >= 0 {
			v = v[:i]
		}
	}
	return v
}

// pnpmSplitKey turns a packages/snapshots key into name and version:
// "/@scope/name@1.2.3(peer@1)" (v6), "name@1.2.3(peer@1)" (v9),
// "/@scope/name/1.2.3_peer@1" (v5). The version follows the last "@" that is
// not the leading scope "@" (v6+), or the last "/" (v5).
func pnpmSplitKey(key string, v5 bool) (name, version string, ok bool) {
	k := strings.TrimPrefix(key, "/")
	if v5 {
		if slash := strings.LastIndexByte(k, '/'); slash > 0 {
			name, version = k[:slash], pnpmVersion(k[slash+1:], true)
		}
	} else {
		k = pnpmVersion(k, false)
		if at := strings.LastIndexByte(k, '@'); at > 0 {
			name, version = k[:at], k[at+1:]
		}
	}
	if name == "" || version == "" {
		return "", "", false
	}
	return name, version, true
}

// pnpmTarget resolves a dependency entry (name: value) to a node ID. value is
// a version ("18.2.0(react@18.2.0)"), an alias target ("string-width@4.2.3"
// in v9, "/string-width@4.2.3" in v6, "/string-width/4.2.3" in v5) or a
// workspace link ("link:../core"), which is not a package.
func pnpmTarget(name, value string, v5 bool) (string, bool) {
	if value == "" || strings.HasPrefix(value, "link:") || strings.HasPrefix(value, "file:") {
		return "", false
	}
	alias := strings.HasPrefix(value, "/") || (!v5 && strings.LastIndexByte(pnpmVersion(value, false), '@') > 0)
	if alias {
		n, ver, ok := pnpmSplitKey(value, v5)
		if !ok {
			return "", false
		}
		return NodeID("node", n, ver), true
	}
	return NodeID("node", name, pnpmVersion(value, v5)), true
}

// parsePnpmLock builds the graph of a pnpm-lock.yaml (v5, v6 or v9).
// manifest (package.json) names the project, else fallback. The project is
// the root and stands for importer "."; every other importer (workspace
// package) becomes a node named by its path, linked from the project.
func parsePnpmLock(data, manifest []byte, fallback string) (*DepGraph, error) {
	var lock pnpmLockfile
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("pnpm-lock.yaml: %w", err)
	}
	if lock.LockfileVersion == "" {
		return nil, fmt.Errorf("pnpm-lock.yaml: missing lockfileVersion")
	}
	v5 := strings.HasPrefix(lock.LockfileVersion, "5")
	g := NewGraph()

	addNodes := func(m map[string]pnpmPackage) error {
		for _, key := range sortedKeys(m) {
			name, version, ok := pnpmSplitKey(key, v5)
			if !ok {
				return fmt.Errorf("pnpm-lock.yaml: cannot parse package key %q", key)
			}
			if g.GetNode(NodeID("node", name, version)) == nil {
				n := NewDepNode("node", name, version)
				if p := m[key]; p.Resolution.Integrity != "" {
					n.WithMetadata("integrity", p.Resolution.Integrity)
				}
				g.AddNode(n)
			}
		}
		return nil
	}
	if err := addNodes(lock.Packages); err != nil {
		return nil, err
	}
	if err := addNodes(lock.Snapshots); err != nil {
		return nil, err
	}

	// v9 keeps dependencies in snapshots; v5/v6 in packages.
	withDeps := lock.Snapshots
	if len(withDeps) == 0 {
		withDeps = lock.Packages
	}
	for _, key := range sortedKeys(withDeps) {
		name, version, _ := pnpmSplitKey(key, v5)
		from := NodeID("node", name, version)
		p := withDeps[key]
		for _, sec := range []map[string]string{p.Dependencies, p.OptionalDependencies} {
			for _, kv := range sortedPairs(sec) {
				if to, ok := pnpmTarget(kv[0], kv[1], v5); ok && g.GetNode(to) != nil {
					g.AddEdge(NewEdge(from, to))
				}
			}
		}
	}

	m, err := parseNpmManifest(manifest)
	if err != nil {
		return nil, err
	}
	project := addProject(g, "node", m.Name, m.Version, fallback)
	importers := lock.Importers
	if len(importers) == 0 {
		importers = map[string]pnpmImporter{".": lock.pnpmImporter}
	}
	importerID := func(p string) string {
		if p == "." {
			return project
		}
		return NodeID("node", p, "")
	}
	for _, p := range sortedKeys(importers) {
		if p != "." {
			g.AddNode(NewDepNode("node", p, ""))
			g.AddEdge(NewEdge(project, importerID(p)))
		}
	}
	for _, p := range sortedKeys(importers) {
		imp := importers[p]
		from := importerID(p)
		for _, sec := range []map[string]pnpmImporterDep{imp.Dependencies, imp.DevDependencies, imp.OptionalDependencies} {
			for _, name := range sortedKeys(sec) {
				v := sec[name].Version
				if rel, ok := strings.CutPrefix(v, "link:"); ok {
					target := path.Clean(path.Join(p, rel))
					if _, ok := importers[target]; ok {
						g.AddEdge(NewEdge(from, importerID(target)))
					}
					continue
				}
				if id, ok := pnpmTarget(name, v, v5); ok && g.GetNode(id) != nil {
					g.AddEdge(NewEdge(from, id))
				}
			}
		}
	}
	return g, nil
}
