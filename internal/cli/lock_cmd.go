package cli

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/crenspire/xpm/internal/lock"
)

// cmdLock handles the `xpm lock` command.
// Without flags, it generates xpm-lock.yaml.
// With --verify, it verifies the current lockfile state against the saved lock.
func cmdLock(args []string) int {
	fs := flag.NewFlagSet("lock", flag.ContinueOnError)
	verify := fs.Bool("verify", false, "verify lockfiles against xpm-lock.yaml")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr)
		showCommandUsage("lock")
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error getting current directory:", err)
		return 1
	}

	if *verify {
		return verifyLock(cwd)
	}

	return generateLock(cwd)
}

// generateLock generates a new xpm-lock.yaml file.
func generateLock(dir string) int {
	fmt.Println("Scanning for lockfiles...")
	fmt.Println()

	unified, err := lock.Generate(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error generating lock:", err)
		return 1
	}

	if unified.IsEmpty() {
		fmt.Println("No lockfiles found.")
		fmt.Println()
		fmt.Println("Supported lockfiles:")
		for _, file := range lock.ListSupportedFiles() {
			fmt.Printf("  - %s\n", file)
		}
		return 0
	}

	if err := lock.WriteUnifiedLock(dir, unified); err != nil {
		fmt.Fprintln(os.Stderr, "error writing lock file:", err)
		return 1
	}

	fmt.Printf("Generated %s\n", lock.LockfileName)
	fmt.Println()
	fmt.Println("Included:")

	// Sort keys for consistent output
	keys := make([]string, 0, len(unified.Locks))
	for k := range unified.Locks {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		info := unified.Locks[key]
		name := formatEcosystemName(key, info.Manager)
		fmt.Printf("  - %s: %d packages\n", name, info.PackageCnt)
	}

	fmt.Println()
	fmt.Printf("Total: %d lockfiles, %d packages\n", unified.Count(), unified.TotalPackages())

	return 0
}

// verifyLock verifies lockfiles against the saved xpm-lock.yaml.
func verifyLock(dir string) int {
	if !lock.UnifiedLockExists(dir) {
		fmt.Fprintf(os.Stderr, "%s not found.\n", lock.LockfileName)
		fmt.Fprintln(os.Stderr, "Run 'xpm lock' to generate it first.")
		return 1
	}

	fmt.Println("Verifying lock state...")
	fmt.Println()

	results, err := lock.Verify(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error verifying lock:", err)
		return 1
	}

	if len(results) == 0 {
		fmt.Println("No lockfiles to verify.")
		return 0
	}

	// Sort results for consistent output
	sort.Slice(results, func(i, j int) bool {
		return results[i].Key < results[j].Key
	})

	for _, r := range results {
		switch r.Status {
		case lock.StatusUnchanged:
			fmt.Printf("✔ %s unchanged\n", r.File)
		case lock.StatusChanged:
			fmt.Printf("✘ %s changed\n", r.File)
		case lock.StatusMissing:
			fmt.Printf("✘ %s missing\n", r.File)
		case lock.StatusError:
			fmt.Printf("✘ %s error: %v\n", r.File, r.Error)
		}
	}

	fmt.Println()

	if lock.VerificationPassed(results) {
		fmt.Println("Verification PASSED.")
		return 0
	}

	fmt.Println("Verification FAILED.")
	fmt.Println()
	fmt.Println("Run 'xpm lock' to update the unified lock file.")
	return 1
}

// formatEcosystemName returns a human-readable name for an ecosystem.
func formatEcosystemName(ecosystem, manager string) string {
	names := map[string]string{
		"node":     "Node.js",
		"python":   "Python",
		"rust":     "Rust",
		"go":       "Go",
		"composer": "PHP",
		"gradle":   "Gradle",
	}

	name, ok := names[ecosystem]
	if !ok {
		// Capitalize first letter
		if len(ecosystem) > 0 {
			name = strings.ToUpper(ecosystem[:1]) + ecosystem[1:]
		} else {
			name = ecosystem
		}
	}

	if manager != "" && manager != ecosystem {
		return fmt.Sprintf("%s (%s)", name, manager)
	}
	return name
}
