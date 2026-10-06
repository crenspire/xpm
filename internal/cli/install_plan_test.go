package cli

import (
	"errors"
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
		if got := isGoModulePath(s); got != want {
			t.Errorf("isGoModulePath(%q) = %v, want %v", s, got, want)
		}
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
		{"composer short name", "monolog", candidate{Result: search.Result{Manager: pm.Composer, Name: "monolog/monolog"}}, true},
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

func TestNonInteractivePick(t *testing.T) {
	yarnProject := map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemNode: {{Name: "yarn.lock", Ecosystem: pm.EcosystemNode, Manager: pm.Yarn}}}
	composer := search.Result{Manager: pm.Composer, Name: "requests/requests"}
	cases := []struct {
		name   string
		cands  []candidate
		prefer []string
		want   string
	}{
		{"several ecosystems, no prefer", buildCandidates([]search.Result{npmAxios, pipAxios, composer}, yarnProject), nil, ""},
		{"one ecosystem", buildCandidates([]search.Result{npmAxios}, map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemNode: {
			{Name: "package-lock.json", Ecosystem: pm.EcosystemNode, Manager: pm.Npm},
			{Name: "yarn.lock", Ecosystem: pm.EcosystemNode, Manager: pm.Yarn}}}), nil, "npm"},
		{"prefer pip", buildCandidates([]search.Result{npmAxios, pipAxios}, nil), []string{"pip"}, "pip"},
		{"prefer names neither", buildCandidates([]search.Result{npmAxios, pipAxios}, nil), []string{"cargo"}, ""},
	}
	for _, tc := range cases {
		sortCandidates(tc.cands, tc.prefer)
		got, ok := nonInteractivePick(tc.cands, tc.prefer)
		if tc.want == "" {
			if ok {
				t.Errorf("%s: picked %s, want no pick", tc.name, got.Result.Manager)
			}
			continue
		}
		if !ok || string(got.Result.Manager) != tc.want {
			t.Errorf("%s: got %v %v, want %s", tc.name, got.Result.Manager, ok, tc.want)
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
		withConfig(t, config.Config{AutoInstallPM: true, Interactive: interactive})
		oldAsk := askYesNo
		askYesNo = func(string) (bool, error) { return false, nil }
		ran := 0
		restoreRun := pm.SetCommandRunner(func(string, ...string) error { ran++; return nil })
		restoreLook := pm.SetLookPath(func(string) (string, error) { return "", errors.New("not found") })

		var err error
		out := captureStdout(t, func() { err = ensureManager(pm.Pnpm) })
		askYesNo = oldAsk
		restoreRun()
		restoreLook()

		if err == nil {
			t.Fatalf("interactive=%v: want an error", interactive)
		}
		if !strings.Contains(out, "npm install -g pnpm") {
			t.Errorf("interactive=%v: output lacks the install hint:\n%s", interactive, out)
		}
		if ran != 0 {
			t.Errorf("interactive=%v: ran %d commands; none may run", interactive, ran)
		}
	}
}

var (
	saber      = search.Result{Manager: pm.Composer, Name: "swlib/saber", Info: "Coroutine HTTP client"}
	nestAxios  = search.Result{Manager: pm.Maven, Name: "org.mvnpm.at.nestjs:axios", Extra: map[string]string{"version": "3.0.0", "group": "org.mvnpm.at.nestjs", "artifact": "axios"}}
	axiosRetry = search.Result{Manager: pm.Maven, Name: "org.webjars.npm:axios-retry", Extra: map[string]string{"version": "4.0.0", "group": "org.webjars.npm", "artifact": "axios-retry"}}
	nodeLock   = map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemNode: {{Name: "package-lock.json", Ecosystem: pm.EcosystemNode, Manager: pm.Npm}}}
	pyReqs     = map[pm.Ecosystem][]pm.ProjectFile{pm.EcosystemPython: {{Name: "requirements.txt", Ecosystem: pm.EcosystemPython, Manager: pm.Pip}}}
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
		{"maven artifactId other case", "Guava", candidate{Result: guava}, true},
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

