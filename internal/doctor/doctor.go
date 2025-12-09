package doctor

import (
	"os"
)

// Config holds configuration for the doctor command.
type Config struct {
	SkipEnv       bool
	SkipSecurity  bool
	SkipConflicts bool
	SkipDrift     bool
}

// Report holds the complete diagnostic report.
type Report struct {
	Environment     []RuntimeInfo
	PackageManagers []PMInfo
	Project         ProjectScanResult
	Conflicts       []Conflict
	Drift           []DriftInfo
	Security        []SecurityResult
	Recommendations []string
}

// Run executes the full diagnostic and returns a report.
func Run(cfg Config) Report {
	report := Report{}

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	// Environment check
	if !cfg.SkipEnv {
		report.Environment = CheckEnvironment()
	}

	// Package manager check
	report.PackageManagers = CheckPackageManagers()

	// Project file scan
	report.Project = ScanProject(cwd)

	// Conflict detection
	if !cfg.SkipConflicts {
		report.Conflicts = DetectConflicts(report.Project)
	}

	// Drift detection
	if !cfg.SkipDrift {
		report.Drift = CheckDrift(cwd)
	}

	// Security audit
	if !cfg.SkipSecurity {
		report.Security = RunSecurityAudit(report.Project, report.PackageManagers)
	}

	// Generate recommendations
	report.Recommendations = GenerateRecommendations(report)

	return report
}

// PrintReport prints the complete diagnostic report.
func PrintReport(report Report, cfg Config) {
	Header()

	// Environment
	if !cfg.SkipEnv && len(report.Environment) > 0 {
		PrintEnvironmentReport(report.Environment)
	}

	// Package managers
	if len(report.PackageManagers) > 0 {
		PrintPMReport(report.PackageManagers)
	}

	// Project files
	PrintProjectReport(report.Project)

	// Conflicts
	if !cfg.SkipConflicts {
		PrintConflictsReport(report.Conflicts)
	}

	// Drift
	if !cfg.SkipDrift && len(report.Drift) > 0 {
		PrintDriftReport(report.Drift)
	}

	// Security
	if !cfg.SkipSecurity && len(report.Security) > 0 {
		PrintSecurityReport(report.Security)
	}

	// Recommendations
	PrintRecommendations(report.Recommendations)

	// Summary
	PrintSummary(report, cfg)

	Footer()
}

// GenerateRecommendations generates actionable recommendations based on the report.
func GenerateRecommendations(report Report) []string {
	var recommendations []string
	seen := make(map[string]bool)

	// Add conflict suggestions
	for _, suggestion := range GetConflictSuggestions(report.Conflicts) {
		if !seen[suggestion] {
			recommendations = append(recommendations, suggestion)
			seen[suggestion] = true
		}
	}

	// Add drift suggestions
	for _, suggestion := range GetDriftSuggestions(report.Drift) {
		if !seen[suggestion] {
			recommendations = append(recommendations, suggestion)
			seen[suggestion] = true
		}
	}

	// Add security suggestions
	for _, suggestion := range GetSecuritySuggestions(report.Security) {
		if !seen[suggestion] {
			recommendations = append(recommendations, suggestion)
			seen[suggestion] = true
		}
	}

	// Add missing lockfile suggestions
	for _, lockFile := range report.Project.MissingLockFiles {
		suggestion := "Generate missing " + lockFile
		if !seen[suggestion] {
			recommendations = append(recommendations, suggestion)
			seen[suggestion] = true
		}
	}

	// Add optional tool suggestions
	for _, pm := range GetMissingOptionalPMs(report.PackageManagers) {
		switch pm {
		case "cargo-audit":
			suggestion := "Install cargo-audit for Rust security scanning: cargo install cargo-audit"
			if !seen[suggestion] {
				recommendations = append(recommendations, suggestion)
				seen[suggestion] = true
			}
		}
	}

	return recommendations
}

// PrintRecommendations prints the recommendations section.
func PrintRecommendations(recommendations []string) {
	if len(recommendations) == 0 {
		return
	}

	Section("Recommendations")
	for _, r := range recommendations {
		Recommendation(r)
	}
}

// PrintSummary prints a summary of the diagnostic.
func PrintSummary(report Report, cfg Config) {
	var good, bad, warn int

	// Count environment results
	if !cfg.SkipEnv {
		installed, missing := CountEnvResults(report.Environment)
		good += installed
		bad += missing
	}

	// Count package manager results
	pmInstalled, pmMissing, pmOptional := CountPMResults(report.PackageManagers)
	good += pmInstalled
	// Don't count missing PMs as "bad" since not all are needed
	_ = pmMissing
	warn += pmOptional

	// Count conflicts
	if !cfg.SkipConflicts {
		warn += CountConflicts(report.Conflicts)
	}

	// Count drift issues
	if !cfg.SkipDrift {
		ok, outdated, missing := CountDriftIssues(report.Drift)
		good += ok
		bad += outdated + missing
	}

	// Count security issues
	if !cfg.SkipSecurity {
		secOK, secVuln, secUnavail := CountSecurityIssues(report.Security)
		good += secOK
		bad += secVuln
		warn += secUnavail
	}

	// Count missing lock files
	bad += len(report.Project.MissingLockFiles)

	Summary(good, bad, warn)
}

// HasIssues checks if the report contains any issues.
func HasIssues(report Report) bool {
	// Check for missing lock files
	if len(report.Project.MissingLockFiles) > 0 {
		return true
	}

	// Check for conflicts
	if len(report.Conflicts) > 0 {
		return true
	}

	// Check for drift issues
	if HasDriftIssues(report.Drift) {
		return true
	}

	// Check for security issues
	if HasSecurityIssues(report.Security) {
		return true
	}

	return false
}

// QuickCheck performs a quick check and returns true if there are issues.
func QuickCheck() bool {
	cfg := Config{
		SkipSecurity: true, // Skip security for quick check
	}
	report := Run(cfg)
	return HasIssues(report)
}

