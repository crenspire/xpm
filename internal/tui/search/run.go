package search

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
	"golang.org/x/term"
)

// SearchResult holds the selected package and package manager for installation.
type SearchResult struct {
	Result search.Result
	PM     pm.ID
}

// Run starts the TUI search interface.
// Returns the selected result and package manager, or an error.
func Run(initialQuery string, opts search.Options) (*SearchResult, error) {
	// Check if we're in a TTY
	if !isTTY() {
		return nil, runNonInteractive(initialQuery, opts)
	}

	// Create and run the TUI
	m := NewModel(initialQuery, opts)
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Run the program
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	// Extract result from final model
	final, ok := finalModel.(model)
	if !ok {
		return nil, fmt.Errorf("invalid model type")
	}

	// Check if user selected a package for installation
	if final.installMode && final.selectedResult != nil && len(final.installPMs) > 0 {
		if final.installCursor < len(final.installPMs) {
			return &SearchResult{
				Result: *final.selectedResult,
				PM:     final.installPMs[final.installCursor],
			}, nil
		}
	}

	return nil, nil // User cancelled
}

// isTTY checks if stdout is a terminal.
func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// runNonInteractive runs search in non-interactive mode (plain text output).
func runNonInteractive(query string, opts search.Options) error {
	if query == "" {
		fmt.Println("Usage: xpm search <query>")
		return nil
	}

	fmt.Printf("Searching for %q...\n\n", query)

	// Perform search
	results, err := search.SearchEverywhereParallel(query, opts)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
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

	return nil
}
