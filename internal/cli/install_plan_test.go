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
