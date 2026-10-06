package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
)

// isolatedHome points the config file at a temp dir on every OS (HOME for
// Unix, USERPROFILE for os.UserHomeDir on Windows, APPDATA for the Windows
// config location) and returns that dir.
func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	return home
}

// TestWriteConfigIsWhatLoadReads guards the test helper: the config it writes
// must be the one config.Load reads, on every OS.
func TestWriteConfigIsWhatLoadReads(t *testing.T) {
	isolatedHome(t)
	writeConfig(t, `{"prefer":["pip"]}`)
	if got := config.Load().Prefer; !reflect.DeepEqual(got, []string{"pip"}) {
		t.Fatalf("Load().Prefer = %v, want [pip]", got)
	}
}

func TestConfigSetValidatesBeforeWriting(t *testing.T) {
	isolatedHome(t)
	for _, tc := range []struct {
		args []string
		want string // text the error must contain
	}{
		{[]string{"prefer", "npm,notapm"}, "notapm"},
		{[]string{"interactive", "maybe"}, "maybe"},
		{[]string{"search.npmx", "false"}, "npmx"},
		{[]string{"search.maven", "nope"}, "nope"},
		{[]string{"colour", "blue"}, "colour"},
	} {
		var code int
		stderr := captureStderr(t, func() { code = setConfigValue(tc.args) })
		if code != 1 {
			t.Errorf("config set %v exited %d, want 1", tc.args, code)
		}
		if !strings.Contains(stderr, tc.want) {
			t.Errorf("config set %v stderr = %q, want it to name %q", tc.args, stderr, tc.want)
		}
	}
	if _, err := os.Stat(config.Path()); err == nil {
		t.Fatal("an invalid `config set` wrote the config file")
	}
}

func TestConfigSetAcceptsValidValues(t *testing.T) {
	isolatedHome(t)
	captureStdout(t, func() {
		for _, args := range [][]string{
			{"prefer", " pnpm, poetry "},
			{"interactive", "off"},
			{"search.maven", "false"},
		} {
			if code := setConfigValue(args); code != 0 {
				t.Fatalf("config set %v exited %d", args, code)
			}
		}
	})
	c := config.Load()
	if !reflect.DeepEqual(c.Prefer, []string{"pnpm", "poetry"}) || c.Interactive || c.Search["maven"] || !c.Search["npm"] {
		t.Fatalf("saved config = prefer %v interactive %v search %v", c.Prefer, c.Interactive, c.Search)
	}
}

func TestEditPreferListReadsTheWholeLine(t *testing.T) {
	var got []string
	captureStdout(t, func() { got = editPreferList(strings.NewReader("pnpm, pip cargo\n"), nil) })
	if got != nil {
		t.Fatalf("got %v; \"pip cargo\" is not an ID, so the list must be kept", got)
	}
	captureStdout(t, func() { got = editPreferList(strings.NewReader("pnpm, pip, cargo\n"), nil) })
	if !reflect.DeepEqual(got, []string{"pnpm", "pip", "cargo"}) {
		t.Fatalf("got %v, want [pnpm pip cargo]", got)
	}
}

func TestShowConfigSearchSettingsAreSorted(t *testing.T) {
	isolatedHome(t)
	first := captureStdout(t, func() { showConfig() })
	second := captureStdout(t, func() { showConfig() })
	if first != second {
		t.Fatalf("showConfig output differs between calls:\n%s\n---\n%s", first, second)
	}
	if c, n := strings.Index(first, "- cargo:"), strings.Index(first, "- npm:"); c < 0 || n < 0 || c > n {
		t.Fatalf("cargo should precede npm in:\n%s", first)
	}
}

func TestSaveConfigLeavesNoTempFiles(t *testing.T) {
	isolatedHome(t)
	c := config.Load()
	if err := saveConfig(c); err != nil {
		t.Fatal(err)
	}
	c.Prefer = []string{"pnpm"}
	if err := saveConfig(c); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(config.Path()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "xpmrc.json" {
		t.Fatalf("config dir holds %v, want only xpmrc.json", entries)
	}
	if got := config.Load().Prefer; !reflect.DeepEqual(got, []string{"pnpm"}) {
		t.Fatalf("Prefer = %v, want [pnpm]", got)
	}
}

func TestSaveConfigWritesThroughSymlink(t *testing.T) {
	isolatedHome(t)
	path := config.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "dotfiles-xpmrc.json")
	if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cfg := config.Load()
	cfg.Prefer = []string{"cargo"}
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("config symlink was replaced by a regular file")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "cargo") {
		t.Fatalf("target not updated: %s", data)
	}
}

func TestSaveConfigKeepsFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes do not apply on Windows")
	}
	isolatedHome(t)
	writeConfig(t, "{}")
	path := config.Path()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(config.Load()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}
