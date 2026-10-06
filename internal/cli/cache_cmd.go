package cli

import (
	"fmt"

	"github.com/crenspire/xpm/internal/cache"
	"github.com/crenspire/xpm/internal/config"
)

// cmdCache handles the cache command and its subcommands.
func cmdCache(args []string) int {
	if len(args) == 0 {
		printCacheUsage()
		return 0
	}

	subcommand := args[0]

	switch subcommand {
	case "tree":
		return cmdCacheTree()
	case "size":
		return cmdCacheSize()
	case "clean":
		return cmdCacheClean()
	case "gc":
		return cmdCacheGC()
	case "verify":
		return cmdCacheVerify()
	case "repair":
		return cmdCacheRepair()
	case "path":
		return cmdCachePath()
	case "help", "-h", "--help":
		printCacheUsage()
		return 0
	default:
		fmt.Printf("error: unknown cache subcommand: %s\n\n", subcommand)
		showCommandUsage("cache")
		return 1
	}
}

// printCacheUsage prints the cache command usage.
func printCacheUsage() {
	fmt.Println("Usage: xpm cache <subcommand>")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  tree      Show cache structure and contents")
	fmt.Println("  size      Show total cache size and statistics")
	fmt.Println("  clean     Remove all cached artifacts")
	fmt.Println("  gc        Run garbage collection")
	fmt.Println("  verify    Verify cache integrity")
	fmt.Println("  repair    Repair cache by removing invalid entries")
	fmt.Println("  path      Show cache directory path")
	fmt.Println()
	fmt.Println("Configuration (.xpmrc):")
	fmt.Println("  cache.enabled      Enable/disable caching (default: true)")
	fmt.Println("  cache.path         Cache directory path (default: ~/.xpm/cache)")
	fmt.Println("  cache.maxAgeDays   Max age for GC (default: 60)")
	fmt.Println("  cache.maxVersions  Max versions per package (default: 5)")
}

// getCacheManager creates a cache manager from config.
func getCacheManager() (*cache.Manager, error) {
	cfg := config.Load()

	cacheCfg := cache.Config{
		Enabled:     cfg.Cache.Enabled,
		Path:        cfg.Cache.Path,
		MaxAgeDays:  cfg.Cache.MaxAgeDays,
		MaxVersions: cfg.Cache.MaxVersions,
	}

	return cache.NewManager(cacheCfg)
}

// cmdCacheTree displays the cache tree structure.
func cmdCacheTree() int {
	mgr, err := getCacheManager()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	if !mgr.IsEnabled() {
		fmt.Println("Cache is disabled.")
		return 0
	}

	tree, err := mgr.GetTree()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	if len(tree.Children) == 0 {
		fmt.Println("Cache is empty.")
		return 0
	}

	cache.PrintTree(tree, "", true)
	return 0
}

// cmdCacheSize displays cache statistics.
func cmdCacheSize() int {
	mgr, err := getCacheManager()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	if !mgr.IsEnabled() {
		fmt.Println("Cache is disabled.")
		return 0
	}

	stats, err := mgr.GetStats()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	cache.PrintStats(stats)
	return 0
}

// cmdCacheClean removes all cached artifacts.
func cmdCacheClean() int {
	mgr, err := getCacheManager()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	if !mgr.IsEnabled() {
		fmt.Println("Cache is disabled.")
		return 0
	}

	// Get size before cleaning
	sizeBefore, _ := mgr.GetTotalSize()

	if err := mgr.Clean(); err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	fmt.Println("Cache cleared.")
	if sizeBefore > 0 {
		fmt.Printf("Freed: %s\n", cache.FormatBytes(sizeBefore))
	}
	return 0
}

// cmdCacheGC runs garbage collection.
func cmdCacheGC() int {
	cfg := config.Load()

	mgr, err := getCacheManager()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	if !mgr.IsEnabled() {
		fmt.Println("Cache is disabled.")
		return 0
	}

	result, err := mgr.GC(cfg.Cache.MaxAgeDays, cfg.Cache.MaxVersions)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	cache.PrintGCResult(result)
	return 0
}

// cmdCacheVerify checks cache integrity.
func cmdCacheVerify() int {
	mgr, err := getCacheManager()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	if !mgr.IsEnabled() {
		fmt.Println("Cache is disabled.")
		return 0
	}

	fmt.Println("Verifying cache integrity...")
	fmt.Println()

	valid, invalid, err := mgr.Verify()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	total := valid + invalid
	if total == 0 {
		fmt.Println("Cache is empty.")
		return 0
	}

	fmt.Printf("  Valid:   %d\n", valid)
	fmt.Printf("  Invalid: %d\n", invalid)
	fmt.Printf("  Total:   %d\n", total)
	fmt.Println()

	if invalid > 0 {
		fmt.Println("Run `xpm cache repair` to fix invalid entries.")
		return 1
	}

	fmt.Println("All entries are valid.")
	return 0
}

// cmdCacheRepair fixes cache integrity issues.
func cmdCacheRepair() int {
	mgr, err := getCacheManager()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	if !mgr.IsEnabled() {
		fmt.Println("Cache is disabled.")
		return 0
	}

	fmt.Println("Repairing cache...")

	repaired, err := mgr.Repair()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return 1
	}

	if repaired == 0 {
		fmt.Println("No issues found.")
	} else {
		fmt.Printf("Removed %d invalid entries.\n", repaired)
	}

	return 0
}

// cmdCachePath shows the cache directory path.
func cmdCachePath() int {
	cfg := config.Load()

	path := cache.ExpandPath(cfg.Cache.Path)
	fmt.Println(path)

	return 0
}
