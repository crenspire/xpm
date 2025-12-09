package search

import (
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/crenspire/xpm/internal/search"
)

// searchCmd performs a search and returns results via a message.
func searchCmd(query string, opts search.Options) tea.Cmd {
	return func() tea.Msg {
		if query == "" {
			return searchMsg{results: []search.Result{}}
		}

		// Use parallel search for better performance
		results, err := search.SearchEverywhereParallel(query, opts)
		if err != nil {
			return errMsg{err: err}
		}

		return searchMsg{results: results, err: nil}
	}
}

// debounceSearchCmd performs a debounced search.
func debounceSearchCmd(query string, opts search.Options, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return searchCmd(query, opts)()
	})
}

// resizeCmd handles terminal resize events.
func resizeCmd() tea.Cmd {
	return func() tea.Msg {
		// This will be handled by tea.WindowSizeMsg
		return nil
	}
}

// installSelectCmd triggers install mode selection.
func installSelectCmd(result search.Result) tea.Cmd {
	return func() tea.Msg {
		return installSelectMsg{result: result}
	}
}

