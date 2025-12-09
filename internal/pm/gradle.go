package pm

import "fmt"

// GradleAdapter implements the Adapter interface for Gradle (Java/Kotlin/Android).
// Since Gradle doesn't have a command-line install, it prints dependency snippets.
type GradleAdapter struct{}

// ID returns the package manager identifier.
func (a GradleAdapter) ID() ID { return Gradle }

// InstallPackage prints Gradle dependency snippets for both Groovy and Kotlin DSL.
// Gradle doesn't support direct command-line dependency addition, so this method
// outputs the dependency declaration to be manually added to build.gradle or build.gradle.kts.
func (a GradleAdapter) InstallPackage(pkg string, global bool, _ []string, extraInfo map[string]string) error {
	group := extraInfo["group"]
	artifact := extraInfo["artifact"]
	version := extraInfo["version"]

	if group == "" || artifact == "" {
		fmt.Println("Gradle artifact info incomplete. Use `xpm which` to inspect coordinates.")
		return nil
	}

	fmt.Printf(`
Gradle dependency suggestion:

Groovy DSL (build.gradle):

    implementation '%s:%s:%s'

Kotlin DSL (build.gradle.kts):

    implementation("%s:%s:%s")

`, group, artifact, versionOrUnknown(version), group, artifact, versionOrUnknown(version))

	return nil
}
