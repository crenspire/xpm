package env

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// CreateShims creates shim executables for all installed runtimes.
func CreateShims(manager *Manager) error {
	// Get all installed runtimes and their binaries
	runtimeBinaries := make(map[string][]string)

	runtimesPath := manager.GetRuntimesPath()
	entries, err := os.ReadDir(runtimesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		runtime := entry.Name()
		installer, err := GetInstaller(runtime)
		if err != nil {
			continue
		}

		// Get active version to determine binary paths
		active, err := manager.ActiveVersion(runtime)
		if err != nil {
			continue
		}
		version := active.Version

		versionPath := filepath.Join(runtimesPath, runtime, version)
		binaryPaths := installer.BinaryPaths(version, versionPath)

		// Extract binary names
		var binaries []string
		for _, binPath := range binaryPaths {
			binName := filepath.Base(binPath)
			binaries = append(binaries, binName)
		}

		runtimeBinaries[runtime] = binaries
	}

	// Create shims for each binary
	for runtime, binaries := range runtimeBinaries {
		for _, binary := range binaries {
			if err := createShim(manager, runtime, binary); err != nil {
				return fmt.Errorf("failed to create shim for %s/%s: %w", runtime, binary, err)
			}
		}
	}

	return nil
}

// createShim creates a single shim executable.
func createShim(manager *Manager, runtime, binaryName string) error {
	shimsPath := manager.GetShimsPath()
	shimPath := filepath.Join(shimsPath, binaryName)

	// Generate shim code
	code, err := generateShimCode(runtime, binaryName, manager.GetEnvPath())
	if err != nil {
		return err
	}

	// Write shim source
	sourcePath := shimPath + ".go"
	if err := os.WriteFile(sourcePath, []byte(code), 0644); err != nil {
		return err
	}

	// Compile shim
	cmd := exec.Command("go", "build", "-o", shimPath, sourcePath)
	cmd.Dir = shimsPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(sourcePath)
		return fmt.Errorf("failed to compile shim: %v\n%s", err, output)
	}

	// Remove source file
	os.Remove(sourcePath)

	return nil
}

