// Package main is the entry point for the xpm (Universal Package Manager) CLI.
//
// XPM provides a unified interface for managing packages across multiple ecosystems
// including npm, pip, composer, cargo, maven, gradle, and go modules.
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
)

func main() {
	code := cli.Run()
	os.Exit(code)
}
