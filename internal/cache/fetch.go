package cache

import (
	"os"
	"path/filepath"
	"strings"
)

// ArtifactInfo holds information about a discovered artifact.
type ArtifactInfo struct {
	Path      string
	Name      string
	Version   string
	Ecosystem Ecosystem
}

// FetchAfterInstall scans for new artifacts after an install operation.
func (m *Manager) FetchAfterInstall(ecosystem Ecosystem, dir string) ([]*CacheObject, error) {
	if !m.IsEnabled() {
		return nil, nil
	}

	var cached []*CacheObject

	artifacts, err := m.FindArtifacts(ecosystem, dir)
	if err != nil {
		return nil, err
	}

	for _, artifact := range artifacts {
		// Check if already cached
		if m.Has(ecosystem, artifact.Name, artifact.Version) {
			continue
		}

		// Store in cache
		obj, err := m.Store(ecosystem, artifact.Name, artifact.Version, "local", artifact.Path)
		if err != nil {
			continue // Skip failed artifacts
		}

		cached = append(cached, obj)
	}

	return cached, nil
}

// FindArtifacts discovers artifacts in a directory based on ecosystem.
func (m *Manager) FindArtifacts(ecosystem Ecosystem, dir string) ([]ArtifactInfo, error) {
	switch ecosystem {
	case EcosystemNode:
		return m.FindNodeArtifacts(dir)
	case EcosystemPython:
		return m.FindPythonArtifacts(dir)
	case EcosystemPHP:
		return m.FindComposerArtifacts(dir)
	case EcosystemRust:
		return m.FindCargoArtifacts(dir)
	case EcosystemGo:
		return m.FindGoArtifacts(dir)
	case EcosystemJava:
		return m.FindJavaArtifacts(dir)
	default:
		return nil, nil
	}
}

// FindNodeArtifacts finds .tgz files in npm cache directories.
func (m *Manager) FindNodeArtifacts(dir string) ([]ArtifactInfo, error) {
	var artifacts []ArtifactInfo

	// Look in common npm cache locations
	cacheDirs := []string{
		filepath.Join(dir, "node_modules", ".cache"),
		filepath.Join(dir, ".npm", "_cacache"),
	}

	for _, cacheDir := range cacheDirs {
		if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}

			if info.IsDir() || !strings.HasSuffix(path, ".tgz") {
				return nil
			}

			// Parse name and version from filename (e.g., axios-1.7.0.tgz)
			name, version := parseNodeArtifactName(filepath.Base(path))
			if name == "" || version == "" {
				return nil
			}

			artifacts = append(artifacts, ArtifactInfo{
				Path:      path,
				Name:      name,
				Version:   version,
				Ecosystem: EcosystemNode,
			})

			return nil
		})

		if err != nil {
			continue
		}
	}

	return artifacts, nil
}

// FindPythonArtifacts finds .whl files in pip cache directories.
func (m *Manager) FindPythonArtifacts(dir string) ([]ArtifactInfo, error) {
	var artifacts []ArtifactInfo

	// Look in pip cache locations
	home, _ := os.UserHomeDir()
	cacheDirs := []string{
		filepath.Join(home, ".cache", "pip", "wheels"),
		filepath.Join(dir, ".pip-cache"),
	}

	for _, cacheDir := range cacheDirs {
		if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if info.IsDir() || !strings.HasSuffix(path, ".whl") {
				return nil
			}

			// Parse name and version from wheel filename
			name, version := parseWheelName(filepath.Base(path))
			if name == "" || version == "" {
				return nil
			}

			artifacts = append(artifacts, ArtifactInfo{
				Path:      path,
				Name:      name,
				Version:   version,
				Ecosystem: EcosystemPython,
			})

			return nil
		})

		if err != nil {
			continue
		}
	}

	return artifacts, nil
}

// FindComposerArtifacts finds .zip files in composer cache directories.
func (m *Manager) FindComposerArtifacts(dir string) ([]ArtifactInfo, error) {
	var artifacts []ArtifactInfo

	home, _ := os.UserHomeDir()
	cacheDirs := []string{
		filepath.Join(home, ".composer", "cache", "files"),
		filepath.Join(home, ".cache", "composer", "files"),
	}

	for _, cacheDir := range cacheDirs {
		if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if info.IsDir() || !strings.HasSuffix(path, ".zip") {
				return nil
			}

			// Parse from path structure: vendor/package/version/hash.zip
			name, version := parseComposerPath(path, cacheDir)
			if name == "" || version == "" {
				return nil
			}

			artifacts = append(artifacts, ArtifactInfo{
				Path:      path,
				Name:      name,
				Version:   version,
				Ecosystem: EcosystemPHP,
			})

			return nil
		})

		if err != nil {
			continue
		}
	}

	return artifacts, nil
}

