package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/crenspire/xpm/internal/pm"
)

// Seams for tests: running a project tool and checking it is on PATH.
var (
	runTool  = runBinaryWithCode
	pmExists = pm.Exists
)

// projectTarget is one project type detected in the current directory.
type projectTarget struct {
	Label string
	Kind  string
	PMs   []pm.ID
}

// detectProjectTargets lists the project types in the current directory.
// Python projects resolve to one tool: Pipfile -> pipenv, poetry.lock ->
// poetry, otherwise pip (requirements.txt and/or pyproject.toml).
func detectProjectTargets() []projectTarget {
	var targets []projectTarget
	add := func(file bool, label, kind string, pms ...pm.ID) {
		if file {
			targets = append(targets, projectTarget{Label: label, Kind: kind, PMs: pms})
		}
	}
	add(fileExists("package.json"), "Node (package.json)", "node", pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun)
	add(fileExists("composer.json"), "PHP (composer.json)", "composer", pm.Composer)
	add(fileExists("Cargo.toml"), "Rust (Cargo.toml)", "cargo", pm.Cargo)
	add(fileExists("go.mod"), "Go (go.mod)", "gomod", pm.GoMod)
	add(fileExists("pom.xml"), "Java (pom.xml)", "maven", pm.Maven)
	add(fileExists("build.gradle") || fileExists("build.gradle.kts"), "Java (Gradle build.gradle)", "gradle", pm.Gradle)
	switch {
	case fileExists("Pipfile") || fileExists("Pipfile.lock"):
		add(true, "Python (Pipfile)", "pipenv", pm.Pipenv)
	case fileExists("poetry.lock"):
		add(true, "Python (poetry.lock)", "poetry", pm.Poetry)
	default:
		add(fileExists("requirements.txt"), "Python (requirements.txt)", "pip-req", pm.Pip)
		add(fileExists("pyproject.toml"), "Python (pyproject.toml)", "pip-pyproject", pm.Pip)
	}
	return targets
}

// pmChoice is how a project's tool was decided: PM is set when it is
// decided (Via names the file or "prefer"), Options when the user must pick.
type pmChoice struct {
	PM      pm.ID
	Via     string
	Options []pm.ID
}

// resolveTargetPM picks the tool for t from the project's lock files
// (files, for t's ecosystem) and the prefer list. One lock file decides;
// several are narrowed by prefer or left to the user; none falls back to
// prefer, then to the ecosystem default (npm).
func resolveTargetPM(t projectTarget, files []pm.ProjectFile, prefer []string) pmChoice {
	if len(t.PMs) == 1 {
		return pmChoice{PM: t.PMs[0]}
	}
	allowed := map[pm.ID]bool{}
	for _, id := range t.PMs {
		allowed[id] = true
	}
	var locked []pm.ProjectFile
	for _, f := range files {
		if allowed[f.Manager] {
			locked = append(locked, f)
		}
	}
	if len(locked) == 1 {
		return pmChoice{PM: locked[0].Manager, Via: locked[0].Name}
	}
	options := t.PMs
	if len(locked) > 1 {
		options = nil
		for _, f := range locked {
			options = append(options, f.Manager)
		}
	}
	if p := pickPreferredPM(options, prefer); p != "" {
		return pmChoice{PM: p, Via: "prefer"}
	}
	if len(locked) > 1 {
		return pmChoice{Options: options}
	}
	return pmChoice{PM: options[0]}
}

// projectCmd is a resolved project: its kind and the tool that runs for it.
type projectCmd struct {
	Kind      string
	PM        pm.ID
	YarnBerry bool // yarn >= 2 (.yarnrc.yml): different flags
}

// manualError means the tool has no command for an action; its message
// tells the user what to do instead. It is not a failure (exit 0).
type manualError struct{ msg string }

func (e manualError) Error() string { return e.msg }

