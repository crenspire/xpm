package search

import (
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// Update handles messages and updates the model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleKeyMsg(m, msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case debounceMsg:
		if msg.seq != m.seq {
			return m, nil // typing continued; a newer debounce is pending
		}
		return m, searchCmd(msg.seq, msg.query, m.searchOpts)
	case searchMsg:
		return handleSearchMsg(m, msg)
	case installSelectMsg:
		return handleInstallSelectMsg(m, msg)
	}
	return m, nil
}

// queryChanged starts a new debounce window for the current query.
func queryChanged(m model) (model, tea.Cmd) {
	m.seq++
	m.cursor = 0
	m.scrollOffset = 0
	m.loading = true
	return m, debounceCmd(m.seq, m.query, m.debounce)
}

// dropLastRune removes the last character (not byte) of s.
func dropLastRune(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

// handleKeyMsg handles keyboard input. In query mode only arrows, ctrl
// keys, Enter and Esc act; every printable key, including q/j/k, is typed.
func handleKeyMsg(m model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.installMode {
		return handleInstallKeyMsg(m, msg)
	}
	if m.registryMode {
		return handleRegistryKeyMsg(m, msg)
	}

	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyEnter:
		if m.cursor < len(m.results) {
			return enterInstallMode(m, m.results[m.cursor])
		}
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		if m.cursor > 0 {
			m.cursor--
			m = adjustScrollOffset(m)
		}
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		if m.cursor < len(m.results)-1 {
			m.cursor++
			m = adjustScrollOffset(m)
		}
		return m, nil
	case tea.KeyPgUp, tea.KeyCtrlU:
		return pageUp(m), nil
	case tea.KeyPgDown, tea.KeyCtrlD:
		return pageDown(m), nil
	case tea.KeyRunes:
		m.query += string(msg.Runes)
		return queryChanged(m)
	case tea.KeySpace:
		m.query += " "
		return queryChanged(m)
	case tea.KeyBackspace, tea.KeyDelete:
		if m.query == "" {
			return m, nil
		}
		m.query = dropLastRune(m.query)
		return queryChanged(m)
	}
	return m, nil
}

// handleInstallKeyMsg handles keyboard input in install mode.
func handleInstallKeyMsg(m model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		// Exit install mode
		m.installMode = false
		m.selectedResult = nil
		m.installPMs = nil
		m.installCursor = 0
		return m, nil
	case "enter":
		if len(m.installPMs) > 0 && m.installCursor < len(m.installPMs) {
			// Return selected result and PM for installation
			m.confirmed = true
			return m, tea.Quit
		}
		return m, nil
	case "up", "k":
		if m.installCursor > 0 {
			m.installCursor--
		}
		return m, nil
	case "down", "j":
		if m.installCursor < len(m.installPMs)-1 {
			m.installCursor++
		}
		return m, nil
	}
	return m, nil
}

// handleSearchMsg applies search results, dropping those of an older query.
func handleSearchMsg(m model, msg searchMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.seq {
		return m, nil
	}
	m.loading = false
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	m.err = nil
	m.results = msg.results
	// Reset cursor and scroll offset if out of bounds
	if m.cursor >= len(m.results) {
		m.cursor = 0
	}
	m.scrollOffset = 0
	return m, nil
}

// handleInstallSelectMsg handles entering install mode.
func handleInstallSelectMsg(m model, msg installSelectMsg) (tea.Model, tea.Cmd) {
	m.installMode = true
	m.selectedResult = &msg.result

	m.installPMs = installManagersFor(msg.result.Manager, m.dir)
	m.installCursor = 0

	return m, nil
}

// enterInstallMode enters install selection mode.
func enterInstallMode(m model, result search.Result) (tea.Model, tea.Cmd) {
	return handleInstallSelectMsg(m, installSelectMsg{result: result})
}

// adjustScrollOffset ensures the cursor is visible by adjusting scroll offset.
func adjustScrollOffset(m model) model {
	if len(m.results) == 0 {
		return m
	}

	availableHeight := m.visibleRows()

	// Calculate visible range
	visibleCount := availableHeight
	if visibleCount > len(m.results) {
		visibleCount = len(m.results)
	}

	// Ensure cursor is within visible range
	startIdx := m.scrollOffset
	endIdx := startIdx + visibleCount - 1

	if m.cursor < startIdx {
		// Cursor is above visible area, scroll up
		m.scrollOffset = m.cursor
	} else if m.cursor > endIdx {
		// Cursor is below visible area, scroll down
		m.scrollOffset = m.cursor - visibleCount + 1
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	}

	return m
}

