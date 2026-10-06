package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// ensurePM is ensureManager; tests replace it to observe ordering.
var ensurePM = ensureManager

func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	global := fs.Bool("global", false, "install globally")
	gShort := fs.Bool("g", false, "install globally (shorthand)")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return 1
	}
	pkgArgs := fs.Args()
	glob := *global || *gShort

	if len(pkgArgs) == 0 {
		return autoInstallDetected(glob)
	}
	return installOne(pkgArgs[0], glob)
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
	chosen, ok := chooseCandidate(cands)
	if !ok {
		return 1
	}
	return installCandidate(chosen, pkg, requestedVersion, global)
}

// chooseCandidate picks automatically when there is one candidate, and
// otherwise prompts (or, non-interactively, takes the first).
func chooseCandidate(cands []candidate) (candidate, bool) {
	if len(cands) == 1 {
		if c := cands[0]; c.Via != "" {
			fmt.Printf("Detected %s - using %s\n\n", c.Via, c.Result.Manager)
		}
		return cands[0], true
	}
	labels := make([]string, len(cands))
	for i, c := range cands {
		labels[i] = candidateLabel(c)
	}
	if !cfg.Interactive {
		fmt.Println("Non-interactive mode: picking", labels[0])
		return cands[0], true
	}
	prompt := promptui.Select{Label: "Select package manager to install from", Items: labels}
	idx, _, err := prompt.Run()
	if err != nil {
		fmt.Println("Cancelled.")
		return candidate{}, false
	}
	return cands[idx], true
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

// ensureManager makes sure id's binary is on PATH, offering to install it
// when config allows. It returns an error the caller should print.
func ensureManager(id pm.ID) error {
	meta, ok := pm.MetaFor(id)
	if !ok {
		return fmt.Errorf("unknown package manager %s", id)
	}
	if pm.Exists(meta.Binary) {
		return nil
	}
	fmt.Printf("%s (%s) is not installed on this system.\n", meta.Name, meta.Binary)
	if !cfg.AutoInstallPM {
		if hint := strings.TrimSpace(pm.InstallHint(id)); hint != "" {
			fmt.Println("Hint:", hint)
		}
		return fmt.Errorf("%s is not installed (auto-install is disabled in config)", meta.Name)
	}
	yes, err := askYesNo(fmt.Sprintf("Attempt to install %s now?", meta.Name))
	if err != nil {
		return fmt.Errorf("cancelled")
	}
	if !yes {
		return fmt.Errorf("%s is not installed", meta.Name)
	}
	if err := pm.InstallPM(id); err != nil {
		return fmt.Errorf("failed to install %s: %w", meta.Name, err)
	}
	return nil
}
