package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EcosystemStats holds statistics for an ecosystem.
type EcosystemStats struct {
	Ecosystem    Ecosystem
	PackageCount int
	TotalSize    int64
}

// CacheStats holds overall cache statistics.
type CacheStats struct {
	TotalSize    int64
	TotalPackages int
	ByEcosystem  map[Ecosystem]*EcosystemStats
}

// TreeNode represents a node in the cache tree.
type TreeNode struct {
	Name     string
	IsDir    bool
	Size     int64
	Children []*TreeNode
}

// GetTotalSize calculates the total size of the cache.
func (m *Manager) GetTotalSize() (int64, error) {
	if !m.IsEnabled() {
		return 0, nil
	}

	var total int64

	err := filepath.Walk(m.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !info.IsDir() {
			total += info.Size()
		}

		return nil
	})

	return total, err
}

// GetStats calculates detailed cache statistics.
func (m *Manager) GetStats() (*CacheStats, error) {
	if !m.IsEnabled() {
		return &CacheStats{
			ByEcosystem: make(map[Ecosystem]*EcosystemStats),
		}, nil
	}

	stats := &CacheStats{
		ByEcosystem: make(map[Ecosystem]*EcosystemStats),
	}

	// Initialize stats for all ecosystems
	for _, eco := range AllEcosystems() {
		stats.ByEcosystem[eco] = &EcosystemStats{
			Ecosystem: eco,
		}
	}

	// Calculate size per ecosystem
	for _, eco := range AllEcosystems() {
		ecoPath := m.EcosystemPath(eco)
		if _, err := os.Stat(ecoPath); os.IsNotExist(err) {
			continue
		}

		var ecoSize int64
		filepath.Walk(ecoPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			ecoSize += info.Size()
			return nil
		})

		stats.ByEcosystem[eco].TotalSize = ecoSize
		stats.TotalSize += ecoSize
	}

	// Count packages per ecosystem
	objects, err := m.ListAll()
	if err != nil {
		return stats, err
	}

	for _, obj := range objects {
		if s, ok := stats.ByEcosystem[obj.Ecosystem]; ok {
			s.PackageCount++
		}
		stats.TotalPackages++
	}

	return stats, nil
}

// GetTree builds a tree structure of the cache.
func (m *Manager) GetTree() (*TreeNode, error) {
	if !m.IsEnabled() {
		return &TreeNode{Name: "cache", IsDir: true}, nil
	}

	root := &TreeNode{
		Name:  "cache",
		IsDir: true,
	}

	for _, eco := range AllEcosystems() {
		ecoPath := m.EcosystemPath(eco)
		if _, err := os.Stat(ecoPath); os.IsNotExist(err) {
			continue
		}

		ecoNode := &TreeNode{
			Name:  string(eco),
			IsDir: true,
		}

		// List packages in this ecosystem
		objects, _ := m.List(eco)

		// Group by package name
		byPackage := make(map[string][]*CacheObject)
		for _, obj := range objects {
			byPackage[obj.Name] = append(byPackage[obj.Name], obj)
		}

		// Sort package names
		var names []string
		for name := range byPackage {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			pkgObjs := byPackage[name]
			pkgNode := &TreeNode{
				Name:  name,
				IsDir: true,
			}

			for _, obj := range pkgObjs {
				artifactNode := &TreeNode{
					Name: fmt.Sprintf("%s/%s (%s)", obj.Version, obj.ArtifactFilename(), FormatBytes(obj.Size)),
					Size: obj.Size,
				}
				pkgNode.Children = append(pkgNode.Children, artifactNode)
				pkgNode.Size += obj.Size
			}

			ecoNode.Children = append(ecoNode.Children, pkgNode)
			ecoNode.Size += pkgNode.Size
		}

		if len(ecoNode.Children) > 0 {
			root.Children = append(root.Children, ecoNode)
			root.Size += ecoNode.Size
		}
	}

	return root, nil
}

