package graph

// Extractor is the interface for ecosystem-specific dependency extractors.
type Extractor interface {
	// Extract extracts dependencies from the given directory.
	Extract(dir string) (*DepGraph, error)

	// Supports checks if this extractor supports the given file.
	Supports(file string) bool

	// Name returns the name of this extractor.
	Name() string
}

// ExtractorResult holds the result of an extraction operation.
type ExtractorResult struct {
	Graph *DepGraph
	Error error
	Name  string
}

