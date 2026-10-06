package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// Seams for tests: ensurePM is ensureManager, installPkg is installOne.
var (
	ensurePM   = ensureManager
	installPkg = installOne
)

func cmdInstall(args []string) int {
	ia, err := parseInstallArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		fmt.Fprintln(os.Stderr)
		showCommandUsage("install")
		return 1
	}
	if len(ia.Packages) == 0 {
		if ia.Global {
			fmt.Fprintln(os.Stderr, "error: -g/--global needs a package name (project dependencies are never global)")
			return 1
		}
		return autoInstallDetected()
	}
	for i, p := range ia.Packages {
		if len(ia.Packages) > 1 {
			fmt.Printf("[%d/%d] %s\n", i+1, len(ia.Packages), p)
		}
		if code := installPkg(p, ia.Global); code != 0 {
			if left := len(ia.Packages) - i - 1; left > 0 {
				fmt.Fprintf(os.Stderr, "Stopped at %s; %d remaining package(s) were not installed.\n", p, left)
			}
			return code
		}
	}
	return 0
}

// installOne resolves one "name[@version]" argument against the registries
// and installs it with the chosen tool.
func installOne(spec string, global bool) int {
	pkg, requestedVersion := parsePackageVersion(spec)
	if err := pm.ValidateGenericPackageName(pkg); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid package name: %v\n\n", err)
		showCommandUsage("install")
		return 1
	}
	if requestedVersion != "" {
		if err := pm.ValidateVersion(requestedVersion); err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid version: %v\n\n", err)
			showCommandUsage("install")
			return 1
		}
	}
	if isGoModulePath(pkg) {
		fmt.Printf("%s is a Go module path; using go modules.\n\n", pkg)
		return installCandidate(goModuleCandidate(pkg), pkg, requestedVersion, global)
	}
	if requestedVersion != "" {
		fmt.Printf("Searching for %q (version %s) across ecosystems...\n\n", pkg, requestedVersion)
	} else {
		fmt.Printf("Searching for %q across ecosystems...\n\n", pkg)
	}

	searchOpts := search.OptionsFromConfig(cfg)
	rep, err := lookupReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	st := classify(rep, searchOpts)
	if len(rep.Results) == 0 {
		fmt.Println("No matches found for", pkg)
		fmt.Print(formatAvailability(st))
		return 1
	}
	if len(st.Unavailable) > 0 {
		fmt.Print(formatAvailability(registryStatus{Unavailable: st.Unavailable}))
		fmt.Println()
	}

	cwd, _ := os.Getwd()
	cands := buildCandidates(rep.Results, pm.ProjectManagers(cwd))
	sortCandidates(cands, cfg.Prefer)
	chosen, ok := chooseCandidate(cands, pkg, rep.UnavailableIDs())
	if !ok {
		return 1
	}
	return installCandidate(chosen, pkg, requestedVersion, global)
}

// chooseCandidate picks the candidate for query. When some candidates are
// exact, closest matches (a registry's unrelated first hit) do not count:
// a single exact candidate is picked automatically if every registry
// answered, and the menu lists exact candidates first. Without a terminal
// decideNonInteractive decides or refuses.
func chooseCandidate(cands []candidate, query string, unavailable []pm.ID) (candidate, bool) {
	exact, _ := splitExact(query, cands)
	primary := cands
	if len(exact) > 0 {
		primary = exact
	}
	if len(primary) == 1 && len(unavailable) == 0 {
		if c := primary[0]; c.Via != "" {
			fmt.Printf("Detected %s - using %s\n\n", c.Via, c.Result.Manager)
		}
		return primary[0], true
	}
	if !cfg.Interactive {
		c, err := decideNonInteractive(query, cands, cfg.Prefer, unavailable)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return candidate{}, false
		}
		fmt.Println("Non-interactive mode: picking", candidateLabel(c))
		return c, true
	}
	items, labels := menuCandidates(query, cands)
	prompt := promptui.Select{Label: "Select package manager to install from", Items: labels}
	idx, _, err := prompt.Run()
	if err != nil {
		fmt.Println("Cancelled.")
		return candidate{}, false
	}
	chosen := items[idx]
	chosen.Picked = true
	return chosen, true
}

