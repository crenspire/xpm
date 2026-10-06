package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
