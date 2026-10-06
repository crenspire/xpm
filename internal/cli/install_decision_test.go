package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// TestDecideInstall covers every branch of the project-first rule. want is
// "pick manager:via", "menu manager,manager,..." or "refuse".
func TestDecideInstall(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		results     []search.Result
		files       map[pm.Ecosystem][]pm.ProjectFile
		ecos        []string
		prefer      []string
		unavailable []pm.ID
		interactive bool
		want        string
		has         string // in the note (pick) or error (refuse)
	}{
		// a. Inside a project: an exact hit in the project's ecosystem wins.
		{"php project: composer beats the npm squatter", "phpunit", []search.Result{npmPhpunit, composerPhpunit}, nil, []string{"php"}, nil, nil, false,
			"pick composer:", "Using composer for this PHP project (also found: Node)."},
		{"php project, terminal: no menu", "phpunit", []search.Result{npmPhpunit, composerPhpunit}, nil, []string{"php"}, nil, nil, true,
			"pick composer:", "Using composer for this PHP project (also found: Node)."},
		{"node project: npm beats the pip namesake", "axios", []search.Result{npmAxios, pipAxios, saber, axiosRetry}, nodeLock, []string{"node"}, nil, nil, false,
			"pick npm:package-lock.json", "Using npm for this Node project (also found: Python)."},
		{"node project, terminal: no menu", "axios", []search.Result{npmAxios, pipAxios, saber}, nodeLock, []string{"node"}, nil, nil, true,
			"pick npm:package-lock.json", "Using npm for this Node project (also found: Python)."},
		{"no other exact hit: no also-found", "axios", []search.Result{npmAxios, saber, axiosRetry}, nodeLock, []string{"node"}, nil, nil, false,
			"pick npm:package-lock.json", "Using npm for this Node project."},
		{"project first beats prefer", "axios", []search.Result{npmAxios, pipAxios}, nodeLock, []string{"node"}, []string{"pip"}, nil, false,
			"pick npm:package-lock.json", ""},
		{"a registry outside the project is down", "axios", []search.Result{npmAxios, saber}, nodeLock, []string{"node"}, nil, []pm.ID{pm.Maven}, false,
			"pick npm:package-lock.json", ""},
		{"the project's own registry is down", "axios", []search.Result{pipAxios}, nodeLock, []string{"node"}, nil, []pm.ID{pm.Npm}, false,
			"refuse", "npm (Node.js) did not answer"},
		{"the project's own registry is down, terminal: menu", "axios", []search.Result{pipAxios, saber}, nodeLock, []string{"node"}, nil, []pm.ID{pm.Npm}, true,
			"menu pip,composer", ""},
		{"python project: pip beats npm", "keyring", []search.Result{npmKeyring, pipKeyring, saber}, pyReqs, []string{"python"}, nil, nil, false,
			"pick pip:requirements.txt", "Using pip for this Python project (also found: Node)."},
		{"gradle project: guava as a gradle snippet", "guava", []search.Result{guava, npmAxios}, gradleBuild, []string{"java"}, nil, nil, false,
			"pick gradle:build.gradle", "Using gradle for this Java project."},
		{"mixed project, exact in both: refuse", "keyring", []search.Result{npmKeyring, pipKeyring}, nodeAndPython, []string{"node", "python"}, nil, nil, false,
			"refuse", "keyring exists in several of this project's ecosystems: npm (Node, via package-lock.json), pip (Python, via requirements.txt)"},
		{"mixed project, exact in both, terminal: menu", "keyring", []search.Result{npmKeyring, pipKeyring}, nodeAndPython, []string{"node", "python"}, nil, nil, true,
			"menu npm,pip", ""},
		{"mixed project, exact in one", "axios", []search.Result{npmAxios, saber}, nodeAndPython, []string{"node", "python"}, nil, nil, false,
			"pick npm:package-lock.json", "Using npm for this Node project."},
		{"no exact hit in the project's ecosystem: refuse", "axios", []search.Result{npmAxios, saber, axiosRetry}, nil, []string{"php"}, []string{"npm"}, nil, false,
			"refuse", `no exact match for "axios" in this PHP project's registry; found: composer (PHP): swlib/saber (closest match), npm (Node), maven (Java): org.webjars.npm:axios-retry (closest match)`},
		{"no exact hit in the project's ecosystem, terminal: project first, then exact, then fuzzy", "axios", []search.Result{npmAxios, saber, axiosRetry}, nil, []string{"php"}, nil, nil, true,
			"menu composer,npm,maven", ""},
		{"go project never has a registry hit", "axios", []search.Result{npmAxios}, nil, []string{"go"}, nil, nil, false,
			"refuse", "this Go project's registry"},
		{"two lockfiles: the first (by prefer) without a terminal", "axios", []search.Result{npmAxios}, nodeTwoLock, []string{"node"}, nil, nil, false,
			"pick npm:package-lock.json", ""},
		{"two lockfiles, terminal: menu", "axios", []search.Result{npmAxios}, nodeTwoLock, []string{"node"}, nil, nil, true,
			"menu npm,yarn", ""},
		{"two lockfiles, terminal, prefer yarn", "axios", []search.Result{npmAxios}, nodeTwoLock, []string{"node"}, []string{"yarn"}, nil, true,
			"pick yarn:yarn.lock", "Using yarn for this Node project."},

		// b. No project files.
		{"one exact hit among closest matches", "axios", []search.Result{npmAxios, saber, axiosRetry}, nil, nil, nil, nil, false,
			"pick npm:", ""},
		{"one exact hit, terminal", "axios", []search.Result{npmAxios, saber, axiosRetry}, nil, nil, nil, nil, true,
			"pick npm:", ""},
		{"an mvnpm repackage is not exact", "axios", []search.Result{npmAxios, saber, nestAxios}, nil, nil, nil, nil, false,
			"pick npm:", ""},
		{"exact in two ecosystems: refuse with the prefer hint", "axios", []search.Result{npmAxios, pipAxios, saber}, nil, nil, nil, nil, false,
			"refuse", `axios exists in several ecosystems: npm (Node), pip (Python). Set "prefer" in config (e.g. xpm config set prefer npm) or run inside a project`},
		{"exact in two ecosystems, terminal: exact first, closest last", "axios", []search.Result{saber, npmAxios, pipAxios, axiosRetry}, nil, nil, nil, nil, true,
			"menu npm,pip,composer,maven", ""},
		{"prefer breaks the tie", "axios", []search.Result{npmAxios, pipAxios}, nil, nil, []string{"pip"}, nil, false,
			"pick pip:", `Using pip ("prefer" in config; also found: Node).`},
		{"prefer breaks the tie, terminal", "axios", []search.Result{npmAxios, pipAxios}, nil, nil, []string{"pip"}, nil, true,
			"pick pip:", ""},
		{"prefer names neither", "axios", []search.Result{npmAxios, pipAxios}, nil, nil, []string{"cargo"}, nil, false,
			"refuse", "several ecosystems"},
		{"express: maven Express is not exact, npm and cargo tie", "express", []search.Result{npmExpress, cargoExpress, royaleExpress}, nil, nil, nil, nil, false,
			"refuse", "npm (Node), cargo (Rust)"},
		{"express with prefer npm", "express", []search.Result{npmExpress, cargoExpress, royaleExpress}, nil, nil, []string{"npm"}, nil, false,
			"pick npm:", ""},
		{"a registry is down: no guess without a terminal", "axios", []search.Result{npmAxios, saber}, nil, nil, nil, []pm.ID{pm.Maven}, false,
			"refuse", "did not answer"},
		{"a registry is down, prefer cannot override it", "axios", []search.Result{npmAxios, pipAxios}, nil, nil, []string{"npm"}, []pm.ID{pm.Maven}, false,
			"refuse", "did not answer"},
		{"a registry is down, terminal: menu", "axios", []search.Result{npmAxios, saber}, nil, nil, nil, []pm.ID{pm.Maven}, true,
			"menu npm,composer", ""},
		{"only a closest match from one registry: picked, confirmed later", "axioss", []search.Result{saber}, nil, nil, nil, nil, false,
			"pick composer:", ""},
		{"closest matches from several registries: refuse", "axioss", []search.Result{saber, axiosRetry}, nil, nil, nil, nil, false,
			"refuse", `no package is named exactly "axioss"; closest matches: composer (PHP): swlib/saber (closest match), maven (Java): org.webjars.npm:axios-retry (closest match)`},
		{"closest matches from several registries, terminal: menu", "axioss", []search.Result{saber, axiosRetry}, nil, nil, nil, nil, true,
			"menu composer,maven", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cands := buildCandidates(tc.results, tc.files)
			sortCandidates(cands, tc.prefer)
			d := decideInstall(installInputs{
				Query: tc.query, Cands: cands, ProjectEcos: tc.ecos, Prefer: tc.prefer,
				Unavailable: tc.unavailable, Interactive: tc.interactive,
			})
			var got, text string
			switch d.Action {
			case actionPick:
				got, text = "pick "+string(d.Pick.Result.Manager)+":"+d.Pick.Via, d.Note
			case actionMenu:
				var ids []string
				for _, c := range d.Menu {
					ids = append(ids, string(c.Result.Manager))
				}
				got = "menu " + strings.Join(ids, ",")
				if len(d.Labels) != len(d.Menu) {
					t.Errorf("%d labels for %d items", len(d.Labels), len(d.Menu))
				}
			case actionRefuse:
				got = "refuse"
				if d.Err != nil {
					text = d.Err.Error()
				}
			}
			if got != tc.want {
				t.Errorf("decision = %q, want %q (note/err %q)", got, tc.want, text)
			}
			if !strings.Contains(text, tc.has) {
				t.Errorf("note/err %q lacks %q", text, tc.has)
			}
			if tc.interactive && d.Action == actionRefuse {
				t.Error("a terminal run never refuses: it asks")
			}
			if !tc.interactive && d.Action == actionMenu {
				t.Error("a run without a terminal never shows a menu")
			}
		})
	}
}