// generateShimCode generates Go code for a shim executable.
func generateShimCode(runtime, binaryName, envPath string) (string, error) {
	tmpl := `package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	runtime := "{{.Runtime}}"
	binary := "{{.Binary}}"
	envPath := {{printf "%q" .EnvPath}}

	// Get active version
	version := getActiveVersion(runtime, envPath)
	if version == "" {
		fmt.Fprintf(os.Stderr, "xpm: no active version found for %s\n", runtime)
		fmt.Fprintf(os.Stderr, "Run: xpm env use %s@<version>\n", runtime)
		os.Exit(1)
	}
	if !safeVersion(version) {
		fmt.Fprintf(os.Stderr, "xpm: refusing unsafe version %q for %s (check .xpm-env)\n", version, runtime)
		os.Exit(1)
	}

	// Locate binary
	binPath := filepath.Join(envPath, "runtimes", runtime, version, "bin", binary)
	
	// Check if binary exists
	if _, err := os.Stat(binPath); err != nil {
		// Try without bin/ prefix
		binPath = filepath.Join(envPath, "runtimes", runtime, version, binary)
		if _, err := os.Stat(binPath); err != nil {
			fmt.Fprintf(os.Stderr, "xpm: binary %s not found for %s@%s\n", binary, runtime, version)
			os.Exit(1)
		}
	}

	// Execute binary
	cmd := exec.Command(binPath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	// Preserve environment variables that might be needed
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		// If it's not an exit error, it might be a signal or other issue
		fmt.Fprintf(os.Stderr, "xpm: failed to execute %s: %v\n", binary, err)
		os.Exit(1)
	}
}

func getActiveVersion(runtime, envPath string) string {
	// Check local .xpm-env
	cwd, err := os.Getwd()
	if err == nil {
		version := findLocalVersion(cwd, runtime)
		if version != "" {
			// Resolve aliases (lts, latest) to actual versions
			if version == "lts" || version == "latest" {
				resolved := resolveAliasToVersion(runtime, version, envPath)
				if resolved != "" {
					return resolved
				}
			}
			return version
		}
	}

	// Check global active.json
	activePath := filepath.Join(envPath, "active.json")
	if data, err := os.ReadFile(activePath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			runtimePrefix := "\"" + runtime + "\""
			if strings.HasPrefix(line, runtimePrefix) {
				// Simple JSON parsing for "runtime": "version"
				parts := strings.Split(line, ":")
				if len(parts) == 2 {
					version := strings.Trim(parts[1], "\", ")
					// Resolve aliases
					if version == "lts" || version == "latest" {
						resolved := resolveAliasToVersion(runtime, version, envPath)
						if resolved != "" {
							return resolved
						}
					}
					return version
				}
			}
		}
	}

	// Check defaults.json
	defaultsPath := filepath.Join(envPath, "defaults.json")
	if data, err := os.ReadFile(defaultsPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			runtimePrefix := "\"" + runtime + "\""
			if strings.HasPrefix(line, runtimePrefix) {
				parts := strings.Split(line, ":")
				if len(parts) == 2 {
					version := strings.Trim(parts[1], "\", ")
					// Resolve aliases
					if version == "lts" || version == "latest" {
						resolved := resolveAliasToVersion(runtime, version, envPath)
						if resolved != "" {
							return resolved
						}
					}
					return version
				}
			}
		}
	}

	return ""
}

func resolveAliasToVersion(runtime, alias, envPath string) string {
	runtimePath := filepath.Join(envPath, "runtimes", runtime)
	entries, err := os.ReadDir(runtimePath)
	if err != nil {
		return ""
	}
	
	// Look for a version directory that has this alias in its metadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		version := entry.Name()
		// Skip alias directories themselves
		if version == "latest" || version == "lts" || version == "" {
			continue
		}
		
		versionPath := filepath.Join(runtimePath, version)
		metaPath := filepath.Join(versionPath, ".xpm-meta.json")
		if data, err := os.ReadFile(metaPath); err == nil {
			// Simple check for alias in JSON
			if strings.Contains(string(data), "\"alias\":\""+alias+"\"") {
				return version
			}
		}
	}
	
	return ""
}

func findLocalVersion(dir, runtime string) string {
	for {
		envFile := filepath.Join(dir, ".xpm-env")
		if data, err := os.ReadFile(envFile); err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, runtime+"=") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// safeVersion mirrors env.ValidateVersionSpec; shims are standalone binaries.
func safeVersion(v string) bool {
	if v == "" || len(v) > 64 || strings.Contains(v, "..") || v[0] == '.' || v[0] == '-' {
		return false
	}
	for _, r := range v {
		ok := r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r == '.' || r == '_' || r == '+' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}
`

	t := template.Must(template.New("shim").Parse(tmpl))
	var buf strings.Builder
	if err := t.Execute(&buf, map[string]string{
		"Runtime": runtime,
		"Binary":  binaryName,
		"EnvPath": envPath,
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// UpdatePATH modifies shell profiles to add shims to PATH.
func UpdatePATH(manager *Manager) error {
	shimsPath := manager.GetShimsPath()
	pathEntry := fmt.Sprintf("export PATH=\"%s:$PATH\"", shimsPath)

	// Detect shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	var profileFiles []string
	if strings.Contains(shell, "zsh") {
		home, _ := os.UserHomeDir()
		profileFiles = []string{
			filepath.Join(home, ".zshrc"),
			filepath.Join(home, ".zprofile"),
		}
	} else if strings.Contains(shell, "fish") {
		home, _ := os.UserHomeDir()
		profileFiles = []string{
			filepath.Join(home, ".config", "fish", "config.fish"),
		}
		pathEntry = fmt.Sprintf("set -gx PATH %s $PATH", shimsPath)
	} else {
		// Default to bash
		home, _ := os.UserHomeDir()
		profileFiles = []string{
			filepath.Join(home, ".bashrc"),
			filepath.Join(home, ".bash_profile"),
			filepath.Join(home, ".profile"),
		}
	}

	// Check if already added
	for _, profileFile := range profileFiles {
		if _, err := os.Stat(profileFile); err != nil {
			continue
		}

		data, err := os.ReadFile(profileFile)
		if err != nil {
			continue
		}

		content := string(data)
		if strings.Contains(content, shimsPath) {
			fmt.Printf("PATH already configured in %s\n", profileFile)
			return nil
		}

		// Append to profile
		entry := "\n# Added by xpm\n" + pathEntry + "\n"
		if err := os.WriteFile(profileFile, append(data, []byte(entry)...), 0644); err != nil {
			return fmt.Errorf("failed to write to %s: %w", profileFile, err)
		}

		fmt.Printf("Added %s to PATH in %s\n", shimsPath, profileFile)
		fmt.Printf("Please restart your shell or run: source %s\n", profileFile)
		return nil
	}

	// No profile file found, create one
	if len(profileFiles) > 0 {
		profileFile := profileFiles[0]
		entry := "# Added by xpm\n" + pathEntry + "\n"
		if err := os.WriteFile(profileFile, []byte(entry), 0644); err != nil {
			return fmt.Errorf("failed to create %s: %w", profileFile, err)
		}
		fmt.Printf("Created %s with PATH configuration\n", profileFile)
		fmt.Printf("Please restart your shell or run: source %s\n", profileFile)
	}

	return nil
}

// CheckPATH verifies if shims are in PATH.
func CheckPATH(manager *Manager) bool {
	shimsPath := manager.GetShimsPath()
	path := os.Getenv("PATH")
	return strings.Contains(path, shimsPath)
}
