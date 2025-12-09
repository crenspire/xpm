package pm

// BunAdapter implements the Adapter interface for Bun.
// It uses `bun add` for package installation.
type BunAdapter struct{}

// ID returns the package manager identifier.
func (a BunAdapter) ID() ID { return Bun }

// InstallPackage installs a package using bun.
// Uses `bun add -g` for global installs, `bun add` for local.
// Version is specified using native bun syntax: package@version
func (a BunAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	args := []string{"add"}
	if global {
		args = append(args, "-g")
	}
	args = append(args, extraArgs...)

	// bun uses package@version syntax natively
	pkgSpec := pkg
	if version := extraInfo["version"]; version != "" {
		pkgSpec = pkg + "@" + version
	}
	args = append(args, pkgSpec)
	return Wrap("bun", args)
}
