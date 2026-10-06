package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellProfile(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	cases := []struct {
		shell, goos string
		envs        map[string]string
		want        string
		fish        bool
	}{
		{"zsh", "darwin", nil, "/h/.zshrc", false},
		{"zsh", "linux", map[string]string{"ZDOTDIR": "/z"}, "/z/.zshrc", false},
		{"bash", "linux", nil, "/h/.bashrc", false},
		{"bash", "darwin", nil, "/h/.bash_profile", false},
		{"fish", "linux", nil, "/h/.config/fish/conf.d/xpm.fish", true},
		{"fish", "linux", map[string]string{"XDG_CONFIG_HOME": "/x"}, "/x/fish/conf.d/xpm.fish", true},
		{"sh", "linux", nil, "/h/.profile", false},
		{"tcsh", "darwin", nil, "/h/.profile", false},
		{"", "linux", nil, "/h/.profile", false},
	}
	for _, c := range cases {
		env = c.envs
		file, fish := shellProfile(c.shell, "/h", c.goos, getenv)
		if file != filepath.FromSlash(c.want) || fish != c.fish {
			t.Errorf("%s/%s: got %s %v, want %s %v", c.shell, c.goos, file, fish, c.want, c.fish)
		}
	}
}

func TestPathLineEscapes(t *testing.T) {
	if got := pathLine(`/a b/"q"/$x/`+"`c`"+`/\`, false); got != `export PATH="/a b/\"q\"/\$x/\`+"`c\\`"+`/\\:$PATH"` {
		t.Errorf("sh line = %s", got)
	}
	if got := pathLine(`/it's/\`, true); got != `fish_add_path --prepend '/it\'s/\\'` {
		t.Errorf("fish line = %s", got)
	}
}

func TestSetupPATHAppendsOnceToOneFile(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	home, _ := os.UserHomeDir()
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")
	rc := filepath.Join(home, ".zshrc")
	// Mentioning the shims dir in a comment is not "configured".
	orig := "# old: " + m.GetShimsPath() + "\nalias ll='ls -l'"
	if err := os.WriteFile(rc, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	edit, err := SetupPATH(m)
	if err != nil || !edit.Changed || edit.File != rc {
		t.Fatalf("SetupPATH = %+v, %v", edit, err)
	}
	data, _ := os.ReadFile(rc)
	want := orig + "\n# Added by xpm\nexport PATH=\"" + m.GetShimsPath() + ":$PATH\"\n"
	if string(data) != want {
		t.Fatalf(".zshrc = %q, want %q", data, want)
	}
	if fi, _ := os.Stat(rc); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode changed to %v", fi.Mode().Perm())
	}
	edit, err = SetupPATH(m)
	if err != nil || edit.Changed {
		t.Fatalf("second SetupPATH = %+v, %v", edit, err)
	}
	if data2, _ := os.ReadFile(rc); string(data2) != want {
		t.Fatal("second run changed the file")
	}
	for _, other := range []string{".zprofile", ".bashrc", ".profile"} {
		if _, err := os.Stat(filepath.Join(home, other)); err == nil {
			t.Fatalf("%s was created", other)
		}
	}
}

func TestSetupPATHFishCreatesConfDir(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	home, _ := os.UserHomeDir()
	t.Setenv("SHELL", "/usr/local/bin/fish")
	t.Setenv("XDG_CONFIG_HOME", "")
	edit, err := SetupPATH(m)
	if err != nil {
		t.Fatal(err)
	}
	if edit.File != filepath.Join(home, ".config", "fish", "conf.d", "xpm.fish") {
		t.Fatalf("file = %s", edit.File)
	}
	data, _ := os.ReadFile(edit.File)
	if !strings.Contains(string(data), "fish_add_path --prepend '"+m.GetShimsPath()+"'") {
		t.Fatalf("content = %q", data)
	}
}

func TestOnPATHExactEntry(t *testing.T) {
	sep := string(os.PathListSeparator)
	shims := filepath.FromSlash("/h/.xpm/env/shims")
	cases := map[string]bool{
		shims + sep + "/usr/bin":                 true,
		"/usr/bin" + sep + shims + "/":           true,
		shims + "-old" + sep + "/usr/bin":        false,
		filepath.FromSlash("/h/.xpm/env/shimsx"): false,
		"":                                       false,
	}
	for p, want := range cases {
		if got := onPATH(p, shims); got != want {
			t.Errorf("onPATH(%q) = %v, want %v", p, got, want)
		}
	}
}
