package doctor

import (
	"os/exec"
	"regexp"
)

// PMInfo holds information about a detected package manager.
type PMInfo struct {
	Name      string
	Binary    string
	Version   string
	Installed bool
	Optional  bool // True if this is an optional tool (like cargo-audit)
}

// PMCheck defines how to detect a package manager.
type PMCheck struct {
	Name      string
	Binary    string
	Args      []string
	VersionRe *regexp.Regexp
	Optional  bool
}

// pmChecks defines all package managers to check.
var pmChecks = []PMCheck{
	// Node.js ecosystem
	{
		Name:      "npm",
		Binary:    "npm",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`(\d+\.\d+\.\d+)`),
	},
	{
		Name:      "yarn",
		Binary:    "yarn",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`(\d+\.\d+\.\d+)`),
	},
	{
		Name:      "pnpm",
		Binary:    "pnpm",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`(\d+\.\d+\.\d+)`),
	},
	{
		Name:      "bun",
		Binary:    "bun",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`(\d+\.\d+\.\d+)`),
	},
	// PHP ecosystem
	{
		Name:      "composer",
		Binary:    "composer",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`Composer.*?(\d+\.\d+\.\d+)`),
	},
	// Python ecosystem
	{
		Name:      "pip",
		Binary:    "pip3",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`pip (\d+\.\d+(?:\.\d+)?)`),
	},
	{
		Name:      "poetry",
		Binary:    "poetry",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`Poetry.*?(\d+\.\d+\.\d+)`),
	},
	{
		Name:      "pipenv",
		Binary:    "pipenv",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`pipenv.*?(\d+\.\d+\.\d+)`),
	},
	// Rust ecosystem
	{
		Name:      "cargo",
		Binary:    "cargo",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`cargo (\d+\.\d+\.\d+)`),
	},
	{
		Name:      "cargo-audit",
		Binary:    "cargo-audit",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`cargo-audit (\d+\.\d+\.\d+)`),
		Optional:  true,
	},
	// Go ecosystem
	{
		Name:      "go",
		Binary:    "go",
		Args:      []string{"version"},
		VersionRe: regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`),
	},
	// Java ecosystem
	{
		Name:      "maven",
		Binary:    "mvn",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`Apache Maven (\d+\.\d+\.\d+)`),
	},
	{
		Name:      "gradle",
		Binary:    "gradle",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`Gradle (\d+\.\d+(?:\.\d+)?)`),
	},
}

// CheckPackageManagers checks all configured package managers.
func CheckPackageManagers() []PMInfo {
	results := make([]PMInfo, 0, len(pmChecks))

	for _, check := range pmChecks {
		info := checkPM(check)
		results = append(results, info)
	}

	return results
}

// checkPM checks a single package manager.
func checkPM(check PMCheck) PMInfo {
	info := PMInfo{
		Name:     check.Name,
		Binary:   check.Binary,
		Optional: check.Optional,
	}

	path, err := lookPath(check.Binary)
	if err != nil {
		info.Installed = false
		return info
	}

	cmd := exec.Command(path, check.Args...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		info.Installed = false
		return info
	}

	info.Installed = true

	matches := check.VersionRe.FindStringSubmatch(string(output))
	if len(matches) >= 2 {
		info.Version = matches[1]
	}

	return info
}

// PrintPMReport prints the package manager check results.
func PrintPMReport(results []PMInfo) {
	Section("Package Managers")

	for _, info := range results {
		if info.Installed {
			detail := ""
			if info.Version != "" {
				detail = "v" + info.Version
			}
			StatusLine(true, info.Name, detail)
		} else {
			if info.Optional {
				WarnLine(info.Name, "(optional, missing)")
			} else {
				StatusLine(false, info.Name, "(missing)")
			}
		}
	}
}

// CountPMResults counts installed and missing package managers.
func CountPMResults(results []PMInfo) (installed, missing, optionalMissing int) {
	for _, info := range results {
		if info.Installed {
			installed++
		} else if info.Optional {
			optionalMissing++
		} else {
			missing++
		}
	}
	return
}

// GetMissingPMs returns names of missing (non-optional) package managers.
func GetMissingPMs(results []PMInfo) []string {
	var missing []string
	for _, info := range results {
		if !info.Installed && !info.Optional {
			missing = append(missing, info.Name)
		}
	}
	return missing
}

// GetMissingOptionalPMs returns names of missing optional tools.
func GetMissingOptionalPMs(results []PMInfo) []string {
	var missing []string
	for _, info := range results {
		if !info.Installed && info.Optional {
			missing = append(missing, info.Name)
		}
	}
	return missing
}

// IsPMInstalled checks if a specific package manager is installed.
func IsPMInstalled(results []PMInfo, name string) bool {
	for _, info := range results {
		if info.Name == name || info.Binary == name {
			return info.Installed
		}
	}
	return false
}
