package doctor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AuditStatus is the outcome of one security audit.
type AuditStatus int

const (
	// AuditOK: the tool ran and its output reports no vulnerabilities.
	AuditOK AuditStatus = iota
	// AuditVulnerable: the tool ran and reported vulnerabilities.
	AuditVulnerable
	// AuditNotInstalled: the audit tool is not on PATH; nothing was run.
	AuditNotInstalled
	// AuditUnavailable: the tool ran but failed, or its output could not be
	// parsed. The project's vulnerability status is unknown.
	AuditUnavailable
)

// SecurityResult holds the result of a security audit.
type SecurityResult struct {
	Ecosystem       string
	Tool            string
	Status          AuditStatus
	Vulnerabilities int
	HighSeverity    int // high + critical
	MediumSeverity  int // moderate / medium
	LowSeverity     int // low + info
	Summary         string
}

// severities accumulates vulnerability counts from an audit report.
// skipped counts dependencies the tool reported but could not audit.
type severities struct {
	total, high, medium, low int
	skipped                  int
}

func (s *severities) add(severity string) {
	s.total++
	switch strings.ToLower(severity) {
	case "critical", "high":
		s.high++
	case "moderate", "medium":
		s.medium++
	default:
		s.low++
	}
}

// auditSpec describes how to run one audit tool and read its output.
type auditSpec struct {
	ecosystem string
	tool      string // shown to the user, e.g. "npm audit"
	binary    string // looked up on PATH
	name      string // command actually run
	args      []string
	okExit    func(code int) bool // exit codes that still carry a valid report
	parse     func(stdout []byte) (severities, error)
	note      string // appended to the summary, e.g. what was audited
}

func exitZeroOrOne(code int) bool { return code == 0 || code == 1 }

// RunSecurityAudit runs the audit tool of each ecosystem detected in dir.
func RunSecurityAudit(dir string, project ProjectScanResult) []SecurityResult {
	var results []SecurityResult

	if HasFile(project, "package.json") {
		switch {
		case HasFile(project, "yarn.lock"):
			if isYarnBerry(filepath.Join(dir, "yarn.lock")) {
				results = append(results, SecurityResult{
					Ecosystem: "node", Tool: "yarn audit", Status: AuditUnavailable,
					Summary: "unavailable: yarn 2+ projects are audited with `yarn npm audit`, whose output xpm does not read",
				})
			} else {
				results = append(results, runAudit(dir, yarnAudit))
			}
		case HasFile(project, "pnpm-lock.yaml"):
			results = append(results, runAudit(dir, pnpmAudit))
		case HasFile(project, "bun.lock") || HasFile(project, "bun.lockb"):
			results = append(results, runAudit(dir, bunAudit))
		default:
			results = append(results, runAudit(dir, npmAudit))
		}
	}

	if HasFile(project, "requirements.txt") {
		spec := pipAudit
		spec.args = []string{"-r", "requirements.txt", "-f", "json"}
		results = append(results, runAudit(dir, spec))
	} else if HasFile(project, "pyproject.toml") {
		spec := pipAudit
		spec.args = []string{"-f", "json"}
		spec.note = "audited the active Python environment"
		results = append(results, runAudit(dir, spec))
	}

	if HasFile(project, "composer.json") {
		results = append(results, runAudit(dir, composerAudit))
	}

	if HasFile(project, "Cargo.toml") {
		results = append(results, runAudit(dir, cargoAudit))
	}

	return results
}

