// Package graph provides a unified dependency graph system for multi-ecosystem projects.
//
// This package extracts dependencies from various package manager ecosystems,
// normalizes them into a unified structure, and provides multiple export formats.
package graph

import (
	"fmt"
	"strings"
)

// DepNode represents a dependency node in the graph.
type DepNode struct {
	// ID is the unique identifier: ecosystem:name@version
	ID string

	// Name is the package name.
	Name string

	// Version is the package version.
	Version string

	// Ecosystem is the ecosystem identifier (node, python, php, rust, go, java).
	Ecosystem string

	// Metadata contains additional information (resolved URL, integrity, etc.).
	Metadata map[string]string
}

// NewDepNode creates a new dependency node with a normalized ID.
func NewDepNode(ecosystem, name, version string) *DepNode {
	return &DepNode{
		ID:        NodeID(ecosystem, name, version),
		Name:      name,
		Version:   version,
		Ecosystem: ecosystem,
		Metadata:  make(map[string]string),
	}
}

// NodeID generates a unique node ID from ecosystem, name, and version.
// Format: ecosystem:name@version
func NodeID(ecosystem, name, version string) string {
	// Normalize name (handle scoped packages like @scope/package)
	normalizedName := strings.ReplaceAll(name, "/", "-")
	return fmt.Sprintf("%s:%s@%s", ecosystem, normalizedName, version)
}

// String returns a human-readable representation of the node.
func (n *DepNode) String() string {
	if n.Version != "" {
		return fmt.Sprintf("%s@%s (%s)", n.Name, n.Version, n.Ecosystem)
	}
	return fmt.Sprintf("%s (%s)", n.Name, n.Ecosystem)
}

// ShortString returns a shorter representation without ecosystem.
func (n *DepNode) ShortString() string {
	if n.Version != "" {
		return fmt.Sprintf("%s@%s", n.Name, n.Version)
	}
	return n.Name
}

// WithMetadata adds metadata to the node.
func (n *DepNode) WithMetadata(key, value string) *DepNode {
	if n.Metadata == nil {
		n.Metadata = make(map[string]string)
	}
	n.Metadata[key] = value
	return n
}

// GetMetadata retrieves metadata by key.
func (n *DepNode) GetMetadata(key string) string {
	if n.Metadata == nil {
		return ""
	}
	return n.Metadata[key]
}

