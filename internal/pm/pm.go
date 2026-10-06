// Package pm provides package manager abstractions and adapters.
//
// This package defines the Adapter interface that all package manager
// implementations must satisfy, along with metadata about each supported
// package manager and utilities for checking availability and installation.
//
// Supported package managers:
//   - npm, yarn, pnpm, bun (Node.js)
//   - pip (Python)
//   - composer (PHP)
//   - cargo (Rust)
//   - go modules (Go)
//   - maven, gradle (Java)
//
// Example usage:
//
//	adapter, err := pm.NewAdapter(pm.Npm)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	err = adapter.InstallPackage("express", false, nil, nil)
package pm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/crenspire/xpm/internal/logx"
)

// ID is a unique identifier for a package manager.
type ID string

// Package manager identifiers.
const (
	Npm      ID = "npm"
	Yarn     ID = "yarn"
	Pnpm     ID = "pnpm"
	Bun      ID = "bun"
	Pip      ID = "pip"
	Poetry   ID = "poetry"
	Pipenv   ID = "pipenv"
	Composer ID = "composer"
	Cargo    ID = "cargo"
	GoMod    ID = "gomod"
	Maven    ID = "maven"
	Gradle   ID = "gradle"
)

// Meta contains metadata about a package manager.
type Meta struct {
	// ID is the unique identifier for this package manager.
	ID ID
	// Name is the human-readable display name.
	Name string
	// Binary is the executable name to look for in PATH.
	Binary string
	// SupportsGlobal indicates whether this PM supports global installs.
	SupportsGlobal bool
}

// metas is the registry of all supported package managers.
var metas = []Meta{
	{Npm, "npm (Node.js)", "npm", true},
	{Yarn, "yarn", "yarn", true},
	{Pnpm, "pnpm", "pnpm", true},
	{Bun, "bun", "bun", true},
	{Pip, "pip (Python)", "pip", true},
	{Poetry, "poetry (Python)", "poetry", false},
	{Pipenv, "pipenv (Python)", "pipenv", false},
	{Composer, "composer (PHP)", "composer", true},
	{Cargo, "cargo (Rust)", "cargo", false},
	{GoMod, "go modules (Go)", "go", true},
	{Maven, "maven (Java)", "mvn", false},
	{Gradle, "gradle (Java)", "gradle", false},
}

// AllMetas returns metadata for all supported package managers.
func AllMetas() []Meta { return metas }

// MetaFor returns the metadata for a specific package manager ID.
// Returns false as the second value if the ID is not found.
func MetaFor(id ID) (Meta, bool) {
	for _, m := range metas {
		if m.ID == id {
			return m, true
		}
	}
	return Meta{}, false
}

// Adapter is the interface that all package manager implementations must satisfy.
// Each adapter knows how to install packages using its specific package manager.
type Adapter interface {
	// ID returns the package manager identifier for this adapter.
	ID() ID
	// InstallPackage installs a package using this package manager.
	// Parameters:
	//   - pkg: the package name to install
	//   - global: whether to install globally (if supported)
	//   - extraArgs: additional command-line arguments to pass
	//   - extraInfo: additional metadata (e.g., version, group for Maven)
	InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error
}

// NewAdapter creates and returns an Adapter for the specified package manager.
// Returns an error if the package manager ID is not recognized.
func NewAdapter(id ID) (Adapter, error) {
	switch id {
	case Npm:
		return NpmAdapter{}, nil
	case Yarn:
		return YarnAdapter{}, nil
	case Pnpm:
		return PnpmAdapter{}, nil
	case Bun:
		return BunAdapter{}, nil
	case Pip:
		return PipAdapter{}, nil
	case Poetry:
		return PoetryAdapter{}, nil
	case Pipenv:
		return PipenvAdapter{}, nil
	case Composer:
		return ComposerAdapter{}, nil
	case Cargo:
		return CargoAdapter{}, nil
	case GoMod:
		return GoModAdapter{}, nil
	case Maven:
		return MavenAdapter{}, nil
	case Gradle:
		return GradleAdapter{}, nil
	default:
		return nil, fmt.Errorf("no adapter for %s", id)
	}
}

// Seams: tests (here and, via SetCommandRunner/SetLookPath, in other
// packages) replace how commands run and how PATH is searched.
var (
	execRunner = runCommand
	lookPath   = exec.LookPath
)

// SetCommandRunner replaces how adapters and InstallPM run commands and
// returns a function that restores the real runner. For tests: with a fake
// runner nothing is executed. Not safe to call concurrently with installs.
func SetCommandRunner(fn func(bin string, args ...string) error) (restore func()) {
	old := execRunner
	execRunner = fn
	return func() { execRunner = old }
}

