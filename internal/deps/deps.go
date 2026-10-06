// Package deps turns a dependency graph into the package lists that
// outdated and audit check, and compares versions.
package deps

import (
	"sort"
	"strings"

	"github.com/crenspire/xpm/internal/graph"
	"github.com/crenspire/xpm/internal/pm"
)

// Dep is one package at one version in one ecosystem.
type Dep struct {
	Ecosystem string // graph ecosystem: node, python, php, rust, go, java
	Name      string
	Version   string // "" when the project files pin no exact version
}

// Direct returns the direct dependencies of g's projects: children of g.Root
// that are not themselves projects (Metadata["project"] == "true" or roots),
// de-duplicated by (ecosystem, name, version), sorted by ecosystem, name, version.
func Direct(g *graph.DepGraph) []Dep {
	if g == nil {
		return nil
	}
	var ids []string
	for _, r := range g.Root {
		ids = append(ids, g.Children(r)...)
	}
	return collect(g, ids)
}

// All returns every non-project node of g, de-duplicated and sorted the same way.
func All(g *graph.DepGraph) []Dep {
	if g == nil {
		return nil
	}
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	return collect(g, ids)
}

// collect turns node IDs into sorted, de-duplicated Deps, skipping unknown
// IDs, project nodes and roots.
func collect(g *graph.DepGraph, ids []string) []Dep {
	isRoot := make(map[string]bool, len(g.Root))
	for _, r := range g.Root {
		isRoot[r] = true
	}
	seen := make(map[Dep]bool)
	var out []Dep
	for _, id := range ids {
		n := g.Nodes[id]
		if n == nil || isRoot[id] || n.Metadata["project"] == "true" {
			continue
		}
		d := Dep{Ecosystem: n.Ecosystem, Name: n.Name, Version: n.Version}
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})
	return out
}

// Manager maps a graph ecosystem to the registry that serves it:
// node→pm.Npm, python→pm.Pip, php→pm.Composer, rust→pm.Cargo, java→pm.Maven,
// go→pm.GoMod; ok=false otherwise.
func Manager(ecosystem string) (pm.ID, bool) {
	switch ecosystem {
	case "node":
		return pm.Npm, true
	case "python":
		return pm.Pip, true
	case "php":
		return pm.Composer, true
	case "rust":
		return pm.Cargo, true
	case "java":
		return pm.Maven, true
	case "go":
		return pm.GoMod, true
	}
	return "", false
}

// OSVEcosystem maps a graph ecosystem to its OSV.dev name:
// node→"npm", python→"PyPI", php→"Packagist", rust→"crates.io", go→"Go",
// java→"Maven"; ok=false otherwise.
func OSVEcosystem(ecosystem string) (string, bool) {
	switch ecosystem {
	case "node":
		return "npm", true
	case "python":
		return "PyPI", true
	case "php":
		return "Packagist", true
	case "rust":
		return "crates.io", true
	case "go":
		return "Go", true
	case "java":
		return "Maven", true
	}
	return "", false
}

// pinnedDenied lists substrings that mark a version as a range, a
// placeholder or a non-registry reference.
var pinnedDenied = []string{
	"${", "*", "^", "~", ">", "<", "=", " ", "||",
	"workspace:", "file:", "link:", "git+", "git:",
}

// Pinned reports whether v is a concrete version usable for lookups:
// non-empty, contains a digit, and contains none of "${", "*", "^", "~",
// ">", "<", "=", " ", "||", "workspace:", "file:", "link:", "git+", "git:",
// and that neither starts with "git" nor has a dot-separated field that is
// exactly "x" or "X" (split on "." and "-"; 1.x, 1.2.x, x.1).
func Pinned(v string) bool {
	if v == "" || !strings.ContainsAny(v, "0123456789") {
		return false
	}
	if strings.HasPrefix(v, "git") {
		return false
	}
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' }) {
		if f == "x" || f == "X" {
			return false
		}
	}
	for _, s := range pinnedDenied {
		if strings.Contains(v, s) {
			return false
		}
	}
	return true
}
