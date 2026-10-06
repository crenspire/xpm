package graph

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// addProject adds the project's own node (ecosystem:name@version) and makes
// it a root of g; parsers then add an edge from it to every direct
// dependency. An empty name falls back to fallback (the directory name).
func addProject(g *DepGraph, eco, name, version, fallback string) string {
	if name == "" {
		name = fallback
	}
	n := NewDepNode(eco, name, version)
	n.WithMetadata("project", "true")
	if g.GetNode(n.ID) == nil {
		g.AddNode(n)
	}
	g.AddRoot(n.ID)
	return n.ID
}

// dirName is the base name of dir, made absolute first so "." names the
// real folder.
func dirName(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Base(dir)
}

// readOptional reads path; a missing file is not an error and yields nil.
func readOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