var (
	npmAudit = auditSpec{
		ecosystem: "node", tool: "npm audit", binary: "npm", name: "npm",
		args: []string{"audit", "--json"}, okExit: exitZeroOrOne, parse: parseNpmAudit,
	}
	pnpmAudit = auditSpec{
		ecosystem: "node", tool: "pnpm audit", binary: "pnpm", name: "pnpm",
		args: []string{"audit", "--json"}, okExit: exitZeroOrOne, parse: parseNpmAudit,
	}
	// yarn v1 exits with a bitmask of the severities found (1 info … 16 critical).
	yarnAudit = auditSpec{
		ecosystem: "node", tool: "yarn audit", binary: "yarn", name: "yarn",
		args: []string{"audit", "--json"}, okExit: func(c int) bool { return c >= 0 && c < 32 }, parse: parseYarnAudit,
	}
	bunAudit = auditSpec{
		ecosystem: "node", tool: "bun audit", binary: "bun", name: "bun",
		args: []string{"audit", "--json"}, okExit: exitZeroOrOne, parse: parseBunAudit,
	}
	pipAudit = auditSpec{
		ecosystem: "python", tool: "pip-audit", binary: "pip-audit", name: "pip-audit",
		okExit: exitZeroOrOne, parse: parsePipAudit,
	}
	// composer audit exits with a bitmask: 1 vulnerable, 2 abandoned packages.
	composerAudit = auditSpec{
		ecosystem: "php", tool: "composer audit", binary: "composer", name: "composer",
		args: []string{"audit", "--format=json"}, okExit: func(c int) bool { return c >= 0 && c <= 3 }, parse: parseComposerAudit,
	}
	cargoAudit = auditSpec{
		ecosystem: "rust", tool: "cargo audit", binary: "cargo-audit", name: "cargo",
		args: []string{"audit", "--json"}, okExit: exitZeroOrOne, parse: parseCargoAudit,
	}
)

// runAudit runs one audit and classifies the outcome. A missing tool is
// AuditNotInstalled; a tool that cannot start, exits with an unexpected
// code, or prints output that does not parse is AuditUnavailable, never OK.
func runAudit(dir string, spec auditSpec) SecurityResult {
	r := SecurityResult{Ecosystem: spec.ecosystem, Tool: spec.tool}
	if _, err := lookPath(spec.binary); err != nil {
		r.Status = AuditNotInstalled
		r.Summary = spec.binary + " not installed"
		return r
	}
	out, code, err := runCommand(dir, spec.name, spec.args...)
	if err != nil {
		r.Status = AuditUnavailable
		r.Summary = "unavailable: " + err.Error()
		return r
	}
	if !spec.okExit(code) {
		r.Status = AuditUnavailable
		r.Summary = "unavailable: " + spec.tool + " exited with status " + strconv.Itoa(code)
		return r
	}
	sev, err := spec.parse(out)
	if err != nil {
		r.Status = AuditUnavailable
		r.Summary = "unavailable: " + err.Error()
		return r
	}
	r.Vulnerabilities = sev.total
	r.HighSeverity = sev.high
	r.MediumSeverity = sev.medium
	r.LowSeverity = sev.low
	if sev.total == 0 {
		r.Status = AuditOK
		r.Summary = "no known vulnerabilities"
	} else {
		r.Status = AuditVulnerable
		r.Summary = formatSeverities(sev)
	}
	if sev.skipped > 0 {
		r.Summary += " (" + strconv.Itoa(sev.skipped) + " not audited)"
	}
	if spec.note != "" {
		r.Summary += " (" + spec.note + ")"
	}
	return r
}

func formatSeverities(s severities) string {
	text := strconv.Itoa(s.total) + " vulnerabilities"
	var parts []string
	if s.high > 0 {
		parts = append(parts, strconv.Itoa(s.high)+" high")
	}
	if s.medium > 0 {
		parts = append(parts, strconv.Itoa(s.medium)+" moderate")
	}
	if s.low > 0 {
		parts = append(parts, strconv.Itoa(s.low)+" low")
	}
	if len(parts) > 0 {
		text += " (" + strings.Join(parts, ", ") + ")"
	}
	return text
}

// npmSeverityCounts is metadata.vulnerabilities in npm (v6 and v7+) and
// pnpm audit reports.
type npmSeverityCounts struct {
	Info     int  `json:"info"`
	Low      int  `json:"low"`
	Moderate int  `json:"moderate"`
	High     int  `json:"high"`
	Critical int  `json:"critical"`
	Total    *int `json:"total"`
}

func (c npmSeverityCounts) severities() severities {
	s := severities{
		high:   c.High + c.Critical,
		medium: c.Moderate,
		low:    c.Low + c.Info,
	}
	s.total = s.high + s.medium + s.low
	if c.Total != nil && *c.Total > s.total {
		s.total = *c.Total
	}
	return s
}

