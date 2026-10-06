package workspace

import (
	"reflect"
	"testing"
)

func TestMatchPath(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		{"packages/*", "packages/ui", true},
		{"packages/*", "packages/ui/sub", false},
		{"packages/**", "packages/ui/sub", true},
		{"packages/**", "packages", true},
		{"**/test/**", "packages/test", true},
		{"**/test/**", "packages/test/unit", true},
		{"**/test/**", "packages/testing", false},
		{"./apps/*/", "apps/web", true},
		{"apps/w?b", "apps/web", true},
		{"apps/[a-c]*", "apps/web", false},
		{".", ".", true},
		{"*", ".", false},
		{"**", ".", true},
	}
	for _, c := range cases {
		if got := matchPath(c.pattern, c.rel); got != c.want {
			t.Errorf("matchPath(%q, %q) = %v, want %v", c.pattern, c.rel, got, c.want)
		}
	}
}

func TestExpandGlobsLiteralSkippedDirIsReachable(t *testing.T) {
	root := writeTree(t, map[string]string{
		"vendor/acme/a/x":   "",
		"vendor/acme/b/x":   "",
		"pkgs/a/vendor/z/x": "",
		"pkgs/.hidden/y/x":  "",
	})
	got := []string{}
	for _, d := range expandGlobs(root, []string{"vendor/acme/*", "pkgs/**"}) {
		got = append(got, relSlash(root, d))
	}
	want := []string{"pkgs", "pkgs/a", "vendor/acme/a", "vendor/acme/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandGlobs = %v, want %v", got, want)
	}
}
