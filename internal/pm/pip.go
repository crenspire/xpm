package pm

// PipAdapter implements the Adapter interface for pip (Python Package Installer).
// It uses `pip install` for package installation.
type PipAdapter struct{}

// ID returns the package manager identifier.
func (a PipAdapter) ID() ID { return Pip }

// InstallPackage installs a package using pip.
// Note: pip doesn't distinguish between global and local installs in the same way
// as Node.js package managers. The global flag is accepted but has no effect.
// For true global installs, users may need to use sudo.
// Version is translated from @ syntax to pip's == syntax: package@1.0.0 -> package==1.0.0
func (a PipAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	args := []string{"install"}
	args = append(args, extraArgs...)

	// pip uses package==version syntax (translate from @)
	pkgSpec := pkg
	if version := extraInfo["version"]; version != "" {
		pkgSpec = pkg + "==" + version
	}
	args = append(args, pkgSpec)
	return Wrap("pip", args)
}
