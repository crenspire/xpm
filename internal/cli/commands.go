package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// cmdInfo shows detailed information about a package.
func cmdInfo(args []string) int {
	pkg, ok := exactlyOneArg("info", args)
	if !ok {
		return 1
	}
	fmt.Printf("Searching for %q...\n\n", pkg)

	searchOpts := search.OptionsFromConfig(cfg)

	rep, err := lookupReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	results := rep.Results
	st := classify(rep, searchOpts)
	if len(results) == 0 {
		fmt.Printf("No package found matching %q\n", pkg)
		fmt.Print(formatAvailability(st))
		return 1
	}

	fmt.Printf("Package: %s\n", pkg)
	fmt.Println(strings.Repeat("=", 40))

	for _, r := range results {
		fmt.Printf("\n📦 %s (%s)\n", r.Name, r.Manager)
		if version, ok := r.Extra["version"]; ok && version != "" {
			fmt.Printf("   Version: %s\n", version)
		}
		if r.Info != "" {
			fmt.Printf("   Description: %s\n", r.Info)
		}

		// Show ecosystem-specific info
		switch r.Manager {
		case pm.Npm:
			fmt.Printf("   Registry: https://www.npmjs.com/package/%s\n", r.Name)
			fmt.Printf("   Install: npm install %s\n", r.Name)
		case pm.Pip:
			fmt.Printf("   Registry: https://pypi.org/project/%s\n", r.Name)
			fmt.Printf("   Install: pip install %s\n", r.Name)
		case pm.Composer:
			fmt.Printf("   Registry: https://packagist.org/packages/%s\n", r.Name)
			fmt.Printf("   Install: composer require %s\n", r.Name)
		case pm.Cargo:
			fmt.Printf("   Registry: https://crates.io/crates/%s\n", r.Name)
			fmt.Printf("   Install: cargo add %s\n", r.Name)
		case pm.Maven:
			if group, ok := r.Extra["group"]; ok {
				fmt.Printf("   Group: %s\n", group)
			}
			if artifact, ok := r.Extra["artifact"]; ok {
				fmt.Printf("   Artifact: %s\n", artifact)
			}
			fmt.Printf("   Registry: https://central.sonatype.com/\n")
		}
	}

	fmt.Print(formatAvailability(registryStatus{Unavailable: st.Unavailable}))
	fmt.Println()
	return 0
}

// runBinaryWithCode runs a binary and returns its exit code.
func runBinaryWithCode(bin string, args []string) int {
	logx.Info("running: %s %v", bin, args)
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}