// SetLookPath replaces the PATH lookup used by Exists and InstallPM and
// returns a function that restores exec.LookPath. For tests.
func SetLookPath(fn func(file string) (string, error)) (restore func()) {
	old := lookPath
	lookPath = fn
	return func() { lookPath = old }
}

// runCommand executes a command with the given binary and arguments.
// stdout, stderr, and stdin are connected to the current process.
func runCommand(bin string, args ...string) error {
	logx.Info("running command: %s %v", bin, args)
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// Wrap executes a command and wraps any error with context.
// This is a convenience function used by adapters.
func Wrap(bin string, args []string) error {
	if err := execRunner(bin, args...); err != nil {
		return fmt.Errorf("%s %v failed: %w", bin, args, err)
	}
	return nil
}

// Exists checks if a binary is available in the system PATH.
func Exists(binary string) bool {
	_, err := lookPath(binary)
	return err == nil
}

// ManualInstallError means xpm will not install a tool itself (its official
// installer is a remote script or a whole toolchain). Steps says how.
type ManualInstallError struct {
	Manager ID
	Steps   string
}

func (e *ManualInstallError) Error() string {
	return fmt.Sprintf("%s must be installed manually: %s", e.Manager, e.Steps)
}

// installers are tools xpm installs with another tool that is already
// present, trying each command in order. Nothing is piped to a shell.
var installers = map[ID][][]string{
	Pnpm:   {{"npm", "install", "-g", "pnpm"}},
	Yarn:   {{"npm", "install", "-g", "yarn"}},
	Pip:    {{"python3", "-m", "ensurepip", "--upgrade", "--default-pip"}, {"python", "-m", "ensurepip", "--upgrade", "--default-pip"}},
	Poetry: {{"pipx", "install", "poetry"}},
	Pipenv: {{"pipx", "install", "pipenv"}, {"python3", "-m", "pip", "install", "--user", "pipenv"}},
}

// manualSteps are the official instructions for tools xpm won't install.
var manualSteps = map[ID]string{
	Bun:      "see https://bun.sh/docs/installation",
	Cargo:    "install Rust with rustup, see https://rustup.rs",
	Composer: "follow https://getcomposer.org/download/ (its installer is verified against https://composer.github.io/installer.sig)",
	GoMod:    "install Go from https://go.dev/dl/",
	Maven:    "install Maven from https://maven.apache.org/download.cgi",
	Gradle:   "install Gradle from https://gradle.org/install/",
	Npm:      "npm comes with Node.js; install Node from https://nodejs.org/",
}

// InstallPM installs a package manager using a tool that is already on
// PATH (npm for pnpm/yarn, python for pip, pipx for poetry/pipenv), then
// checks that the new binary is on PATH. Tools whose official installer is
// a remote script (bun, rustup, composer) are never run: a
// *ManualInstallError carries the official instructions instead.
func InstallPM(id ID) error {
	if steps, ok := manualSteps[id]; ok {
		return &ManualInstallError{Manager: id, Steps: steps}
	}
	attempts, ok := installers[id]
	if !ok {
		return fmt.Errorf("auto-install not implemented for %s", id)
	}
	meta, _ := MetaFor(id)
	var lastErr error
	for _, a := range attempts {
		if _, err := lookPath(a[0]); err != nil {
			lastErr = fmt.Errorf("%s is not installed", a[0])
			continue
		}
		fmt.Printf("Installing %s: %s\n", meta.Name, strings.Join(a, " "))
		if err := execRunner(a[0], a[1:]...); err != nil {
			lastErr = fmt.Errorf("%s failed: %w", strings.Join(a, " "), err)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return fmt.Errorf("could not install %s: %w", meta.Name, lastErr)
	}
	if _, err := lookPath(meta.Binary); err != nil {
		return fmt.Errorf("%s was installed but %q is not on PATH yet; open a new shell or add its bin directory to PATH", meta.Name, meta.Binary)
	}
	return nil
}

// ManualInstallSteps returns the official instructions for a tool xpm will
// not install itself (bun, cargo, composer, go, maven, gradle, npm), and
// whether id is such a tool.
func ManualInstallSteps(id ID) (string, bool) {
	steps, ok := manualSteps[id]
	return steps, ok
}

// InstallHint returns how to install the specified package manager, or "".
func InstallHint(id ID) string {
	if steps, ok := manualSteps[id]; ok {
		return steps
	}
	if attempts, ok := installers[id]; ok {
		return strings.Join(attempts[0], " ")
	}
	return ""
}
