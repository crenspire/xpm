package doctor

// Conflict represents a detected configuration conflict.
type Conflict struct {
	Type        ConflictType
	Description string
	Files       []string
	Suggestion  string
}

// ConflictType categorizes conflicts.
type ConflictType int

const (
	ConflictTypeLockFile ConflictType = iota
	ConflictTypeBuildSystem
	ConflictTypeEnvManager
)

// ConflictRule defines a conflict detection rule.
type ConflictRule struct {
	Files       []string
	Type        ConflictType
	Description string
	Suggestion  string
}

// conflictRules defines all conflict detection rules.
var conflictRules = []ConflictRule{
	// Node.js lock file conflicts
	{
		Files:       []string{"package-lock.json", "yarn.lock"},
		Type:        ConflictTypeLockFile,
		Description: "Conflicting lockfiles: package-lock.json and yarn.lock",
		Suggestion:  "Remove one lockfile to avoid dependency confusion. Keep the one matching your team's package manager.",
	},
	{
		Files:       []string{"package-lock.json", "pnpm-lock.yaml"},
		Type:        ConflictTypeLockFile,
		Description: "Conflicting lockfiles: package-lock.json and pnpm-lock.yaml",
		Suggestion:  "Remove one lockfile. Use either npm or pnpm, not both.",
	},
	{
		Files:       []string{"yarn.lock", "pnpm-lock.yaml"},
		Type:        ConflictTypeLockFile,
		Description: "Conflicting lockfiles: yarn.lock and pnpm-lock.yaml",
		Suggestion:  "Remove one lockfile. Use either yarn or pnpm, not both.",
	},
	{
		Files:       []string{"package-lock.json", "bun.lockb"},
		Type:        ConflictTypeLockFile,
		Description: "Conflicting lockfiles: package-lock.json and bun.lockb",
		Suggestion:  "Remove one lockfile. Use either npm or bun, not both.",
	},
	{
		Files:       []string{"yarn.lock", "bun.lockb"},
		Type:        ConflictTypeLockFile,
		Description: "Conflicting lockfiles: yarn.lock and bun.lockb",
		Suggestion:  "Remove one lockfile. Use either yarn or bun, not both.",
	},

	// Python environment conflicts
	{
		Files:       []string{"requirements.txt", "Pipfile"},
		Type:        ConflictTypeEnvManager,
		Description: "Multiple Python dependency managers: requirements.txt and Pipfile",
		Suggestion:  "Consider using one dependency format. Pipfile (pipenv) is more modern.",
	},
	{
		Files:       []string{"requirements.txt", "pyproject.toml"},
		Type:        ConflictTypeEnvManager,
		Description: "Multiple Python dependency formats: requirements.txt and pyproject.toml",
		Suggestion:  "pyproject.toml is the modern standard. Consider migrating from requirements.txt.",
	},
	{
		Files:       []string{"Pipfile", "pyproject.toml"},
		Type:        ConflictTypeEnvManager,
		Description: "Multiple Python dependency managers: Pipfile and pyproject.toml",
		Suggestion:  "Choose one: pyproject.toml (with poetry) or Pipfile (with pipenv).",
	},

	// Java build system conflicts
	{
		Files:       []string{"pom.xml", "build.gradle"},
		Type:        ConflictTypeBuildSystem,
		Description: "Multiple Java build systems: Maven (pom.xml) and Gradle (build.gradle)",
		Suggestion:  "Use one build system. Having both causes confusion and duplicate configurations.",
	},
	{
		Files:       []string{"pom.xml", "build.gradle.kts"},
		Type:        ConflictTypeBuildSystem,
		Description: "Multiple Java build systems: Maven (pom.xml) and Gradle (build.gradle.kts)",
		Suggestion:  "Use one build system. Having both causes confusion and duplicate configurations.",
	},
}

// DetectConflicts checks for conflicting configurations.
func DetectConflicts(projectResult ProjectScanResult) []Conflict {
	var conflicts []Conflict

	for _, rule := range conflictRules {
		if matchesRule(projectResult, rule) {
			conflicts = append(conflicts, Conflict{
				Type:        rule.Type,
				Description: rule.Description,
				Files:       rule.Files,
				Suggestion:  rule.Suggestion,
			})
		}
	}

	return conflicts
}

// matchesRule checks if all files in a rule exist.
func matchesRule(result ProjectScanResult, rule ConflictRule) bool {
	for _, file := range rule.Files {
		if !HasFile(result, file) {
			return false
		}
	}
	return true
}

// PrintConflictsReport prints the conflicts check results.
func PrintConflictsReport(conflicts []Conflict) {
	Section("Conflicts")

	if len(conflicts) == 0 {
		NoIssues()
		return
	}

	for _, c := range conflicts {
		Warn(c.Description)
	}
}

// CountConflicts returns the number of conflicts.
func CountConflicts(conflicts []Conflict) int {
	return len(conflicts)
}

// GetConflictSuggestions returns suggestions for all conflicts.
func GetConflictSuggestions(conflicts []Conflict) []string {
	suggestions := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		if c.Suggestion != "" {
			suggestions = append(suggestions, c.Suggestion)
		}
	}
	return suggestions
}

// HasLockFileConflicts checks if there are any lock file conflicts.
func HasLockFileConflicts(conflicts []Conflict) bool {
	for _, c := range conflicts {
		if c.Type == ConflictTypeLockFile {
			return true
		}
	}
	return false
}

