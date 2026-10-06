package cli

import (
	"fmt"
	"os"
	"strings"
)

// installArgs is `xpm install`'s command line.
type installArgs struct {
	Global    bool
	Workspace bool
	Packages  []string
}

// parseInstallArgs accepts -g/--global and -w/--workspace anywhere (`xpm install axios -g`).
// "--" ends flag parsing; any other dash argument is an error rather than
// being passed on as a package name.
func parseInstallArgs(args []string) (installArgs, error) {
	var out installArgs
	for i, a := range args {
		switch {
		case a == "--":
			out.Packages = append(out.Packages, args[i+1:]...)
			return out, nil
		case a == "-g" || a == "--global" || a == "-global":
			out.Global = true
		case a == "-w" || a == "--workspace" || a == "-workspace":
			out.Workspace = true
		case strings.HasPrefix(a, "--global=") || strings.HasPrefix(a, "-global=") || strings.HasPrefix(a, "-g="):
			return installArgs{}, fmt.Errorf("unknown flag %q (install accepts -g/--global; omit it for a local install)", a)
		case strings.HasPrefix(a, "-"):
			return installArgs{}, fmt.Errorf("unknown flag %q (install accepts -g/--global and -w/--workspace)", a)
		default:
			out.Packages = append(out.Packages, a)
		}
	}
	return out, nil
}

// exactlyOneArg returns the only argument of cmd. Missing or extra
// arguments print an error and the command's usage instead of being
// silently ignored.
func exactlyOneArg(cmd string, args []string) (string, bool) {
	switch len(args) {
	case 1:
		return args[0], true
	case 0:
		fmt.Fprintln(os.Stderr, "error: missing package name")
	default:
		fmt.Fprintf(os.Stderr, "error: %s takes one package name, got %d: %s\n", cmd, len(args), strings.Join(args, " "))
	}
	fmt.Fprintln(os.Stderr)
	showCommandUsage(cmd)
	return "", false
}

// atMostOneArg is exactlyOneArg for commands whose argument is optional.
func atMostOneArg(cmd string, args []string) (string, bool) {
	if len(args) == 0 {
		return "", true
	}
	return exactlyOneArg(cmd, args)
}