// withPkg appends pkg when it is set.
func withPkg(args []string, pkg string) []string {
	if pkg == "" {
		return args
	}
	return append(args, pkg)
}

// args returns the native command line for action ("install", "ci",
// "list", "update", "remove"); pkg is the package for update/remove.
// "ci" is a frozen install that fails instead of changing the lock file.
func (c projectCmd) args(action, pkg string) ([]string, error) {
	switch c.Kind {
	case "node":
		return c.nodeArgs(action, pkg)
	case "composer":
		return pick(action, pkg, []string{"install"}, []string{"install"}, []string{"show"},
			withPkg([]string{"update"}, pkg), []string{"remove", pkg})
	case "cargo":
		update := []string{"update"}
		if pkg != "" {
			update = []string{"update", "-p", pkg}
		}
		return pick(action, pkg, []string{"build"}, []string{"build", "--locked"}, []string{"tree", "--depth", "1"},
			update, []string{"remove", pkg})
	case "gomod":
		if action == "remove" {
			return nil, manualError{"To remove a Go dependency, delete its imports and run: go mod tidy"}
		}
		update := []string{"get", "-u", "./..."}
		if pkg != "" {
			update = []string{"get", "-u", pkg}
		}
		return pick(action, pkg, []string{"mod", "tidy"}, []string{"mod", "download"}, []string{"list", "-m", "all"}, update, nil)
	case "maven":
		if action == "remove" {
			return nil, manualError{fmt.Sprintf("Remove the dependency for %s from pom.xml.", pkg)}
		}
		return pick(action, pkg, []string{"install"}, []string{"install"}, []string{"dependency:list"},
			[]string{"versions:use-latest-releases"}, nil)
	case "gradle":
		switch action {
		case "update":
			return nil, manualError{"Gradle has no built-in update command; consider the Gradle Versions Plugin."}
		case "remove":
			return nil, manualError{fmt.Sprintf("Remove the dependency for %s from build.gradle.", pkg)}
		}
		return pick(action, pkg, []string{"build"}, []string{"build"},
			[]string{"dependencies", "--configuration", "implementation"}, nil, nil)
	case "pip-req":
		update := []string{"install", "--upgrade", "-r", "requirements.txt"}
		if pkg != "" {
			update = []string{"install", "--upgrade", pkg}
		}
		return pick(action, pkg, []string{"install", "-r", "requirements.txt"}, []string{"install", "-r", "requirements.txt"},
			[]string{"list"}, update, []string{"uninstall", "-y", pkg})
	case "pip-pyproject":
		update := []string{"install", "--upgrade", "."}
		if pkg != "" {
			update = []string{"install", "--upgrade", pkg}
		}
		return pick(action, pkg, []string{"install", "."}, []string{"install", "."}, []string{"list"}, update, []string{"uninstall", "-y", pkg})
	case "poetry":
		return pick(action, pkg, []string{"install"}, []string{"install"}, []string{"show"},
			withPkg([]string{"update"}, pkg), []string{"remove", pkg})
	case "pipenv":
		return pick(action, pkg, []string{"install"}, []string{"install", "--deploy"}, []string{"graph"},
			withPkg([]string{"update"}, pkg), []string{"uninstall", pkg})
	}
	return nil, fmt.Errorf("unknown project kind %q", c.Kind)
}

