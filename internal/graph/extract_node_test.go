package graph

import (
	"os"
	"path/filepath"
	"testing"
)

// copyFixtureDir copies testdata/<rel>/* into a fresh temp dir and returns it.
func copyFixtureDir(t *testing.T, rel string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join("testdata", filepath.FromSlash(rel))
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestNodeExtractorReadsPackageLockAndManifest(t *testing.T) {
	g, err := (&NodeExtractor{}).Extract(copyFixtureDir(t, "npm/v1-nested"), ExtractOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantGraph(t, g, 10, 9, 1, []string{"node:legacy-app@1.0.0 -> node:send@0.18.0"}, []string{"node:legacy-app@1.0.0"})
}

func TestNodeExtractorMalformedPackageLockIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&NodeExtractor{}).Extract(dir, ExtractOptions{}); err == nil {
		t.Fatal("want error for a malformed package-lock.json")
	}
}
