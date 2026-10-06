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

// pnpmPackage is a packages/snapshots entry. Name and Version are set (in
// v5/v6 lockfiles) for git, tarball and file: packages, whose keys are not
// "name@version".
type pnpmPackage struct {
	Name                 string            `yaml:"name"`
	Version              string            `yaml:"version"`
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
// "/@scope/name/1.2.3_peer@1" (v5). The version follows the first "@" that
// is not the leading scope "@" (v6+; a name has no other "@", while a git URL
// version such as "git+ssh://git@host/x.git" may), or the last "/" (v5).
func pnpmSplitKey(key string, v5 bool) (name, version string, ok bool) {
	k := strings.TrimPrefix(key, "/")
	if v5 {
		if slash := strings.LastIndexByte(k, '/'); slash > 0 {
			name, version = k[:slash], pnpmVersion(k[slash+1:], true)
		}
	} else {
		k = pnpmVersion(k, false)
		if len(k) > 1 {
			if at := strings.IndexByte(k[1:], '@'); at >= 0 {
				name, version = k[:at+1], k[at+2:]
			}
		}
	}
	if name == "" || version == "" {
		return "", "", false
	}
	return name, version, true
}

// pnpmTarget resolves a dependency entry (name: value) to a node ID. value is
// a version ("18.2.0(react@18.2.0)"), an alias target ("string-width@4.2.3"
// in v9, "/string-width@4.2.3" in v6, "/string-width/4.2.3" in v5), a
// non-registry version (a git or tarball URL, "file:..."), which names the
// package as is, or a workspace link ("link:../core"), which is not a
// package. A value containing ":" is never an alias: "git@host:x.git" and
// URLs carry an "@" of their own.
func pnpmTarget(name, value string, v5 bool) (string, bool) {
	if value == "" || strings.HasPrefix(value, "link:") {
		return "", false
	}
	bare := pnpmVersion(value, false)
	alias := strings.HasPrefix(value, "/") ||
		(!v5 && !strings.Contains(bare, ":") && strings.IndexByte(bare, '@') > 0)
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
//
// In v5/v6 lockfiles git, tarball and file: packages are keyed by their
// source ("github.com/acme/lib/0a1b2c3d") and named by their name/version
// fields; such an entry without those fields is skipped and reported to
// warn (nil discards it), so one odd dependency does not lose the graph.
// Dependency values equal to such a key resolve to its node.
func parsePnpmLock(data, manifest []byte, fallback string, warn func(format string, a ...any)) (*DepGraph, error) {
	var lock pnpmLockfile
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("pnpm-lock.yaml: %w", err)
	}
	if lock.LockfileVersion == "" {
		return nil, fmt.Errorf("pnpm-lock.yaml: missing lockfileVersion")
	}
	v5 := strings.HasPrefix(lock.LockfileVersion, "5")
	legacy := v5 || strings.HasPrefix(lock.LockfileVersion, "6")
	g := NewGraph()

	// keyIDs maps each packages/snapshots key, with and without its leading
	// "/", to its node.
	keyIDs := map[string]string{}
	addNodes := func(m map[string]pnpmPackage) error {
		for _, key := range sortedKeys(m) {
			p := m[key]
			name, version, ok := pnpmSplitKey(key, v5)
			if legacy && p.Name != "" && p.Version != "" {
				name, version, ok = p.Name, p.Version, true
			}
			if !ok {
				if !legacy {
					return fmt.Errorf("pnpm-lock.yaml: cannot parse package key %q", key)
				}
				if warn != nil {
					warn("pnpm-lock.yaml: skipping package %q: no name and version", key)
				}
				continue
			}
			id := NodeID("node", name, version)
			keyIDs[key] = id
			keyIDs[strings.TrimPrefix(key, "/")] = id
			if g.GetNode(id) == nil {
				n := NewDepNode("node", name, version)
				if p.Resolution.Integrity != "" {
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
	target := func(name, value string) (string, bool) {
		if id, ok := keyIDs[value]; ok {
			return id, true
		}
		if id, ok := pnpmTarget(name, value, v5); ok && g.GetNode(id) != nil {
			return id, true
		}
		return "", false
	}
	for _, key := range sortedKeys(withDeps) {
		from, ok := keyIDs[key]
		if !ok {
			continue
		}
		p := withDeps[key]
		for _, sec := range []map[string]string{p.Dependencies, p.OptionalDependencies} {
			for _, kv := range sortedPairs(sec) {
				if to, ok := target(kv[0], kv[1]); ok {
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
				if id, ok := target(name, v); ok {
					g.AddEdge(NewEdge(from, id))
				}
			}
		}
	}
	return g, nil
}
