package cli

import (
	"fmt"
	"os"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
	tuisearch "github.com/crenspire/xpm/internal/tui/search"
)

// cmdSearch handles the search command.
func cmdSearch(args []string) int {
	cfg := config.Load()

	// Check if TUI is enabled
	if !cfg.SearchUI.Enabled {
		// Fallback to non-interactive search
		return cmdSearchNonInteractive(args)
	}

	// Get initial query
	initialQuery := ""
	if len(args) > 0 {
		initialQuery = args[0]
	}

	// Build search options
	searchOpts := search.Options{
		Enable: make(map[pm.ID]bool),
	}
	for id := range map[pm.ID]struct{}{
		pm.Npm: {}, pm.Pip: {}, pm.Composer: {}, pm.Cargo: {}, pm.Maven: {},
	} {
		name := string(id)
		enabled := true
		if v, ok := cfg.Search[name]; ok {
			enabled = v
		}
		searchOpts.Enable[id] = enabled
	}

	// Run TUI search
	result, err := tuisearch.Run(initialQuery, searchOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// If user selected a package for installation
	if result != nil {
		// Install the selected package using the chosen PM
		return installFromSearchResult(result.Result, result.PM)
	}

	return 0
}

// cmdSearchNonInteractive provides a fallback non-interactive search.
func cmdSearchNonInteractive(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "search: missing package name")
		fmt.Fprintln(os.Stderr, "Usage: xpm search <package>")
		return 1
	}

	pkg := args[0]
	fmt.Printf("Searching for %q...\n\n", pkg)

	cfg := config.Load()
	searchOpts := search.Options{
		Enable: make(map[pm.ID]bool),
	}
	for id := range map[pm.ID]struct{}{
		pm.Npm: {}, pm.Pip: {}, pm.Composer: {}, pm.Cargo: {}, pm.Maven: {},
	} {
		name := string(id)
		enabled := true
		if v, ok := cfg.Search[name]; ok {
			enabled = v
		}
		searchOpts.Enable[id] = enabled
	}

	results, err := search.SearchEverywhereParallel(pkg, searchOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return 0
	}

	// Print results
	for _, result := range results {
		meta, ok := pm.MetaFor(result.Manager)
		ecosystemName := string(result.Manager)
		if ok {
			ecosystemName = meta.Name
		}

		version := ""
		if v, ok := result.Extra["version"]; ok {
			version = "@" + v
		}

		description := result.Info
		if description == "" {
			description = "No description"
		}

		fmt.Printf("%s: %s%s - %s\n", ecosystemName, result.Name, version, description)
	}

	return 0
}

// installFromSearchResult installs a package from a search result.
func installFromSearchResult(result search.Result, pmID pm.ID) int {
	cfg := config.Load()
	meta, ok := pm.MetaFor(pmID)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unsupported package manager: %s\n", pmID)
		return 1
	}

	fmt.Printf("\nInstalling %s via %s...\n\n", result.Name, meta.Name)

	adapter, err := pm.NewAdapter(pmID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Check if PM is installed
	if !pm.Exists(meta.Binary) {
		fmt.Printf("%s (%s) is not installed on this system.\n", meta.Name, meta.Binary)
		if !cfg.AutoInstallPM {
			fmt.Println("Auto-install is disabled in config. Install it manually and re-run.")
			return 1
		}

		yes, err := askYesNo(fmt.Sprintf("Attempt to install %s now?", meta.Name))
		if err != nil || !yes {
			fmt.Println("Aborted.")
			return 1
		}

		if err := pm.InstallPM(pmID); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to install package manager: %v\n", err)
			return 1
		}
	}

	// Names here come from registry responses, not the user: validate them too.
	if err := pm.ValidatePackageName(result.Name, pmID); err != nil {
		fmt.Fprintf(os.Stderr, "Refusing to install %q: %v\n", result.Name, err)
		return 1
	}

	// Install the package
	if err := adapter.InstallPackage(result.Name, false, nil, result.Extra); err != nil {
		fmt.Fprintf(os.Stderr, "Package install failed: %v\n", err)
		return 1
	}

	fmt.Println("\nDone ✅")
	return 0
}
