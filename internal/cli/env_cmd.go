package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	goruntime "runtime"
	"strings"
	"syscall"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/env"
	_ "github.com/crenspire/xpm/internal/env/runtimes" // register runtime installers
)

// Seams for tests.
var (
	envGOOS       = goruntime.GOOS
	newEnvManager = env.NewManager
)

const envUsage = `Usage: xpm env <command>

Commands:
  install <runtime>@<version>          Install a version (does not touch .xpm-env)
  use <runtime>@<version> [--global]   Pin a version here (.xpm-env) or globally
  list                                 List installed versions
  ls-remote <runtime>                  List available versions (newest 20)
  current                              Show the version in effect for each runtime
  remove <runtime>@<version>           Remove an installed version
  reshim                               Recreate the shims (links to xpm)
  setup-path                           Put the shims directory on your shell's PATH

Runtimes: node, go, python, java, rust, bun, deno, php
Versions: exact (20.11.0), partial (20), latest, lts (node, java)
`

// cmdEnv handles the `xpm env` command group.
func cmdEnv(args []string) int {
	if envGOOS == "windows" {
		fmt.Fprintln(os.Stderr, "xpm env is not supported on Windows yet: runtime downloads, shims and PATH setup are Unix-only (macOS, Linux).")
		return 1
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, envUsage)
		return 1
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(envUsage)
		return 0
	}

	cfg := config.Load()
	if !cfg.Env.Enabled {
		fmt.Fprintln(os.Stderr, "Runtime version management is disabled (env.enabled is false in your xpm config).")
		return 1
	}
	m, err := newEnvManager(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "install":
		return cmdEnvInstall(m, rest)
	case "use":
		return cmdEnvUse(m, rest)
	case "list", "ls":
		return cmdEnvList(m, rest)
	case "ls-remote":
		return cmdEnvListRemote(rest)
	case "current":
		return cmdEnvCurrent(m, rest)
	case "remove", "rm", "uninstall":
		return cmdEnvRemove(m, rest)
	case "reshim":
		return cmdEnvReshim(m, rest)
	case "setup-path":
		return cmdEnvSetupPath(m, rest)
	}
	fmt.Fprintf(os.Stderr, "Unknown env command: %s\n\n%s", sub, envUsage)
	return 1
}

// envUsageError prints a one-line usage for a subcommand.
func envUsageError(usage string) int {
	fmt.Fprintf(os.Stderr, "Usage: xpm env %s\n", usage)
	return 1
}

// interruptible returns a context cancelled by Ctrl-C or SIGTERM.
func interruptible() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// reshim recreates the shim set; a failure is a warning, not a failed command.
func reshim(m *env.Manager) {
	if err := env.CreateShims(m); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update shims: %v\n", err)
	}
}

// pathHint tells the user to run setup-path while shims are not on PATH.
func pathHint(m *env.Manager) {
	if !env.CheckPATH(m) {
		fmt.Printf("\nShims are not on your PATH yet. Run: xpm env setup-path\n")
	}
}

func cmdEnvInstall(m *env.Manager, args []string) int {
	if len(args) != 1 {
		return envUsageError("install <runtime>@<version>   (e.g. node@20, go@latest, java@lts)")
	}
	rt, spec, err := parseRuntimeVersion(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	ctx, stop := interruptible()
	defer stop()
	if _, err := env.InstallRuntime(ctx, m, rt, spec); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "Cancelled; nothing was installed.")
			return 130
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	reshim(m)
	pathHint(m)
	return 0
}

// parseUseArgs accepts --global/-g anywhere and exactly one runtime@version.
func parseUseArgs(args []string) (spec string, global bool, err error) {
	var specs []string
	for _, a := range args {
		switch {
		case a == "--global" || a == "-g":
			global = true
		case strings.HasPrefix(a, "-"):
			return "", false, fmt.Errorf("unknown flag %s", a)
		default:
			specs = append(specs, a)
		}
	}
	if len(specs) != 1 {
		return "", false, errors.New("expected exactly one <runtime>@<version>")
	}
	return specs[0], global, nil
}

func cmdEnvUse(m *env.Manager, args []string) int {
	arg, global, err := parseUseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return envUsageError("use <runtime>@<version> [--global]")
	}
	rt, spec, err := parseRuntimeVersion(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	a, err := env.UseVersion(m, rt, spec, global)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Using %s@%s (%s)\n", rt, a.Version, a.Source)
	if global {
		if here, err := m.ActiveVersion(rt); err == nil && !here.Global {
			fmt.Printf("Note: %s pins %s@%s in this directory\n", here.Source, rt, here.Version)
		}
	}
	reshim(m)
	pathHint(m)
	return 0
}

func cmdEnvList(m *env.Manager, args []string) int {
	if len(args) != 0 {
		return envUsageError("list")
	}
	installed, err := env.ListInstalled(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Print(env.FormatInstalled(installed))
	return 0
}

func cmdEnvListRemote(args []string) int {
	if len(args) != 1 {
		return envUsageError("ls-remote <runtime>")
	}
	if err := env.ValidateRuntimeName(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	ctx, stop := interruptible()
	defer stop()
	versions, err := env.ListRemote(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Print(env.FormatRemote(args[0], versions))
	return 0
}

// writeCurrent prints "<rt> <version> (<source>)" per configured runtime,
// sorted by runtime; problems go to errOut as warnings.
func writeCurrent(m *env.Manager, out, errOut io.Writer) {
	found := false
	for _, rt := range env.ListRuntimes() {
		a, err := m.ActiveVersion(rt)
		switch {
		case errors.Is(err, env.ErrNoVersion):
			continue
		case errors.Is(err, env.ErrNotInstalled):
			_, _ = fmt.Fprintf(out, "%s %s (%s)\n", rt, a.Version, a.Source)
			_, _ = fmt.Fprintf(errOut, "warning: %v; run: xpm env install %s@%s\n", err, rt, a.Version)
		case err != nil:
			_, _ = fmt.Fprintf(errOut, "warning: %v\n", err)
			continue
		default:
			_, _ = fmt.Fprintf(out, "%s %s (%s)\n", rt, a.Version, a.Source)
		}
		found = true
	}
	if !found {
		_, _ = fmt.Fprintln(out, "No runtime versions configured. Install one: xpm env install <runtime>@<version>")
	}
}

func cmdEnvCurrent(m *env.Manager, args []string) int {
	if len(args) != 0 {
		return envUsageError("current")
	}
	writeCurrent(m, os.Stdout, os.Stderr)
	return 0
}

func cmdEnvRemove(m *env.Manager, args []string) int {
	if len(args) != 1 {
		return envUsageError("remove <runtime>@<version>")
	}
	rt, version, err := parseRuntimeVersion(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	ctx, stop := interruptible()
	defer stop()
	if err := env.RemoveVersion(ctx, m, rt, version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Removed %s@%s\n", rt, version)
	reshim(m)
	return 0
}

func cmdEnvReshim(m *env.Manager, args []string) int {
	if len(args) != 0 {
		return envUsageError("reshim")
	}
	if err := env.CreateShims(m); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Shims updated in %s\n", m.GetShimsPath())
	return 0
}

func cmdEnvSetupPath(m *env.Manager, args []string) int {
	if len(args) != 0 {
		return envUsageError("setup-path")
	}
	edit, err := env.SetupPATH(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if !edit.Changed {
		fmt.Printf("PATH already configured in %s\n", edit.File)
		return 0
	}
	fmt.Printf("Added %s to PATH in %s\nRestart your shell or run: source %s\n", m.GetShimsPath(), edit.File, edit.File)
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
