package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/crenspire/xpm/internal/config"
	"github.com/manifoldco/promptui"
)

// cmdConfig handles the config command.
func cmdConfig(args []string) int {
	if len(args) == 0 {
		return showConfig()
	}

	subCmd := args[0]
	rest := args[1:]

	switch subCmd {
	case "show":
		return showConfig()
	case "path":
		return showConfigPath()
	case "edit":
		return editConfig()
	case "set":
		return setConfigValue(rest)
	case "reset":
		return resetConfig()
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", subCmd)
		fmt.Println("\nUsage: xpm config [show|path|edit|set|reset]")
		return 1
	}
}

// showConfig displays the current configuration.
func showConfig() int {
	cfg := config.Load()

	fmt.Println("Current configuration:")
	fmt.Println()

	// Display prefer list
	fmt.Println("Preferred package managers:")
	if len(cfg.Prefer) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, p := range cfg.Prefer {
			fmt.Printf("  - %s\n", p)
		}
	}
	fmt.Println()

	// Display search settings
	fmt.Println("Search settings:")
	for k, v := range cfg.Search {
		status := "enabled"
		if !v {
			status = "disabled"
		}
		fmt.Printf("  - %s: %s\n", k, status)
	}
	fmt.Println()

	// Display other settings
	fmt.Printf("Auto-install package managers: %v\n", cfg.AutoInstallPM)
	fmt.Printf("Interactive mode: %v\n", cfg.Interactive)

	return 0
}

// showConfigPath displays the configuration file path.
func showConfigPath() int {
	path := getConfigPath()
	if path == "" {
		fmt.Println("Could not determine config path")
		return 1
	}

	fmt.Println("Configuration file:", path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("(file does not exist - using defaults)")
	}

	return 0
}

// getConfigPath returns the platform-specific config file path.
func getConfigPath() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("APPDATA")
		if base == "" {
			return ""
		}
		return filepath.Join(base, "xpm", "xpmrc.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "xpm", "xpmrc.json")
}

// editConfig opens an interactive editor for the configuration.
func editConfig() int {
	if !cfg.Interactive {
		fmt.Fprintln(os.Stderr, "Interactive mode is disabled")
		return 1
	}

	currentCfg := config.Load()

	// Menu items
	items := []string{
		"Edit preferred package managers",
		"Toggle search ecosystems",
		"Toggle auto-install package managers",
		"Toggle interactive mode",
		"Save and exit",
		"Cancel",
	}

	for {
		prompt := promptui.Select{
			Label: "Configuration editor",
			Items: items,
		}

		idx, _, err := prompt.Run()
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}

		switch idx {
		case 0:
			currentCfg.Prefer = editPreferList(currentCfg.Prefer)
		case 1:
			currentCfg.Search = editSearchSettings(currentCfg.Search)
		case 2:
			currentCfg.AutoInstallPM = !currentCfg.AutoInstallPM
			fmt.Printf("Auto-install PM: %v\n", currentCfg.AutoInstallPM)
		case 3:
			currentCfg.Interactive = !currentCfg.Interactive
			fmt.Printf("Interactive mode: %v\n", currentCfg.Interactive)
		case 4:
			if err := saveConfig(currentCfg); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save config: %v\n", err)
				return 1
			}
			fmt.Println("Configuration saved.")
			return 0
		case 5:
			fmt.Println("Cancelled.")
			return 0
		}
	}
}

// editPreferList allows editing the prefer list.
func editPreferList(current []string) []string {
	fmt.Println("\nCurrent prefer list:", strings.Join(current, ", "))
	fmt.Println("Enter package manager IDs separated by commas (e.g., npm,pip,cargo)")
	fmt.Println("Leave empty to clear the list.")
	fmt.Print("> ")

	var input string
	fmt.Scanln(&input)

	input = strings.TrimSpace(input)
	if input == "" {
		return []string{}
	}

	parts := strings.Split(input, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// editSearchSettings allows toggling search ecosystems.
func editSearchSettings(current map[string]bool) map[string]bool {
	if current == nil {
		current = map[string]bool{
			"npm":      true,
			"pip":      true,
			"composer": true,
			"cargo":    true,
			"maven":    true,
		}
	}

	ecosystems := []string{"npm", "pip", "composer", "cargo", "maven", "gomod", "gradle"}

	items := []string{}
	for _, eco := range ecosystems {
		status := "enabled"
		if !current[eco] {
			status = "disabled"
		}
		items = append(items, fmt.Sprintf("%s (%s)", eco, status))
	}
	items = append(items, "Done")

	for {
		prompt := promptui.Select{
			Label: "Toggle search ecosystems",
			Items: items,
		}

		idx, _, err := prompt.Run()
		if err != nil || idx == len(items)-1 {
			return current
		}

		eco := ecosystems[idx]
		current[eco] = !current[eco]

		// Update display
		status := "enabled"
		if !current[eco] {
			status = "disabled"
		}
		items[idx] = fmt.Sprintf("%s (%s)", eco, status)
		fmt.Printf("Toggled %s to %s\n", eco, status)
	}
}

// setConfigValue sets a configuration value from the command line.
func setConfigValue(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: xpm config set <key> <value>")
		fmt.Fprintln(os.Stderr, "Keys: prefer, autoInstallPM, interactive, search.<ecosystem>")
		return 1
	}

	key := args[0]
	value := args[1]

	currentCfg := config.Load()

	switch key {
	case "prefer":
		if value == "" {
			currentCfg.Prefer = []string{}
		} else {
			currentCfg.Prefer = strings.Split(value, ",")
		}
	case "autoInstallPM":
		currentCfg.AutoInstallPM = value == "true" || value == "1" || value == "yes"
	case "interactive":
		currentCfg.Interactive = value == "true" || value == "1" || value == "yes"
	default:
		if strings.HasPrefix(key, "search.") {
			eco := strings.TrimPrefix(key, "search.")
			if currentCfg.Search == nil {
				currentCfg.Search = map[string]bool{}
			}
			currentCfg.Search[eco] = value == "true" || value == "1" || value == "yes"
		} else {
			fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
			return 1
		}
	}

	if err := saveConfig(currentCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save config: %v\n", err)
		return 1
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return 0
}

// resetConfig resets the configuration to defaults.
func resetConfig() int {
	path := getConfigPath()
	if path == "" {
		fmt.Fprintln(os.Stderr, "Could not determine config path")
		return 1
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("No config file exists - already using defaults.")
		return 0
	}

	if cfg.Interactive {
		prompt := promptui.Select{
			Label: "Reset configuration to defaults?",
			Items: []string{"Yes", "No"},
		}

		idx, _, err := prompt.Run()
		if err != nil || idx == 1 {
			fmt.Println("Cancelled.")
			return 0
		}
	}

	if err := os.Remove(path); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove config file: %v\n", err)
		return 1
	}

	fmt.Println("Configuration reset to defaults.")
	return 0
}

// saveConfig saves the configuration to the config file.
func saveConfig(c config.Config) error {
	path := getConfigPath()
	if path == "" {
		return fmt.Errorf("could not determine config path")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
