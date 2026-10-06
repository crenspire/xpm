package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// fakeRegistries serves a tiny, fixed world: npm has axios, lodash and
// typescript; Packagist finds monolog/monolog for "monolog"; Maven Central
// has guava. Everything else is "not found".
func fakeRegistries(t *testing.T) {
	t.Helper()
	npm := map[string]string{
		"axios":      `{"version":"1.7.9","description":"Promise based HTTP client"}`,
		"lodash":     `{"version":"4.17.21","description":"Lodash modular utilities."}`,
		"typescript": `{"version":"5.6.3","description":"TypeScript is a language for application scale JavaScript"}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, q := r.URL.Path, r.URL.Query().Get("q")
		switch {
		case strings.HasPrefix(p, "/npm/") && strings.HasSuffix(p, "/latest"):
			if body, ok := npm[strings.TrimSuffix(strings.TrimPrefix(p, "/npm/"), "/latest")]; ok {
				fmt.Fprint(w, body)
				return
			}
		case p == "/pypi/requests/json":
			fmt.Fprint(w, `{"info":{"name":"requests","summary":"Python HTTP for Humans.","version":"2.32.3"}}`)
			return
		case p == "/packagist/search.json":
			if q == "monolog" {
				fmt.Fprint(w, `{"results":[{"name":"monolog/monolog","description":"Sends your logs to files, sockets, inboxes, databases and various web services"}]}`)
				return
			}
			fmt.Fprint(w, `{"results":[]}`)
			return
		case p == "/maven/select":
			if q == "guava" {
				fmt.Fprint(w, `{"response":{"docs":[{"id":"com.google.guava:guava","g":"com.google.guava","a":"guava","latestVersion":"33.3.1-jre"}]}}`)
				return
			}
			fmt.Fprint(w, `{"response":{"docs":[]}}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(search.SetEndpoints(search.Endpoints{
		Npm:         srv.URL + "/npm",
		PyPI:        srv.URL + "/pypi",
		Packagist:   srv.URL + "/packagist",
		CratesAPI:   srv.URL + "/crates-api",
		CratesIndex: srv.URL + "/crates-index",
		MavenSearch: srv.URL + "/maven/select",
	}))
	t.Cleanup(search.SetCacheDir(""))
}

// recordAllCommands makes every tool "installed" and records, instead of
// running, every command xpm would execute (adapters and project installs).
func recordAllCommands(t *testing.T) *[]string {
	t.Helper()
	var ran []string
	t.Cleanup(pm.SetCommandRunner(func(bin string, args ...string) error {
		ran = append(ran, bin+" "+strings.Join(args, " "))
		return nil
	}))
	t.Cleanup(pm.SetLookPath(func(file string) (string, error) { return "/usr/local/bin/" + file, nil }))
	oldRun := runTool
	runTool = func(bin string, args []string) int {
		ran = append(ran, bin+" "+strings.Join(args, " "))
		return 0
	}
	t.Cleanup(func() { runTool = oldRun })
	return &ran
}

// TestREADMEInstallExamples runs `xpm ...` end to end (no TTY, so
// non-interactive) against fake registries and checks the exact commands.
func TestREADMEInstallExamples(t *testing.T) {
	for _, c := range []struct {
		name   string
		files  []string
		tty    bool // a terminal is attached and the user answers yes to prompts
		args   string
		code   int
		ran    []string
		outHas string
	}{
		{"search then install with the lockfile's tool", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "Detected package-lock.json - using npm"},
		{"pin a version", nil, false, "install axios@1.7.0", 0, []string{"npm install axios@1.7.0"}, "Will install axios@1.7.0 via npm (Node.js)."},
		{"pip pin uses ==", nil, false, "install requests@2.31", 0, []string{"pip install requests==2.31"}, "Will install requests@2.31 via pip"},
		{"no implicit pin", nil, false, "install axios", 0, []string{"npm install axios"}, ""},
		{"global install", nil, false, "install -g typescript", 0, []string{"npm install -g typescript"}, ""},
		{"flag after the package", nil, false, "install typescript -g", 0, []string{"npm install -g typescript"}, ""},
		{"several packages", nil, false, "install axios lodash", 0, []string{"npm install axios", "npm install lodash"}, "[2/2] lodash"},
		{"yarn.lock selects yarn", []string{"package.json", "yarn.lock"}, false, "install axios", 0, []string{"yarn add axios"}, ""},
		{"fuzzy hit is refused without a terminal", []string{"composer.json"}, false, "install monolog", 1, nil, `"monolog" matched monolog/monolog.`},
		{"fuzzy hit confirmed on a terminal installs the real name", []string{"composer.json"}, true, "install monolog", 0, []string{"composer require monolog/monolog"}, `"monolog" matched monolog/monolog.`},
		{"go module path", []string{"go.mod"}, false, "install github.com/gin-gonic/gin", 0, []string{"go get github.com/gin-gonic/gin@latest"}, "is a Go module path"},
		{"gradle build file prints a gradle snippet", []string{"build.gradle.kts"}, false, "install guava", 0, nil, `implementation("com.google.guava:guava:33.3.1-jre")`},
		{"no match exits 1", nil, false, "install nope-not-a-package", 1, nil, "No matches found for nope-not-a-package"},
		{"project install uses the lockfile's tool", []string{"package.json", "pnpm-lock.yaml"}, false, "install", 0, []string{"pnpm install"}, ""},
		{"ci is a frozen install", []string{"package.json", "package-lock.json"}, false, "ci", 0, []string{"npm ci"}, ""},
		{"which", nil, false, "which axios", 0, nil, "- npm: axios @1.7.9 - Promise based HTTP client"},
	} {
		t.Run(c.name, func(t *testing.T) {
			withConfig(t, cfg)
			isolatedHome(t)
			inProject(t, c.files...)
			fakeRegistries(t)
			ran := recordAllCommands(t)
			oldArgs, oldTTY := os.Args, isInteractiveTerminal
			t.Cleanup(func() { os.Args, isInteractiveTerminal = oldArgs, oldTTY })
			isInteractiveTerminal = func() bool { return c.tty }
			if c.tty {
				oldAsk := askYesNo
				askYesNo = func(string) (bool, error) { return true, nil }
				t.Cleanup(func() { askYesNo = oldAsk })
			}
			os.Args = append([]string{"xpm"}, strings.Fields(c.args)...)

			var code int
			out := captureStdout(t, func() { code = Run() })
			if code != c.code {
				t.Errorf("exit %d, want %d\n%s", code, c.code, out)
			}
			if !reflect.DeepEqual(*ran, c.ran) && (len(*ran) != 0 || len(c.ran) != 0) {
				t.Errorf("ran %q, want %q\n%s", *ran, c.ran, out)
			}
			if c.outHas != "" && !strings.Contains(out, c.outHas) {
				t.Errorf("output missing %q:\n%s", c.outHas, out)
			}
		})
	}
}
