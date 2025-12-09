package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// View renders the TUI.
func (m model) View() string {
	if m.installMode {
		return renderInstallSelector(m)
	}

	// Show registry selection screen first
	if m.registryMode {
		return renderRegistrySelector(m)
	}

	var sections []string

	// Header (2 lines)
	sections = append(sections, renderHeader())

	// Query input (1 line)
	sections = append(sections, renderQuery(m))

	// Separator (1 line)
	sections = append(sections, separatorStyle.Render(strings.Repeat("─", m.width)))

	// Calculate available height for results
	// Header: 2, Query: 1, Separator: 1, Footer: 1 = 5 lines used
	headerHeight := 5
	availableHeight := m.height - headerHeight
	if availableHeight < 1 {
		availableHeight = 1
	}

	// Results or loading/error
	if m.loading {
		sections = append(sections, renderLoading())
	} else if m.err != nil {
		sections = append(sections, renderError(m.err))
	} else {
		sections = append(sections, renderResults(m, availableHeight))
	}

	// Footer (1 line)
	sections = append(sections, renderFooter(m))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderHeader renders the title and instructions.
func renderHeader() string {
	title := titleStyle.Render("XPM Package Search")
	instructions := footerStyle.Render("Type to search packages across npm / pip / composer / cargo / go / maven...")
	return lipgloss.JoinVertical(lipgloss.Left, title, instructions)
}

// renderQuery renders the query input field.
func renderQuery(m model) string {
	queryLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("Query: ")
	queryText := m.query
	if queryText == "" {
		queryText = " "
	}
	queryDisplay := queryStyle.Render(queryText)
	return lipgloss.JoinHorizontal(lipgloss.Left, queryLabel, queryDisplay)
}

// renderResults renders the search results list with pagination.
func renderResults(m model, maxHeight int) string {
	if len(m.results) == 0 {
		if m.query == "" {
			return footerStyle.Render("Start typing to search...")
		}
		return errorStyle.Render("No results found")
	}

	// Calculate visible range
	visibleCount := maxHeight
	if visibleCount > len(m.results) {
		visibleCount = len(m.results)
	}

	// Ensure scrollOffset is valid
	maxOffset := len(m.results) - visibleCount
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Get visible results
	startIdx := m.scrollOffset
	endIdx := startIdx + visibleCount
	if endIdx > len(m.results) {
		endIdx = len(m.results)
	}

	var lines []string
	for i := startIdx; i < endIdx; i++ {
		result := m.results[i]
		isSelected := i == m.cursor
		line := renderResult(result, isSelected, m.width)
		lines = append(lines, line)
	}

	// Add pagination indicator if there are more results
	result := lipgloss.JoinVertical(lipgloss.Left, lines...)
	
	// Show pagination info if there are more results than visible
	if len(m.results) > visibleCount {
		paginationInfo := fmt.Sprintf("Showing %d-%d of %d results", 
			startIdx+1, endIdx, len(m.results))
		paginationLine := footerStyle.Render(paginationInfo)
		result = lipgloss.JoinVertical(lipgloss.Left, result, paginationLine)
	}

	return result
}

// renderResult renders a single search result.
func renderResult(result search.Result, selected bool, width int) string {
	// Get ecosystem name
	meta, ok := pm.MetaFor(result.Manager)
	ecosystemName := string(result.Manager)
	if ok {
		ecosystemName = meta.Name
	}

	// Format ecosystem (12 chars for better visibility)
	ecosystem := getEcosystemStyle(ecosystemName).Render(truncate(ecosystemName, 12))

	// Format package name (dynamic width based on terminal size, minimum 30 chars)
	// Calculate available width: width - indicator(2) - ecosystem(12) - version(14) - spacing(8) - description(min 20)
	minPackageWidth := 30
	descMinWidth := 20
	usedWidth := 2 + 12 + 14 + 8 + descMinWidth // indicator + ecosystem + version + spacing + description
	packageWidth := width - usedWidth
	if packageWidth < minPackageWidth {
		packageWidth = minPackageWidth
	}
	packageName := truncate(result.Name, packageWidth)
	packageStyle := resultStyle
	if selected {
		packageStyle = selectedStyle
	}
	packageDisplay := packageStyle.Render(packageName)

	// Format version (14 chars for better visibility)
	version := ""
	if v, ok := result.Extra["version"]; ok {
		version = versionStyle.Render(truncate(v, 14))
	} else {
		version = versionStyle.Render("")
	}

	// Format description (remaining width after package name)
	actualUsedWidth := 2 + 12 + len(packageName) + 14 + 8 // indicator + ecosystem + package + version + spacing
	descWidth := width - actualUsedWidth
	if descWidth < 10 {
		descWidth = 10
	}
	description := descriptionStyle.Render(truncate(result.Info, descWidth))

	// Selection indicator
	indicator := " "
	if selected {
		indicator = ">"
		indicator = selectedStyle.Render(indicator)
	} else {
		indicator = resultStyle.Render(indicator)
	}

	// Combine
	return lipgloss.JoinHorizontal(lipgloss.Left,
		indicator,
		ecosystem,
		packageDisplay,
		version,
		description,
	)
}

// renderFooter renders the footer with help text.
func renderFooter(m model) string {
	if m.installMode {
		return footerStyle.Render("↑/↓: Navigate  Enter: Select  Esc: Cancel")
	}
	
	// Add pagination hints if there are many results
	hints := "↑/↓: Navigate"
	if len(m.results) > 0 {
		visibleCount := m.height - 5 // Header + query + separator + footer
		if len(m.results) > visibleCount {
			hints += "  PgUp/PgDn: Scroll"
		}
	}
	hints += "  Enter: Install  Esc: Exit  Ctrl+C: Quit"
	
	return footerStyle.Render(hints)
}

// renderRegistrySelector renders the registry selection screen.
func renderRegistrySelector(m model) string {
	var sections []string

	// Title
	title := titleStyle.Render("Select Registries to Search")
	sections = append(sections, title)
	sections = append(sections, "")

	// Available registries
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

	var lines []string
	for i, reg := range availableRegistries {
		isSelected := i == m.registryCursor
		isEnabled := false
		
		if reg.id == "all" {
			// Check if all are selected
			allSelected := true
			for _, r := range availableRegistries {
				if r.id != "all" {
					if !m.selectedRegistries[r.id] {
						allSelected = false
						break
					}
				}
			}
			isEnabled = allSelected
		} else {
			isEnabled = m.selectedRegistries[reg.id]
		}

		indicator := " "
		if isSelected {
			indicator = ">"
			indicator = selectedStyle.Render(indicator)
		} else {
			indicator = resultStyle.Render(indicator)
		}

		checkbox := "[ ]"
		if isEnabled {
			checkbox = "[✓]"
		}
		if isSelected {
			checkbox = selectedStyle.Render(checkbox)
		} else {
			checkbox = resultStyle.Render(checkbox)
		}

		regName := reg.name
		if isSelected {
			regName = selectedStyle.Render(regName)
		} else {
			regName = resultStyle.Render(regName)
		}

		line := lipgloss.JoinHorizontal(lipgloss.Left,
			indicator,
			checkbox,
			" ",
			regName,
		)
		lines = append(lines, line)
	}

	sections = append(sections, lipgloss.JoinVertical(lipgloss.Left, lines...))
	sections = append(sections, "")
	sections = append(sections, footerStyle.Render("Space: Toggle  Enter: Confirm  Esc: Cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderLoading renders the loading indicator.
func renderLoading() string {
	return loadingStyle.Render("Searching...")
}

// renderError renders an error message.
func renderError(err error) string {
	return errorStyle.Render(fmt.Sprintf("Error: %v", err))
}

// renderInstallSelector renders the package manager selection popup.
func renderInstallSelector(m model) string {
	if m.selectedResult == nil {
		return ""
	}

	var sections []string

	// Title
	title := titleStyle.Render("Install using:")
	sections = append(sections, title)

	// Get available package managers for this ecosystem
	ecosystem := pm.EcosystemForManager(m.selectedResult.Manager)
	var availablePMs []pm.ID

	if ecosystem == pm.EcosystemNode {
		availablePMs = []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun}
	} else if ecosystem == pm.EcosystemPython {
		availablePMs = []pm.ID{pm.Pip, pm.Poetry, pm.Pipenv}
	} else {
		// Single PM ecosystem
		availablePMs = []pm.ID{m.selectedResult.Manager}
	}

	// Render PM list
	var pmLines []string
	for i, pmID := range availablePMs {
		isSelected := i == m.installCursor
		meta, _ := pm.MetaFor(pmID)
		pmName := meta.Name

		indicator := " "
		style := resultStyle
		if isSelected {
			indicator = ">"
			style = selectedStyle
			indicator = selectedStyle.Render(indicator)
		} else {
			indicator = resultStyle.Render(indicator)
		}

		pmLine := lipgloss.JoinHorizontal(lipgloss.Left,
			indicator,
			style.Render(pmName),
		)
		pmLines = append(pmLines, pmLine)
	}

	sections = append(sections, lipgloss.JoinVertical(lipgloss.Left, pmLines...))
	sections = append(sections, footerStyle.Render("Press Enter to install"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// truncate truncates a string to max length with ellipsis.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

