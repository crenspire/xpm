package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Store adds an artifact to the cache.
func (m *Manager) Store(ecosystem Ecosystem, name, version, source, filePath string) (*CacheObject, error) {
	if !m.IsEnabled() {
		return nil, fmt.Errorf("cache is disabled")
	}

	// Create cache object with metadata
	obj, err := NewCacheObject(ecosystem, name, version, source, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache object: %w", err)
	}

	// Create package directory
	pkgDir := m.PackagePath(ecosystem, name, version)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create package directory: %w", err)
	}

	// Copy artifact to cache
	destPath := m.ArtifactPath(obj)
	if err := copyFile(filePath, destPath); err != nil {
		return nil, fmt.Errorf("failed to copy artifact: %w", err)
	}

	// Update artifact path in object
	obj.ArtifactPath = destPath

	// Save metadata
	if err := obj.SaveMetadata(m.MetadataPath()); err != nil {
		// Clean up copied file on metadata failure
		os.Remove(destPath)
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	return obj, nil
}

// Get retrieves a cached artifact path by ecosystem, name, and version.
func (m *Manager) Get(ecosystem Ecosystem, name, version string) (string, error) {
	if !m.IsEnabled() {
		return "", fmt.Errorf("cache is disabled")
	}

	// Load metadata to get artifact filename
	obj, err := LoadMetadataByKey(m.MetadataPath(), ecosystem, name, version)
	if err != nil {
		return "", fmt.Errorf("artifact not in cache: %w", err)
	}

	// Verify artifact still exists
	if _, err := os.Stat(obj.ArtifactPath); err != nil {
		return "", fmt.Errorf("cached artifact missing: %w", err)
	}

	return obj.ArtifactPath, nil
}

// Has checks if an artifact is in the cache.
func (m *Manager) Has(ecosystem Ecosystem, name, version string) bool {
	if !m.IsEnabled() {
		return false
	}

	_, err := m.Get(ecosystem, name, version)
	return err == nil
}

// Delete removes an artifact from the cache.
func (m *Manager) Delete(ecosystem Ecosystem, name, version string) error {
	if !m.IsEnabled() {
		return fmt.Errorf("cache is disabled")
	}

	// Load metadata
	obj, err := LoadMetadataByKey(m.MetadataPath(), ecosystem, name, version)
	if err != nil {
		return fmt.Errorf("artifact not in cache: %w", err)
	}

	// Remove artifact file
	if err := os.Remove(obj.ArtifactPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove artifact: %w", err)
	}

	// Remove metadata file
	metadataPath := filepath.Join(m.MetadataPath(), obj.MetadataFilename())
	if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove metadata: %w", err)
	}

	// Try to remove empty parent directories
	pkgDir := m.PackagePath(ecosystem, name, version)
	os.Remove(pkgDir) // Remove version dir if empty
	os.Remove(filepath.Dir(pkgDir)) // Remove package dir if empty

	return nil
}

// List returns all cached packages for an ecosystem.
func (m *Manager) List(ecosystem Ecosystem) ([]*CacheObject, error) {
	if !m.IsEnabled() {
		return nil, fmt.Errorf("cache is disabled")
	}

	var objects []*CacheObject

	metadataDir := m.MetadataPath()
	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return objects, nil
		}
		return nil, fmt.Errorf("failed to read metadata directory: %w", err)
	}

	prefix := string(ecosystem) + "-"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !hasPrefix(name, prefix) || filepath.Ext(name) != ".json" {
			continue
		}

		obj, err := LoadMetadata(filepath.Join(metadataDir, name))
		if err != nil {
			continue // Skip invalid metadata
		}

		if obj.Ecosystem == ecosystem {
			objects = append(objects, obj)
		}
	}

	// Sort by name and version
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].Name != objects[j].Name {
			return objects[i].Name < objects[j].Name
		}
		return objects[i].Version < objects[j].Version
	})

	return objects, nil
}

// ListAll returns all cached packages across all ecosystems.
func (m *Manager) ListAll() ([]*CacheObject, error) {
	if !m.IsEnabled() {
		return nil, fmt.Errorf("cache is disabled")
	}

	var objects []*CacheObject

	metadataDir := m.MetadataPath()
	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return objects, nil
		}
		return nil, fmt.Errorf("failed to read metadata directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		obj, err := LoadMetadata(filepath.Join(metadataDir, entry.Name()))
		if err != nil {
			continue // Skip invalid metadata
		}

		objects = append(objects, obj)
	}

	// Sort by ecosystem, name, and version
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].Ecosystem != objects[j].Ecosystem {
			return objects[i].Ecosystem < objects[j].Ecosystem
		}
		if objects[i].Name != objects[j].Name {
			return objects[i].Name < objects[j].Name
		}
		return objects[i].Version < objects[j].Version
	})

	return objects, nil
}

// GetObject retrieves the full cache object metadata.
func (m *Manager) GetObject(ecosystem Ecosystem, name, version string) (*CacheObject, error) {
	if !m.IsEnabled() {
		return nil, fmt.Errorf("cache is disabled")
	}

	return LoadMetadataByKey(m.MetadataPath(), ecosystem, name, version)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// hasPrefix checks if a string has a prefix (for filename matching).
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