// FindCargoArtifacts finds .crate files in cargo cache directories.
func (m *Manager) FindCargoArtifacts(dir string) ([]ArtifactInfo, error) {
	var artifacts []ArtifactInfo

	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cargo", "registry", "cache")

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return artifacts, nil
	}

	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() || !strings.HasSuffix(path, ".crate") {
			return nil
		}

		// Parse name and version from crate filename (e.g., serde-1.0.196.crate)
		name, version := parseCrateName(filepath.Base(path))
		if name == "" || version == "" {
			return nil
		}

		artifacts = append(artifacts, ArtifactInfo{
			Path:      path,
			Name:      name,
			Version:   version,
			Ecosystem: EcosystemRust,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return artifacts, nil
}

// FindGoArtifacts finds module zip files in Go module cache.
func (m *Manager) FindGoArtifacts(dir string) ([]ArtifactInfo, error) {
	var artifacts []ArtifactInfo

	// Go module cache location
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}

	cacheDir := filepath.Join(gopath, "pkg", "mod", "cache", "download")

	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return artifacts, nil
	}

	err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() || !strings.HasSuffix(path, ".zip") {
			return nil
		}

		// Parse module path and version from path structure
		name, version := parseGoModulePath(path, cacheDir)
		if name == "" || version == "" {
			return nil
		}

		artifacts = append(artifacts, ArtifactInfo{
			Path:      path,
			Name:      name,
			Version:   version,
			Ecosystem: EcosystemGo,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return artifacts, nil
}

// FindJavaArtifacts finds .jar files in Maven/Gradle cache directories.
func (m *Manager) FindJavaArtifacts(dir string) ([]ArtifactInfo, error) {
	var artifacts []ArtifactInfo

	home, _ := os.UserHomeDir()
	cacheDirs := []string{
		filepath.Join(home, ".m2", "repository"),
		filepath.Join(home, ".gradle", "caches", "modules-2", "files-2.1"),
	}

	for _, cacheDir := range cacheDirs {
		if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if info.IsDir() || !strings.HasSuffix(path, ".jar") {
				return nil
			}

			// Skip sources and javadoc jars
			base := filepath.Base(path)
			if strings.Contains(base, "-sources") || strings.Contains(base, "-javadoc") {
				return nil
			}

			// Parse from path structure
			name, version := parseJarPath(path, cacheDir)
			if name == "" || version == "" {
				return nil
			}

			artifacts = append(artifacts, ArtifactInfo{
				Path:      path,
				Name:      name,
				Version:   version,
				Ecosystem: EcosystemJava,
			})

			return nil
		})

		if err != nil {
			continue
		}
	}

	return artifacts, nil
}

// parseNodeArtifactName extracts name and version from npm tarball filename.
func parseNodeArtifactName(filename string) (string, string) {
	// Format: package-version.tgz or @scope-package-version.tgz
	name := strings.TrimSuffix(filename, ".tgz")

	// Find the last hyphen followed by a version-like string
	idx := strings.LastIndex(name, "-")
	if idx == -1 {
		return "", ""
	}

	version := name[idx+1:]
	pkgName := name[:idx]

	// Basic version validation
	if !isVersionLike(version) {
		return "", ""
	}

	return pkgName, version
}

// parseWheelName extracts name and version from Python wheel filename.
func parseWheelName(filename string) (string, string) {
	// Format: package-version-py3-none-any.whl
	parts := strings.Split(filename, "-")
	if len(parts) < 2 {
		return "", ""
	}

	return parts[0], parts[1]
}

// parseCrateName extracts name and version from Rust crate filename.
func parseCrateName(filename string) (string, string) {
	// Format: crate-version.crate
	name := strings.TrimSuffix(filename, ".crate")

	idx := strings.LastIndex(name, "-")
	if idx == -1 {
		return "", ""
	}

	return name[:idx], name[idx+1:]
}

// parseComposerPath extracts name and version from composer cache path.
func parseComposerPath(path, cacheDir string) (string, string) {
	rel, err := filepath.Rel(cacheDir, path)
	if err != nil {
		return "", ""
	}

	// Format: vendor/package/version/hash.zip
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 3 {
		return "", ""
	}

	name := parts[0] + "/" + parts[1]
	version := parts[2]

	return name, version
}

// parseGoModulePath extracts module path and version from Go module cache path.
func parseGoModulePath(path, cacheDir string) (string, string) {
	rel, err := filepath.Rel(cacheDir, path)
	if err != nil {
		return "", ""
	}

	// Format: module/path/@v/version.zip
	dir := filepath.Dir(rel)
	base := filepath.Base(rel)

	version := strings.TrimSuffix(base, ".zip")
	modulePath := strings.TrimSuffix(dir, "/@v")

	return modulePath, version
}

// parseJarPath extracts artifact name and version from Maven/Gradle cache path.
func parseJarPath(path, cacheDir string) (string, string) {
	rel, err := filepath.Rel(cacheDir, path)
	if err != nil {
		return "", ""
	}

	// Maven format: group/artifact/version/artifact-version.jar
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 3 {
		return "", ""
	}

	// Get version from second-to-last directory
	version := parts[len(parts)-2]
	artifact := parts[len(parts)-3]

	return artifact, version
}

// isVersionLike checks if a string looks like a version number.
func isVersionLike(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Version should start with a digit
	if s[0] < '0' || s[0] > '9' {
		return false
	}

	// Should contain at least one dot
	return strings.Contains(s, ".")
}

