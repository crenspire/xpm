package pm

// YarnAdapter implements the Adapter interface for Yarn.
// It uses `yarn add` for local installs and `yarn global add` for global.
type YarnAdapter struct{}

// ID returns the package manager identifier.
func (a YarnAdapter) ID() ID { return Yarn }

// InstallPackage installs a package using yarn.
// Uses `yarn global add` for global installs, `yarn add` for local.
// Version is specified using native yarn syntax: package@version
func (a YarnAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	var args []string
	if global {
		args = []string{"global", "add"}
	} else {
		args = []string{"add"}
	}
	args = append(args, extraArgs...)

	// yarn uses package@version syntax natively
	pkgSpec := pkg
	if version := extraInfo["version"]; version != "" {
		pkgSpec = pkg + "@" + version
	}
	args = append(args, pkgSpec)
	return Wrap("yarn", args)
}
