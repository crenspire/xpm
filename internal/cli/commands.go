package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
	"github.com/manifoldco/promptui"
)

// cmdList lists installed packages for the detected project.
func cmdList(args []string) int {
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println("No known dependency files found (package.json, composer.json, etc).")
		return 1
	}

	// Select target if multiple
	var target projectTarget
	if len(targets) == 1 {
		target = targets[0]
	} else if cfg.Interactive {
		items := []string{}
		for _, t := range targets {
			items = append(items, t.Label)
		}

		prompt := promptui.Select{
			Label: "Select project to list packages for",
			Items: items,
		}

		idx, _, err := prompt.Run()
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		target = targets[idx]
	} else {
		target = targets[0]
	}

	fmt.Printf("Listing packages for %s\n\n", target.Label)

	// Get the appropriate PM
	chosenPM := target.PMs[0]
	if len(target.PMs) > 1 {
		preferred := pickPreferredPM(target.PMs, cfg.Prefer)
		if preferred != "" {
			chosenPM = preferred
		}
	}

	meta, ok := pm.MetaFor(chosenPM)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown package manager: %s\n", chosenPM)
		return 1
	}

	if !pm.Exists(meta.Binary) {
		fmt.Printf("%s (%s) is not installed.\n", meta.Name, meta.Binary)
		return 1
	}

	// Run list command based on project type
	var listArgs []string
	switch target.Kind {
	case "node":
		listArgs = []string{"list", "--depth=0"}
	case "composer":
		listArgs = []string{"show"}
	case "cargo":
		listArgs = []string{"pkgid"}
		fmt.Println("Note: Use 'cargo tree' for a full dependency tree.")
	case "gomod":
		listArgs = []string{"list", "-m", "all"}
	case "maven":
		listArgs = []string{"dependency:list"}
	case "gradle":
		listArgs = []string{"dependencies", "--configuration", "implementation"}
	case "pip-req", "pip-pyproject":
		listArgs = []string{"list"}
	default:
		fmt.Printf("List not implemented for %s\n", target.Kind)
		return 1
	}

	return runBinaryWithCode(meta.Binary, listArgs)
}

// cmdUpdate updates packages in the detected project.
func cmdUpdate(args []string) int {
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println("No known dependency files found (package.json, composer.json, etc).")
		return 1
	}

	// Select target if multiple
	var target projectTarget
	if len(targets) == 1 {
		target = targets[0]
	} else if cfg.Interactive {
		items := []string{}
		for _, t := range targets {
			items = append(items, t.Label)
		}

		prompt := promptui.Select{
			Label: "Select project to update",
			Items: items,
		}

		idx, _, err := prompt.Run()
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		target = targets[idx]
	} else {
		target = targets[0]
	}

	fmt.Printf("Updating packages for %s\n\n", target.Label)

	// Get the appropriate PM
	chosenPM := target.PMs[0]
	if len(target.PMs) > 1 {
		preferred := pickPreferredPM(target.PMs, cfg.Prefer)
		if preferred != "" {
			chosenPM = preferred
		}
	}

	meta, ok := pm.MetaFor(chosenPM)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown package manager: %s\n", chosenPM)
		return 1
	}

	if !pm.Exists(meta.Binary) {
		fmt.Printf("%s (%s) is not installed.\n", meta.Name, meta.Binary)
		return 1
	}

	// Build update command based on project type
	var updateArgs []string
	switch target.Kind {
	case "node":
		if len(args) > 0 {
			// Validate package name before passing to command
			if err := pm.ValidatePackageName(args[0], chosenPM); err != nil {
				fmt.Fprintf(os.Stderr, "Invalid package name: %v\n", err)
				return 1
			}
			updateArgs = []string{"update", args[0]}
		} else {
			updateArgs = []string{"update"}
		}
	case "composer":
		if len(args) > 0 {
			// Validate package name before passing to command
			if err := pm.ValidatePackageName(args[0], chosenPM); err != nil {
				fmt.Fprintf(os.Stderr, "Invalid package name: %v\n", err)
				return 1
			}
			updateArgs = []string{"update", args[0]}
		} else {
			updateArgs = []string{"update"}
		}
	case "cargo":
		updateArgs = []string{"update"}
	case "gomod":
		if len(args) > 0 {
			// Validate package name before passing to command
			if err := pm.ValidatePackageName(args[0], chosenPM); err != nil {
				fmt.Fprintf(os.Stderr, "Invalid package name: %v\n", err)
				return 1
			}
			updateArgs = []string{"get", "-u", args[0]}
		} else {
			updateArgs = []string{"get", "-u", "./..."}
		}
	case "maven":
		updateArgs = []string{"versions:use-latest-releases"}
	case "gradle":
		fmt.Println("Gradle doesn't have a built-in update command.")
		fmt.Println("Consider using the Gradle Versions Plugin.")
		return 0
	case "pip-req", "pip-pyproject":
		if len(args) > 0 {
			// Validate package name before passing to command
			if err := pm.ValidatePackageName(args[0], chosenPM); err != nil {
				fmt.Fprintf(os.Stderr, "Invalid package name: %v\n", err)
				return 1
			}
			updateArgs = []string{"install", "--upgrade", args[0]}
		} else {
			updateArgs = []string{"install", "--upgrade", "-r", "requirements.txt"}
		}
	default:
		fmt.Printf("Update not implemented for %s\n", target.Kind)
		return 1
	}

	return runBinaryWithCode(meta.Binary, updateArgs)
}

