package search

import (
	"time"

	"github.com/charmbracelet/bubbletea"
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
	case searchMsg:
		return handleSearchMsg(m, msg)
	case errMsg:
		return handleErrMsg(m, msg)
	case installSelectMsg:
		return handleInstallSelectMsg(m, msg)
	}
	return m, nil
}

// handleKeyMsg handles keyboard input.
func handleKeyMsg(m model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.installMode {
		return handleInstallKeyMsg(m, msg)
	}

	// Handle registry selection mode
	if m.registryMode {
		return handleRegistryKeyMsg(m, msg)
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		return m, tea.Quit
	case "enter":
		if len(m.results) > 0 && m.cursor < len(m.results) {
			selected := m.results[m.cursor]
			return enterInstallMode(m, selected)
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m = adjustScrollOffset(m)
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.results)-1 {
			m.cursor++
			m = adjustScrollOffset(m)
		}
		return m, nil
	case "pgup", "ctrl+u":
		m = pageUp(m)
		return m, nil
	case "pgdown", "ctrl+d":
		m = pageDown(m)
		return m, nil
	default:
		// Typing - update query
		if msg.Type == tea.KeyRunes {
			m.query += string(msg.Runes)
			m.cursor = 0
			m.scrollOffset = 0
			m.loading = true
			return m, debounceSearchCmd(m.query, m.searchOpts, 200*time.Millisecond)
		} else if msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
			if len(m.query) > 0 {
				m.query = m.query[:len(m.query)-1]
				m.cursor = 0
				m.scrollOffset = 0
				m.loading = true
				return m, debounceSearchCmd(m.query, m.searchOpts, 200*time.Millisecond)
			}
		}
		return m, nil
	}
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

// handleSearchMsg handles search results.
func handleSearchMsg(m model, msg searchMsg) (tea.Model, tea.Cmd) {
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

// handleErrMsg handles errors.
func handleErrMsg(m model, msg errMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.err = msg.err
	return m, nil
}

// handleInstallSelectMsg handles entering install mode.
func handleInstallSelectMsg(m model, msg installSelectMsg) (tea.Model, tea.Cmd) {
	m.installMode = true
	m.selectedResult = &msg.result

	m.installPMs = installManagersFor(msg.result.Manager)
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

	// Calculate available height
	availableHeight := m.height - 5 // Header + query + separator + footer
	if availableHeight < 1 {
		availableHeight = 1
	}

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

	availableHeight := m.height - 5
	if availableHeight < 1 {
		availableHeight = 1
	}

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

	availableHeight := m.height - 5
	if availableHeight < 1 {
		availableHeight = 1
	}

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

// handleRegistryKeyMsg handles keyboard input in registry selection mode.
func handleRegistryKeyMsg(m model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	availableRegistries := []struct {
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

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		return m, tea.Quit
	case "enter":
		// Confirm selection and start search
		m.registryMode = false
		// Update search options based on selected registries
		if m.searchOpts.Enable == nil {
			m.searchOpts.Enable = make(map[pm.ID]bool)
		}
		for id := range m.searchOpts.Enable {
			m.searchOpts.Enable[id] = m.selectedRegistries[id]
		}
		// Set all registries based on selection
		for _, reg := range availableRegistries {
			if reg.id != "all" {
				m.searchOpts.Enable[reg.id] = m.selectedRegistries[reg.id]
			}
		}
		// If query is already set, start searching
		if m.query != "" {
			m.loading = true
			return m, debounceSearchCmd(m.query, m.searchOpts, 200*time.Millisecond)
		}
		return m, nil
	case "up", "k":
		if m.registryCursor > 0 {
			m.registryCursor--
		}
		return m, nil
	case "down", "j":
		if m.registryCursor < len(availableRegistries)-1 {
			m.registryCursor++
		}
		return m, nil
	case " ":
		// Toggle selection
		if m.registryCursor < len(availableRegistries) {
			reg := availableRegistries[m.registryCursor]
			if reg.id == "all" {
				// Toggle all
				allSelected := true
				for _, r := range availableRegistries {
					if r.id != "all" {
						if !m.selectedRegistries[r.id] {
							allSelected = false
							break
						}
					}
				}
				// If all selected, deselect all. Otherwise, select all.
				newState := !allSelected
				for _, r := range availableRegistries {
					if r.id != "all" {
						m.selectedRegistries[r.id] = newState
					}
				}
			} else {
				// Toggle individual registry
				m.selectedRegistries[reg.id] = !m.selectedRegistries[reg.id]
			}
		}
		return m, nil
	default:
		// Typing - update query (exit registry mode and start searching)
		if msg.Type == tea.KeyRunes {
			m.query = string(msg.Runes)
			m.registryMode = false
			// Update search options
			if m.searchOpts.Enable == nil {
				m.searchOpts.Enable = make(map[pm.ID]bool)
			}
			for _, reg := range availableRegistries {
				if reg.id != "all" {
					m.searchOpts.Enable[reg.id] = m.selectedRegistries[reg.id]
				}
			}
			m.cursor = 0
			m.scrollOffset = 0
			m.loading = true
			return m, debounceSearchCmd(m.query, m.searchOpts, 200*time.Millisecond)
		} else if msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
			// Exit registry mode if backspace is pressed
			m.registryMode = false
			return m, nil
		}
		return m, nil
	}
}

// installManagersFor lists the tools that can install a hit from registry
// id: every manager of its ecosystem (npm hit: npm, yarn, pnpm, bun; Maven
// hit: maven, gradle), or just id for single-tool ecosystems.
func installManagersFor(id pm.ID) []pm.ID {
	if ids := pm.ManagersInEcosystem(pm.EcosystemForManager(id)); len(ids) > 0 {
		return ids
	}
	return []pm.ID{id}
}
