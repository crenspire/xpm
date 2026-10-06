package cli

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/crenspire/xpm/internal/lock"
)

// cmdLock handles `xpm lock` (write xpm-lock.yaml) and `xpm lock --verify`.
// Results go to stdout; errors and warnings go to stderr.
func cmdLock(args []string) int {
	fs := flag.NewFlagSet("lock", flag.ContinueOnError)
	verify := fs.Bool("verify", false, "verify lockfiles against xpm-lock.yaml")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr)
		showCommandUsage("lock")
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "lock: unexpected argument %q\n", fs.Arg(0))
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

// generateLock writes xpm-lock.yaml for dir, leaving the file untouched when
// its content would not change.
func generateLock(dir string) int {
	unified, warnings, err := lock.Generate(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error generating lock:", err)
		return 1
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}

	if unified.IsEmpty() {
		fmt.Println("No lockfiles found in the project root.")
		fmt.Println()
		fmt.Println("Supported lockfiles:")
		for _, file := range lock.ListSupportedFiles() {
			fmt.Printf("  - %s\n", file)
		}
		return 0
	}

	changed, err := lock.WriteUnifiedLock(dir, unified)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error writing lock file:", err)
		return 1
	}
	if changed {
		fmt.Printf("Generated %s\n", lock.LockfileName)
	} else {
		fmt.Printf("%s is up to date\n", lock.LockfileName)
	}
	fmt.Println()

	paths := make([]string, 0, len(unified.Locks))
	for p := range unified.Locks {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		info := unified.Locks[p]
		fmt.Printf("  %s (%s): %d packages\n", p, info.Manager, info.Packages)
	}
	fmt.Println()
	fmt.Printf("Total: %d lockfiles, %d packages\n", unified.Count(), unified.TotalPackages())
	return 0
}

// verifyLock checks the lockfiles in dir against xpm-lock.yaml. It returns 1
// when any lockfile changed, disappeared, appeared, or could not be checked.
func verifyLock(dir string) int {
	if !lock.UnifiedLockExists(dir) {
		fmt.Fprintf(os.Stderr, "%s not found.\n", lock.LockfileName)
		fmt.Fprintln(os.Stderr, "Run 'xpm lock' to generate it first.")
		return 1
	}

	results, err := lock.Verify(dir) // sorted by path
	if err != nil {
		fmt.Fprintln(os.Stderr, "error verifying lock:", err)
		return 1
	}
	if len(results) == 0 {
		fmt.Println("No lockfiles to verify.")
		return 0
	}

	for _, r := range results {
		fmt.Println(formatVerifyLine(r))
	}
	fmt.Println()

	if lock.VerificationPassed(results) {
		fmt.Println("Verification PASSED.")
		return 0
	}
	fmt.Println("Verification FAILED.")
	fmt.Printf("Run 'xpm lock' to update %s.\n", lock.LockfileName)
	return 1
}

// formatVerifyLine renders one verification result.
func formatVerifyLine(r lock.VerificationResult) string {
	switch r.Status {
	case lock.StatusUnchanged:
		return fmt.Sprintf("✔ %s unchanged", r.Key)
	case lock.StatusChanged:
		return fmt.Sprintf("✘ %s changed", r.Key)
	case lock.StatusMissing:
		return fmt.Sprintf("✘ %s missing", r.Key)
	case lock.StatusAdded:
		return fmt.Sprintf("✘ %s added (not in %s)", r.Key, lock.LockfileName)
	default:
		return fmt.Sprintf("✘ %s error: %v", r.Key, r.Error)
	}
}