// pageUp scrolls up one page.
func pageUp(m model) model {
	if len(m.results) == 0 {
		return m
	}

	availableHeight := m.visibleRows()

	// Scroll up by one page
	m.scrollOffset -= availableHeight
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Adjust cursor to stay within visible range
	visibleCount := availableHeight
	if visibleCount > len(m.results) {
		visibleCount = len(m.results)
	}
	endIdx := m.scrollOffset + visibleCount - 1
	if m.cursor > endIdx {
		m.cursor = endIdx
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	return m
}

// pageDown scrolls down one page.
func pageDown(m model) model {
	if len(m.results) == 0 {
		return m
	}

	availableHeight := m.visibleRows()

	// Calculate max offset
	visibleCount := availableHeight
	if visibleCount > len(m.results) {
		visibleCount = len(m.results)
	}
	maxOffset := len(m.results) - visibleCount
	if maxOffset < 0 {
		maxOffset = 0
	}

	// Scroll down by one page
	m.scrollOffset += availableHeight
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}

	// Adjust cursor to stay within visible range
	startIdx := m.scrollOffset
	if m.cursor < startIdx {
		m.cursor = startIdx
	}
	if m.cursor >= len(m.results) {
		m.cursor = len(m.results) - 1
	}

	return m
}

// registryChoices are the rows of the registry selection screen.
var registryChoices = []struct {
	id   pm.ID
	name string
}{
	{pm.Npm, "npm (Node.js)"},
	{pm.Pip, "pip (Python)"},
	{pm.Composer, "composer (PHP)"},
	{pm.Cargo, "cargo (Rust)"},
	{pm.Maven, "maven (Java)"},
	{pm.ID("all"), "All Registries"},
}

// applyRegistrySelection copies the checked registries into searchOpts.
func applyRegistrySelection(m model) model {
	enable := make(map[pm.ID]bool, len(search.Registries))
	for _, id := range search.Registries {
		enable[id] = m.selectedRegistries[id]
	}
	m.searchOpts.Enable = enable
	m.registryMode = false
	return m
}

// handleRegistryKeyMsg handles keyboard input in registry selection mode.
// Typing a printable key (q/j/k included) starts the query.
func handleRegistryKeyMsg(m model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyEnter:
		m = applyRegistrySelection(m)
		if m.query != "" {
			return queryChanged(m)
		}
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		if m.registryCursor > 0 {
			m.registryCursor--
		}
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		if m.registryCursor < len(registryChoices)-1 {
			m.registryCursor++
		}
		return m, nil
	case tea.KeySpace:
		reg := registryChoices[m.registryCursor]
		if reg.id != "all" {
			m.selectedRegistries[reg.id] = !m.selectedRegistries[reg.id]
			return m, nil
		}
		allSelected := true
		for _, id := range search.Registries {
			allSelected = allSelected && m.selectedRegistries[id]
		}
		for _, id := range search.Registries {
			m.selectedRegistries[id] = !allSelected
		}
		return m, nil
	case tea.KeyRunes:
		m = applyRegistrySelection(m)
		m.query += string(msg.Runes)
		return queryChanged(m)
	case tea.KeyBackspace, tea.KeyDelete:
		m.registryMode = false
		return m, nil
	}
	return m, nil
}

// installManagersFor lists the tools that can install a hit from registry
// id: every manager of its ecosystem (npm hit: npm, yarn, pnpm, bun; Maven
// hit: maven, gradle), or just id for single-tool ecosystems. Tools that
// dir's lock or build files point at come first, so Enter-Enter in a yarn
// project installs with yarn.
func installManagersFor(id pm.ID, dir string) []pm.ID {
	eco := pm.EcosystemForManager(id)
	all := pm.ManagersInEcosystem(eco)
	if len(all) == 0 {
		return []pm.ID{id}
	}
	out := make([]pm.ID, 0, len(all))
	seen := map[pm.ID]bool{}
	for _, f := range pm.ProjectManagers(dir)[eco] {
		if !seen[f.Manager] {
			out = append(out, f.Manager)
			seen[f.Manager] = true
		}
	}
	for _, m := range all {
		if !seen[m] {
			out = append(out, m)
		}
	}
	return out
}