func TestDecideNonInteractive(t *testing.T) {
	pipKeyring := search.Result{Manager: pm.Pip, Name: "keyring"}
	npmKeyring := search.Result{Manager: pm.Npm, Name: "keyring"}
	cases := []struct {
		name        string
		query       string
		results     []search.Result
		project     map[pm.Ecosystem][]pm.ProjectFile
		prefer      []string
		unavailable []pm.ID
		want        string // manager:via, or "" for a refusal
		errHas      string
	}{
		{"node project, fuzzy composer and maven dropped", "axios", []search.Result{npmAxios, saber, axiosRetry}, nodeLock, nil, nil, "npm:package-lock.json", ""},
		{"node project beats an artifactId-equal maven hit", "axios", []search.Result{npmAxios, saber, nestAxios}, nodeLock, nil, nil, "npm:package-lock.json", ""},
		{"empty dir, mvnpm repackage is not exact", "axios", []search.Result{npmAxios, saber, nestAxios}, nil, nil, nil, "npm:", ""},
		{"node project, maven down outside the ecosystem", "axios", []search.Result{npmAxios, saber}, nodeLock, nil, []pm.ID{pm.Maven}, "npm:package-lock.json", ""},
		{"empty dir, one exact among fuzzy", "axios", []search.Result{npmAxios, saber, axiosRetry}, nil, nil, nil, "npm:", ""},
		{"empty dir, one exact but a registry was down", "axios", []search.Result{npmAxios, saber}, nil, nil, []pm.ID{pm.Maven}, "", "did not answer"},
		{"python project, exact on pip and npm", "keyring", []search.Result{npmKeyring, pipKeyring, saber}, pyReqs, nil, nil, "pip:requirements.txt", ""},
		{"empty dir, exact on npm and pip", "keyring", []search.Result{npmKeyring, pipKeyring, saber}, nil, nil, nil, "", "npm (Node), pip (Python)"},
		{"empty dir, exact on npm and pip, prefer pip", "keyring", []search.Result{npmKeyring, pipKeyring, saber}, nil, []string{"pip"}, nil, "pip:", ""},
		{"refusal lists only exact candidates", "keyring", []search.Result{npmKeyring, pipKeyring, saber}, nil, nil, nil, "", "keyring exists in several ecosystems: npm (Node), pip (Python)."},
		{"only fuzzy hits: unchanged, several ecosystems refuse", "axioss", []search.Result{saber, axiosRetry}, nil, nil, nil, "", "composer, maven (Java)"},
		{"only fuzzy hits: a registry down refuses", "axioss", []search.Result{saber}, nil, nil, []pm.ID{pm.Pip}, "", "did not answer"},
	}
	for _, tc := range cases {
		cands := buildCandidates(tc.results, tc.project)
		sortCandidates(cands, tc.prefer)
		got, err := decideNonInteractive(tc.query, cands, tc.prefer, tc.unavailable)
		if tc.want == "" {
			if err == nil {
				t.Errorf("%s: picked %s, want a refusal", tc.name, got.Result.Manager)
			} else if !strings.Contains(err.Error(), tc.errHas) {
				t.Errorf("%s: error %q lacks %q", tc.name, err, tc.errHas)
			}
			continue
		}
		if err != nil || string(got.Result.Manager)+":"+got.Via != tc.want {
			t.Errorf("%s: got %s:%s, %v; want %s", tc.name, got.Result.Manager, got.Via, err, tc.want)
		}
	}
}

func TestNonInteractivePickKeepsSingleToolEcosystemsApart(t *testing.T) {
	cands := []candidate{
		{Result: search.Result{Manager: pm.Composer, Name: "a/b"}},
		{Result: search.Result{Manager: pm.Cargo, Name: "b"}},
	}
	if c, ok := nonInteractivePick(cands, nil); ok {
		t.Fatalf("composer and cargo are different ecosystems; picked %s", c.Result.Manager)
	}
	for _, c := range cands {
		if got := candidateChoice(c); strings.Contains(got, "()") {
			t.Errorf("candidateChoice = %q, want no empty parentheses", got)
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

func TestMenuListsExactCandidatesFirst(t *testing.T) {
	cands := buildCandidates([]search.Result{saber, npmAxios, axiosRetry}, nil)
	items, labels := menuCandidates("axios", cands)
	if managers(items) != "npm:,composer:,maven:" {
		t.Fatalf("menu order = %s, want the exact npm hit first", managers(items))
	}
	if strings.Contains(labels[0], "closest match") || !strings.Contains(labels[1], "(closest match)") || !strings.Contains(labels[2], "(closest match)") {
		t.Fatalf("labels = %q", labels)
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
