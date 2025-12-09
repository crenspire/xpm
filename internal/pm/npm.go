package pm

// NpmAdapter implements the Adapter interface for npm (Node Package Manager).
// It uses the `npm install` command for package installation.
type NpmAdapter struct{}

// ID returns the package manager identifier.
func (a NpmAdapter) ID() ID { return Npm }

// InstallPackage installs a package using npm.
// Uses `npm install -g` for global installs, `npm install` for local.
// Version is specified using native npm syntax: package@version
func (a NpmAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	args := []string{"install"}
	if global {
		args = append(args, "-g")
	}
	args = append(args, extraArgs...)

	// npm uses package@version syntax natively
	pkgSpec := pkg
	if version := extraInfo["version"]; version != "" {
		pkgSpec = pkg + "@" + version
	}
	args = append(args, pkgSpec)
	return Wrap("npm", args)
}
