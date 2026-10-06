package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// CargoExtractor extracts dependencies from Rust Cargo lockfiles.
type CargoExtractor struct{}

func (e *CargoExtractor) Name() string {
	return "rust"
}

func (e *CargoExtractor) Supports(file string) bool {
	return file == "Cargo.lock"
}

func (e *CargoExtractor) Extract(dir string, _ ExtractOptions) (*DepGraph, error) {
	data, err := os.ReadFile(filepath.Join(dir, "Cargo.lock"))
	if err != nil {
		return nil, err
	}
	return parseCargoLock(data)
}

type cargoPackage struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Source       string   `toml:"source"`
	Dependencies []string `toml:"dependencies"`
}

// parseCargoLock builds the graph of a Cargo.lock (v1–v4). Nodes are keyed by
// name+version, so two versions of one crate stay distinct. A dependency entry
// is "name" (the only locked version), "name version", or
// "name version (source)". Roots are the packages without a source: the
// workspace members.
func parseCargoLock(data []byte) (*DepGraph, error) {
	var lock struct {
		Package []cargoPackage `toml:"package"`
	}
	if _, err := toml.Decode(string(data), &lock); err != nil {
		return nil, fmt.Errorf("parse Cargo.lock: %w", err)
	}
	g := NewGraph()
	byName := map[string][]string{} // name -> versions
	for _, p := range lock.Package {
		if p.Name == "" || p.Version == "" {
			return nil, fmt.Errorf("parse Cargo.lock: [[package]] without name or version")
		}
		n := NewDepNode("rust", p.Name, p.Version)
		if g.GetNode(n.ID) == nil {
			if p.Source != "" {
				n.WithMetadata("source", p.Source)
			}
			g.AddNode(n)
			byName[p.Name] = append(byName[p.Name], p.Version)
		}
		if p.Source == "" {
			g.AddRoot(n.ID)
		}
	}
	for _, p := range lock.Package {
		from := NodeID("rust", p.Name, p.Version)
		for _, spec := range p.Dependencies {
			f := strings.Fields(spec)
			if len(f) == 0 {
				return nil, fmt.Errorf("parse Cargo.lock: empty dependency in %s %s", p.Name, p.Version)
			}
			version := ""
			if len(f) >= 2 {
				version = f[1]
			} else if vs := byName[f[0]]; len(vs) == 1 {
				version = vs[0]
			}
			to := NodeID("rust", f[0], version)
			if version == "" || g.GetNode(to) == nil {
				return nil, fmt.Errorf("parse Cargo.lock: %s %s depends on %q, which is not locked", p.Name, p.Version, spec)
			}
			g.AddEdge(NewEdge(from, to))
		}
	}
	return g, nil
}
