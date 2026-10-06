// Package search provides an interactive TUI for package search.
package search

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// UIOptions are the searchUI.* config values.
type UIOptions struct {
	// DebounceMs is how long typing must pause before a search starts
	// (searchUI.debounceMs); 0 means 200 ms.
	DebounceMs int
	// PageSize caps the result rows shown at once (searchUI.pageSize);
	// 0 means as many as fit the terminal.
	PageSize int
}

// defaultDebounce is used when searchUI.debounceMs is unset.
const defaultDebounce = 200 * time.Millisecond

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
	debounce           time.Duration  // pause before a query is searched
	pageSize           int            // max rows shown; 0 = fit the terminal
	seq                int            // bumped on every query change; older results are dropped
	confirmed          bool           // Enter was pressed on the install-manager picker
}

// NewModel creates a new TUI model with initial state.
func NewModel(initialQuery string, opts search.Options, ui UIOptions) model {
	selectedRegistries := make(map[pm.ID]bool)
	for _, reg := range search.Registries {
		selectedRegistries[reg] = search.Enabled(opts, reg)
	}
	debounce := time.Duration(ui.DebounceMs) * time.Millisecond
	if debounce <= 0 {
		debounce = defaultDebounce
	}
	return model{
		query:              initialQuery,
		searchOpts:         opts,
		results:            []search.Result{},
		width:              80,
		height:             24,
		registryMode:       true, // Start in registry selection mode
		selectedRegistries: selectedRegistries,
		debounce:           debounce,
		pageSize:           ui.PageSize,
	}
}

// Init returns the initial command to run.
func (m model) Init() tea.Cmd {
	// On the registry screen nothing is searched yet; Enter there starts it.
	if m.query != "" && !m.registryMode {
		return debounceCmd(m.seq, m.query, m.debounce)
	}
	return nil
}

// visibleRows is how many results fit on one page: the terminal height
// minus header, query, separator and footer, capped by pageSize.
func (m model) visibleRows() int {
	rows := m.height - 5
	if m.pageSize > 0 && m.pageSize < rows {
		rows = m.pageSize
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

// debounceMsg fires when typing has paused; seq identifies the keystroke.
type debounceMsg struct {
	seq   int
	query string
}

// searchMsg carries the results of the search started for seq.
type searchMsg struct {
	seq     int
	results []search.Result
	err     error
}

// installSelectMsg is sent when entering install mode.
type installSelectMsg struct {
	result search.Result
}
