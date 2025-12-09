package pm

import "fmt"

// CargoAdapter implements the Adapter interface for Cargo (Rust).
// It uses `cargo add` for local dependencies and `cargo install` for binaries.
type CargoAdapter struct{}

// ID returns the package manager identifier.
func (a CargoAdapter) ID() ID { return Cargo }

// InstallPackage installs a crate using cargo.
// For global (binary) installs, uses `cargo install`.
// For local dependencies, uses `cargo add` (requires cargo-edit or Rust 1.62+).
// Version is specified using --version flag for cargo install, or @version for cargo add.
func (a CargoAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	version := extraInfo["version"]

	if global {
		args := []string{"install"}
		if version != "" {
			args = append(args, "--version", version)
		}
		args = append(args, extraArgs...)
		args = append(args, pkg)
		return Wrap("cargo", args)
	}

	// cargo add uses package@version syntax
	args := []string{"add"}
	args = append(args, extraArgs...)
	pkgSpec := pkg
	if version != "" {
		pkgSpec = pkg + "@" + version
	}
	args = append(args, pkgSpec)
	err := Wrap("cargo", args)
	if err != nil {
		return fmt.Errorf("cargo add failed (ensure cargo-edit is installed): %w", err)
	}
	return nil
}
