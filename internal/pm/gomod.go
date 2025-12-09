package pm

import "fmt"

// GoModAdapter implements the Adapter interface for Go modules.
// It uses `go get` for dependencies and `go install` for binaries.
type GoModAdapter struct{}

// ID returns the package manager identifier.
func (a GoModAdapter) ID() ID { return GoMod }

// InstallPackage installs a Go module.
// For global (binary) installs, uses `go install module@version`.
// For local dependencies, uses `go get module@version` and updates go.mod.
func (a GoModAdapter) InstallPackage(pkg string, global bool, extraArgs []string, extraInfo map[string]string) error {
	module := pkg
	if m, ok := extraInfo["module"]; ok && m != "" {
		module = m
	}
	version := extraInfo["version"]

	if global {
		tag := "@latest"
		if version != "" {
			tag = "@" + version
		}
		args := []string{"install"}
		args = append(args, extraArgs...)
		args = append(args, module+tag)
		fmt.Printf("\nGo global install suggestion:\n  go install %s%s\n\n", module, tag)
		return Wrap("go", args)
	}

	tag := "@latest"
	if version != "" {
		tag = "@" + version
	}
	args := []string{"get"}
	args = append(args, extraArgs...)
	args = append(args, module+tag)

	fmt.Printf(`
Go module detected.

Add to go.mod (if not done automatically):

    require %s %s

Or run (xpm is doing this now):

    go get %s%s

`, module, versionOrLatest(version), module, tag)

	err := Wrap("go", args)
	if err != nil {
		return fmt.Errorf("go get failed (ensure module path is valid): %w", err)
	}
	return nil
}

// versionOrLatest returns the version string or "latest" if empty.
func versionOrLatest(v string) string {
	if v == "" {
		return "latest"
	}
	return v
}