// installCandidate installs (or prints the snippet for) one candidate.
// query is what the user typed; it is only used to explain a renamed match.
func installCandidate(c candidate, query, requestedVersion string, global bool) int {
	id := c.Result.Manager
	meta, ok := pm.MetaFor(id)
	if !ok {
		fmt.Fprintln(os.Stderr, "Unsupported package manager:", id)
		return 1
	}
	name, extra := installSpec(c, requestedVersion)

	// The name comes from a registry response, not the user: validate it
	// before anything else happens (including offering to install the tool).
	if err := pm.ValidatePackageName(name, id); err != nil {
		fmt.Fprintf(os.Stderr, "Refusing to install %q: %v\n", name, err)
		return 1
	}

	if name != query {
		fmt.Printf("%q matched %s.\n", query, name)
	}
	if needsRenameConfirmation(query, c) && !c.Picked {
		if !cfg.Interactive {
			fmt.Fprintf(os.Stderr, "%q is not an exact match for %q; re-run with the exact name\n", name, query)
			return 1
		}
		yes, err := askYesNo(fmt.Sprintf("Install %s (closest match for %q)?", name, query))
		if err != nil || !yes {
			fmt.Println("Cancelled.")
			return 1
		}
	}
	// Maven and Gradle only print a dependency snippet: no tool is needed.
	if id == pm.Maven || id == pm.Gradle {
		return printSnippet(id, name, extra)
	}
	target := name
	if requestedVersion != "" {
		target += "@" + requestedVersion
	}
	fmt.Printf("Will install %s via %s.\n\n", target, meta.Name)

	if global && !meta.SupportsGlobal {
		fmt.Printf("%s does not support global installs in the same way. Ignoring --global.\n\n", meta.Name)
		global = false
	}
	if global && id == pm.Pip {
		fmt.Println("Global pip installs often require sudo and can affect system Python.")
		yes, err := askYesNo("Show recommended sudo command instead of running pip?")
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		if yes {
			fmt.Printf("\nRun this manually:\n  sudo pip install %s\n", name)
			return 0
		}
		fmt.Println("Proceeding without sudo (may fail if permissions are insufficient)...")
	}

	if err := ensurePM(id); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	adapter, err := pm.NewAdapter(id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "adapter error:", err)
		return 1
	}
	if err := adapter.InstallPackage(name, global, nil, extra); err != nil {
		fmt.Fprintln(os.Stderr, "package install failed:", err)
		return 1
	}
	fmt.Println("\nDone ✅")
	return 0
}

// printSnippet prints the dependency to add for a Maven or Gradle hit.
func printSnippet(id pm.ID, name string, extra map[string]string) int {
	coord := name
	if v := extra["version"]; v != "" {
		coord += ":" + v
	}
	file := "pom.xml"
	if id == pm.Gradle {
		file = "build.gradle(.kts)"
	}
	fmt.Printf("Add %s to %s:\n", coord, file)
	adapter, err := pm.NewAdapter(id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "adapter error:", err)
		return 1
	}
	if err := adapter.InstallPackage(name, false, nil, extra); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// ensureManager makes sure id's binary is on PATH. A missing tool that xpm
// can install with another tool (pnpm, yarn, pip, poetry, pipenv) is offered
// when config allows; any other tool gets its official install steps. It
// returns an error the caller should print.
func ensureManager(id pm.ID) error {
	meta, ok := pm.MetaFor(id)
	if !ok {
		return fmt.Errorf("unknown package manager %s", id)
	}
	if pm.Exists(meta.Binary) {
		return nil
	}
	fmt.Printf("%s (%s) is not installed on this system.\n", meta.Name, meta.Binary)
	// Tools with official manual steps are never offered: the answer
	// could only be "do it yourself".
	if steps, manual := pm.ManualInstallSteps(id); manual {
		return manualInstallError(meta, &pm.ManualInstallError{Manager: id, Steps: steps})
	}
	if !cfg.AutoInstallPM {
		if hint := strings.TrimSpace(pm.InstallHint(id)); hint != "" {
			fmt.Println("Hint:", hint)
		}
		return fmt.Errorf("%s is not installed (auto-install is disabled in config)", meta.Name)
	}
	yes, err := askYesNo(fmt.Sprintf("Attempt to install %s now?", meta.Name))
	if err != nil {
		if hint := strings.TrimSpace(pm.InstallHint(id)); hint != "" {
			fmt.Println("To install it:", hint)
		}
		return fmt.Errorf("cancelled")
	}
	if !yes {
		if hint := strings.TrimSpace(pm.InstallHint(id)); hint != "" {
			fmt.Println("To install it:", hint)
		}
		return fmt.Errorf("%s is not installed", meta.Name)
	}
	if err := pm.InstallPM(id); err != nil {
		var manual *pm.ManualInstallError
		if errors.As(err, &manual) {
			return manualInstallError(meta, manual)
		}
		return fmt.Errorf("failed to install %s: %w", meta.Name, err)
	}
	return nil
}

// manualInstallError explains a tool xpm will not install itself. If the
// user installed it already but the shell cannot see it (bun puts itself in
// ~/.bun/bin, rustup in ~/.cargo/bin), the PATH advice is what they need.
func manualInstallError(meta pm.Meta, manual *pm.ManualInstallError) error {
	return fmt.Errorf("%s is not installed; xpm does not run remote install scripts. To install it, %s, then re-run xpm. "+
		"If it is already installed, restart your shell or add its bin directory to PATH (installers usually print where it went, e.g. ~/.bun/bin or ~/.cargo/bin) so that %q is found",
		meta.Name, manual.Steps, meta.Binary)
}
