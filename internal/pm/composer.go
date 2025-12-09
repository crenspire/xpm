package pm

// ComposerAdapter implements the Adapter interface for Composer (PHP).
// It uses `composer require` for package installation.
type ComposerAdapter struct{}

// ID returns the package manager identifier.
func (a ComposerAdapter) ID() ID { return Composer }

// InstallPackage installs a package using composer.
// Uses `composer global require` for global installs, `composer require` for local.
// Version is translated from @ syntax to composer's : syntax: vendor/package@1.0.0 -> vendor/package:^1.0.0
func (a ComposerAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	args := []string{}
	if global {
		args = append(args, "global")
	}
	args = append(args, "require")
	args = append(args, extraArgs...)

	// composer uses package:version syntax (translate from @)
	pkgSpec := pkg
	if version := extraInfo["version"]; version != "" {
		pkgSpec = pkg + ":" + version
	}
	args = append(args, pkgSpec)
	return Wrap("composer", args)
}
