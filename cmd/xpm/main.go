// Package main is the entry point for the xpm (Universal Package Manager) CLI.
//
// XPM provides a unified interface for managing packages across multiple ecosystems
// including npm, pip, composer, cargo, maven, gradle, and go modules.
//
// When started through a runtime shim (a symlink named node, python, go, ...
// pointing at xpm), it runs that runtime's active version instead.
//
// Usage:
//
//	xpm install <package>     Install a package (searches all ecosystems)
//	xpm install               Auto-detect and install project dependencies
//	xpm which <package>       Check which ecosystems have a package
//	xpm doctor                Check installed package managers
//	xpm version               Show version information
//	xpm help                  Show help
package main

import (
	"os"

	"github.com/crenspire/xpm/internal/cli"
	"github.com/crenspire/xpm/internal/env"
	_ "github.com/crenspire/xpm/internal/env/runtimes" // register runtime installers for shim dispatch
)

func main() {
	if name, ok := env.ShimName(os.Args[0]); ok {
		os.Exit(env.RunShim(name, os.Args[1:]))
	}
	os.Exit(cli.Run())
}
