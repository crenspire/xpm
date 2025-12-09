package pm

// PoetryAdapter implements the Adapter interface for Poetry (Python).
// It uses `poetry add` for package installation.
type PoetryAdapter struct{}

// ID returns the package manager identifier.
func (a PoetryAdapter) ID() ID { return Poetry }

// InstallPackage installs a package using poetry.
// Poetry doesn't have a global install concept - it always installs to the project.
// Version is specified using poetry's syntax: package==version or package@version
func (a PoetryAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	if global {
		// Poetry doesn't support global installs, but we can use pipx for that
		// For now, just warn and proceed with local install
		return Wrap("pipx", append([]string{"install"}, pkg))
	}

	args := []string{"add"}
	args = append(args, extraArgs...)

	// Poetry accepts package==version or package@version syntax
	pkgSpec := pkg
	if version := extraInfo["version"]; version != "" {
		pkgSpec = pkg + "==" + version
	}
	args = append(args, pkgSpec)
	return Wrap("poetry", args)
}


