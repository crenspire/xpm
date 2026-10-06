package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
)

// inProject chdirs into a temp dir containing files (dirs end in "/").
func inProject(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		p := filepath.Join(dir, f)
		if strings.HasSuffix(f, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	return dir
}

// recordTools replaces tool execution with a recorder; every tool "exists".
func recordTools(t *testing.T) *[]string {
	t.Helper()
	var ran []string
	oldRun, oldExists, oldEnsure := runTool, pmExists, ensurePM
	runTool = func(bin string, args []string) int {
		ran = append(ran, bin+" "+strings.Join(args, " "))
		return 0
	}
	pmExists = func(string) bool { return true }
	ensurePM = func(pm.ID) error { return nil }
	t.Cleanup(func() { runTool, pmExists, ensurePM = oldRun, oldExists, oldEnsure })
	return &ran
}

func TestDetectPythonTool(t *testing.T) {
	for files, want := range map[string]string{
		"Pipfile":                      "pipenv",
		"poetry.lock,pyproject.toml":   "poetry",
		"poetry.lock,requirements.txt": "poetry",
		"requirements.txt":             "pip-req",
	} {
		inProject(t, strings.Split(files, ",")...)
		var kinds []string
		for _, tg := range detectProjectTargets() {
			kinds = append(kinds, tg.Kind)
		}
		if strings.Join(kinds, ",") != want {
			t.Errorf("%s: kinds = %v, want %s", files, kinds, want)
		}
	}
}

func TestResolveTargetPM(t *testing.T) {
	node := projectTarget{Kind: "node", PMs: []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun}}
	lock := func(name string, id pm.ID) pm.ProjectFile {
		return pm.ProjectFile{Name: name, Ecosystem: pm.EcosystemNode, Manager: id}
	}
	for _, c := range []struct {
		name   string
		files  []pm.ProjectFile
		prefer []string
		want   string
	}{
		{"one lock file decides", []pm.ProjectFile{lock("bun.lock", pm.Bun)}, []string{"pnpm"}, "{bun bun.lock []}"},
		{"two lock files, prefer narrows", []pm.ProjectFile{lock("package-lock.json", pm.Npm), lock("yarn.lock", pm.Yarn)}, []string{"yarn"}, "{yarn prefer []}"},
		{"two lock files, user picks", []pm.ProjectFile{lock("package-lock.json", pm.Npm), lock("yarn.lock", pm.Yarn)}, nil, "{  [npm yarn]}"},
		{"no lock file, prefer", nil, []string{"pnpm"}, "{pnpm prefer []}"},
		{"no lock file, default npm", nil, nil, "{npm  []}"},
	} {
		if got := fmt.Sprint(resolveTargetPM(node, c.files, c.prefer)); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	if got := resolveTargetPM(projectTarget{Kind: "cargo", PMs: []pm.ID{pm.Cargo}}, nil, nil); got.PM != pm.Cargo {
		t.Errorf("single-tool project: %+v", got)
	}
}

func TestProjectArgs(t *testing.T) {
	for _, c := range []struct {
		cmd         projectCmd
		action, pkg string
		want        string
	}{
		{projectCmd{Kind: "node", PM: pm.Npm}, "ci", "", "ci"},
		{projectCmd{Kind: "node", PM: pm.Pnpm}, "ci", "", "install --frozen-lockfile"},
		{projectCmd{Kind: "node", PM: pm.Yarn}, "ci", "", "install --frozen-lockfile"},
		{projectCmd{Kind: "node", PM: pm.Yarn, YarnBerry: true}, "ci", "", "install --immutable"},
		{projectCmd{Kind: "node", PM: pm.Bun}, "ci", "", "install --frozen-lockfile"},
		{projectCmd{Kind: "node", PM: pm.Yarn}, "update", "axios", "upgrade axios"},
		{projectCmd{Kind: "node", PM: pm.Yarn, YarnBerry: true}, "update", "", "up *"},
		{projectCmd{Kind: "node", PM: pm.Npm}, "remove", "axios", "uninstall axios"},
		{projectCmd{Kind: "node", PM: pm.Bun}, "list", "", "pm ls"},
		{projectCmd{Kind: "composer", PM: pm.Composer}, "ci", "", "install"},
		{projectCmd{Kind: "pip-req", PM: pm.Pip}, "ci", "", "install -r requirements.txt"},
		{projectCmd{Kind: "pipenv", PM: pm.Pipenv}, "ci", "", "install --deploy"},
		{projectCmd{Kind: "poetry", PM: pm.Poetry}, "remove", "requests", "remove requests"},
		{projectCmd{Kind: "cargo", PM: pm.Cargo}, "ci", "", "build --locked"},
		{projectCmd{Kind: "cargo", PM: pm.Cargo}, "update", "serde", "update -p serde"},
		{projectCmd{Kind: "gomod", PM: pm.GoMod}, "install", "", "mod tidy"},
		{projectCmd{Kind: "gomod", PM: pm.GoMod}, "ci", "", "mod download"},
	} {
		got, err := c.cmd.args(c.action, c.pkg)
		if err != nil || strings.Join(got, " ") != c.want {
			t.Errorf("%+v %s %q: got %q (%v), want %q", c.cmd, c.action, c.pkg, strings.Join(got, " "), err, c.want)
		}
	}
	if _, err := (projectCmd{Kind: "gomod", PM: pm.GoMod}).args("remove", "x"); !strings.Contains(fmt.Sprint(err), "go mod tidy") {
		t.Errorf("go remove must explain the manual step, got %v", err)
	}
}

func TestCleanDirsNeverTouchGoVendor(t *testing.T) {
	for c, want := range map[projectCmd][]string{
		{Kind: "node", PM: pm.Npm}:          nil,
		{Kind: "node", PM: pm.Pnpm}:         {"node_modules"},
		{Kind: "composer", PM: pm.Composer}: {"vendor"},
		{Kind: "gomod", PM: pm.GoMod}:       nil,
	} {
		if got := cleanDirs(c); !reflect.DeepEqual(got, want) {
			t.Errorf("cleanDirs(%+v) = %v, want %v", c, got, want)
		}
	}
}

func TestProjectCommandsUseTheLockfileTool(t *testing.T) {
	withConfig(t, config.Config{})
	inProject(t, "package.json", "pnpm-lock.yaml")
	ran := recordTools(t)
	captureStdout(t, func() {
		cmdInstall(nil)
		cmdList(nil)
		cmdUpdate([]string{"axios"})
		cmdRemove([]string{"axios"})
		cmdCleanInstall(nil)
	})
	want := []string{"pnpm install", "pnpm list --depth=0", "pnpm update axios", "pnpm remove axios", "pnpm install --frozen-lockfile"}
	if !reflect.DeepEqual(*ran, want) {
		t.Fatalf("ran %q\nwant %q", *ran, want)
	}
}

func TestPoetryAndPipenvProjects(t *testing.T) {
	withConfig(t, config.Config{})
	ran := recordTools(t)
	inProject(t, "pyproject.toml", "poetry.lock")
	captureStdout(t, func() { cmdInstall(nil) })
	inProject(t, "Pipfile", "Pipfile.lock")
	captureStdout(t, func() { cmdCleanInstall(nil) })
	if want := []string{"poetry install", "pipenv install --deploy"}; !reflect.DeepEqual(*ran, want) {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
}

func TestCIDeletesNothingWithoutConfirmation(t *testing.T) {
	withConfig(t, config.Config{Interactive: false})
	dir := inProject(t, "composer.json", "composer.lock", "vendor/", "package.json", "yarn.lock", ".yarnrc.yml", "node_modules/")
	ran := recordTools(t)
	var code int
	captureStdout(t, func() { code = cmdCleanInstall(nil) })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, f := range []string{"composer.lock", "yarn.lock", "vendor", "node_modules"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s was deleted without confirmation", f)
		}
	}
	if want := []string{"yarn install --immutable", "composer install"}; !reflect.DeepEqual(*ran, want) {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
}

func TestGlobalWithoutPackagesIsAnError(t *testing.T) {
	withConfig(t, config.Config{})
	inProject(t, "package.json")
	ran := recordTools(t)
	if code := cmdInstall([]string{"-g"}); code != 1 || len(*ran) != 0 {
		t.Fatalf("code=%d ran=%v", code, *ran)
	}
}
