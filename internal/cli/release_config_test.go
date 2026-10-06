package cli

import (
	"os"
	"path/filepath"
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