// cmdRemove removes a package from the current project.
func cmdRemove(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: missing package name")
		fmt.Fprintln(os.Stderr)
		showCommandUsage("remove")
		return 1
	}

	pkg := args[0]

	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println("No known dependency files found (package.json, composer.json, etc).")
		return 1
	}

	// Select target if multiple
	var target projectTarget
	if len(targets) == 1 {
		target = targets[0]
	} else if cfg.Interactive {
		items := []string{}
		for _, t := range targets {
			items = append(items, t.Label)
		}

		prompt := promptui.Select{
			Label: "Select project to remove package from",
			Items: items,
		}

		idx, _, err := prompt.Run()
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		target = targets[idx]
	} else {
		target = targets[0]
	}

	// Get the appropriate PM
	chosenPM := target.PMs[0]
	if len(target.PMs) > 1 {
		preferred := pickPreferredPM(target.PMs, cfg.Prefer)
		if preferred != "" {
			chosenPM = preferred
		}
	}

	meta, ok := pm.MetaFor(chosenPM)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown package manager: %s\n", chosenPM)
		return 1
	}

	if !pm.Exists(meta.Binary) {
		fmt.Printf("%s (%s) is not installed.\n", meta.Name, meta.Binary)
		return 1
	}

	// Validate package name before passing to command
	if err := pm.ValidatePackageName(pkg, chosenPM); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid package name: %v\n", err)
		return 1
	}

	fmt.Printf("Removing %s from %s using %s...\n\n", pkg, target.Label, meta.Name)

	// Build remove command based on project type
	var removeArgs []string
	switch target.Kind {
	case "node":
		removeArgs = []string{"uninstall", pkg}
	case "composer":
		removeArgs = []string{"remove", pkg}
	case "cargo":
		removeArgs = []string{"remove", pkg}
	case "gomod":
		// Go doesn't have a direct remove, but we can edit go.mod
		fmt.Println("To remove a Go dependency:")
		fmt.Printf("  1. Remove the import from your code\n")
		fmt.Printf("  2. Run: go mod tidy\n")
		return 0
	case "maven":
		fmt.Println("Maven dependencies are managed in pom.xml.")
		fmt.Printf("Remove the dependency for %s from your pom.xml.\n", pkg)
		return 0
	case "gradle":
		fmt.Println("Gradle dependencies are managed in build.gradle.")
		fmt.Printf("Remove the dependency for %s from your build.gradle.\n", pkg)
		return 0
	case "pip-req", "pip-pyproject":
		removeArgs = []string{"uninstall", "-y", pkg}
	default:
		fmt.Printf("Remove not implemented for %s\n", target.Kind)
		return 1
	}

	return runBinaryWithCode(meta.Binary, removeArgs)
}

// cmdInfo shows detailed information about a package.
func cmdInfo(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: missing package name")
		fmt.Fprintln(os.Stderr)
		showCommandUsage("info")
		return 1
	}

	pkg := args[0]

	fmt.Printf("Searching for %q...\n\n", pkg)

	searchOpts := search.OptionsFromConfig(cfg)

	results, err := search.SearchEverywhere(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}

	if len(results) == 0 {
		fmt.Printf("No package found matching %q\n", pkg)
		return 0
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
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}
