package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

var (
	npmAxios = search.Result{Manager: pm.Npm, Name: "axios", Extra: map[string]string{"version": "1.7.9"}}
	pipAxios = search.Result{Manager: pm.Pip, Name: "axios", Extra: map[string]string{"version": "0.1"}}
	guava    = search.Result{Manager: pm.Maven, Name: "com.google.guava:guava", Info: "Maven artifact",
		Extra: map[string]string{"version": "33.3.1-jre", "group": "com.google.guava", "artifact": "guava"}}
)

func managers(cands []candidate) string {
	var parts []string
	for _, c := range cands {
		parts = append(parts, string(c.Result.Manager)+":"+c.Via)
	}
	return strings.Join(parts, ",")
}

func TestLockfileNarrowsOnlyItsOwnEcosystem(t *testing.T) {
	project := map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemNode: {{Name: "yarn.lock", Ecosystem: pm.EcosystemNode, Manager: pm.Yarn}}}
	got := buildCandidates([]search.Result{npmAxios, pipAxios}, project)
	if managers(got) != "yarn:yarn.lock,pip:" {
		t.Fatalf("candidates = %s, want yarn (from yarn.lock) and pip: a lock file must not hide another ecosystem", managers(got))
	}
}

func TestSingleEcosystemWithLockfileIsOneCandidate(t *testing.T) {
	project := map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemNode: {{Name: "pnpm-lock.yaml", Ecosystem: pm.EcosystemNode, Manager: pm.Pnpm}}}
	got := buildCandidates([]search.Result{npmAxios}, project)
	if managers(got) != "pnpm:pnpm-lock.yaml" || got[0].Result.Name != "axios" {
		t.Fatalf("candidates = %s", managers(got))
	}
}

func TestSeveralLockfilesInOneEcosystemAreAllOffered(t *testing.T) {
	project := map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemNode: {
		{Name: "package-lock.json", Ecosystem: pm.EcosystemNode, Manager: pm.Npm},
		{Name: "yarn.lock", Ecosystem: pm.EcosystemNode, Manager: pm.Yarn},
	}}
	if got := managers(buildCandidates([]search.Result{npmAxios}, project)); got != "npm:package-lock.json,yarn:yarn.lock" {
		t.Fatalf("candidates = %s", got)
	}
}

func TestGradleBuildFileTurnsMavenHitIntoGradle(t *testing.T) {
	project := map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemJava: {{Name: "build.gradle.kts", Ecosystem: pm.EcosystemJava, Manager: pm.Gradle}}}
	got := buildCandidates([]search.Result{guava}, project)
	if managers(got) != "gradle:build.gradle.kts" || got[0].Result.Extra["artifact"] != "guava" {
		t.Fatalf("candidates = %+v", got)
	}
}

func TestNoProjectFilesKeepsRegistryHits(t *testing.T) {
	if got := managers(buildCandidates([]search.Result{npmAxios, pipAxios, guava}, nil)); got != "npm:,pip:,maven:" {
		t.Fatalf("candidates = %s", got)
	}
}

func TestSortCandidatesHonoursPreferAndIsStable(t *testing.T) {
	cands := buildCandidates([]search.Result{npmAxios, pipAxios, guava}, nil)
	sortCandidates(cands, []string{"maven"})
	if managers(cands) != "maven:,npm:,pip:" {
		t.Fatalf("order = %s, want maven first then registry order", managers(cands))
	}
}

func TestInstallSpecUsesRegistryNameAndNoImplicitPin(t *testing.T) {
	composer := candidate{Result: search.Result{Manager: pm.Composer, Name: "monolog/monolog", Extra: map[string]string{"version": "3.8.1"}}}
	name, extra := installSpec(composer, "")
	if name != "monolog/monolog" {
		t.Errorf("name = %q, want the registry's full name", name)
	}
	if _, pinned := extra["version"]; pinned {
		t.Errorf("version %q passed although the user asked for none", extra["version"])
	}
	if composer.Result.Extra["version"] != "3.8.1" {
		t.Error("installSpec mutated the search result")
	}
	if _, extra = installSpec(candidate{Result: npmAxios}, "1.7.0"); extra["version"] != "1.7.0" {
		t.Errorf("requested version lost: %v", extra)
	}
	if _, extra = installSpec(candidate{Result: guava}, ""); extra["version"] != "33.3.1-jre" {
		t.Errorf("maven snippet needs the registry version, got %v", extra)
	}
}

