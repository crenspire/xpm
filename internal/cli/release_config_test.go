package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestGoreleaserCaskListsEveryManPage keeps the Homebrew cask's manpages list
// in .goreleaser.yaml in sync with commandTable: casks take explicit paths,
// not globs, so a new command must be added there too.
func TestGoreleaserCaskListsEveryManPage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	var cfg struct {
		Casks []struct {
			Manpages []string `yaml:"manpages"`
		} `yaml:"homebrew_casks"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}
	if len(cfg.Casks) != 1 {
		t.Fatalf("want 1 homebrew_casks entry, got %d", len(cfg.Casks))
	}
	var want []string
	for _, c := range commandTable {
		want = append(want, "manpages/xpm-"+c.name+".1")
	}
	got := append([]string(nil), cfg.Casks[0].Manpages...)
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("homebrew_casks[0].manpages = %v\nwant (one per commandTable entry) %v", got, want)
	}
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// TestMakefileStampsVersionWithoutV keeps `git describe` tags (v0.1.0) from
// reaching the binary as "v0.1.0" (the banner adds its own "v").
func TestMakefileStampsVersionWithoutV(t *testing.T) {
	mk := readRepoFile(t, "Makefile")
	if !strings.Contains(mk, "VERSION_NOV := $(patsubst v%,%,$(VERSION))") {
		t.Error("Makefile does not define VERSION_NOV by stripping a leading v from VERSION")
	}
	if !strings.Contains(mk, "internal/cli.Version=$(VERSION_NOV)") {
		t.Error("Makefile LDFLAGS must stamp $(VERSION_NOV), not $(VERSION)")
	}
}

// TestReleaseWorkflowPinsGoreleaserLikeMakefile keeps CI and local release
// checks on the same GoReleaser version.
func TestReleaseWorkflowPinsGoreleaserLikeMakefile(t *testing.T) {
	m := regexp.MustCompile(`goreleaser/v2@(v[0-9.]+)`).FindStringSubmatch(readRepoFile(t, "Makefile"))
	if m == nil {
		t.Fatal("Makefile has no pinned goreleaser/v2@vX.Y.Z")
	}
	wf := readRepoFile(t, filepath.Join(".github", "workflows", "release.yml"))
	if !strings.Contains(wf, "version: '"+m[1]+"'") {
		t.Errorf("release.yml goreleaser-action version is not pinned to %s (Makefile)", m[1])
	}
}
