package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
)

// isolatedHome points the config file at a temp dir.
func isolatedHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
}

func TestConfigSetValidatesBeforeWriting(t *testing.T) {
	isolatedHome(t)
	for _, args := range [][]string{
		{"prefer", "npm,notapm"},
		{"interactive", "maybe"},
		{"search.npmx", "false"},
		{"search.maven", "nope"},
		{"colour", "blue"},
	} {
		var code int
		captureStdout(t, func() { code = setConfigValue(args) })
		if code != 1 {
			t.Errorf("config set %v exited %d, want 1", args, code)
		}
	}
	if _, err := os.Stat(getConfigPath()); err == nil {
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
