package doctor

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
)

// SecurityResult holds the result of a security audit.
type SecurityResult struct {
	Ecosystem       string
	Tool            string
	Available       bool
	Vulnerabilities int
	HighSeverity    int
	MediumSeverity  int
	LowSeverity     int
	Summary         string
	Error           string
}

// RunSecurityAudit runs security audits for detected ecosystems.
func RunSecurityAudit(projectResult ProjectScanResult, pmResults []PMInfo) []SecurityResult {
	var results []SecurityResult

	// Check Node.js projects - detect which package manager to use
	if HasFile(projectResult, "package.json") {
		// Detect which package manager based on lockfiles
		if HasFile(projectResult, "yarn.lock") {
			results = append(results, auditYarn())
		} else if HasFile(projectResult, "pnpm-lock.yaml") {
			results = append(results, auditPnpm())
		} else if HasFile(projectResult, "bun.lockb") {
			results = append(results, auditBun())
		} else {
			// Default to npm if no lockfile or package-lock.json exists
			results = append(results, auditNpm())
		}
	}

	// Check pip if Python project detected
	if HasFile(projectResult, "requirements.txt") || HasFile(projectResult, "pyproject.toml") {
		results = append(results, auditPip())
	}

	// Check composer if composer.json exists
	if HasFile(projectResult, "composer.json") {
		results = append(results, auditComposer())
	}

	// Check cargo if Cargo.toml exists
	if HasFile(projectResult, "Cargo.toml") {
		results = append(results, auditCargo(pmResults))
	}

	return results
}

// auditNpm runs npm audit.
func auditNpm() SecurityResult {
	result := SecurityResult{
		Ecosystem: "node",
		Tool:      "npm audit",
	}

	// Check if npm is available
	if _, err := exec.LookPath("npm"); err != nil {
		result.Available = false
		result.Summary = "npm not available"
		return result
	}

	result.Available = true

	// Run npm audit --json
	cmd := exec.Command("npm", "audit", "--json")
	output, _ := cmd.CombinedOutput()

	// Parse JSON output
	var auditResult struct {
		Metadata struct {
			Vulnerabilities struct {
				Info     int `json:"info"`
				Low      int `json:"low"`
				Moderate int `json:"moderate"`
				High     int `json:"high"`
				Critical int `json:"critical"`
				Total    int `json:"total"`
			} `json:"vulnerabilities"`
		} `json:"metadata"`
		// npm v7+ format
		Vulnerabilities map[string]interface{} `json:"vulnerabilities"`
	}

	if err := json.Unmarshal(output, &auditResult); err != nil {
		// Try to parse old format or handle error
		result.Summary = "OK (no vulnerabilities or unable to parse)"
		return result
	}

	// Calculate totals
	total := auditResult.Metadata.Vulnerabilities.Total
	if total == 0 && len(auditResult.Vulnerabilities) > 0 {
		total = len(auditResult.Vulnerabilities)
	}

	result.Vulnerabilities = total
	result.HighSeverity = auditResult.Metadata.Vulnerabilities.High + auditResult.Metadata.Vulnerabilities.Critical
	result.MediumSeverity = auditResult.Metadata.Vulnerabilities.Moderate
	result.LowSeverity = auditResult.Metadata.Vulnerabilities.Low + auditResult.Metadata.Vulnerabilities.Info

	if total == 0 {
		result.Summary = "OK"
	} else {
		result.Summary = strconv.Itoa(total) + " vulnerabilities"
		if result.HighSeverity > 0 {
			result.Summary += " (" + strconv.Itoa(result.HighSeverity) + " high)"
		}
	}

	return result
}

