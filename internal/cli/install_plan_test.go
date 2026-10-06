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
		"Will install com.google.guava:guava via maven (Java).",
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
