package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
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
	keys := make([]string, 0, len(cfg.Search))
	for k := range cfg.Search {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := cfg.Search[k]
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
	path := config.Path()
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
			currentCfg.Prefer = editPreferList(os.Stdin, currentCfg.Prefer)
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

// editPreferList reads a whole line ("pnpm, pip") from in. Invalid IDs are
// reported and the current list is kept.
func editPreferList(in io.Reader, current []string) []string {
	fmt.Println("\nCurrent prefer list:", strings.Join(current, ", "))
	fmt.Println("Enter package manager IDs separated by commas (e.g., npm,pip,cargo)")
	fmt.Println("Leave empty to clear the list.")
	fmt.Print("> ")

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return current
	}
	list, err := parsePreferList(line)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return current
	}
	return list
}

// parsePreferList splits "pnpm, pip" into IDs and validates them.
func parsePreferList(s string) ([]string, error) {
	list := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			list = append(list, p)
		}
	}
	if errs := pm.ValidateConfig(list, nil); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return list, nil
}

// parseBool accepts true/false, yes/no, on/off and 1/0.
func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "on", "1":
		return true, nil
	case "false", "no", "off", "0":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q (use true or false)", s)
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

// setConfigValue sets a configuration value from the command line. Values
// are validated before anything is written.
func setConfigValue(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: xpm config set <key> <value>")
		fmt.Fprintln(os.Stderr, "Keys: prefer, autoInstallPM, interactive, search.<ecosystem>")
		return 1
	}
	key, value := args[0], args[1]
	currentCfg := config.Load()

	var err error
	switch {
	case key == "prefer":
		currentCfg.Prefer, err = parsePreferList(value)
	case key == "autoInstallPM":
		currentCfg.AutoInstallPM, err = parseBool(value)
	case key == "interactive":
		currentCfg.Interactive, err = parseBool(value)
	case strings.HasPrefix(key, "search."):
		eco := strings.TrimPrefix(key, "search.")
		var on bool
		if on, err = parseBool(value); err == nil {
			if errs := pm.ValidateConfig(nil, map[string]bool{eco: on}); len(errs) > 0 {
				err = errors.Join(errs...)
			}
		}
		if err == nil {
			if currentCfg.Search == nil {
				currentCfg.Search = map[string]bool{}
			}
			currentCfg.Search[eco] = on
		}
	default:
		err = fmt.Errorf("unknown config key %q", key)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
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
	path := config.Path()
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
	path := config.Path()
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

	// Write through a symlinked config (dotfile managers) and keep the
	// existing file's mode; 0644 only applies to a new file.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
		dir = filepath.Dir(path)
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}

	// Write to a temp file in the same directory, then rename, so a crash
	// never leaves a truncated config behind.
	tmp, err := os.CreateTemp(dir, ".xpmrc-*.json")
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
