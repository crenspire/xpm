package cli

import (
	"errors"
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

// fakeRegistries serves a tiny, fixed world full of the namesakes live
// registries have: npm has axios, lodash, typescript, keyring, express,
// requests and a phpunit squatter; PyPI has requests, keyring, axios and
// lodash; crates.io has express and requests; Packagist has
// monolog/monolog and phpunit/phpunit (not its first hit); Maven Central has
// guava (not its first hit) and org.apache.royale.framework:Express. Like
// the live Packagist and Maven searches, both also return unrelated hits
// for popular names (axios -> swlib/saber). Everything else is "not found".
func fakeRegistries(t *testing.T, w fakeWorld) {
	t.Helper()
	npm := map[string]string{
		"axios":      `{"version":"1.7.9","description":"Promise based HTTP client"}`,
		"lodash":     `{"version":"4.17.21","description":"Lodash modular utilities."}`,
		"typescript": `{"version":"5.6.3","description":"TypeScript is a language for application scale JavaScript"}`,
		"keyring":    `{"version":"0.1.0","description":"A keyring for node"}`,
		"express":    `{"version":"4.21.1","description":"Fast, unopinionated, minimalist web framework"}`,
		"requests":   `{"version":"0.3.0","description":"An streaming XHR abstraction"}`,
		"phpunit":    `{"version":"0.0.1-security","description":"security holding package"}`,
	}
	pypi := map[string]string{
		"requests": `{"info":{"name":"requests","summary":"Python HTTP for Humans.","version":"2.32.3"}}`,
		"keyring":  `{"info":{"name":"keyring","summary":"Store and access your passwords safely.","version":"25.5.0"}}`,
		"axios":    `{"info":{"name":"axios","summary":"","version":"0.0.1"}}`,
		"lodash":   `{"info":{"name":"lodash","summary":"A port of lodash to Python","version":"0.0.1"}}`,
	}
	crates := map[string]string{ // sparse-index path -> index file
		"ex/pr/express":  `{"name":"express","vers":"0.1.2","yanked":false}`,
		"re/qu/requests": `{"name":"requests","vers":"0.0.30","yanked":false}`,
	}
	packagist := map[string][]string{
		"monolog":    {"monolog/monolog"},
		"phpunit":    {"sebastian/phpunit-helper", "phpunit/phpunit"},
		"axios":      {"swlib/saber"},
		"axioss":     {"swlib/saber"},
		"lodash":     {"lodash-php/lodash-php"},
		"requests":   {"psr/http-message"},
		"keyring":    {"psr/http-message"},
		"typescript": {"acme/ts-transpiler"},
	}
	mavenAxios := w.mavenAxios
	if mavenAxios == "" {
		mavenAxios = "org.webjars.npm:axios-retry"
	}
	maven := map[string][]string{
		"guava":      {"com.google.guava:guava-testlib:33.3.1-jre", "com.google.guava:guava:33.3.1-jre"},
		"axios":      {mavenAxios + ":1.0.0"},
		"lodash":     {"org.webjars.npm:lodash.merge:4.6.2"},
		"requests":   {"org.webjars.npm:axios-retry:1.0.0"},
		"express":    {"org.apache.royale.framework:Express:0.9.10"},
		"typescript": {"org.mvnpm:typescript:5.6.3"},
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
		case strings.HasPrefix(p, "/crates-index/"):
			if body, ok := crates[strings.TrimPrefix(p, "/crates-index/")]; ok {
				fmt.Fprintln(w2, body)
				return
			}
		case p == "/packagist/search.json":
			var hits []string
			for _, name := range packagist[q] {
				hits = append(hits, fmt.Sprintf(`{"name":%q,"description":"Packagist hit for %s"}`, name, q))
			}
			fmt.Fprintf(w2, `{"results":[%s]}`, strings.Join(hits, ","))
			return
		case p == "/maven/select":
			if w.mavenDown {
				<-r.Context().Done()
				return
			}
			var docs []string
			for _, coord := range maven[q] {
				parts := strings.Split(coord, ":")
				docs = append(docs, fmt.Sprintf(`{"id":"%s:%s","g":%q,"a":%q,"latestVersion":%q}`, parts[0], parts[1], parts[0], parts[1], parts[2]))
			}
			fmt.Fprintf(w2, `{"response":{"docs":[%s]}}`, strings.Join(docs, ","))
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
		config string // xpmrc.json, if set
		errHas string
	}{
		{"search then install with the lockfile's tool", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "Using npm for this Node project (also found: Python).", fakeWorld{}, "", ""},
		{"pin a version", []string{"package.json", "package-lock.json"}, false, "install axios@1.7.0", 0, []string{"npm install axios@1.7.0"}, "Will install axios@1.7.0 via npm (Node.js).", fakeWorld{}, "", ""},
		{"pip pin uses ==", []string{"requirements.txt"}, false, "install requests@2.31", 0, []string{"pip install requests==2.31"}, "Will install requests@2.31 via pip", fakeWorld{}, "", ""},
		{"no implicit pin", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "", fakeWorld{}, "", ""},
		{"global install", nil, false, "install -g typescript", 0, []string{"npm install -g typescript"}, "", fakeWorld{}, "", ""},
		{"flag after the package", nil, false, "install typescript -g", 0, []string{"npm install -g typescript"}, "", fakeWorld{}, "", ""},
		{"several packages", []string{"package.json", "package-lock.json"}, false, "install axios lodash", 0, []string{"npm install axios", "npm install lodash"}, "[2/2] lodash", fakeWorld{}, "", ""},
		{"yarn.lock selects yarn", []string{"package.json", "yarn.lock"}, false, "install axios", 0, []string{"yarn add axios"}, "", fakeWorld{}, "", ""},
		{"vendor/name equal to the query is exact", []string{"composer.json"}, false, "install monolog", 0, []string{"composer require monolog/monolog"}, `"monolog" matched monolog/monolog.`, fakeWorld{}, "", ""},
		{"fuzzy hit is refused without a terminal", nil, false, "install axioss", 1, nil, `"axioss" matched swlib/saber.`, fakeWorld{}, "", "not an exact match"},
		{"fuzzy hit confirmed on a terminal installs the real name", nil, true, "install axioss", 0, []string{"composer require swlib/saber"}, `"axioss" matched swlib/saber.`, fakeWorld{}, "", ""},
		{"go -g installs a binary", nil, false, "install -g golang.org/x/tools/gopls", 0, []string{"go install golang.org/x/tools/gopls@latest"}, "", fakeWorld{}, "", ""},
		{"go module path", []string{"go.mod"}, false, "install github.com/gin-gonic/gin", 0, []string{"go get github.com/gin-gonic/gin@latest"}, "is a Go module path", fakeWorld{}, "", ""},
		{"gradle build file prints a gradle snippet", []string{"build.gradle.kts"}, false, "install guava", 0, nil, `implementation("com.google.guava:guava:33.3.1-jre")`, fakeWorld{}, "", ""},
		{"no match exits 1", nil, false, "install nope-not-a-package", 1, nil, "No matches found for nope-not-a-package", fakeWorld{}, "", ""},
		{"project install uses the lockfile's tool", []string{"package.json", "pnpm-lock.yaml"}, false, "install", 0, []string{"pnpm install"}, "", fakeWorld{}, "", ""},
		{"ci is a frozen install", []string{"package.json", "package-lock.json"}, false, "ci", 0, []string{"npm ci"}, "", fakeWorld{}, "", ""},
		{"which", nil, false, "which axios", 0, nil, "- npm: axios @1.7.9 - Promise based HTTP client", fakeWorld{}, "", ""},
		// Packagist and Maven return their first, often unrelated, hit.
		{"node project ignores unrelated composer and maven hits", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "Will install axios via npm", fakeWorld{mavenAxios: "org.mvnpm.at.nestjs:axios"}, "", ""},
		{"node project still uses npm while maven times out", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "maven (Java): timed out", fakeWorld{mavenDown: true}, "", ""},
		{"empty dir: the one exact hit wins over closest matches and an mvnpm repackage", nil, false, "install typescript", 0, []string{"npm install typescript"}, "Will install typescript via npm", fakeWorld{}, "", ""},
		{"python project: exact on pip and npm uses pip", []string{"requirements.txt"}, false, "install keyring", 0, []string{"pip install keyring"}, "", fakeWorld{}, "", ""},
		{"empty dir: exact on npm and pip is refused", nil, false, "install keyring", 1, nil, "", fakeWorld{}, "", "keyring exists in several ecosystems: npm (Node), pip (Python)."},
		// Project first: the project's ecosystem beats namesakes elsewhere.
		{"php project: phpunit is composer's, not the npm squatter", []string{"composer.json"}, false, "install phpunit", 0, []string{"composer require phpunit/phpunit"}, "Using composer for this PHP project (also found: Node).", fakeWorld{}, "", ""},
		{"node project: axios is npm's, not the pip namesake", []string{"package.json", "package-lock.json"}, false, "install axios", 0, []string{"npm install axios"}, "", fakeWorld{}, "", ""},
		{"python project: requests is pip's, not npm's or crates'", []string{"requirements.txt"}, false, "install requests", 0, []string{"pip install requests"}, "Using pip for this Python project (also found: Node, Rust).", fakeWorld{}, "", ""},
		{"gradle project: guava although it is not maven's first hit", []string{"build.gradle"}, false, "install guava", 0, nil, `implementation 'com.google.guava:guava:33.3.1-jre'`, fakeWorld{}, "", ""},
		{"empty dir: npm and pip axios are refused with a hint", nil, false, "install axios", 1, nil, "", fakeWorld{}, "", `axios exists in several ecosystems: npm (Node), pip (Python). Set "prefer" in config (e.g. xpm config set prefer npm) or run inside a project`},
		{"empty dir: prefer settles npm vs pip", nil, false, "install axios", 0, []string{"npm install axios"}, `Using npm ("prefer" in config; also found: Python).`, fakeWorld{}, `{"prefer":["npm"]}`, ""},
		{"empty dir: Maven Express is not express, npm vs cargo is refused", nil, false, "install express", 1, nil, "", fakeWorld{}, "", "express exists in several ecosystems: npm (Node), cargo (Rust)."},
		{"empty dir: prefer settles express", nil, false, "install express", 0, []string{"npm install express"}, "", fakeWorld{}, `{"prefer":["npm"]}`, ""},
		{"terminal in a node project: no menu", []string{"package.json", "package-lock.json"}, true, "install axios", 0, []string{"npm install axios"}, "Using npm for this Node project (also found: Python).", fakeWorld{}, "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			withConfig(t, cfg)
			isolatedHome(t)
			inProject(t, c.files...)
			fakeRegistries(t, c.world)
			if c.world.mavenDown {
				writeConfig(t, `{"timeout":{"perRegistry":{"maven":1}}}`)
			}
			if c.config != "" {
				writeConfig(t, c.config)
			}
			ran := recordAllCommands(t)
			oldArgs, oldTTY := os.Args, isInteractiveTerminal
			t.Cleanup(func() { os.Args, isInteractiveTerminal = oldArgs, oldTTY })
			isInteractiveTerminal = func() bool { return c.tty }
			oldSelect := selectCandidate
			selectCandidate = func(string, []string) (int, error) {
				t.Error("a menu was shown")
				return 0, errors.New("no menu expected")
			}
			t.Cleanup(func() { selectCandidate = oldSelect })
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
