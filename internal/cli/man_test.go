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
	for _, c := range []string{"install", "ci", "run", "which", "list", "update", "remove", "info", "search", "lock", "graph", "workspaces", "env", "doctor", "config", "version", "help", "man"} {
		if !strings.Contains(out, c) {
			t.Errorf("usage missing %q", c)
		}
	}
}

func TestUsageMarksExperimental(t *testing.T) {
	out := captureStdout(t, usage)
	for _, c := range []string{"env", "graph", "lock", "workspaces"} {
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
	for in, want := range map[string]string{"i": "install", "-V": "version", "--version": "version", "-v": "version", "-h": "help", "--help": "help", "rm": "remove", "ci": "ci"} {
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
		"workspaces": showWorkspacesHelp,
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

func TestManUnknownCommandExitsOne(t *testing.T) {
	var code int
	captureStdout(t, func() { code = cmdMan([]string{"nosuchcommand"}) })
	if code != 1 {
		t.Fatalf("cmdMan(unknown) = %d, want 1", code)
	}
}

func TestManListPrintsExperimentalNote(t *testing.T) {
	out := captureStdout(t, listCommands)
	if !strings.Contains(out, experimentalNote) {
		t.Errorf("xpm man list lacks the experimental note")
	}
}

func TestRunHelpPutsWorkspaceFlagBeforeTask(t *testing.T) {
	if out := captureStdout(t, showRunHelp); !strings.Contains(out, "xpm run -w <task>") {
		t.Errorf("run help lacks `xpm run -w <task>`")
	}
	page, err := GenerateManPage("run")
	if err != nil {
		t.Fatal(err)
	}
	page = roffFontRe.ReplaceAllString(page, "")
	if !strings.Contains(page, "[-w|--workspace] [task]") {
		t.Errorf("run man synopsis must put -w before the task:\n%s", page)
	}
}

func TestManPageTHLine(t *testing.T) {
	page, err := GenerateManPage("config")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page, `.TH "XPM-CONFIG" "1" "`) || !strings.Contains(page, `"User Commands"`) {
		t.Errorf("malformed .TH line:\n%s", page)
	}
}

// runCLI runs Run with the given arguments and returns the exit code and stderr.
func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	isolatedHome(t)
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = append([]string{"xpm"}, args...)
	var code int
	var stderr string
	captureStdout(t, func() {
		stderr = captureStderr(t, func() { code = Run() })
	})
	return code, stderr
}

func TestRemovedCacheCommandIsUnknown(t *testing.T) {
	wantCode, _ := runCLI(t, "definitely-not-a-command")
	if wantCode == 0 {
		t.Fatalf("unknown command exit code = 0")
	}
	for _, c := range []string{"cache", "cc", "cg"} {
		code, stderr := runCLI(t, c)
		if code != wantCode {
			t.Errorf("%s: exit code %d, want %d", c, code, wantCode)
		}
		if want := "unknown command: " + c; !strings.Contains(stderr, want) {
			t.Errorf("%s: stderr %q does not contain %q", c, stderr, want)
		}
	}
}

func TestUsageHasNoCache(t *testing.T) {
	out := captureStdout(t, usage)
	if strings.Contains(out, "cache") {
		t.Errorf("usage still mentions cache:\n%s", out)
	}
}

func TestGraphAndLockDocsCoverFlagsAndExitStatus(t *testing.T) {
	graphPage, err := GenerateManPage("graph")
	if err != nil {
		t.Fatal(err)
	}
	lockPage, err := GenerateManPage("lock")
	if err != nil {
		t.Fatal(err)
	}
	graphPage = roffFontRe.ReplaceAllString(graphPage, "")
	lockPage = roffFontRe.ReplaceAllString(lockPage, "")
	graphHelp := ansiRe.ReplaceAllString(captureStdout(t, showGraphHelp), "")
	lockHelp := ansiRe.ReplaceAllString(captureStdout(t, showLockHelp), "")

	for label, out := range map[string]string{"graph help": graphHelp, "graph man": graphPage} {
		wantW := "(-w)" // help: "--workspace ... (-w)"
		if label == "graph man" {
			wantW = "--workspace, -w"
		}
		for _, want := range []string{"--json", "--svg", "--exec", "--depth", "--workspace", wantW, "exits with status 2"} {
			if !strings.Contains(strings.ToLower(out), want) {
				t.Errorf("%s: missing %q", label, want)
			}
		}
	}
	for label, out := range map[string]string{"lock help": lockHelp, "lock man": lockPage} {
		for _, want := range []string{"unchanged, changed or missing", "as added", "(or there is nothing to verify)", "does not exist"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: missing %q", label, want)
			}
		}
	}

	top, err := GenerateManPage("install")
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(top, ".SH EXIT STATUS")
	j := strings.Index(top, ".SH ENVIRONMENT")
	if i < 0 || j < i || !strings.Contains(roffFontRe.ReplaceAllString(top[i:j], ""), "2 on a graph usage error") {
		t.Errorf("EXIT STATUS section does not describe exit 2 for graph:\n%s", top)
	}
}
