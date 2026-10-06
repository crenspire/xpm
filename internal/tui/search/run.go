package search

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// SearchResult holds the selected package and package manager for installation.
type SearchResult struct {
	Result search.Result
	PM     pm.ID
}

// Run starts the TUI search interface. It needs a terminal; callers print
// plain results otherwise. Returns the selected result and package manager,
// or nil if the user cancelled.
func Run(initialQuery string, opts search.Options, ui UIOptions) (*SearchResult, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, errors.New("the search UI needs a terminal")
	}

	p := tea.NewProgram(NewModel(initialQuery, opts, ui), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	final, ok := finalModel.(model)
	if !ok {
		return nil, fmt.Errorf("invalid model type")
	}
	if final.installMode && final.selectedResult != nil && final.installCursor < len(final.installPMs) {
		return &SearchResult{
			Result: *final.selectedResult,
			PM:     final.installPMs[final.installCursor],
		}, nil
	}
	return nil, nil // User cancelled
}
