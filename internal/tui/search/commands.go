package search

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crenspire/xpm/internal/search"
)

// runSearch is the registry search; tests replace it.
var runSearch = search.SearchEverywhereParallel

// debounceCmd waits d and then reports which query change (seq) it was
// for. Only the latest change starts a search.
func debounceCmd(seq int, query string, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return debounceMsg{seq: seq, query: query}
	})
}

// searchCmd searches for query; the reply carries seq so a slower, older
// search can never overwrite newer results.
func searchCmd(seq int, query string, opts search.Options) tea.Cmd {
	return func() tea.Msg {
		if query == "" {
			return searchMsg{seq: seq}
		}
		results, err := runSearch(query, opts)
		return searchMsg{seq: seq, results: results, err: err}
	}
}