func (c projectCmd) nodeArgs(action, pkg string) ([]string, error) {
	switch action {
	case "install":
		return []string{"install"}, nil
	case "ci":
		switch {
		case c.PM == pm.Npm:
			return []string{"ci"}, nil
		case c.PM == pm.Yarn && c.YarnBerry:
			return []string{"install", "--immutable"}, nil
		default: // yarn 1, pnpm, bun
			return []string{"install", "--frozen-lockfile"}, nil
		}
	case "list":
		switch {
		case c.PM == pm.Bun:
			return []string{"pm", "ls"}, nil
		case c.PM == pm.Yarn && c.YarnBerry:
			return []string{"info", "--name-only"}, nil
		default:
			return []string{"list", "--depth=0"}, nil
		}
	case "update":
		switch {
		case c.PM == pm.Yarn && c.YarnBerry:
			if pkg == "" {
				pkg = "*"
			}
			return []string{"up", pkg}, nil
		case c.PM == pm.Yarn:
			return withPkg([]string{"upgrade"}, pkg), nil
		default:
			return withPkg([]string{"update"}, pkg), nil
		}
	case "remove":
		if c.PM == pm.Npm {
			return []string{"uninstall", pkg}, nil
		}
		return []string{"remove", pkg}, nil
	}
	return nil, fmt.Errorf("unknown action %q", action)
}

