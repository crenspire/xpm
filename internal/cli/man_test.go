package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// roffFontRe matches roff font escapes (\fB, \fI, \fR, \fP).
var roffFontRe = regexp.MustCompile(`\\f[BIRP]`)

func lineWith(out, name string) string {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(ansiRe.ReplaceAllString(l, ""))
		if len(f) > 0 && strings.TrimSuffix(f[0], ",") == name {
			return l
		}
	}
	return ""
}

func TestUsageListsEveryDispatchedCommand(t *testing.T) {
	out := captureStdout(t, usage)
	for _, c := range []string{"install", "ci", "run", "which", "list", "update", "remove", "info", "search", "lock", "cache", "cc", "cg", "graph", "workspaces", "env", "doctor", "config", "version", "help", "man"} {
		if !strings.Contains(out, c) {
			t.Errorf("usage missing %q", c)
		}
	}
}

func TestUsageMarksExperimental(t *testing.T) {
	out := captureStdout(t, usage)
	for _, c := range []string{"env", "graph", "lock", "workspaces", "cache"} {
		l := lineWith(out, c)
		if l == "" || !strings.Contains(l, "(experimental)") {
			t.Errorf("%s line not marked experimental: %q", c, l)
		}
	}
	for _, c := range []string{"install", "which", "search"} {
		l := lineWith(out, c)
		if l == "" || strings.Contains(l, "(experimental)") {
			t.Errorf("%s line wrong: %q", c, l)
		}
	}
}

func TestManListMatchesUsage(t *testing.T) {
	u := captureStdout(t, usage)
	m := captureStdout(t, listCommands)
	for _, c := range commandTable {
		ul, ml := lineWith(u, c.name), lineWith(m, c.name)
		if ul == "" || ul != ml {
			t.Errorf("%s: usage %q vs man %q", c.name, ul, ml)
		}
	}
}

func TestNormalizeCommandAliases(t *testing.T) {
	for in, want := range map[string]string{"cc": "cache", "cg": "cache", "i": "install", "-V": "version", "--version": "version", "-v": "version", "-h": "help", "--help": "help", "rm": "remove", "ci": "ci"} {
		if got := normalizeCommand(in); got != want {
			t.Errorf("normalizeCommand(%q)=%q want %q", in, got, want)
		}
	}
}

func TestEveryCommandHasManPage(t *testing.T) {
	names := []string{"ci"}
	for _, c := range commandTable {
		names = append(names, c.name)
	}
	for _, name := range names {
		if d := getManPageData(name); d.Description == "xpm command" {
			t.Errorf("%s: no man page data (default fallthrough)", name)
		}
		if _, err := GenerateManPage(name); err != nil {
			t.Errorf("%s: GenerateManPage: %v", name, err)
		}
		out := captureStdout(t, func() { showCommandHelp(name) })
		if strings.Contains(out, "Available commands") {
			t.Errorf("xpm man %s falls through to the command list", name)
		}
	}
}

func TestGenerateAllManPagesCoversTable(t *testing.T) {
	dir := t.TempDir()
	var err error
	captureStdout(t, func() { err = GenerateAllManPages(dir) })
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range commandTable {
		if _, err := os.Stat(filepath.Join(dir, "xpm-"+c.name+".1")); err != nil {
			t.Errorf("missing page for %s: %v", c.name, err)
		}
	}
}

func TestInstallManDescribesCurrentFlags(t *testing.T) {
	page, err := GenerateManPage("install")
	if err != nil {
		t.Fatal(err)
	}
	page = roffFontRe.ReplaceAllString(page, "")
	help := ansiRe.ReplaceAllString(captureStdout(t, showInstallHelp), "")
	for label, out := range map[string]string{"xpm man install": help, "xpm-install.1": page} {
		for _, want := range []string{"[--]", "ends flag parsing", "several", "-g", "before or after the packages", "first failure", "go get", "exit status 1"} {
			if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
				t.Errorf("%s: missing %q", label, want)
			}
		}
		if strings.Contains(out, "--workspace") {
			t.Errorf("%s: mentions --workspace", label)
		}
	}
}

func TestManPagesShareExitAndEnvSections(t *testing.T) {
	for _, c := range commandTable {
		page, err := GenerateManPage(c.name)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{".SH EXIT STATUS", ".SH ENVIRONMENT", "XPM_NO_CACHE", "XPM_CACHE_DIR", ".SH FILES"} {
			if !strings.Contains(page, want) {
				t.Errorf("%s page missing %q", c.name, want)
			}
		}
	}
}

func TestExperimentalManPagesSayExperimental(t *testing.T) {
	for name, fn := range map[string]func(){
		"env": showEnvHelp, "graph": showGraphHelp, "lock": showLockHelp,
		"workspaces": showWorkspacesHelp, "cache": showCacheHelp,
	} {
		if out := captureStdout(t, fn); !strings.Contains(out, "experimental") {
			t.Errorf("xpm man %s does not say experimental", name)
		}
		page, err := GenerateManPage(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(page, "experimental") {
			t.Errorf("xpm-%s.1 does not say experimental", name)
		}
	}
}

func TestWorkspacesManHasNoInstallWorkspace(t *testing.T) {
	out := ansiRe.ReplaceAllString(captureStdout(t, showWorkspacesHelp), "")
	if strings.Contains(out, "install --workspace") {
		t.Errorf("workspaces help mentions the nonexistent install --workspace:\n%s", out)
	}
	page, _ := GenerateManPage("workspaces")
	if strings.Contains(page, "install --workspace") {
		t.Error("workspaces page mentions install --workspace")
	}
}

func TestCiManPage(t *testing.T) {
	help := ansiRe.ReplaceAllString(captureStdout(t, showCiHelp), "")
	page, err := GenerateManPage("ci")
	if err != nil {
		t.Fatal(err)
	}
	for label, out := range map[string]string{"xpm man ci": help, "xpm-ci.1": page} {
		for _, want := range []string{"npm ci", "--frozen-lockfile", "--immutable", "pipenv install --deploy", "cargo build --locked", "go mod download", "node_modules/", "vendor/", "no arguments"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q", label, want)
			}
		}
	}
}

func TestConfigManListsSettableKeys(t *testing.T) {
	help := ansiRe.ReplaceAllString(captureStdout(t, showConfigHelp), "")
	page, _ := GenerateManPage("config")
	for label, out := range map[string]string{"xpm man config": help, "xpm-config.1": page} {
		for _, want := range []string{"prefer", "autoInstallPM", "interactive", "search.", "unknown"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q", label, want)
			}
		}
	}
}
