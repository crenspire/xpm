package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/env"
	_ "github.com/crenspire/xpm/internal/env/runtimes" // Import to register runtime installers
)

// cmdEnv handles the `xpm env` command group.
func cmdEnv(args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: xpm env <command>")
		fmt.Println("\nCommands:")
		fmt.Println("  install <runtime>@<version>  Install a runtime version")
		fmt.Println("  use <runtime>@<version>       Switch to a runtime version")
		fmt.Println("  list                          List installed versions")
		fmt.Println("  ls-remote <runtime>          List available remote versions")
		fmt.Println("  current                      Show active versions")
		fmt.Println("  remove <runtime>@<version>    Remove a runtime version")
		return 1
	}

	cfg := config.Load()
	if !cfg.Env.Enabled {
		fmt.Println("Runtime version management is disabled. Enable it in your config.")
		return 1
	}

	manager, err := env.NewManager(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	subcmd := args[0]
	rest := args[1:]

	switch subcmd {
	case "install":
		return cmdEnvInstall(manager, rest)
	case "use":
		return cmdEnvUse(manager, rest)
	case "list", "ls":
		return cmdEnvList(manager)
	case "ls-remote":
		return cmdEnvListRemote(manager, rest)
	case "current":
		return cmdEnvCurrent(manager)
	case "remove", "rm":
		return cmdEnvRemove(manager, rest)
	case "setup-path":
		return cmdEnvSetupPath(manager)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", subcmd)
		return 1
	}
}

// cmdEnvInstall handles `xpm env install <runtime>@<version>`.
func cmdEnvInstall(manager *env.Manager, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: xpm env install <runtime>@<version>\n")
		fmt.Fprintf(os.Stderr, "\nAvailable runtimes: node, python, php, go, java, rust, bun, deno\n")
		fmt.Fprintf(os.Stderr, "\nSpecial version aliases:\n")
		fmt.Fprintf(os.Stderr, "  - latest: Install the latest version (node, python, php, go)\n")
		fmt.Fprintf(os.Stderr, "  - lts: Install the latest LTS version (node only)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  xpm env install node@latest\n")
		fmt.Fprintf(os.Stderr, "  xpm env install node@lts\n")
		fmt.Fprintf(os.Stderr, "  xpm env install python@latest\n")
		fmt.Fprintf(os.Stderr, "  xpm env install php@latest\n")
		fmt.Fprintf(os.Stderr, "  xpm env install go@latest\n")
		fmt.Fprintf(os.Stderr, "\nNote: Package managers (npm, pip, composer, etc.) are not runtimes.\n")
		fmt.Fprintf(os.Stderr, "      Install the runtime instead (e.g., 'node' includes npm, 'python' includes pip).\n")
		return 1
	}

	spec := args[0]
	runtime, version, err := parseRuntimeVersion(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Detect if an alias was used
	var alias string
	if version == "latest" || version == "lts" {
		alias = version
	}

	if err := env.InstallRuntimeWithAlias(manager, runtime, version, alias); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	return 0
}

// cmdEnvUse handles `xpm env use <runtime>@<version>`.
func cmdEnvUse(manager *env.Manager, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: xpm env use <runtime>@<version> [--global]\n")
		return 1
	}

	spec := args[0]
	global := false
	if len(args) > 1 && args[1] == "--global" {
		global = true
	}

	runtime, version, err := parseRuntimeVersion(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	a, err := env.UseVersion(manager, runtime, version, global)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Using %s@%s (%s)\n", runtime, a.Version, a.Source)
	if err := env.CreateShims(manager); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update shims: %v\n", err)
	}

	return 0
}

// cmdEnvList handles `xpm env list`.
func cmdEnvList(manager *env.Manager) int {
	installed, err := env.ListInstalled(manager)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Print(env.FormatInstalled(installed))
	return 0
}

// cmdEnvListRemote handles `xpm env ls-remote <runtime>`.
func cmdEnvListRemote(manager *env.Manager, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: xpm env ls-remote <runtime>\n")
		return 1
	}

	runtime := args[0]
	versions, err := env.ListRemote(manager, runtime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Print(env.FormatRemote(runtime, versions))
	return 0
}

// cmdEnvCurrent handles `xpm env current`.
func cmdEnvCurrent(manager *env.Manager) int {
	runtimes := env.ListRuntimes()
	found := false

	for _, runtime := range runtimes {
		a, err := manager.ActiveVersion(runtime)
		if err == nil {
			fmt.Printf("%s %s (%s)\n", runtime, a.Version, a.Source)
			found = true
		}
	}

	if !found {
		fmt.Println("No active runtime versions found.")
	}

	return 0
}

// cmdEnvRemove handles `xpm env remove <runtime>@<version>`.
func cmdEnvRemove(manager *env.Manager, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: xpm env remove <runtime>@<version>\n")
		return 1
	}

	spec := args[0]
	runtime, version, err := parseRuntimeVersion(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if err := env.RemoveVersion(context.Background(), manager, runtime, version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Removed %s@%s\n", runtime, version)

	return 0
}

// cmdEnvSetupPath handles `xpm env setup-path`.
func cmdEnvSetupPath(manager *env.Manager) int {
	if err := env.UpdatePATH(manager); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// parseRuntimeVersion parses and validates a runtime@version specification.
func parseRuntimeVersion(spec string) (runtime, version string, err error) {
	runtime, version, ok := strings.Cut(spec, "@")
	if !ok {
		return "", "", fmt.Errorf("invalid format %q: expected <runtime>@<version>", spec)
	}
	if err := env.ValidateRuntimeName(runtime); err != nil {
		return "", "", err
	}
	if err := env.ValidateVersionSpec(version); err != nil {
		return "", "", err
	}
	return runtime, version, nil
}