// pick returns the args for action from the per-action lists. A nil list
// means the tool has no such command.
func pick(action, pkg string, install, ci, list, update, remove []string) ([]string, error) {
	var out []string
	switch action {
	case "install":
		out = install
	case "ci":
		out = ci
	case "list":
		out = list
	case "update":
		out = update
	case "remove":
		if pkg == "" {
			return nil, errors.New("remove needs a package name")
		}
		out = remove
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
	if out == nil {
		return nil, fmt.Errorf("%s is not supported for this project", action)
	}
	return out, nil
}

// cleanDirs are the directories `xpm ci` offers to delete before a frozen
// install: node_modules for yarn/pnpm/bun (npm ci clears it itself) and
// vendor/ only for Composer (Go's vendor/ is source and is never touched).
func cleanDirs(c projectCmd) []string {
	switch {
	case c.Kind == "node" && c.PM != pm.Npm:
		return []string{"node_modules"}
	case c.Kind == "composer":
		return []string{"vendor"}
	}
	return nil
}

// selectTarget picks the project to act on: the only one, a prompt, or
// (non-interactive) the first.
func selectTarget(targets []projectTarget, label string) (projectTarget, bool) {
	if len(targets) == 1 || !cfg.Interactive {
		return targets[0], true
	}
	items := make([]string, len(targets))
	for i, t := range targets {
		items[i] = t.Label
	}
	idx, _, err := (&promptui.Select{Label: label, Items: items}).Run()
	if err != nil {
		fmt.Println("Cancelled.")
		return projectTarget{}, false
	}
	return targets[idx], true
}

// projectCmdFor decides which tool runs for t in the current directory.
func projectCmdFor(t projectTarget) (projectCmd, error) {
	files := pm.ProjectManagers(".")[pm.EcosystemForManager(t.PMs[0])]
	choice := resolveTargetPM(t, files, cfg.Prefer)
	id := choice.PM
	switch {
	case id == "" && !cfg.Interactive:
		id = choice.Options[0]
		fmt.Printf("Non-interactive mode: several lock files found, using %s\n", id)
	case id == "":
		items := make([]string, len(choice.Options))
		for i, o := range choice.Options {
			items[i] = string(o)
		}
		idx, _, err := (&promptui.Select{Label: "Several lock files found; package manager for " + t.Label, Items: items}).Run()
		if err != nil {
			return projectCmd{}, errors.New("cancelled")
		}
		id = choice.Options[idx]
	case choice.Via != "" && choice.Via != "prefer":
		fmt.Printf("Detected %s - using %s\n", choice.Via, id)
	}
	return projectCmd{Kind: t.Kind, PM: id, YarnBerry: id == pm.Yarn && fileExists(".yarnrc.yml")}, nil
}

// runProject runs action for t with its native tool and returns the exit code.
func runProject(t projectTarget, action, pkg string) int {
	pc, err := projectCmdFor(t)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	meta, _ := pm.MetaFor(pc.PM)
	if pkg != "" {
		if err := pm.ValidatePackageName(pkg, pc.PM); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid package name: %v\n", err)
			return 1
		}
	}
	args, err := pc.args(action, pkg)
	var manual manualError
	if errors.As(err, &manual) {
		fmt.Println(manual.msg)
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if action == "install" || action == "ci" {
		if err := ensurePM(pc.PM); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	} else if !pmExists(meta.Binary) {
		fmt.Printf("%s (%s) is not installed.\n", meta.Name, meta.Binary)
		return 1
	}
	fmt.Printf("Running: %s %s\n\n", meta.Binary, strings.Join(args, " "))
	return runTool(meta.Binary, args)
}

// noProjectFiles is printed when no project type is detected.
const noProjectFiles = "No known dependency files found (package.json, composer.json, etc)."

// autoInstallDetected installs dependencies for the detected projects:
// all of them, or the one the user picks.
func autoInstallDetected() int {
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	if len(targets) > 1 && cfg.Interactive {
		items := make([]string, 0, len(targets)+1)
		for _, t := range targets {
			items = append(items, t.Label)
		}
		items = append(items, "Run All")
		idx, _, err := (&promptui.Select{Label: "Detected multiple project types", Items: items}).Run()
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		if idx < len(targets) {
			targets = targets[idx : idx+1]
		}
	} else if len(targets) > 1 {
		fmt.Println("Non-interactive mode: running installs for all detected project types.")
	}
	return runEach(targets, "install")
}

// runEach runs action for every target, stopping at the first failure.
func runEach(targets []projectTarget, action string) int {
	for _, t := range targets {
		fmt.Printf("Detected: %s\n", t.Label)
		if code := runProject(t, action, ""); code != 0 {
			return code
		}
	}
	return 0
}

// cmdCleanInstall (`xpm ci`) runs each detected project's frozen install
// (npm ci, pnpm/yarn/bun --frozen-lockfile, yarn --immutable, composer
// install, pip install -r, pipenv install --deploy, cargo build --locked,
// go mod download). It never deletes lock files, and deletes
// node_modules/ or vendor/ only after the user confirms.
func cmdCleanInstall(args []string) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "error: ci takes no arguments, got: %s\n", strings.Join(args, " "))
		return 1
	}
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	for _, t := range targets {
		fmt.Printf("Detected: %s\n", t.Label)
		pc, err := projectCmdFor(t)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		for _, dir := range cleanDirs(pc) {
			if !fileExists(dir) {
				continue
			}
			if yes, err := askYesNo(fmt.Sprintf("Delete %s/ before the frozen install?", dir)); err == nil && yes {
				fmt.Printf("Removing %s/\n", dir)
				if err := os.RemoveAll(dir); err != nil {
					fmt.Fprintln(os.Stderr, "error:", err)
					return 1
				}
			}
		}
		if code := runProject(t, "ci", ""); code != 0 {
			return code
		}
	}
	return 0
}

// cmdList lists installed packages for the detected project.
func cmdList(args []string) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "error: list takes no arguments, got: %s\n", strings.Join(args, " "))
		return 1
	}
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	t, ok := selectTarget(targets, "Select project to list packages for")
	if !ok {
		return 1
	}
	fmt.Printf("Listing packages for %s\n\n", t.Label)
	return runProject(t, "list", "")
}

// cmdUpdate updates one package, or all of them, in the detected project.
func cmdUpdate(args []string) int {
	pkg, ok := atMostOneArg("update", args)
	if !ok {
		return 1
	}
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	t, ok := selectTarget(targets, "Select project to update")
	if !ok {
		return 1
	}
	fmt.Printf("Updating packages for %s\n\n", t.Label)
	return runProject(t, "update", pkg)
}

// cmdRemove removes a package from the detected project.
func cmdRemove(args []string) int {
	pkg, ok := exactlyOneArg("remove", args)
	if !ok {
		return 1
	}
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	t, ok := selectTarget(targets, "Select project to remove package from")
	if !ok {
		return 1
	}
	fmt.Printf("Removing %s from %s\n\n", pkg, t.Label)
	return runProject(t, "remove", pkg)
}
