package workspace

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/crenspire/xpm/internal/pm"
)

// DetectJavaWorkspace detects Java multi-module projects (Maven/Gradle).
func DetectJavaWorkspace(root string) (*Workspace, error) {
	// Try Maven first
	if ws := detectMavenWorkspace(root); ws != nil && len(ws.Projects) > 0 {
		return ws, nil
	}

	// Try Gradle
	if ws := detectGradleWorkspace(root); ws != nil && len(ws.Projects) > 0 {
		return ws, nil
	}

	return nil, nil
}

// detectMavenWorkspace detects Maven multi-module projects.
func detectMavenWorkspace(root string) *Workspace {
	pomPath := filepath.Join(root, "pom.xml")
	if _, err := os.Stat(pomPath); err != nil {
		return nil
	}

	var pom struct {
		XMLName xml.Name `xml:"project"`
		Modules struct {
			Module []string `xml:"module"`
		} `xml:"modules"`
	}

	data, err := os.ReadFile(pomPath)
	if err != nil {
		return nil
	}

	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil
	}

	if len(pom.Modules.Module) == 0 {
		return nil
	}

	var projects []Project
	for _, module := range pom.Modules.Module {
		modulePath := filepath.Join(root, module)
		modulePomPath := filepath.Join(modulePath, "pom.xml")

		if _, err := os.Stat(modulePomPath); err != nil {
			continue
		}

		// Read module pom.xml to get artifactId
		var modulePom struct {
			XMLName    xml.Name `xml:"project"`
			ArtifactID string   `xml:"artifactId"`
			GroupID    string   `xml:"groupId"`
			Parent     struct {
				ArtifactID string `xml:"artifactId"`
			} `xml:"parent"`
		}

		moduleData, err := os.ReadFile(modulePomPath)
		if err != nil {
			continue
		}

		if err := xml.Unmarshal(moduleData, &modulePom); err != nil {
			continue
		}

		name := modulePom.ArtifactID
		if name == "" {
			name = modulePom.Parent.ArtifactID
		}
		if name == "" {
			name = filepath.Base(modulePath)
		}

		projects = append(projects, Project{
			Name:      name,
			Path:      modulePath,
			Ecosystem: "java",
			Manifest:  modulePomPath,
			Lockfile:  "", // Maven doesn't use lockfiles in the traditional sense
			PM:        pm.Maven,
		})
	}

	if len(projects) == 0 {
		return nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "java",
	}
}

// detectGradleWorkspace detects Gradle multi-module projects.
func detectGradleWorkspace(root string) *Workspace {
	// Check for settings.gradle or settings.gradle.kts
	settingsFiles := []string{
		filepath.Join(root, "settings.gradle"),
		filepath.Join(root, "settings.gradle.kts"),
	}

	var settingsPath string
	for _, path := range settingsFiles {
		if _, err := os.Stat(path); err == nil {
			settingsPath = path
			break
		}
	}

	if settingsPath == "" {
		return nil
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	// Parse include() calls
	// Pattern: include("app", "lib") or include "app", "lib"
	includeRegex := regexp.MustCompile(`include\s*\(?\s*["']([^"']+)["']`)
	matches := includeRegex.FindAllStringSubmatch(string(data), -1)

	if len(matches) == 0 {
		// Try alternative pattern: include(":app", ":lib")
		includeRegex2 := regexp.MustCompile(`include\s*\(?\s*["']:([^"']+)["']`)
		matches = includeRegex2.FindAllStringSubmatch(string(data), -1)
	}

	if len(matches) == 0 {
		return nil
	}

	var projects []Project
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		moduleName := match[1]
		// Remove leading colon if present
		moduleName = strings.TrimPrefix(moduleName, ":")

		// Try common locations
		possiblePaths := []string{
			filepath.Join(root, moduleName),
			filepath.Join(root, strings.ReplaceAll(moduleName, ":", string(filepath.Separator))),
		}

		for _, modulePath := range possiblePaths {
			buildGradlePath := filepath.Join(modulePath, "build.gradle")
			buildGradleKtsPath := filepath.Join(modulePath, "build.gradle.kts")

			var manifestPath string
			if _, err := os.Stat(buildGradlePath); err == nil {
				manifestPath = buildGradlePath
			} else if _, err := os.Stat(buildGradleKtsPath); err == nil {
				manifestPath = buildGradleKtsPath
			}

			if manifestPath == "" {
				continue
			}

			projects = append(projects, Project{
				Name:      moduleName,
				Path:      modulePath,
				Ecosystem: "java",
				Manifest:  manifestPath,
				Lockfile:  "", // Gradle uses gradle.lockfile but it's optional
				PM:        pm.Gradle,
			})
			break
		}
	}

	if len(projects) == 0 {
		return nil
	}

	return &Workspace{
		Root:      root,
		Projects:  projects,
		Ecosystem: "java",
	}
}