func TestInstallCandidateExplainsRenamedMatch(t *testing.T) {
	withConfig(t, config.Config{})
	old := ensurePM
	ensurePM = func(pm.ID) error { return nil }
	t.Cleanup(func() { ensurePM = old })

	var code int
	out := captureStdout(t, func() { code = installCandidate(candidate{Result: guava}, "guava", "", false) })
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{
		`"guava" matched com.google.guava:guava.`,
		"Add com.google.guava:guava:33.3.1-jre to pom.xml:",
		"<artifactId>guava</artifactId>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestTUINameIsValidatedBeforeOfferingToInstallTheTool(t *testing.T) {
	withConfig(t, config.Config{AutoInstallPM: true, Interactive: true})
	called := false
	old := ensurePM
	ensurePM = func(pm.ID) error { called = true; return errors.New("would prompt") }
	t.Cleanup(func() { ensurePM = old })

	var code int
	captureStdout(t, func() {
		code = installFromSearchResult(search.Result{Manager: pm.Npm, Name: "--registry=http://evil"}, pm.Bun)
	})
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if called {
		t.Fatal("the user was offered to install bun for a name that is then refused")
	}
}

func TestIsGoModulePath(t *testing.T) {
	for s, want := range map[string]bool{
		"github.com/gin-gonic/gin": true,
		"golang.org/x/term":        true,
		"gopkg.in/yaml.v3":         true,
		"axios":                    false,
		"@types/node":              false,
		"monolog/monolog":          false,
		"com.google.guava:guava":   false,
		"example.com":              false,
		"lodash.merge":             false,
	} {
		t.Run(s, func(t *testing.T) {
			if got := isGoModulePath(s); got != want {
				t.Errorf("isGoModulePath(%q) = %v, want %v", s, got, want)
			}
		})
	}
}

func TestGoModulePathSkipsRegistries(t *testing.T) {
	withConfig(t, config.Config{})
	old := lookupReport
	lookupReport = func(string, search.Options) (search.Report, error) {
		t.Error("registries were queried for a Go module path")
		return search.Report{}, nil
	}
	t.Cleanup(func() { lookupReport = old })
	oldEnsure := ensurePM
	ensurePM = func(id pm.ID) error {
		if id != pm.GoMod {
			t.Errorf("tool = %s, want gomod", id)
		}
		return errors.New("stop before running go")
	}
	t.Cleanup(func() { ensurePM = oldEnsure })

	out := captureStdout(t, func() { installOne("github.com/gin-gonic/gin@v1.10.0", false) })
	if !strings.Contains(out, "Will install github.com/gin-gonic/gin@v1.10.0 via go modules (Go).") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestNeedsRenameConfirmation(t *testing.T) {
	cases := []struct {
		name  string
		query string
		c     candidate
		want  bool
	}{
		{"exact", "axios", candidate{Result: npmAxios}, false},
		{"case-insensitive", "Axios", candidate{Result: npmAxios}, false},
		{"fuzzy npm", "axioss", candidate{Result: npmAxios}, true},
		{"composer <q>/<q> is the package itself", "monolog", candidate{Result: search.Result{Manager: pm.Composer, Name: "monolog/monolog"}}, false},
		{"composer <q>/<q> other case", "PHPUnit", candidate{Result: search.Result{Manager: pm.Composer, Name: "phpunit/phpunit"}}, false},
		{"composer other vendor", "monolog", candidate{Result: search.Result{Manager: pm.Composer, Name: "acme/monolog"}}, true},
		{"composer full name", "monolog/monolog", candidate{Result: search.Result{Manager: pm.Composer, Name: "monolog/monolog"}}, false},
		{"pypi underscore vs dash", "flask_sqlalchemy", candidate{Result: search.Result{Manager: pm.Pip, Name: "Flask-SQLAlchemy"}}, false},
		{"pypi dot vs dash", "zope.interface", candidate{Result: search.Result{Manager: pm.Pip, Name: "zope-interface"}}, false},
		{"pypi typo", "reqests", candidate{Result: search.Result{Manager: pm.Pip, Name: "requests"}}, true},
		{"maven snippet only", "guava", candidate{Result: guava}, false},
	}
	for _, tc := range cases {
		if got := needsRenameConfirmation(tc.query, tc.c); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestInstallCandidateRefusesFuzzyMatchNonInteractively(t *testing.T) {
	withConfig(t, config.Config{})
	called := false
	old := ensurePM
	ensurePM = func(pm.ID) error { called = true; return nil }
	t.Cleanup(func() { ensurePM = old })
	c := candidate{Result: search.Result{Manager: pm.Composer, Name: "expressive/expressive"}}
	if code := installCandidate(c, "axioss", "", false); code != 1 || called {
		t.Fatalf("exit %d, ensurePM called=%v; want refusal before running anything", code, called)
	}
}

func TestEnsureManagerExplainsManualInstallAndPath(t *testing.T) {
	withConfig(t, config.Config{AutoInstallPM: true, Interactive: true})
	oldAsk := askYesNo
	askYesNo = func(string) (bool, error) { return true, nil }
	t.Cleanup(func() { askYesNo = oldAsk })
	ran := 0
	restoreRun := pm.SetCommandRunner(func(string, ...string) error { ran++; return nil })
	restoreLook := pm.SetLookPath(func(string) (string, error) { return "", errors.New("not found") })
	t.Cleanup(func() { restoreRun(); restoreLook() })

	err := ensureManager(pm.Bun)
	if err == nil {
		t.Fatal("want an error for bun")
	}
	for _, want := range []string{"does not run remote install scripts", "https://bun.sh/docs/installation", "~/.bun/bin", "PATH"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if ran != 0 {
		t.Fatalf("ran %d commands; none may run", ran)
	}
}

func TestEnsureManagerPrintsInstallHintWhenDeclinedOrNonInteractive(t *testing.T) {
	for _, interactive := range []bool{true, false} {
		t.Run(fmt.Sprintf("interactive=%v", interactive), func(t *testing.T) {
			withConfig(t, config.Config{AutoInstallPM: true, Interactive: interactive})
			oldAsk := askYesNo
			askYesNo = func(string) (bool, error) { return false, nil }
			t.Cleanup(func() { askYesNo = oldAsk })
			ran := 0
			t.Cleanup(pm.SetCommandRunner(func(string, ...string) error { ran++; return nil }))
			t.Cleanup(pm.SetLookPath(func(string) (string, error) { return "", errors.New("not found") }))

			var err error
			out := captureStdout(t, func() { err = ensureManager(pm.Pnpm) })

			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(out, "To install it: npm install -g pnpm") {
				t.Errorf("output lacks the install hint:\n%s", out)
			}
			if ran != 0 {
				t.Errorf("ran %d commands; none may run", ran)
			}
		})
	}
}

var (
	saber      = search.Result{Manager: pm.Composer, Name: "swlib/saber", Info: "Coroutine HTTP client"}
	nestAxios  = search.Result{Manager: pm.Maven, Name: "org.mvnpm.at.nestjs:axios", Extra: map[string]string{"version": "3.0.0", "group": "org.mvnpm.at.nestjs", "artifact": "axios"}}
	axiosRetry = search.Result{Manager: pm.Maven, Name: "org.webjars.npm:axios-retry", Extra: map[string]string{"version": "4.0.0", "group": "org.webjars.npm", "artifact": "axios-retry"}}
	nodeLock   = map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemNode: {{Name: "package-lock.json", Ecosystem: pm.EcosystemNode, Manager: pm.Npm}}}
	pyReqs     = map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemPython: {{Name: "requirements.txt", Ecosystem: pm.EcosystemPython, Manager: pm.Pip}}}
)

var (
	npmKeyring      = search.Result{Manager: pm.Npm, Name: "keyring"}
	pipKeyring      = search.Result{Manager: pm.Pip, Name: "keyring"}
	npmPhpunit      = search.Result{Manager: pm.Npm, Name: "phpunit", Extra: map[string]string{"version": "0.0.1-security"}}
	composerPhpunit = search.Result{Manager: pm.Composer, Name: "phpunit/phpunit"}
	npmExpress      = search.Result{Manager: pm.Npm, Name: "express"}
	cargoExpress    = search.Result{Manager: pm.Cargo, Name: "express"}
	royaleExpress   = search.Result{Manager: pm.Maven, Name: "org.apache.royale.framework:Express",
		Extra: map[string]string{"group": "org.apache.royale.framework", "artifact": "Express"}}
	gradleBuild = map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemJava: {{Name: "build.gradle", Ecosystem: pm.EcosystemJava, Manager: pm.Gradle}}}
	nodeTwoLock = map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemNode: {
		{Name: "package-lock.json", Ecosystem: pm.EcosystemNode, Manager: pm.Npm},
		{Name: "yarn.lock", Ecosystem: pm.EcosystemNode, Manager: pm.Yarn},
	}}
	nodeAndPython = map[pm.Ecosystem][]pm.ProjectFile{
		pm.EcosystemNode:   nodeLock[pm.EcosystemNode],
		pm.EcosystemPython: pyReqs[pm.EcosystemPython],
	}
)

func TestIsExact(t *testing.T) {
	cases := []struct {
		name  string
		query string
		c     candidate
		want  bool
	}{
		{"npm same name", "axios", candidate{Result: npmAxios}, true},
		{"npm other case", "Axios", candidate{Result: npmAxios}, true},
		{"npm typo", "axioss", candidate{Result: npmAxios}, false},
		{"pypi normalised", "flask_sqlalchemy", candidate{Result: search.Result{Manager: pm.Pip, Name: "Flask-SQLAlchemy"}}, true},
		{"composer unrelated", "axios", candidate{Result: saber}, false},
		{"composer full name", "swlib/saber", candidate{Result: saber}, true},
		{"maven artifactId equals query", "guava", candidate{Result: guava}, true},
		{"maven artifactId other case", "Guava", candidate{Result: guava}, false},
		{"maven artifactId case differs the other way", "express", candidate{Result: royaleExpress}, false},
		{"maven artifactId same case", "Express", candidate{Result: royaleExpress}, true},
		{"composer <q>/<q>", "phpunit", candidate{Result: composerPhpunit}, true},
		{"composer <q>/<q> other case", "PHPUnit", candidate{Result: composerPhpunit}, true},
		{"composer other vendor", "phpunit", candidate{Result: search.Result{Manager: pm.Composer, Name: "acme/phpunit"}}, false},
		{"maven artifactId differs", "axios", candidate{Result: axiosRetry}, false},
		{"maven full coordinate", "com.google.guava:guava", candidate{Result: guava}, true},
		{"gradle uses the artifactId too", "guava", candidate{Result: search.Result{Manager: pm.Gradle, Name: "com.google.guava:guava"}}, true},
		{"mvnpm repackage of an npm name", "axios", candidate{Result: nestAxios}, false},
		{"webjars repackage of an npm name", "lodash", candidate{Result: search.Result{Manager: pm.Maven, Name: "org.webjars.npm:lodash"}}, false},
		{"webjars by full coordinate", "org.webjars:jquery", candidate{Result: search.Result{Manager: pm.Maven, Name: "org.webjars:jquery"}}, true},
		{"gradle mvnpm repackage", "axios", candidate{Result: search.Result{Manager: pm.Gradle, Name: "org.mvnpm.at.nestjs:axios"}}, false},
		{"gradle unrelated", "requests", candidate{Result: search.Result{Manager: pm.Gradle, Name: "org.webjars.npm:axios-retry"}}, false},
	}
	for _, tc := range cases {
		if got := isExact(tc.query, tc.c); got != tc.want {
			t.Errorf("%s: isExact(%q, %s) = %v, want %v", tc.name, tc.query, tc.c.Result.Name, got, tc.want)
		}
	}
}

func TestCandidateChoiceNamesTheEcosystem(t *testing.T) {
	for _, tc := range []struct {
		c    candidate
		want string
	}{
		{candidate{Result: search.Result{Manager: pm.Composer, Name: "a/b"}}, "composer (PHP)"},
		{candidate{Result: search.Result{Manager: pm.Cargo, Name: "b"}}, "cargo (Rust)"},
		{candidate{Result: search.Result{Manager: pm.Yarn, Name: "b"}, Via: "yarn.lock"}, "yarn (Node, via yarn.lock)"},
	} {
		if got := candidateChoice(tc.c); got != tc.want {
			t.Errorf("candidateChoice = %q, want %q", got, tc.want)
		}
	}
}

func TestMenuChoiceOfAClosestMatchIsNotConfirmedTwice(t *testing.T) {
	withConfig(t, config.Config{Interactive: true})
	asked := 0
	oldAsk := askYesNo
	askYesNo = func(string) (bool, error) { asked++; return false, nil }
	t.Cleanup(func() { askYesNo = oldAsk })
	ran := recordAllCommands(t)
	c := candidate{Result: saber, Picked: true}
	var code int
	captureStdout(t, func() { code = installCandidate(c, "axios", "", false) })
	if code != 0 || asked != 0 || len(*ran) != 1 || (*ran)[0] != "composer require swlib/saber" {
		t.Fatalf("code=%d asked=%d ran=%q; a menu pick that showed the full name must not be confirmed again", code, asked, *ran)
	}
}

func TestMavenAndGradleSnippetsNeedNoTool(t *testing.T) {
	for _, id := range []pm.ID{pm.Maven, pm.Gradle} {
		withConfig(t, config.Config{AutoInstallPM: true})
		ran := 0
		restoreRun := pm.SetCommandRunner(func(string, ...string) error { ran++; return nil })
		restoreLook := pm.SetLookPath(func(string) (string, error) { return "", errors.New("not found") })
		oldEnsure := ensurePM
		ensurePM = ensureManager

		c := candidate{Result: guava}
		c.Result.Manager = id
		var code int
		out := captureStdout(t, func() { code = installCandidate(c, "guava", "", false) })
		ensurePM = oldEnsure
		restoreRun()
		restoreLook()

		want := "Add com.google.guava:guava:33.3.1-jre to pom.xml:"
		if id == pm.Gradle {
			want = "Add com.google.guava:guava:33.3.1-jre to build.gradle(.kts):"
		}
		if code != 0 || ran != 0 || !strings.Contains(out, want) || strings.Contains(out, "not installed") || strings.Contains(out, "Will install") {
			t.Errorf("%s: code=%d ran=%d, want %q and no tool check:\n%s", id, code, ran, want, out)
		}
	}
}

func TestEnsureManagerDoesNotOfferToInstallManualTools(t *testing.T) {
	for _, id := range []pm.ID{pm.Bun, pm.Cargo, pm.Composer, pm.GoMod, pm.Maven, pm.Gradle, pm.Npm} {
		withConfig(t, config.Config{AutoInstallPM: true, Interactive: true})
		asked := false
		oldAsk := askYesNo
		askYesNo = func(string) (bool, error) { asked = true; return true, nil }
		restoreLook := pm.SetLookPath(func(string) (string, error) { return "", errors.New("not found") })
		steps, _ := pm.ManualInstallSteps(id)

		var err error
		captureStdout(t, func() { err = ensureManager(id) })
		askYesNo = oldAsk
		restoreLook()

		if asked || err == nil || steps == "" || !strings.Contains(err.Error(), steps) {
			t.Errorf("%s: asked=%v err=%v; want the official steps (%q) without a prompt", id, asked, err, steps)
		}
	}
}

func TestEnsureManagerPrintsHintWhenThePromptFails(t *testing.T) {
	withConfig(t, config.Config{AutoInstallPM: true, Interactive: true})
	oldAsk := askYesNo
	askYesNo = func(string) (bool, error) { return false, errors.New("^C") }
	t.Cleanup(func() { askYesNo = oldAsk })
	t.Cleanup(pm.SetLookPath(func(string) (string, error) { return "", errors.New("not found") }))

	var err error
	out := captureStdout(t, func() { err = ensureManager(pm.Pnpm) })
	if err == nil || !strings.Contains(out, "npm install -g pnpm") {
		t.Fatalf("err=%v, output lacks the hint:\n%s", err, out)
	}
}
