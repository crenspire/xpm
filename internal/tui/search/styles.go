package search

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// titleStyle styles the header/title.
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5FD7FF")).
			Bold(true).
			Padding(0, 1)

	// queryStyle styles the query input.
	queryStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#3C3C3C")).
			Padding(0, 1)

	// resultStyle styles normal results.
	resultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC")).
			Padding(0, 1)

	// selectedStyle styles the selected/highlighted result.
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7AF")).
			Bold(true).
			Padding(0, 1)

	// ecosystemStyle styles ecosystem labels.
	ecosystemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Width(10).
			Align(lipgloss.Left)

	// versionStyle styles version text.
	versionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA")).
			Width(12).
			Align(lipgloss.Left)

	// descriptionStyle styles description text.
	descriptionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC"))

	// footerStyle styles the footer/help text.
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true)

	// errorStyle styles error messages.
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true)

	// loadingStyle styles loading indicator.
	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5FD7FF")).
			Italic(true)

	// separatorStyle styles separators.
	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444444"))
)

// ecosystemColor returns a color for an ecosystem.
func ecosystemColor(ecosystem string) lipgloss.Color {
	colors := map[string]lipgloss.Color{
		"npm":      lipgloss.Color("#CB3837"),
		"yarn":     lipgloss.Color("#2C8EBB"),
		"pnpm":     lipgloss.Color("#F69220"),
		"bun":      lipgloss.Color("#FBF0DF"),
		"pip":      lipgloss.Color("#3776AB"),
		"poetry":   lipgloss.Color("#60A5FA"),
		"pipenv":   lipgloss.Color("#CECE5A"),
		"composer": lipgloss.Color("#F28D1A"),
		"cargo":    lipgloss.Color("#000000"),
		"gomod":    lipgloss.Color("#00ADD8"),
		"maven":    lipgloss.Color("#C71A36"),
		"gradle":   lipgloss.Color("#02303A"),
	}
	if color, ok := colors[ecosystem]; ok {
		return color
	}
	return lipgloss.Color("#888888")
}

// getEcosystemStyle returns a styled ecosystem label.
func getEcosystemStyle(ecosystem string) lipgloss.Style {
	return ecosystemStyle.Copy().
		Foreground(ecosystemColor(ecosystem)).
		Bold(true)
}

