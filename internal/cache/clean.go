package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// GCResult holds the results of a garbage collection run.
type GCResult struct {
	RemovedByAge       int
	RemovedByVersion   int
	OrphanedMetadata   int
	FreedBytes         int64
}

// Clean removes all cached artifacts.
func (m *Manager) Clean() error {
	if !m.IsEnabled() {
		return fmt.Errorf("cache is disabled")
	}

	// Remove cache directory
	if err := os.RemoveAll(m.basePath); err != nil {
		return fmt.Errorf("failed to remove cache directory: %w", err)
	}

	// Recreate empty directories
	return m.EnsureDirs()
}

// GC performs garbage collection based on configuration.
func (m *Manager) GC(maxAgeDays, maxVersions int) (*GCResult, error) {
	if !m.IsEnabled() {
		return nil, fmt.Errorf("cache is disabled")
	}

	result := &GCResult{}

	// Remove artifacts older than maxAgeDays
	if maxAgeDays > 0 {
		removed, freed, err := m.RemoveOldArtifacts(maxAgeDays)
		if err != nil {
			return result, err
		}
		result.RemovedByAge = removed
		result.FreedBytes += freed
	}

	// Prune old versions
	if maxVersions > 0 {
		removed, freed, err := m.PruneVersions(maxVersions)
		if err != nil {
			return result, err
		}
		result.RemovedByVersion = removed
		result.FreedBytes += freed
	}

	// Remove orphaned metadata
	orphaned, err := m.RemoveOrphanedMetadata()
	if err != nil {
		return result, err
	}
	result.OrphanedMetadata = orphaned

	return result, nil
}

// RemoveOldArtifacts removes artifacts older than maxAgeDays.
func (m *Manager) RemoveOldArtifacts(maxAgeDays int) (removed int, freed int64, err error) {
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)

	objects, err := m.ListAll()
	if err != nil {
		return 0, 0, err
	}

	for _, obj := range objects {
		if obj.Created.Before(cutoff) {
			// Remove artifact
			if err := m.Delete(obj.Ecosystem, obj.Name, obj.Version); err != nil {
				continue // Skip failed removals
			}
			removed++
			freed += obj.Size
		}
	}

	return removed, freed, nil
}

// PruneVersions keeps only the latest N versions of each package.
func (m *Manager) PruneVersions(maxVersions int) (removed int, freed int64, err error) {
	objects, err := m.ListAll()
	if err != nil {
		return 0, 0, err
	}

	// Group by ecosystem + package name
	type pkgKey struct {
		ecosystem Ecosystem
		name      string
	}
	byPackage := make(map[pkgKey][]*CacheObject)

	for _, obj := range objects {
		key := pkgKey{obj.Ecosystem, obj.Name}
		byPackage[key] = append(byPackage[key], obj)
	}

	// For each package, keep only the latest N versions
	for _, pkgObjects := range byPackage {
		if len(pkgObjects) <= maxVersions {
			continue
		}

		// Sort by created time (newest first)
		sort.Slice(pkgObjects, func(i, j int) bool {
			return pkgObjects[i].Created.After(pkgObjects[j].Created)
		})

		// Remove old versions
		for _, obj := range pkgObjects[maxVersions:] {
			if err := m.Delete(obj.Ecosystem, obj.Name, obj.Version); err != nil {
				continue
			}
			removed++
			freed += obj.Size
		}
	}

	return removed, freed, nil
}

// RemoveOrphanedMetadata removes metadata files without corresponding artifacts.
func (m *Manager) RemoveOrphanedMetadata() (removed int, err error) {
	metadataDir := m.MetadataPath()

	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(metadataDir, entry.Name())
		obj, err := LoadMetadata(path)
		if err != nil {
			// Invalid metadata, remove it
			os.Remove(path)
			removed++
			continue
		}

		// Check if artifact exists
		if _, err := os.Stat(obj.ArtifactPath); os.IsNotExist(err) {
			// Artifact missing, remove metadata
			os.Remove(path)
			removed++
		}
	}

	return removed, nil
}

// RemoveEmptyDirs removes empty directories in the cache.
func (m *Manager) RemoveEmptyDirs() error {
	return filepath.Walk(m.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		// Skip base path and metadata
		if path == m.basePath || path == m.MetadataPath() {
			return nil
		}

		// Try to remove (will fail if not empty)
		os.Remove(path)
		return nil
	})
}

// PrintGCResult prints the garbage collection results.
func PrintGCResult(result *GCResult) {
	fmt.Println("Garbage collection:")
	fmt.Println()

	if result.RemovedByAge > 0 {
		fmt.Printf("  - Removed %d artifacts by age\n", result.RemovedByAge)
	}

	if result.RemovedByVersion > 0 {
		fmt.Printf("  - Pruned %d old versions\n", result.RemovedByVersion)
	}

	if result.OrphanedMetadata > 0 {
		fmt.Printf("  - Cleaned %d orphaned metadata files\n", result.OrphanedMetadata)
	}

	total := result.RemovedByAge + result.RemovedByVersion + result.OrphanedMetadata
	if total == 0 {
		fmt.Println("  No garbage to collect.")
	} else {
		fmt.Println()
		fmt.Printf("  Freed: %s\n", FormatBytes(result.FreedBytes))
	}
}

// DeleteByEcosystem removes all cached artifacts for an ecosystem.
func (m *Manager) DeleteByEcosystem(ecosystem Ecosystem) (int, error) {
	if !m.IsEnabled() {
		return 0, fmt.Errorf("cache is disabled")
	}

	objects, err := m.List(ecosystem)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, obj := range objects {
		if err := m.Delete(obj.Ecosystem, obj.Name, obj.Version); err != nil {
			continue
		}
		removed++
	}

	// Remove ecosystem directory if empty
	ecoPath := m.EcosystemPath(ecosystem)
	os.Remove(ecoPath)

	return removed, nil
}

// Verify checks cache integrity.
func (m *Manager) Verify() (valid, invalid int, err error) {
	if !m.IsEnabled() {
		return 0, 0, nil
	}

	objects, err := m.ListAll()
	if err != nil {
		return 0, 0, err
	}

	for _, obj := range objects {
		// Check artifact exists
		if _, err := os.Stat(obj.ArtifactPath); os.IsNotExist(err) {
			invalid++
			continue
		}

		// Verify hash
		actualHash, err := ComputeFileHash(obj.ArtifactPath)
		if err != nil {
			invalid++
			continue
		}

		if actualHash != obj.Hash {
			invalid++
			continue
		}

		valid++
	}

	return valid, invalid, nil
}

// Repair fixes cache integrity issues.
func (m *Manager) Repair() (repaired int, err error) {
	if !m.IsEnabled() {
		return 0, fmt.Errorf("cache is disabled")
	}

	objects, err := m.ListAll()
	if err != nil {
		return 0, err
	}

	for _, obj := range objects {
		needsRepair := false

		// Check artifact exists
		if _, err := os.Stat(obj.ArtifactPath); os.IsNotExist(err) {
			needsRepair = true
		} else {
			// Verify hash
			actualHash, err := ComputeFileHash(obj.ArtifactPath)
			if err != nil || actualHash != obj.Hash {
				needsRepair = true
			}
		}

		if needsRepair {
			// Remove invalid entry
			m.Delete(obj.Ecosystem, obj.Name, obj.Version)
			repaired++
		}
	}

	// Remove orphaned metadata
	orphaned, _ := m.RemoveOrphanedMetadata()
	repaired += orphaned

	// Remove empty directories
	m.RemoveEmptyDirs()

	return repaired, nil
}