// PrintTree prints the cache tree to stdout.
func PrintTree(node *TreeNode, indent string, isLast bool) {
	// Print current node
	prefix := "├── "
	if isLast {
		prefix = "└── "
	}

	if indent == "" {
		// Root node
		fmt.Printf("Cache Tree (%s total)\n\n", FormatBytes(node.Size))
	} else {
		fmt.Printf("%s%s%s\n", indent, prefix, node.Name)
	}

	// Update indent for children
	childIndent := indent
	if indent != "" {
		if isLast {
			childIndent += "    "
		} else {
			childIndent += "│   "
		}
	}

	// Print children
	for i, child := range node.Children {
		isChildLast := i == len(node.Children)-1
		if child.IsDir {
			PrintTree(child, childIndent, isChildLast)
		} else {
			childPrefix := "├── "
			if isChildLast {
				childPrefix = "└── "
			}
			fmt.Printf("%s%s%s\n", childIndent, childPrefix, child.Name)
		}
	}
}

// PrintStats prints cache statistics to stdout.
func PrintStats(stats *CacheStats) {
	fmt.Println("Cache Statistics:")
	fmt.Println()

	// Sort ecosystems by size (largest first)
	type ecoStat struct {
		eco  Ecosystem
		stat *EcosystemStats
	}
	var sorted []ecoStat
	for eco, stat := range stats.ByEcosystem {
		if stat.PackageCount > 0 || stat.TotalSize > 0 {
			sorted = append(sorted, ecoStat{eco, stat})
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].stat.TotalSize > sorted[j].stat.TotalSize
	})

	// Print ecosystem stats
	for _, es := range sorted {
		fmt.Printf("  %-10s %10s (%d packages)\n",
			es.eco+":",
			FormatBytes(es.stat.TotalSize),
			es.stat.PackageCount,
		)
	}

	if len(sorted) == 0 {
		fmt.Println("  (empty)")
	}

	fmt.Println()
	fmt.Printf("  Total:     %10s (%d packages)\n",
		FormatBytes(stats.TotalSize),
		stats.TotalPackages,
	)
}

// PrintSize prints just the total size.
func (m *Manager) PrintSize() error {
	size, err := m.GetTotalSize()
	if err != nil {
		return err
	}

	fmt.Printf("Total cache size: %s\n", FormatBytes(size))
	return nil
}

// CountPackages returns the total number of cached packages.
func (m *Manager) CountPackages() (int, error) {
	objects, err := m.ListAll()
	if err != nil {
		return 0, err
	}
	return len(objects), nil
}

// GetOldestObject returns the oldest cached object.
func (m *Manager) GetOldestObject() (*CacheObject, error) {
	objects, err := m.ListAll()
	if err != nil {
		return nil, err
	}

	if len(objects) == 0 {
		return nil, nil
	}

	oldest := objects[0]
	for _, obj := range objects[1:] {
		if obj.Created.Before(oldest.Created) {
			oldest = obj
		}
	}

	return oldest, nil
}

// GetNewestObject returns the newest cached object.
func (m *Manager) GetNewestObject() (*CacheObject, error) {
	objects, err := m.ListAll()
	if err != nil {
		return nil, err
	}

	if len(objects) == 0 {
		return nil, nil
	}

	newest := objects[0]
	for _, obj := range objects[1:] {
		if obj.Created.After(newest.Created) {
			newest = obj
		}
	}

	return newest, nil
}

// FormatTree returns the tree as a string instead of printing.
func FormatTree(node *TreeNode) string {
	var sb strings.Builder
	formatTreeNode(&sb, node, "", true)
	return sb.String()
}

func formatTreeNode(sb *strings.Builder, node *TreeNode, indent string, isRoot bool) {
	if isRoot {
		sb.WriteString(fmt.Sprintf("Cache Tree (%s total)\n\n", FormatBytes(node.Size)))
	} else {
		sb.WriteString(indent + node.Name + "\n")
	}

	for i, child := range node.Children {
		isLast := i == len(node.Children)-1
		childIndent := indent
		if !isRoot {
			if isLast {
				childIndent += "    "
			} else {
				childIndent += "│   "
			}
		}

		prefix := "├── "
		if isLast {
			prefix = "└── "
		}

		if child.IsDir {
			formatTreeNode(sb, child, childIndent+prefix[:0], false)
		} else {
			sb.WriteString(childIndent + prefix + child.Name + "\n")
		}
	}
}

