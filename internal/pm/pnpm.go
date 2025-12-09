package pm

// PnpmAdapter implements the Adapter interface for pnpm.
// It uses `pnpm add` for package installation.
type PnpmAdapter struct{}

// ID returns the package manager identifier.
func (a PnpmAdapter) ID() ID { return Pnpm }

// InstallPackage installs a package using pnpm.
// Uses `pnpm add -g` for global installs, `pnpm add` for local.
// Version is specified using native pnpm syntax: package@version
func (a PnpmAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	args := []string{"add"}
	if global {
		args = append(args, "-g")
	}
	args = append(args, extraArgs...)

	// pnpm uses package@version syntax natively
	pkgSpec := pkg
	if version := extraInfo["version"]; version != "" {
		pkgSpec = pkg + "@" + version
	}
	args = append(args, pkgSpec)
	return Wrap("pnpm", args)
}
