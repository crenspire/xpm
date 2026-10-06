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

// fakeWorld varies the fake registries per test.
type fakeWorld struct {
	// mavenAxios is Maven Central's first hit for "axios"; "" means the
	// unrelated org.webjars.npm:axios-retry.
	mavenAxios string
	// mavenDown makes Maven never answer, so it times out.
	mavenDown bool
}

// fakeRegistries serves a tiny, fixed world: npm has axios, lodash,
// typescript and keyring; PyPI has requests and keyring; Packagist finds
// monolog/monolog for "monolog"; Maven Central has guava. Like the live
// Packagist and Maven searches, both also return an unrelated first hit
// for the popular names (axios -> swlib/saber). Everything else is "not
// found".
func fakeRegistries(t *testing.T, w fakeWorld) {
	t.Helper()
	npm := map[string]string{
		"axios":      `{"version":"1.7.9","description":"Promise based HTTP client"}`,
		"lodash":     `{"version":"4.17.21","description":"Lodash modular utilities."}`,
		"typescript": `{"version":"5.6.3","description":"TypeScript is a language for application scale JavaScript"}`,
		"keyring":    `{"version":"0.1.0","description":"A keyring for node"}`,
	}
	pypi := map[string]string{
		"requests": `{"info":{"name":"requests","summary":"Python HTTP for Humans.","version":"2.32.3"}}`,
		"keyring":  `{"info":{"name":"keyring","summary":"Store and access your passwords safely.","version":"25.5.0"}}`,
	}
	packagist := map[string]string{
		"monolog":  "monolog/monolog",
		"axios":    "swlib/saber",
		"lodash":   "lodash-php/lodash-php",
		"requests": "psr/http-message",
		"keyring":  "psr/http-message",
	}
	mavenAxios := w.mavenAxios
	if mavenAxios == "" {
		mavenAxios = "org.webjars.npm:axios-retry"
	}
	maven := map[string]string{
		"guava":    "com.google.guava:guava:33.3.1-jre",
		"axios":    mavenAxios + ":1.0.0",
		"lodash":   "org.webjars.npm:lodash.merge:4.6.2",
		"requests": "org.webjars.npm:axios-retry:1.0.0",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w2 http.ResponseWriter, r *http.Request) {
		p, q := r.URL.Path, r.URL.Query().Get("q")
		switch {
		case strings.HasPrefix(p, "/npm/") && strings.HasSuffix(p, "/latest"):
			if body, ok := npm[strings.TrimSuffix(strings.TrimPrefix(p, "/npm/"), "/latest")]; ok {
				fmt.Fprint(w2, body)
				return
			}
		case strings.HasPrefix(p, "/pypi/") && strings.HasSuffix(p, "/json"):
			if body, ok := pypi[strings.TrimSuffix(strings.TrimPrefix(p, "/pypi/"), "/json")]; ok {
				fmt.Fprint(w2, body)
				return
			}
		case p == "/packagist/search.json":
			if name, ok := packagist[q]; ok {
				fmt.Fprintf(w2, `{"results":[{"name":%q,"description":"first Packagist hit for %s"}]}`, name, q)
				return
			}
			fmt.Fprint(w2, `{"results":[]}`)
			return
		case p == "/maven/select":
			if w.mavenDown {
				<-r.Context().Done()
				return
			}
			if coord, ok := maven[q]; ok {
				parts := strings.Split(coord, ":")
				fmt.Fprintf(w2, `{"response":{"docs":[{"id":"%s:%s","g":%q,"a":%q,"latestVersion":%q}]}}`, parts[0], parts[1], parts[0], parts[1], parts[2])
				return
			}
			fmt.Fprint(w2, `{"response":{"docs":[]}}`)
			return
		}
		http.NotFound(w2, r)
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
		world  fakeWorld
		errHas string
	}{
		{"search then install with the lockfile's tool", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "Detected package-lock.json - using npm", fakeWorld{}, ""},
		{"pin a version", nil, false, "install axios@1.7.0", 0, []string{"npm install axios@1.7.0"}, "Will install axios@1.7.0 via npm (Node.js).", fakeWorld{}, ""},
		{"pip pin uses ==", nil, false, "install requests@2.31", 0, []string{"pip install requests==2.31"}, "Will install requests@2.31 via pip", fakeWorld{}, ""},
		{"no implicit pin", nil, false, "install axios", 0, []string{"npm install axios"}, "", fakeWorld{}, ""},
		{"global install", nil, false, "install -g typescript", 0, []string{"npm install -g typescript"}, "", fakeWorld{}, ""},
		{"flag after the package", nil, false, "install typescript -g", 0, []string{"npm install -g typescript"}, "", fakeWorld{}, ""},
		{"several packages", nil, false, "install axios lodash", 0, []string{"npm install axios", "npm install lodash"}, "[2/2] lodash", fakeWorld{}, ""},
		{"yarn.lock selects yarn", []string{"package.json", "yarn.lock"}, false, "install axios", 0, []string{"yarn add axios"}, "", fakeWorld{}, ""},
		{"fuzzy hit is refused without a terminal", []string{"composer.json"}, false, "install monolog", 1, nil, `"monolog" matched monolog/monolog.`, fakeWorld{}, ""},
		{"fuzzy hit confirmed on a terminal installs the real name", []string{"composer.json"}, true, "install monolog", 0, []string{"composer require monolog/monolog"}, `"monolog" matched monolog/monolog.`, fakeWorld{}, ""},
		{"go -g installs a binary", nil, false, "install -g golang.org/x/tools/gopls", 0, []string{"go install golang.org/x/tools/gopls@latest"}, "", fakeWorld{}, ""},
		{"go module path", []string{"go.mod"}, false, "install github.com/gin-gonic/gin", 0, []string{"go get github.com/gin-gonic/gin@latest"}, "is a Go module path", fakeWorld{}, ""},
		{"gradle build file prints a gradle snippet", []string{"build.gradle.kts"}, false, "install guava", 0, nil, `implementation("com.google.guava:guava:33.3.1-jre")`, fakeWorld{}, ""},
		{"no match exits 1", nil, false, "install nope-not-a-package", 1, nil, "No matches found for nope-not-a-package", fakeWorld{}, ""},
		{"project install uses the lockfile's tool", []string{"package.json", "pnpm-lock.yaml"}, false, "install", 0, []string{"pnpm install"}, "", fakeWorld{}, ""},
		{"ci is a frozen install", []string{"package.json", "package-lock.json"}, false, "ci", 0, []string{"npm ci"}, "", fakeWorld{}, ""},
		{"which", nil, false, "which axios", 0, nil, "- npm: axios @1.7.9 - Promise based HTTP client", fakeWorld{}, ""},
		// Packagist and Maven return their first, often unrelated, hit.
		{"node project ignores unrelated composer and maven hits", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "Will install axios via npm", fakeWorld{mavenAxios: "org.mvnpm.at.nestjs:axios"}, ""},
		{"node project still uses npm while maven times out", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "maven (Java): timed out", fakeWorld{mavenDown: true}, ""},
		{"empty dir: the one exact hit wins over closest matches", nil, false, "install lodash", 0, []string{"npm install lodash"}, "Will install lodash via npm", fakeWorld{}, ""},
		{"python project: exact on pip and npm uses pip", []string{"requirements.txt"}, false, "install keyring", 0, []string{"pip install keyring"}, "", fakeWorld{}, ""},
		{"empty dir: exact on npm and pip is refused", nil, false, "install keyring", 1, nil, "", fakeWorld{}, "keyring exists in several ecosystems: npm (Node), pip (Python)."},
		{"empty dir: a maven artifactId equal to the name is exact too", nil, false, "install axios", 1, nil, "", fakeWorld{mavenAxios: "org.mvnpm.at.nestjs:axios"}, "npm (Node), maven (Java)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			withConfig(t, cfg)
			isolatedHome(t)
			inProject(t, c.files...)
			fakeRegistries(t, c.world)
			if c.world.mavenDown {
				writeConfig(t, `{"timeout":{"perRegistry":{"maven":1}}}`)
			}
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
			var errOut string
			out := captureStdout(t, func() { errOut = captureStderr(t, func() { code = Run() }) })
			if code != c.code {
				t.Errorf("exit %d, want %d\n%s", code, c.code, out)
			}
			if !reflect.DeepEqual(*ran, c.ran) && (len(*ran) != 0 || len(c.ran) != 0) {
				t.Errorf("ran %q, want %q\n%s", *ran, c.ran, out)
			}
			if c.outHas != "" && !strings.Contains(out, c.outHas) {
				t.Errorf("output missing %q:\n%s", c.outHas, out)
			}
			if c.errHas != "" && !strings.Contains(errOut, c.errHas) {
				t.Errorf("stderr missing %q:\n%s", c.errHas, errOut)
			}
		})
	}
}