// auditYarn runs yarn audit.
func auditYarn() SecurityResult {
	result := SecurityResult{
		Ecosystem: "node",
		Tool:      "yarn audit",
	}

	// Check if yarn is available
	if _, err := exec.LookPath("yarn"); err != nil {
		result.Available = false
		result.Summary = "yarn not available"
		return result
	}

	result.Available = true

	// Run yarn audit --json
	cmd := exec.Command("yarn", "audit", "--json")
	output, _ := cmd.CombinedOutput()

	// Parse yarn audit JSON output (line-delimited JSON)
	lines := strings.Split(string(output), "\n")
	total := 0
	high := 0
	moderate := 0
	low := 0
	vulnDetails := make([]map[string]interface{}, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Yarn audit outputs different types of entries
		if entryType, ok := entry["type"].(string); ok {
			if entryType == "auditSummary" {
				// Summary entry
				if data, ok := entry["data"].(map[string]interface{}); ok {
					if vulns, ok := data["vulnerabilities"].(map[string]interface{}); ok {
						if info, ok := vulns["info"].(float64); ok {
							low += int(info)
						}
						if lowVal, ok := vulns["low"].(float64); ok {
							low += int(lowVal)
						}
						if moderateVal, ok := vulns["moderate"].(float64); ok {
							moderate += int(moderateVal)
						}
						if highVal, ok := vulns["high"].(float64); ok {
							high += int(highVal)
						}
						if critical, ok := vulns["critical"].(float64); ok {
							high += int(critical)
						}
						if totalVal, ok := vulns["total"].(float64); ok {
							total = int(totalVal)
						}
					}
				}
			} else if entryType == "auditAdvisory" {
				// Individual vulnerability entry
				if data, ok := entry["data"].(map[string]interface{}); ok {
					vulnDetails = append(vulnDetails, data)
				}
			}
		}
	}

	// If we didn't get totals from summary, count from details
	if total == 0 && len(vulnDetails) > 0 {
		total = len(vulnDetails)
		// Try to extract severity from details
		for _, vuln := range vulnDetails {
			if severity, ok := vuln["severity"].(string); ok {
				severity = strings.ToLower(severity)
				if severity == "critical" || severity == "high" {
					high++
				} else if severity == "moderate" {
					moderate++
				} else {
					low++
				}
			}
		}
	}

	result.Vulnerabilities = total
	result.HighSeverity = high
	result.MediumSeverity = moderate
	result.LowSeverity = low

	if total == 0 {
		result.Summary = "OK"
	} else {
		result.Summary = strconv.Itoa(total) + " vulnerabilities"
		parts := []string{}
		if high > 0 {
			parts = append(parts, strconv.Itoa(high)+" high")
		}
		if moderate > 0 {
			parts = append(parts, strconv.Itoa(moderate)+" moderate")
		}
		if low > 0 {
			parts = append(parts, strconv.Itoa(low)+" low")
		}
		if len(parts) > 0 {
			result.Summary += " (" + strings.Join(parts, ", ") + ")"
		}
	}

	return result
}

// auditPnpm runs pnpm audit.
func auditPnpm() SecurityResult {
	result := SecurityResult{
		Ecosystem: "node",
		Tool:      "pnpm audit",
	}

	// Check if pnpm is available
	if _, err := exec.LookPath("pnpm"); err != nil {
		result.Available = false
		result.Summary = "pnpm not available"
		return result
	}

	result.Available = true

	// Run pnpm audit --json
	cmd := exec.Command("pnpm", "audit", "--json")
	output, _ := cmd.CombinedOutput()

	// Parse pnpm audit JSON (similar to npm)
	var auditResult struct {
		Metadata struct {
			Vulnerabilities struct {
				Info     int `json:"info"`
				Low      int `json:"low"`
				Moderate int `json:"moderate"`
				High     int `json:"high"`
				Critical int `json:"critical"`
				Total    int `json:"total"`
			} `json:"vulnerabilities"`
		} `json:"metadata"`
		Vulnerabilities map[string]interface{} `json:"vulnerabilities"`
	}

	if err := json.Unmarshal(output, &auditResult); err != nil {
		result.Summary = "OK (no vulnerabilities or unable to parse)"
		return result
	}

	total := auditResult.Metadata.Vulnerabilities.Total
	if total == 0 && len(auditResult.Vulnerabilities) > 0 {
		total = len(auditResult.Vulnerabilities)
	}

	result.Vulnerabilities = total
	result.HighSeverity = auditResult.Metadata.Vulnerabilities.High + auditResult.Metadata.Vulnerabilities.Critical
	result.MediumSeverity = auditResult.Metadata.Vulnerabilities.Moderate
	result.LowSeverity = auditResult.Metadata.Vulnerabilities.Low + auditResult.Metadata.Vulnerabilities.Info

	if total == 0 {
		result.Summary = "OK"
	} else {
		result.Summary = strconv.Itoa(total) + " vulnerabilities"
		parts := []string{}
		if result.HighSeverity > 0 {
			parts = append(parts, strconv.Itoa(result.HighSeverity)+" high")
		}
		if result.MediumSeverity > 0 {
			parts = append(parts, strconv.Itoa(result.MediumSeverity)+" moderate")
		}
		if result.LowSeverity > 0 {
			parts = append(parts, strconv.Itoa(result.LowSeverity)+" low")
		}
		if len(parts) > 0 {
			result.Summary += " (" + strings.Join(parts, ", ") + ")"
		}
	}

	return result
}

