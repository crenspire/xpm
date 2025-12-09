package pm

import "fmt"

// MavenAdapter implements the Adapter interface for Maven (Java).
// Since Maven doesn't have a command-line install, it prints dependency XML.
type MavenAdapter struct{}

// ID returns the package manager identifier.
func (a MavenAdapter) ID() ID { return Maven }

// InstallPackage prints Maven dependency XML to be added to pom.xml.
// Maven doesn't support direct command-line dependency addition, so this method
// outputs the XML dependency declaration to be manually added to the pom.xml file.
func (a MavenAdapter) InstallPackage(pkg string, global bool, _ []string, extraInfo map[string]string) error {
	group := extraInfo["group"]
	artifact := extraInfo["artifact"]
	version := extraInfo["version"]

	if group == "" || artifact == "" {
		fmt.Println("Maven artifact info incomplete. Use `xpm which` to inspect coordinates.")
		return nil
	}

	fmt.Printf(`
Maven artifact detected.

Add this dependency to your pom.xml:

    <dependency>
        <groupId>%s</groupId>
        <artifactId>%s</artifactId>
        <version>%s</version>
    </dependency>

`, group, artifact, versionOrUnknown(version))

	return nil
}

// versionOrUnknown returns the version string or a placeholder if empty.
func versionOrUnknown(v string) string {
	if v == "" {
		return "(choose a version)"
	}
	return v
}
