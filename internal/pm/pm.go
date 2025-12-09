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
	"runtime"

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
	{GoMod, "go modules (Go)", "go", false},
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
	if err := runCommand(bin, args...); err != nil {
		return fmt.Errorf("%s %v failed: %w", bin, args, err)
	}
	return nil
}

// Exists checks if a binary is available in the system PATH.
func Exists(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

// runShell executes a shell script.
// On Windows, it prints the command for manual execution.
func runShell(script string) error {
	logx.Info("running shell script: %s", script)
	if runtime.GOOS == "windows" {
		fmt.Println("Please run this command manually in PowerShell:")
		fmt.Println(script)
		return nil
	}
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// InstallPM attempts to install the specified package manager.
// Not all package managers support automatic installation; those that don't
// will return an error with instructions for manual installation.
func InstallPM(id ID) error {
	switch id {
	case Bun:
		fmt.Println("Installing bun...")
		return runShell("curl -fsSL https://bun.sh/install | bash")
	case Pnpm:
		fmt.Println("Installing pnpm via npm (requires Node.js)...")
		return runShell("npm install -g pnpm")
	case Yarn:
		fmt.Println("Installing yarn via npm (requires Node.js)...")
		return runShell("npm install -g yarn")
	case Pip:
		fmt.Println("Attempting to ensure pip via python3 -m ensurepip --upgrade")
		return runShell("python3 -m ensurepip --upgrade || python -m ensurepip --upgrade")
	case Poetry:
		fmt.Println("Installing poetry via pipx (recommended) or pip...")
		return runShell("pipx install poetry || pip install poetry")
	case Pipenv:
		fmt.Println("Installing pipenv via pip...")
		return runShell("pip install --user pipenv")
	case Composer:
		script := `
php -r "copy('https://getcomposer.org/installer', 'composer-setup.php');" &&
php composer-setup.php &&
rm composer-setup.php &&
mv composer.phar /usr/local/bin/composer
`
		fmt.Println("Attempting to install composer globally...")
		return runShell(script)
	case Cargo:
		fmt.Println("Installing Rust (cargo) via rustup...")
		return runShell("curl https://sh.rustup.rs -sSf | sh -s -- -y")
	case GoMod:
		return fmt.Errorf("please install Go from https://go.dev/dl/ and ensure 'go' is in PATH")
	case Maven:
		return fmt.Errorf("please install Maven from https://maven.apache.org/download.cgi and ensure 'mvn' is in PATH")
	case Gradle:
		return fmt.Errorf("please install Gradle from https://gradle.org/install/ and ensure 'gradle' is in PATH")
	case Npm:
		return fmt.Errorf("npm comes with Node.js. Install Node from https://nodejs.org/")
	default:
		return fmt.Errorf("auto-install not implemented for %s", id)
	}
}

// InstallHint returns a helpful hint for installing the specified package manager.
// Returns an empty string if no hint is available.
func InstallHint(id ID) string {
	switch id {
	case Bun:
		return "curl -fsSL https://bun.sh/install | bash"
	case Pnpm:
		return "npm install -g pnpm"
	case Yarn:
		return "npm install -g yarn"
	case Pip:
		return "python3 -m ensurepip --upgrade"
	case Poetry:
		return "pipx install poetry (or pip install poetry)"
	case Pipenv:
		return "pip install --user pipenv"
	case Composer:
		return "see https://getcomposer.org/download/"
	case Cargo:
		return "curl https://sh.rustup.rs -sSf | sh"
	case GoMod:
		return "install Go from https://go.dev/dl/"
	case Maven:
		return "install Maven from https://maven.apache.org/"
	case Gradle:
		return "install Gradle from https://gradle.org/install/"
	case Npm:
		return "install Node.js from https://nodejs.org/"
	default:
		return ""
	}
}