func TestMenuLabelsMarkClosestMatches(t *testing.T) {
	d := decideInstall(installInputs{Query: "axios", Cands: buildCandidates([]search.Result{saber, npmAxios, pipAxios}, nil), Interactive: true})
	if d.Action != actionMenu || len(d.Labels) != 3 {
		t.Fatalf("decision = %+v", d)
	}
	if strings.Contains(d.Labels[0], "closest match") || strings.Contains(d.Labels[1], "closest match") || !strings.HasSuffix(d.Labels[2], "(closest match)") {
		t.Fatalf("labels = %q", d.Labels)
	}
}

func TestProjectEcosystems(t *testing.T) {
	for _, tc := range []struct {
		files []string
		want  []string
	}{
		{nil, nil},
		{[]string{"package.json"}, []string{"node"}},
		{[]string{"yarn.lock"}, []string{"node"}},
		{[]string{"requirements.txt"}, []string{"python"}},
		{[]string{"pyproject.toml"}, []string{"python"}},
		{[]string{"Pipfile"}, []string{"python"}},
		{[]string{"poetry.lock"}, []string{"python"}},
		{[]string{"composer.json"}, []string{"php"}},
		{[]string{"Cargo.toml"}, []string{"rust"}},
		{[]string{"go.mod"}, []string{"go"}},
		{[]string{"pom.xml"}, []string{"java"}},
		{[]string{"build.gradle"}, []string{"java"}},
		{[]string{"build.gradle.kts"}, []string{"java"}},
		{[]string{"package.json", "package-lock.json", "go.mod", "pom.xml", "build.gradle"}, []string{"node", "go", "java"}},
	} {
		dir := t.TempDir()
		for _, f := range tc.files {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if got := projectEcosystems(dir); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: projectEcosystems = %v, want %v", tc.files, got, tc.want)
		}
	}
}
