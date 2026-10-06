// Package cache provides a global cross-language dependency cache system.
//
// This package enables UPM to store and reuse artifacts (tarballs, wheels,
// crates, zips, jars, modules) across different package managers and installs.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Ecosystem represents a package ecosystem.
type Ecosystem string

// Supported ecosystems.
const (
	EcosystemNode   Ecosystem = "node"
	EcosystemPython Ecosystem = "python"
	EcosystemPHP    Ecosystem = "php"
	EcosystemRust   Ecosystem = "rust"
	EcosystemGo     Ecosystem = "go"
	EcosystemJava   Ecosystem = "java"
)

// AllEcosystems returns all supported ecosystems.
func AllEcosystems() []Ecosystem {
	return []Ecosystem{
		EcosystemNode,
		EcosystemPython,
		EcosystemPHP,
		EcosystemRust,
		EcosystemGo,
		EcosystemJava,
	}
}

// CacheObject holds metadata about a cached artifact.
type CacheObject struct {
	// Ecosystem identifies the package ecosystem (node, python, etc.).
	Ecosystem Ecosystem `json:"ecosystem"`

	// Name is the package name.
	Name string `json:"name"`

	// Version is the package version.
	Version string `json:"version"`

	// Hash is the SHA256 hash of the artifact file.
	Hash string `json:"hash"`

	// Created is when the artifact was cached.
	Created time.Time `json:"created"`

	// Source is where the artifact was downloaded from.
	Source string `json:"source"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// Filename is the original filename of the artifact.
	Filename string `json:"filename"`

	// ArtifactPath is the full path to the cached artifact.
	ArtifactPath string `json:"artifactPath"`
}

// NewCacheObject creates a new CacheObject from a file.
func NewCacheObject(ecosystem Ecosystem, name, version, source, filePath string) (*CacheObject, error) {
	// Get file info
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Compute hash
	hash, err := ComputeFileHash(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash: %w", err)
	}

	return &CacheObject{
		Ecosystem:    ecosystem,
		Name:         name,
		Version:      version,
		Hash:         hash,
		Created:      time.Now().UTC(),
		Source:       source,
		Size:         info.Size(),
		Filename:     filepath.Base(filePath),
		ArtifactPath: filePath,
	}, nil
}

// MetadataKey returns the unique key for this object's metadata file.
func (o *CacheObject) MetadataKey() string {
	return fmt.Sprintf("%s-%s-%s", o.Ecosystem, sanitizeName(o.Name), o.Version)
}

// MetadataFilename returns the metadata filename.
func (o *CacheObject) MetadataFilename() string {
	return o.MetadataKey() + ".json"
}

// ArtifactFilename returns the cached artifact filename with hash.
func (o *CacheObject) ArtifactFilename() string {
	ext := filepath.Ext(o.Filename)
	return fmt.Sprintf("%s%s", o.Hash[:16], ext)
}

// SaveMetadata writes the metadata to a JSON file.
func (o *CacheObject) SaveMetadata(metadataDir string) error {
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	path := filepath.Join(metadataDir, o.MetadataFilename())
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// LoadMetadata reads metadata from a JSON file.
func LoadMetadata(path string) (*CacheObject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var obj CacheObject
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &obj, nil
}

// LoadMetadataByKey loads metadata by ecosystem, name, and version.
func LoadMetadataByKey(metadataDir string, ecosystem Ecosystem, name, version string) (*CacheObject, error) {
	key := fmt.Sprintf("%s-%s-%s.json", ecosystem, sanitizeName(name), version)
	path := filepath.Join(metadataDir, key)
	return LoadMetadata(path)
}

// ComputeFileHash computes the SHA256 hash of a file.
func ComputeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// sanitizeName replaces characters that aren't safe for filenames.
// Validates input length and handles empty strings.
// Maximum length is 255 characters (typical filesystem limit).
//
// Edge cases:
//   - Empty string: returns "unnamed"
//   - Very long names (>255 chars): truncates to 255 characters
//   - Names with only dangerous chars: returns "unnamed"
//   - Scoped packages (@scope/pkg): converts @ and / to dashes
//
// Dangerous characters replaced: /, \, :, *, ?, ", <, >, |, @
func sanitizeName(name string) string {
	// Handle empty string
	if len(name) == 0 {
		return "unnamed"
	}

	// Limit length to prevent filesystem issues (255 is typical max filename length)
	const maxLength = 255
	if len(name) > maxLength {
		name = name[:maxLength]
	}

	// Replace / with -- for scoped packages like @scope/package
	result := ""
	for _, c := range name {
		switch c {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '@':
			result += "-"
		default:
			result += string(c)
		}
	}

	// Ensure result is not empty after sanitization
	if len(result) == 0 {
		return "unnamed"
	}

	return result
}

// Age returns how old the cached object is.
func (o *CacheObject) Age() time.Duration {
	return time.Since(o.Created)
}

// AgeDays returns the age in days.
func (o *CacheObject) AgeDays() int {
	return int(o.Age().Hours() / 24)
}

// FormatSize returns a human-readable size string.
func (o *CacheObject) FormatSize() string {
	return FormatBytes(o.Size)
}

// FormatBytes formats bytes as a human-readable string.
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// GetExtension returns the typical file extension for an ecosystem.
func GetExtension(ecosystem Ecosystem) string {
	switch ecosystem {
	case EcosystemNode:
		return ".tgz"
	case EcosystemPython:
		return ".whl"
	case EcosystemPHP:
		return ".zip"
	case EcosystemRust:
		return ".crate"
	case EcosystemGo:
		return ".zip"
	case EcosystemJava:
		return ".jar"
	default:
		return ""
	}
}
