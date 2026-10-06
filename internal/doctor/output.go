// Package doctor provides comprehensive multi-language diagnostic capabilities.
//
// This package implements Doctor+, a diagnostic engine that scans environments,
// detects configuration issues, version mismatches, dependency drift, lockfile
// conflicts, and provides actionable recommendations.
package doctor

import (
	"fmt"
	"strings"
)

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// Symbols for status indicators
const (
	symbolGood = "✔"
	symbolBad  = "✘"
	symbolWarn = "⚠"
	symbolInfo = "•"
)

// Good prints a success message with a green checkmark.
func Good(msg string) {
	fmt.Printf("%s%s %s%s\n", colorGreen, symbolGood, msg, colorReset)
}

// Bad prints an error message with a red cross.
func Bad(msg string) {
	fmt.Printf("%s%s %s%s\n", colorRed, symbolBad, msg, colorReset)
}

// Warn prints a warning message with a yellow warning symbol.
func Warn(msg string) {
	fmt.Printf("%s%s %s%s\n", colorYellow, symbolWarn, msg, colorReset)
}

// Info prints an informational message with a bullet point.
func Info(msg string) {
	fmt.Printf("%s%s %s%s\n", colorCyan, symbolInfo, msg, colorReset)
}

// Section prints a section header with decorative lines.
func Section(title string) {
	fmt.Println()
	fmt.Printf("%s[ %s ]%s\n", colorBold, title, colorReset)
}

// Header prints the main report header.
func Header() {
	fmt.Println()
	fmt.Printf("%sUPM Doctor+ Report%s\n", colorBold, colorReset)
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────")
}

// Footer prints the report footer.
func Footer() {
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────")
}

// Recommendation prints a recommendation item.
func Recommendation(msg string) {
	fmt.Printf("  %s- %s%s\n", colorCyan, msg, colorReset)
}

// StatusLine prints a status line with proper alignment.
func StatusLine(status bool, name string, detail string) {
	if status {
		if detail != "" {
			Good(fmt.Sprintf("%-20s %s", name, detail))
		} else {
			Good(name)
		}
	} else {
		if detail != "" {
			Bad(fmt.Sprintf("%-20s %s", name, detail))
		} else {
			Bad(name)
		}
	}
}

// WarnLine prints a warning status line.
func WarnLine(name string, detail string) {
	if detail != "" {
		Warn(fmt.Sprintf("%-20s %s", name, detail))
	} else {
		Warn(name)
	}
}

// IndentedLine prints an indented informational line.
func IndentedLine(msg string) {
	fmt.Printf("    %s→ %s%s\n", colorCyan, msg, colorReset)
}

// PrintList prints a list of items with a prefix.
func PrintList(items []string, prefix string) {
	for _, item := range items {
		fmt.Printf("  %s %s\n", prefix, item)
	}
}

// Separator prints a simple separator line.
func Separator() {
	fmt.Println(strings.Repeat("─", 46))
}

// Summary prints a summary line.
func Summary(good, bad, warn int) {
	fmt.Println()
	fmt.Printf("Summary: ")
	if good > 0 {
		fmt.Printf("%s%d passed%s ", colorGreen, good, colorReset)
	}
	if bad > 0 {
		fmt.Printf("%s%d failed%s ", colorRed, bad, colorReset)
	}
	if warn > 0 {
		fmt.Printf("%s%d warnings%s", colorYellow, warn, colorReset)
	}
	fmt.Println()
}

// NoIssues prints a message indicating no issues were found.
func NoIssues() {
	Good("No issues found")
}

// Skipped prints a message indicating a section was skipped.
func Skipped(reason string) {
	fmt.Printf("  %s(skipped: %s)%s\n", colorYellow, reason, colorReset)
}
