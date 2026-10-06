package graph

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// extractors lists every ecosystem extractor in the fixed order ExtractAll
// runs and merges them.
func extractors() []Extractor {
	return []Extractor{
		&NodeExtractor{},
		&PythonExtractor{},
		&ComposerExtractor{},
		&CargoExtractor{},
		&GoExtractor{},
		&JavaExtractor{},
	}
}

// ExtractAll runs the extractor of every ecosystem detected in dir, one after
// another in a fixed order, and merges their graphs. Parsing is fast and
// external tools run only with opts.Exec, so there is no goroutine fan-out:
// the merge order, the warnings and opts.Run calls stay deterministic.
//
// An extractor that fails is reported through opts.Warn when another one
// succeeded; when every detected extractor fails the joined errors are
// returned instead. No ecosystem detected yields an empty graph.
func ExtractAll(dir string, opts ExtractOptions) (*DepGraph, error) {
	detected := DetectEcosystems(dir)
	merged := NewGraph()
	var failures []error
	ok := 0
	for _, ext := range extractors() {
		if !detected[ext.Name()] {
			continue
		}
		g, err := ext.Extract(dir, opts)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", ext.Name(), err))
			continue
		}
		ok++
		merged.Merge(g)
	}
	if ok == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("extract dependencies: %w", errors.Join(failures...))
	}
	for _, err := range failures {
		opts.warn("%v", err)
	}
	return merged, nil
}

// ecosystemFiles maps the files whose presence enables an extractor.
// Manifests without a lockfile are listed too, so their extractor reports
// what is missing instead of the ecosystem being silently ignored.
var ecosystemFiles = []struct {
	file      string
	ecosystem string
}{
	{"package-lock.json", "node"},
	{"pnpm-lock.yaml", "node"},
	{"yarn.lock", "node"},
	{"bun.lock", "node"},
	{"bun.lockb", "node"},
	{"package.json", "node"},
	{"poetry.lock", "python"},
	{"pyproject.toml", "python"},
	{"requirements.txt", "python"},
	{"composer.lock", "php"},
	{"composer.json", "php"},
	{"Cargo.lock", "rust"},
	{"Cargo.toml", "rust"},
	{"go.mod", "go"},
	{"pom.xml", "java"},
	{"gradle.lockfile", "java"},
	{"build.gradle", "java"},
	{"build.gradle.kts", "java"},
	{"settings.gradle", "java"},
	{"settings.gradle.kts", "java"},
}

// DetectEcosystems reports which ecosystems have a manifest or lockfile in dir.
func DetectEcosystems(dir string) map[string]bool {
	detected := make(map[string]bool)
	for _, f := range ecosystemFiles {
		if _, err := os.Stat(filepath.Join(dir, f.file)); err == nil {
			detected[f.ecosystem] = true
		}
	}
	return detected
}
