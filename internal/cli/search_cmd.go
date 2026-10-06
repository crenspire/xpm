package cli

import (
	"fmt"
	"os"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
	tuisearch "github.com/crenspire/xpm/internal/tui/search"
)

// cmdSearch handles the search command.
func cmdSearch(args []string) int {
	if _, ok := atMostOneArg("search", args); !ok {
		return 1
	}

	// The TUI needs a terminal; pipes and CI get plain output.
	if !cfg.SearchUI.Enabled || !stdoutIsTerminal() {
		return cmdSearchNonInteractive(args)
	}

	// Get initial query
	initialQuery := ""
	if len(args) > 0 {
		initialQuery = args[0]
	}

	searchOpts := search.OptionsFromConfig(cfg)

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

	searchOpts := search.OptionsFromConfig(cfg)

	rep, err := searchReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	results := rep.Results
	st := classify(rep, searchOpts)
	if len(results) == 0 {
		fmt.Println("No results found.")
		fmt.Print(formatAvailability(st))
		return 1
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

	fmt.Print(formatAvailability(registryStatus{Unavailable: st.Unavailable}))
	return 0
}

// installFromSearchResult installs the package picked in the TUI with the
// tool picked there. The registry-supplied name is validated before anything
// else, and no version is pinned (the tool resolves its own latest).
func installFromSearchResult(result search.Result, pmID pm.ID) int {
	c := candidate{Result: result}
	c.Result.Manager = pmID
	return installCandidate(c, result.Name, "", false)
}
