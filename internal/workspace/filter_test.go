package workspace

import (
	"path/filepath"
	"reflect"
	"testing"
)

func filterFixture() []Workspace {
	root := filepath.FromSlash("/repo")
	proj := func(rel string) Project {
		return Project{Name: rel, Path: filepath.Join(root, filepath.FromSlash(rel))}
	}
	return []Workspace{
		{Root: root, Ecosystem: "node", Projects: []Project{
			proj("apps/web"), proj("packages/legacy-ui"), proj("packages/tools/cli"), proj("packages/ui"),
		}},
		{Root: root, Ecosystem: "go", Projects: []Project{proj("services/api")}},
		{Root: root, Ecosystem: "python", Projects: []Project{proj(".")}},
	}
}

func filtered(ws []Workspace) map[string][]string {
	out := map[string][]string{}
	for _, w := range ws {
		for _, p := range w.Projects {
			out[w.Ecosystem] = append(out[w.Ecosystem], p.Name)
		}
	}
	return out
}

func TestFilter(t *testing.T) {
	cases := []struct {
		name             string
		include, exclude []string
		want             map[string][]string
	}{
		{"no patterns keeps all", nil, nil, map[string][]string{
			"node":   {"apps/web", "packages/legacy-ui", "packages/tools/cli", "packages/ui"},
			"go":     {"services/api"},
			"python": {"."},
		}},
		{"include double star, exclude wins, empty workspaces dropped",
			[]string{"packages/**"}, []string{"packages/legacy-*"},
			map[string][]string{"node": {"packages/tools/cli", "packages/ui"}}},
		{"single star stays in one segment", []string{"packages/*"}, nil,
			map[string][]string{"node": {"packages/legacy-ui", "packages/ui"}}},
		{"exclude only", nil, []string{"apps/*", "services/**", "."},
			map[string][]string{"node": {"packages/legacy-ui", "packages/tools/cli", "packages/ui"}}},
		{"root project matched by dot", []string{"."}, nil,
			map[string][]string{"python": {"."}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := filterFixture()
			got := filtered(Filter(in, c.include, c.exclude))
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Filter = %v, want %v", got, c.want)
			}
			if len(in[0].Projects) != 4 {
				t.Fatalf("Filter modified its input")
			}
		})
	}
}
