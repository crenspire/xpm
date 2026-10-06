package env

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

// ProfileEdit reports what SetupPATH did.
type ProfileEdit struct {
	File    string // the one shell startup file xpm uses
	Line    string // the PATH line
	Changed bool   // false when File already had Line
}

// shellProfile picks the startup file for a shell (base name of $SHELL).
func shellProfile(shell, home, goos string, getenv func(string) string) (file string, fish bool) {
	switch shell {
	case "zsh":
		dir := getenv("ZDOTDIR")
		if dir == "" {
			dir = home
		}
		return filepath.Join(dir, ".zshrc"), false
	case "bash":
		if goos == "darwin" {
			return filepath.Join(home, ".bash_profile"), false
		}
		return filepath.Join(home, ".bashrc"), false
	case "fish":
		dir := getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".config")
		}
		return filepath.Join(dir, "fish", "conf.d", "xpm.fish"), true
	}
	return filepath.Join(home, ".profile"), false
}

// pathLine is the line that puts shims first on PATH.
func pathLine(shims string, fish bool) string {
	if fish {
		r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
		return "fish_add_path --prepend '" + r.Replace(shims) + "'"
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")
	return `export PATH="` + r.Replace(shims) + `:$PATH"`
}

// hasLine reports whether file contains line (whitespace-trimmed, whole line).
func hasLine(file, line string) bool {
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}

// SetupPATH appends the shims PATH line to exactly one startup file for the
// user's shell ($SHELL), unless that file already has it.
func SetupPATH(m *Manager) (ProfileEdit, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ProfileEdit{}, err
	}
	file, fish := shellProfile(filepath.Base(os.Getenv("SHELL")), home, goruntime.GOOS, os.Getenv)
	edit := ProfileEdit{File: file, Line: pathLine(m.GetShimsPath(), fish)}
	if hasLine(file, edit.Line) {
		return edit, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return edit, err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return edit, err
	}
	if _, err := f.WriteString("\n# Added by xpm\n" + edit.Line + "\n"); err != nil {
		_ = f.Close()
		return edit, err
	}
	if err := f.Close(); err != nil {
		return edit, err
	}
	edit.Changed = true
	return edit, nil
}

// CheckPATH reports whether the shims dir is an exact PATH entry.
func CheckPATH(m *Manager) bool {
	return onPATH(os.Getenv("PATH"), m.GetShimsPath())
}

func onPATH(pathEnv, dir string) bool {
	want := filepath.Clean(dir)
	for _, p := range filepath.SplitList(pathEnv) {
		if p != "" && filepath.Clean(p) == want {
			return true
		}
	}
	return false
}
