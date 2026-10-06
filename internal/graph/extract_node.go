package graph

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// NodeExtractor extracts dependencies from Node.js lockfiles.
type NodeExtractor struct{}

func (e *NodeExtractor) Name() string {
	return "node"
}

func (e *NodeExtractor) Supports(file string) bool {
	return file == "package-lock.json" || file == "yarn.lock" ||
		file == "pnpm-lock.yaml" || file == "bun.lock" || file == "bun.lockb"
}

// Extract parses the first lockfile found, in the order package-lock.json,
// pnpm-lock.yaml, yarn.lock. Bun lockfiles are detected but not parsed.
func (e *NodeExtractor) Extract(dir string, opts ExtractOptions) (*DepGraph, error) {
	manifest, err := readOptional(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	locks := []struct {
		file  string
		parse func(data, manifest []byte, fallback string) (*DepGraph, error)
	}{
		{"package-lock.json", parseNpmLock},
		{"pnpm-lock.yaml", func(data, manifest []byte, fallback string) (*DepGraph, error) {
			return parsePnpmLock(data, manifest, fallback, opts.warn)
		}},
		{"yarn.lock", parseYarnLock},
	}
	for _, l := range locks {
		data, err := os.ReadFile(filepath.Join(dir, l.file))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return l.parse(data, manifest, dirName(dir))
	}
	for _, bun := range []string{"bun.lock", "bun.lockb"} {
		if _, err := os.Stat(filepath.Join(dir, bun)); err == nil {
			return nil, fmt.Errorf("%s is not supported yet", bun)
		}
	}
	return nil, fmt.Errorf("no package-lock.json, pnpm-lock.yaml or yarn.lock found")
}