// auditBun runs bun audit.
func auditBun() SecurityResult {
	result := SecurityResult{
		Ecosystem: "node",
		Tool:      "bun audit",
	}

	// Check if bun is available
	if _, err := exec.LookPath("bun"); err != nil {
		result.Available = false
		result.Summary = "bun not available"
		return result
	}

	result.Available = true

	// Run bun audit (bun uses npm-compatible audit)
	cmd := exec.Command("bun", "audit", "--json")
	output, _ := cmd.CombinedOutput()

	// Parse similar to npm
	var auditResult struct {
		Metadata struct {
			Vulnerabilities struct {
				Info     int `json:"info"`
				Low      int `json:"low"`
				Moderate int `json:"moderate"`
				High     int `json:"high"`
				Critical int `json:"critical"`
				Total    int `json:"total"`
			} `json:"vulnerabilities"`
		} `json:"metadata"`
		Vulnerabilities map[string]interface{} `json:"vulnerabilities"`
	}

	if err := json.Unmarshal(output, &auditResult); err != nil {
		result.Summary = "OK (no vulnerabilities or unable to parse)"
		return result
	}

	total := auditResult.Metadata.Vulnerabilities.Total
	if total == 0 && len(auditResult.Vulnerabilities) > 0 {
		total = len(auditResult.Vulnerabilities)
	}

	result.Vulnerabilities = total
	result.HighSeverity = auditResult.Metadata.Vulnerabilities.High + auditResult.Metadata.Vulnerabilities.Critical
	result.MediumSeverity = auditResult.Metadata.Vulnerabilities.Moderate
	result.LowSeverity = auditResult.Metadata.Vulnerabilities.Low + auditResult.Metadata.Vulnerabilities.Info

	if total == 0 {
		result.Summary = "OK"
	} else {
		result.Summary = strconv.Itoa(total) + " vulnerabilities"
		parts := []string{}
		if result.HighSeverity > 0 {
			parts = append(parts, strconv.Itoa(result.HighSeverity)+" high")
		}
		if result.MediumSeverity > 0 {
			parts = append(parts, strconv.Itoa(result.MediumSeverity)+" moderate")
		}
		if result.LowSeverity > 0 {
			parts = append(parts, strconv.Itoa(result.LowSeverity)+" low")
		}
		if len(parts) > 0 {
			result.Summary += " (" + strings.Join(parts, ", ") + ")"
		}
	}

	return result
}

// auditPip runs pip-audit if available.
func auditPip() SecurityResult {
	result := SecurityResult{
		Ecosystem: "python",
		Tool:      "pip-audit",
	}

	// Check if pip-audit is available
	if _, err := exec.LookPath("pip-audit"); err != nil {
		result.Available = false
		result.Summary = "pip-audit not installed"
		return result
	}

	result.Available = true

	// Run pip-audit --format json
	cmd := exec.Command("pip-audit", "--format", "json")
	output, err := cmd.CombinedOutput()

	if err != nil {
		// pip-audit returns non-zero if vulnerabilities found
		// Try to parse output anyway
	}

	// Parse JSON output
	var vulns []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Vulns   []struct {
			ID          string   `json:"id"`
			FixVersions []string `json:"fix_versions"`
		} `json:"vulns"`
	}

	if err := json.Unmarshal(output, &vulns); err != nil {
		result.Summary = "OK (no vulnerabilities or unable to parse)"
		return result
	}

	total := 0
	for _, pkg := range vulns {
		total += len(pkg.Vulns)
	}

	result.Vulnerabilities = total

	if total == 0 {
		result.Summary = "OK"
	} else {
		result.Summary = strconv.Itoa(total) + " vulnerabilities"
	}

	return result
}

// auditComposer runs composer audit.
func auditComposer() SecurityResult {
	result := SecurityResult{
		Ecosystem: "php",
		Tool:      "composer audit",
	}

	// Check if composer is available
	if _, err := exec.LookPath("composer"); err != nil {
		result.Available = false
		result.Summary = "composer not available"
		return result
	}

	result.Available = true

	// Run composer audit --format=json
	cmd := exec.Command("composer", "audit", "--format=json")
	output, _ := cmd.CombinedOutput()

	// Parse JSON output
	var auditResult struct {
		Advisories map[string][]struct {
			Title    string `json:"title"`
			Severity string `json:"severity"`
		} `json:"advisories"`
	}

	if err := json.Unmarshal(output, &auditResult); err != nil {
		// Composer audit might not be available in older versions
		result.Summary = "audit not available or no vulnerabilities"
		return result
	}

	total := 0
	high := 0
	for _, advs := range auditResult.Advisories {
		total += len(advs)
		for _, adv := range advs {
			if strings.ToLower(adv.Severity) == "high" || strings.ToLower(adv.Severity) == "critical" {
				high++
			}
		}
	}

	result.Vulnerabilities = total
	result.HighSeverity = high

	if total == 0 {
		result.Summary = "OK"
	} else {
		result.Summary = strconv.Itoa(total) + " vulnerabilities"
		if high > 0 {
			result.Summary += " (" + strconv.Itoa(high) + " high)"
		}
	}

	return result
}

