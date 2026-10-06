// Package search provides an interactive TUI for package search.
package search

import (
	"github.com/charmbracelet/bubbletea"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// model represents the TUI application state.
type model struct {
	query              string
	results            []search.Result
	cursor             int
	scrollOffset       int // Offset for pagination
	loading            bool
	err                error
	width              int
	height             int
	searchOpts         search.Options
	installMode        bool
	selectedResult     *search.Result
	installPMs         []pm.ID
	installCursor      int
	registryMode       bool           // Whether we're in registry selection mode
	registryCursor     int            // Cursor for registry selection
	selectedRegistries map[pm.ID]bool // Selected registries for search
}

// NewModel creates a new TUI model with initial state.
func NewModel(initialQuery string, opts search.Options) model {
	// Available registries
	availableRegistries := []pm.ID{
		pm.Npm,
		pm.Pip,
		pm.Composer,
		pm.Cargo,
		pm.Maven,
	}

	// Initialize selected registries - all enabled by default
	selectedRegistries := make(map[pm.ID]bool)
	for _, reg := range availableRegistries {
		// Check if enabled in opts, default to true
		if opts.Enable == nil {
			selectedRegistries[reg] = true
		} else if enabled, ok := opts.Enable[reg]; !ok || enabled {
			selectedRegistries[reg] = true
		} else {
			selectedRegistries[reg] = false
		}
	}

	return model{
		query:              initialQuery,
		searchOpts:         opts,
		results:            []search.Result{},
		cursor:             0,
		scrollOffset:       0,
		loading:            false,
		width:              80,
		height:             24,
		registryMode:       true, // Start in registry selection mode
		registryCursor:     0,
		selectedRegistries: selectedRegistries,
	}
}

// Init returns the initial command to run.
func (m model) Init() tea.Cmd {
	if m.query != "" {
		return debounceSearchCmd(m.query, m.searchOpts, 200)
	}
	return nil
}

// searchMsg is sent when search results are received.
type searchMsg struct {
	results []search.Result
	err     error
}

// errMsg is sent when an error occurs.
type errMsg struct {
	err error
}

// resizeMsg is sent when the terminal is resized.
type resizeMsg struct {
	width  int
	height int
}

// installSelectMsg is sent when entering install mode.
type installSelectMsg struct {
	result search.Result
}
