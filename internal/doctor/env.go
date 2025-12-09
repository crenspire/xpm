package doctor

import (
	"os/exec"
	"regexp"
	"strings"
)

// RuntimeInfo holds information about a detected runtime.
type RuntimeInfo struct {
	Name      string
	Binary    string
	Version   string
	Installed bool
}

// RuntimeCheck defines how to detect a runtime.
type RuntimeCheck struct {
	Name       string
	Binary     string
	Args       []string
	VersionRe  *regexp.Regexp
	AltBinary  string   // Alternative binary to check
	AltArgs    []string // Args for alternative binary
}

// runtimeChecks defines all runtimes to check.
var runtimeChecks = []RuntimeCheck{
	{
		Name:      "Node.js",
		Binary:    "node",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`v?(\d+\.\d+\.\d+)`),
	},
	{
		Name:      "Python",
		Binary:    "python3",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`Python (\d+\.\d+\.\d+)`),
		AltBinary: "python",
		AltArgs:   []string{"--version"},
	},
	{
		Name:      "PHP",
		Binary:    "php",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`PHP (\d+\.\d+\.\d+)`),
	},
	{
		Name:      "Go",
		Binary:    "go",
		Args:      []string{"version"},
		VersionRe: regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`),
	},
	{
		Name:      "Rust",
		Binary:    "rustc",
		Args:      []string{"--version"},
		VersionRe: regexp.MustCompile(`rustc (\d+\.\d+\.\d+)`),
	},
	{
		Name:      "Java (JDK)",
		Binary:    "java",
		Args:      []string{"-version"},
		VersionRe: regexp.MustCompile(`(?:version|openjdk) "?(\d+(?:\.\d+)*)`),
	},
}

// CheckEnvironment checks all configured runtimes.
func CheckEnvironment() []RuntimeInfo {
	results := make([]RuntimeInfo, 0, len(runtimeChecks))

	for _, check := range runtimeChecks {
		info := checkRuntime(check)
		results = append(results, info)
	}

	return results
}

// checkRuntime checks a single runtime.
func checkRuntime(check RuntimeCheck) RuntimeInfo {
	info := RuntimeInfo{
		Name:   check.Name,
		Binary: check.Binary,
	}

	// Try primary binary
	version, ok := getVersion(check.Binary, check.Args, check.VersionRe)
	if ok {
		info.Installed = true
		info.Version = version
		return info
	}

	// Try alternative binary if specified
	if check.AltBinary != "" {
		args := check.AltArgs
		if args == nil {
			args = check.Args
		}
		version, ok = getVersion(check.AltBinary, args, check.VersionRe)
		if ok {
			info.Installed = true
			info.Version = version
			info.Binary = check.AltBinary
			return info
		}
	}

	info.Installed = false
	return info
}

// getVersion executes a command and extracts the version.
func getVersion(binary string, args []string, versionRe *regexp.Regexp) (string, bool) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", false
	}

	cmd := exec.Command(path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Some commands (like java -version) write to stderr and exit 0
		// but CombinedOutput captures both, so we try to parse anyway
		if len(output) == 0 {
			return "", false
		}
	}

	matches := versionRe.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", false
	}

	return matches[1], true
}

// PrintEnvironmentReport prints the environment check results.
func PrintEnvironmentReport(results []RuntimeInfo) {
	Section("Environment")

	for _, info := range results {
		if info.Installed {
			StatusLine(true, info.Name, "v"+info.Version)
		} else {
			StatusLine(false, info.Name, "missing")
		}
	}
}

// CountEnvResults counts installed and missing runtimes.
func CountEnvResults(results []RuntimeInfo) (installed, missing int) {
	for _, info := range results {
		if info.Installed {
			installed++
		} else {
			missing++
		}
	}
	return
}

// GetMissingRuntimes returns names of missing runtimes.
func GetMissingRuntimes(results []RuntimeInfo) []string {
	var missing []string
	for _, info := range results {
		if !info.Installed {
			missing = append(missing, info.Name)
		}
	}
	return missing
}

// HasRuntime checks if a specific runtime is installed.
func HasRuntime(results []RuntimeInfo, name string) bool {
	name = strings.ToLower(name)
	for _, info := range results {
		if strings.ToLower(info.Name) == name || strings.ToLower(info.Binary) == name {
			return info.Installed
		}
	}
	return false
}