// parseNpmAudit reads `npm audit --json` (v6 and v7+) and `pnpm audit --json`.
// The report must carry metadata.vulnerabilities; npm's error object
// ({"error": {...}}) is reported as an error.
func parseNpmAudit(out []byte) (severities, error) {
	var report struct {
		Error *struct {
			Code    string `json:"code"`
			Summary string `json:"summary"`
		} `json:"error"`
		Metadata *struct {
			Vulnerabilities *npmSeverityCounts `json:"vulnerabilities"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		return severities{}, fmt.Errorf("could not parse audit output: %w", err)
	}
	if report.Error != nil {
		msg := strings.TrimSpace(report.Error.Code + " " + firstLine(report.Error.Summary))
		return severities{}, fmt.Errorf("audit failed: %s", msg)
	}
	if report.Metadata == nil || report.Metadata.Vulnerabilities == nil {
		return severities{}, errors.New("audit output has no vulnerability summary")
	}
	return report.Metadata.Vulnerabilities.severities(), nil
}

// parseYarnAudit reads `yarn audit --json` (yarn v1): one JSON object per
// line, ending with an "auditSummary" object.
func parseYarnAudit(out []byte) (severities, error) {
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var entry struct {
			Type string `json:"type"`
			Data struct {
				Vulnerabilities *npmSeverityCounts `json:"vulnerabilities"`
			} `json:"data"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type == "auditSummary" && entry.Data.Vulnerabilities != nil {
			return entry.Data.Vulnerabilities.severities(), nil
		}
	}
	return severities{}, errors.New("yarn audit printed no auditSummary")
}

// parseBunAudit reads `bun audit --json`: an object mapping package names to
// lists of advisories, each with a "severity".
func parseBunAudit(out []byte) (severities, error) {
	var report map[string][]struct {
		Severity string `json:"severity"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		return severities{}, fmt.Errorf("could not parse bun audit output: %w", err)
	}
	if report == nil {
		return severities{}, errors.New("bun audit printed no report")
	}
	var s severities
	for _, advisories := range report {
		for _, a := range advisories {
			s.add(a.Severity)
		}
	}
	return s, nil
}

// pipAuditDependency is one entry of pip-audit's JSON report.
// SkipReason is set, without vulns, for a dependency pip-audit could not
// audit (not on PyPI, local or editable).
type pipAuditDependency struct {
	Name       string `json:"name"`
	SkipReason string `json:"skip_reason"`
	Vulns      []struct {
		ID string `json:"id"`
	} `json:"vulns"`
}

// parsePipAudit reads `pip-audit -f json` in both formats: the current object
// {"dependencies": [...], "fixes": [...]} and the legacy top-level array.
// pip-audit reports no severities, so every finding counts as low. Skipped
// dependencies are counted, not audited; a report that audited none is an
// error (unavailable), never "no known vulnerabilities".
func parsePipAudit(out []byte) (severities, error) {
	trimmed := bytes.TrimSpace(out)
	var deps []pipAuditDependency
	switch {
	case len(trimmed) > 0 && trimmed[0] == '[':
		if err := json.Unmarshal(trimmed, &deps); err != nil {
			return severities{}, fmt.Errorf("could not parse pip-audit output: %w", err)
		}
	default:
		var report struct {
			Dependencies *[]pipAuditDependency `json:"dependencies"`
		}
		if err := json.Unmarshal(trimmed, &report); err != nil {
			return severities{}, fmt.Errorf("could not parse pip-audit output: %w", err)
		}
		if report.Dependencies == nil {
			return severities{}, errors.New("pip-audit output has no dependencies list")
		}
		deps = *report.Dependencies
	}
	var s severities
	for _, d := range deps {
		if d.SkipReason != "" {
			s.skipped++
			continue
		}
		for range d.Vulns {
			s.add("")
		}
	}
	if s.skipped == len(deps) {
		if s.skipped > 0 {
			return severities{}, fmt.Errorf("pip-audit audited no dependencies (%d skipped)", s.skipped)
		}
		return severities{}, errors.New("pip-audit audited no dependencies")
	}
	return s, nil
}

// parseComposerAudit reads `composer audit --format=json`. "advisories" maps
// package names to advisory lists, or is [] when there are none (PHP encodes
// an empty array that way).
func parseComposerAudit(out []byte) (severities, error) {
	var report struct {
		Advisories json.RawMessage `json:"advisories"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		return severities{}, fmt.Errorf("could not parse composer audit output: %w", err)
	}
	raw := bytes.TrimSpace(report.Advisories)
	if len(raw) == 0 {
		return severities{}, errors.New("composer audit output has no advisories field")
	}
	type advisory struct {
		Severity string `json:"severity"`
	}
	var lists [][]advisory
	switch raw[0] {
	case '[':
		if err := json.Unmarshal(raw, &lists); err != nil {
			return severities{}, fmt.Errorf("could not parse composer advisories: %w", err)
		}
	case '{':
		var byPkg map[string][]advisory
		if err := json.Unmarshal(raw, &byPkg); err != nil {
			return severities{}, fmt.Errorf("could not parse composer advisories: %w", err)
		}
		for _, l := range byPkg {
			lists = append(lists, l)
		}
	default:
		return severities{}, errors.New("composer advisories is neither an object nor an array")
	}
	var s severities
	for _, l := range lists {
		for _, a := range l {
			s.add(a.Severity)
		}
	}
	return s, nil
}

// parseCargoAudit reads `cargo audit --json`. RustSec advisories carry a
// CVSS vector rather than a severity word, so findings count as low unless
// an advisory has an explicit "severity".
func parseCargoAudit(out []byte) (severities, error) {
	var report struct {
		Vulnerabilities *struct {
			Count int `json:"count"`
			List  []struct {
				Advisory struct {
					Severity string `json:"severity"`
				} `json:"advisory"`
			} `json:"list"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		return severities{}, fmt.Errorf("could not parse cargo audit output: %w", err)
	}
	if report.Vulnerabilities == nil {
		return severities{}, errors.New("cargo audit output has no vulnerabilities section")
	}
	var s severities
	for _, v := range report.Vulnerabilities.List {
		s.add(v.Advisory.Severity)
	}
	if report.Vulnerabilities.Count > s.total {
		s.low += report.Vulnerabilities.Count - s.total
		s.total = report.Vulnerabilities.Count
	}
	return s, nil
}

// isYarnBerry reports whether a yarn.lock was written by yarn 2+.
func isYarnBerry(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte("\n__metadata:")) || bytes.HasPrefix(data, []byte("__metadata:"))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// PrintSecurityReport prints the security audit results.
func PrintSecurityReport(results []SecurityResult) {
	Section("Security Scan")

	if len(results) == 0 {
		Info("No ecosystems to audit")
		return
	}

	for _, r := range results {
		label := r.Ecosystem + " (" + r.Tool + ")"
		switch r.Status {
		case AuditOK:
			StatusLine(true, label, r.Summary)
		case AuditVulnerable:
			switch {
			case r.HighSeverity > 0:
				Bad(label + ": " + r.Summary)
			case r.MediumSeverity > 0:
				Warn(label + ": " + r.Summary)
			default:
				WarnLine(label, r.Summary)
			}
		default:
			WarnLine(label, r.Summary)
		}
	}
}

// CountSecurityIssues counts audits that passed, found vulnerabilities, or
// could not run (not installed or unavailable).
func CountSecurityIssues(results []SecurityResult) (ok, vulnerable, unavailable int) {
	for _, r := range results {
		switch r.Status {
		case AuditOK:
			ok++
		case AuditVulnerable:
			vulnerable++
		default:
			unavailable++
		}
	}
	return
}

// GetSecuritySuggestions returns suggestions for security issues.
func GetSecuritySuggestions(results []SecurityResult) []string {
	var suggestions []string

	for _, r := range results {
		switch r.Status {
		case AuditNotInstalled:
			switch r.Ecosystem {
			case "rust":
				suggestions = append(suggestions, "Install cargo-audit for Rust security scanning: cargo install cargo-audit")
			case "python":
				suggestions = append(suggestions, "Install pip-audit for Python security scanning: pip install pip-audit")
			}
		case AuditUnavailable:
			suggestions = append(suggestions, "Run `"+r.Tool+"` yourself to see why the audit failed")
		case AuditVulnerable:
			switch r.Ecosystem {
			case "node":
				switch {
				case strings.Contains(r.Tool, "yarn"):
					suggestions = append(suggestions, "Run `yarn audit` to see details, then update vulnerable packages")
				case strings.Contains(r.Tool, "pnpm"):
					suggestions = append(suggestions, "Run `pnpm audit` to see details, then update vulnerable packages")
				case strings.Contains(r.Tool, "bun"):
					suggestions = append(suggestions, "Run `bun audit` to see details, then update vulnerable packages")
				default:
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

// HasSecurityIssues reports whether any audit found vulnerabilities. Audits
// that could not run are warnings, not failures.
func HasSecurityIssues(results []SecurityResult) bool {
	for _, r := range results {
		if r.Status == AuditVulnerable {
			return true
		}
	}
	return false
}