// auditCargo runs cargo audit if available.
func auditCargo(pmResults []PMInfo) SecurityResult {
	result := SecurityResult{
		Ecosystem: "rust",
		Tool:      "cargo audit",
	}

	// Check if cargo-audit is available
	if !IsPMInstalled(pmResults, "cargo-audit") {
		if _, err := exec.LookPath("cargo-audit"); err != nil {
			result.Available = false
			result.Summary = "cargo-audit not installed"
			return result
		}
	}

	result.Available = true

	// Run cargo audit --json
	cmd := exec.Command("cargo", "audit", "--json")
	output, _ := cmd.CombinedOutput()

	// Parse JSON output
	var auditResult struct {
		Vulnerabilities struct {
			List []struct {
				Advisory struct {
					Severity string `json:"severity"`
				} `json:"advisory"`
			} `json:"list"`
			Count int `json:"count"`
		} `json:"vulnerabilities"`
	}

	if err := json.Unmarshal(output, &auditResult); err != nil {
		result.Summary = "OK (no vulnerabilities or unable to parse)"
		return result
	}

	total := auditResult.Vulnerabilities.Count
	high := 0
	for _, v := range auditResult.Vulnerabilities.List {
		if strings.ToLower(v.Advisory.Severity) == "high" || strings.ToLower(v.Advisory.Severity) == "critical" {
			high++
		}
	}

	result.Vulnerabilities = total
	result.HighSeverity = high

	if total == 0 {
		result.Summary = "OK"
	} else {
		result.Summary = strconv.Itoa(total) + " vulnerabilities"
		if high > 0 {
			result.Summary += " (" + strconv.Itoa(high) + " high)"
		}
	}

	return result
}

// PrintSecurityReport prints the security audit results.
func PrintSecurityReport(results []SecurityResult) {
	Section("Security Scan")

	if len(results) == 0 {
		Info("No ecosystems to audit")
		return
	}

	for _, r := range results {
		if !r.Available {
			WarnLine(r.Ecosystem+" ("+r.Tool+")", r.Summary)
		} else if r.Vulnerabilities == 0 {
			StatusLine(true, r.Ecosystem+" ("+r.Tool+")", r.Summary)
		} else {
			// Show detailed breakdown
			details := r.Summary
			if r.HighSeverity > 0 {
				Bad(r.Ecosystem + " (" + r.Tool + "): " + details)
			} else if r.MediumSeverity > 0 {
				Warn(r.Ecosystem + " (" + r.Tool + "): " + details)
			} else {
				WarnLine(r.Ecosystem+" ("+r.Tool+")", details)
			}
		}
	}
}

// CountSecurityIssues counts security issues.
func CountSecurityIssues(results []SecurityResult) (ok, vulnerable, unavailable int) {
	for _, r := range results {
		if !r.Available {
			unavailable++
		} else if r.Vulnerabilities == 0 {
			ok++
		} else {
			vulnerable++
		}
	}
	return
}

// GetSecuritySuggestions returns suggestions for security issues.
func GetSecuritySuggestions(results []SecurityResult) []string {
	var suggestions []string

	for _, r := range results {
		if !r.Available {
			switch r.Ecosystem {
			case "rust":
				suggestions = append(suggestions, "Install cargo-audit for Rust security scanning: cargo install cargo-audit")
			case "python":
				suggestions = append(suggestions, "Install pip-audit for Python security scanning: pip install pip-audit")
			}
		} else if r.Vulnerabilities > 0 {
			switch r.Ecosystem {
			case "node":
				if strings.Contains(r.Tool, "yarn") {
					suggestions = append(suggestions, "Run `yarn audit` to see details, then update vulnerable packages")
				} else if strings.Contains(r.Tool, "pnpm") {
					suggestions = append(suggestions, "Run `pnpm audit` to see details, then update vulnerable packages")
				} else {
					suggestions = append(suggestions, "Run `npm audit fix` to fix npm vulnerabilities")
				}
			case "php":
				suggestions = append(suggestions, "Run `composer update` to update vulnerable PHP packages")
			case "rust":
				suggestions = append(suggestions, "Run `cargo update` to update vulnerable Rust crates")
			case "python":
				suggestions = append(suggestions, "Update vulnerable Python packages listed in pip-audit output")
			}
		}
	}

	return suggestions
}

// HasSecurityIssues checks if there are any security vulnerabilities.
func HasSecurityIssues(results []SecurityResult) bool {
	for _, r := range results {
		if r.Available && r.Vulnerabilities > 0 {
			return true
		}
	}
	return false
}
