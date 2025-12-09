package pm

// PipenvAdapter implements the Adapter interface for Pipenv (Python).
// It uses `pipenv install` for package installation.
type PipenvAdapter struct{}

// ID returns the package manager identifier.
func (a PipenvAdapter) ID() ID { return Pipenv }

// InstallPackage installs a package using pipenv.
// Pipenv doesn't have a traditional global install - it manages virtual environments.
// Version is specified using pip-style syntax: package==version
func (a PipenvAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	args := []string{"install"}
	args = append(args, extraArgs...)

	// Pipenv uses package==version syntax (same as pip)
	pkgSpec := pkg
	if version := extraInfo["version"]; version != "" {
		pkgSpec = pkg + "==" + version
	}
	args = append(args, pkgSpec)
	return Wrap("pipenv", args)
}


