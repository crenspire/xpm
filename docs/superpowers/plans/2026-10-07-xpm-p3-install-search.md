# Plan C — P3 Install/Search Correctness

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `install`/`which`/`info`/`search`/`ci` do what the user meant, every time: the right package name, installed by the right tool, without surprise version pins, deletions, hangs or guesses. Registry outages must be reported as outages, not as "not found".

**Architecture:**
- **Registry layer (`internal/search`, Tasks 1–4).**
  - Every registry base URL becomes an injectable variable, and every lookup takes a `context.Context`.
  - One deadline-bounded fan-out (`fanOut`) serves both the exact lookups (`which`/`install`/`info`) and the multi-result search (TUI, `xpm search`). It returns a `Report{Results, Unavailable}`, so callers can tell "not found" from "didn't answer".
  - crates.io moves to the sparse index (CDN). Its description comes from the API, best-effort.
  - Timeouts come from config through `OptionsFromConfig`, which replaces five copy-pasted option builders.
- **CLI decision logic (`internal/cli`, Tasks 5–11).**
  - Decisions move out of the prompt code into small pure functions: `classify`, `buildCandidates`, `installSpec`, `resolveTargetPM`, `projectCmd.args`, `cleanDirs`, `nonInteractivePick`, `parseInstallArgs`.
  - These are table-tested. The `promptui` shells stay thin.
  - Test seams are package variables: `lookupReport`, `searchReport`, `ensurePM`, `installPkg`, `runTool`, `pmExists`, `isInteractiveTerminal`, `stdoutIsTerminal`.
- **Package managers (`internal/pm`, Tasks 6, 7, 12).**
  - Lockfile and build-file detection is ordered and deterministic.
  - `InstallPM` never pipes a remote script into a shell. It re-checks `PATH` after installing.
  - `SetCommandRunner`/`SetLookPath` let any package's tests record commands instead of running them.
- **TUI, config, perf, docs (Tasks 13–16).**
  - TUI: typed `q`/`j`/`k`, real debounce with sequence IDs, rune-safe truncation, `searchUI.*` honoured.
  - Config: `config set` validates before writing.
  - Perf: `perf.sh` reports hyperfine failures and uses a private cache.
  - Tests: an end-to-end table covers every README install example against `httptest` registries.

Every code block in this plan was compiled and its tests run (`go test -race`, golangci-lint v2.5.0: 0 issues) in a scratch copy of the repo on 2026-10-07. They also build under `GOTOOLCHAIN=go1.22.12` (`go vet ./...` clean).

**Tech Stack:** Go 1.22 (module floor), stdlib `net/http`, `httptest`, `context`, bubbletea v0.25, promptui, `golang.org/x/term` (already a dependency), golangci-lint v2.5.0, hyperfine (perf only).

**Spec:** `docs/superpowers/plans/2026-10-06-xpm-roadmap.md` — section "P3 — Install/search correctness" (items 1–13) and the performance-budget table. This plan also covers these items deferred from the P0–P2 reviews:
- crates.io sparse index;
- injectable registry URLs and tests for every registry;
- "unavailable" reporting, plus the TUI deadline and cache;
- `InstallPM` without remote scripts;
- the validation helper, and validating names before the install-tool prompt;
- config `null`/warning fixes;
- `perf.sh` hardening and `XPM_CACHE_DIR`;
- README updates.

**Out of scope:**
- the env/runtime manager (P5);
- graph, lock, workspace, cache and doctor (P6);
- release tooling (P7);
- `usage()`/`man` page text, which belongs to lane D (P4). The README is updated here.

## Global Constraints

- Go module floor stays `go 1.22`; no APIs newer than 1.22 (no t.Chdir, no os.CopyFS, no range-over-int / range-over-func); no new module dependencies (golang.org/x/term is already a dependency and may be used).
- Binary `xpm`, module `github.com/crenspire/xpm`.
- Commit messages must not contain any Co-Authored-By or "Generated with" line (a local commit-msg hook rejects them).
- Every task ends with `go build ./... && go vet ./... && go test ./...` green and `gofmt -l .` empty before its commit; golangci-lint v2.5.0 (`go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`) must report 0 issues for code the task touched (ratchet `new-from-rev` is already configured).
- No network in unit tests: registries are faked with httptest or injectable lookup tables; the on-disk lookup cache must be disabled in tests (existing `withLookups` helper does this).
- Never push; work on `feat/p3-install-search`.

House rules that follow from the lint config (`.golangci.yml`):
- In new or edited import blocks, put `github.com/crenspire/xpm/...` imports in their own last group (goimports `local-prefixes`).
- Write to an `io.Writer` other than `os.Stdout`/`os.Stderr` only via `_, _ =` or a `strings.Builder` (errcheck).
- Wrap errors with `%w` and compare them with `errors.Is`/`errors.As` (errorlint).

## Review Focus

1. **crates.io's API stalls (its tail has been measured at 46 s) while the sparse index answers in about 50 ms.** The crate must come back as found, without a description, before the caller's deadline, and must not be reported as timed out. Test in Task 2: `TestExistsInCratesAPIStallStillFound`.
2. **xpm run in a pipe or CI (no TTY) while one registry is down.** Without a terminal xpm picks for the user, so it must refuse to auto-install a guess when the match list may be incomplete (exit 1, nothing run). Test in Task 11: `TestInstallWithoutTerminalAndMissingRegistryExitsOne`.
3. **A project with `yarn.lock` asking for a name that exists on npm and PyPI.** The lockfile may only turn npm into yarn; it must not silently drop the PyPI option. Test in Task 7: `TestLockfileNarrowsOnlyItsOwnEcosystem`.
4. **`xpm ci` in a repo with `vendor/` (Composer or Go) or `node_modules/`, without a terminal.** Nothing is deleted. Lockfiles are never deleted. Go's `vendor/` is never even offered for deletion. Tests in Task 10: `TestCIDeletesNothingWithoutConfirmation`, `TestCleanDirsNeverTouchGoVendor`.
5. **Typing fast in the TUI over a slow network.** An older, slower search must never overwrite newer results, and typing `q`/`j`/`k` must type rather than quit or move. Tests in Task 13: `TestOnlyTheLatestKeystrokeSearches`, `TestQueryModeTypesQJK`, `TestRegistryModeTypingQStartsTheQuery`.

---

## File map

| File | Change | Task |
|---|---|---|
| `internal/search/endpoints.go` | new: base-URL vars, `Endpoints`, `SetEndpoints`, `SetCacheDir` | 1 |
| `internal/search/search.go` | ctx-aware lookups, `decodeJSON`, injectable URLs; `Options.Timeout`/`RegistryTimeout` | 1, 2, 3, 4 |
| `internal/search/http.go` | `httpGet(ctx, url)`, `httpGetAccept`; `statusError` with no trailing `": "`; drop `SetTimeoutForRegistry` | 1, 4 |
| `internal/search/{npm,pip,composer,cargo,maven}.go` | multi-result searches via `httpGet` (User-Agent, caps, ctx) | 1 |
| `internal/search/registry_test.go` | new: httptest coverage for pip/composer/maven + multi-search UA | 1 |
| `internal/search/crates_index.go`, `crates_test.go` | new: sparse index + best-effort description | 2 |
| `internal/search/fanout.go` | new: `Report`, `RegistryFailure`, `fanOut`, `SearchEverywhereReport` | 3, 4 |
| `internal/search/parallel.go` | rewritten: `SearchReport` on the shared fan-out | 3, 4 |
| `internal/search/lookupcache.go` | `cachedSearch`, `writeJSONAtomic`; `XPM_CACHE_DIR` | 1, 3, 15 |
| `internal/search/report_test.go` | new | 3 |
| `internal/search/options.go`, `options_test.go` | new: `Registries`, `OptionsFromConfig`, `timeoutFor` | 4 |
| `internal/config/config.go`, `config_test.go` | `timeout.default` 0 = built-in; `null` search; testable warning | 4, 14 |
| `internal/cli/lookup.go`, `lookup_test.go`, `helpers_test.go` | new: `classify`, `formatAvailability`, seams, test helpers | 5 |
| `internal/cli/cli.go` | `which` reports unavailable; install code moves out; TTY → non-interactive | 4, 5, 7, 9, 10, 11 |
| `internal/cli/commands.go` | `info` reports unavailable; `list`/`update`/`remove` move to `project.go` | 4, 5, 9, 10 |
| `internal/cli/search_cmd.go` | global cfg, non-TTY plain output, validated TUI installs, `UIOptions` | 4, 5, 7, 9, 13 |
| `internal/pm/lockfiles.go`, `lockfiles_test.go` | ordered detection, `bun.lock`, `Pipfile`, Java build files | 6 |
| `internal/cli/install_plan.go`, `install.go`, `install_plan_test.go` | new: candidates, `installSpec`, Go module routing, install flow | 7, 8, 9, 11, 12 |
| `internal/pm/validation.go`, `validation_test.go` | `rejectLeadingDash`, Composer pattern, poetry/pipenv IDs | 7, 14 |
| `internal/tui/search/*` | Java managers; keys, debounce/seq, runes, `UIOptions` | 8, 13 |
| `internal/cli/args.go`, `args_test.go` | new: `parseInstallArgs`, `exactlyOneArg`, `atMostOneArg` | 9 |
| `internal/cli/project.go`, `project_test.go` | new: project tool resolution, native args, `xpm ci` | 10 |
| `internal/cli/tty.go`, `tty_test.go` | new: `effectiveInteractive`, `nonInteractivePick` | 11 |
| `internal/pm/pm.go`, `install_test.go` | `InstallPM` without scripts; `SetCommandRunner`, `SetLookPath` | 12 |
| `internal/cli/config_cmd.go`, `config_cmd_test.go` | validated `config set`, whole-line prefer editor | 14 |
| `scripts/perf.sh` | FAIL rows, private cache, trap cleanup, `xpm search` row | 15 |
| `internal/cli/e2e_test.go` | new: README install examples end to end | 16 |
| `README.md`, roadmap | Status, usage, exit codes, env vars | 15, 16 |

Order and lanes: one lane (A), executed sequentially in one checkout:
- **Tasks 1–4** are the registry layer.
- **Tasks 5–12** are the CLI. Task 5 (#13, "unavailable") lands before Task 11 (#8, non-TTY non-interactive default), as required.
- **Tasks 13–16** are the TUI, config, perf and docs.

---
### Task 1: Registry plumbing — injectable endpoints, context-aware lookups, honest errors

**Files:**
- Create: `internal/search/endpoints.go`, `internal/search/registry_test.go`
- Replace: `internal/search/search.go` (whole file), `internal/search/npm.go`, `internal/search/pip.go`, `internal/search/composer.go`, `internal/search/cargo.go`
- Modify: `internal/search/http.go` (`httpGet`, `statusError`), `internal/search/maven.go` (`SearchMavenCentral`), `internal/search/parallel.go` (the five `search*Multiple` wrappers), `internal/search/lookupcache.go` (`cachedLookup`)
- Modify tests: `internal/search/everywhere_test.go`, `internal/search/lookupcache_test.go`, `internal/search/npm_test.go`, `internal/search/search_test.go`

**Interfaces:**
- Consumes: the existing `lookup`/`exactLookups`/`lookupCacheDir`/`lookupDeadline`, `maxMetadataBytes`, `userAgent`, and the `mavenSearchResponse` type from `maven.go`.
- Produces (package `search`):
  - `type lookup struct { id pm.ID; fn func(ctx context.Context, pkg string) (*Result, error) }`. **Signature change:** every fake lookup now takes a ctx.
  - `func existsInNpm/existsInPip/existsInComposer/existsInCrates/existsInMaven(ctx context.Context, pkg string) (*Result, error)`
  - `func cachedLookup(ctx context.Context, dir string, l lookup, pkg string) (*Result, error)`
  - `func httpGet(ctx context.Context, rawURL string) (*http.Response, error)`
  - `func httpGetAccept(ctx context.Context, rawURL, accept string) (*http.Response, error)`
  - `func decodeJSON(registry, pkg string, body io.Reader, v any) error`: errors read `"<registry>: decode <pkg>: ..."`.
  - `func statusError(registry string, resp *http.Response) error`: with an empty body the message is exactly `"<registry> registry returned status <code>"`.
  - Package vars `npmRegistryURL, pypiURL, packagistURL, cratesAPIURL, cratesIndexURL, mavenSearchURL`, and const `CratesIndexURL = "https://index.crates.io"`.
  - `type Endpoints struct{ Npm, PyPI, Packagist, CratesAPI, CratesIndex, MavenSearch string }`
  - `func SetEndpoints(e Endpoints) (restore func())`
  - `func SetCacheDir(dir string) (restore func())`
  - `func SearchNpmPackages(ctx, query string, limit int)`, `SearchPackagistPackages(ctx, ...)`, `SearchCratesIO(ctx, ...)`, `SearchMavenCentral(ctx, ...)`, each `([]Result, error)`.
  - `SearchEverywhere(pkg string, opts Options) ([]Result, error)`: unchanged signature. It now cancels in-flight requests at the deadline.
- Removed: `searchPip`, `SearchPyPIPackages`, `pypiPackageResponse` (dead duplicates of `existsInPip`).

- [ ] **Step 1: Write the failing tests**

Create `internal/search/registry_test.go`:

```go
package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeRegistry starts an httptest server, points every registry base URL at
// it (each under its own path prefix) and restores them afterwards.
func fakeRegistry(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	restore := SetEndpoints(Endpoints{
		Npm:         srv.URL + "/npm",
		PyPI:        srv.URL + "/pypi",
		Packagist:   srv.URL + "/packagist",
		CratesAPI:   srv.URL + "/crates-api",
		CratesIndex: srv.URL + "/crates-index",
		MavenSearch: srv.URL + "/maven/select",
	})
	t.Cleanup(restore)
}

var bg = context.Background()

func TestExistsInPipFoundUsesCanonicalName(t *testing.T) {
	var gotPath string
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"info":{"name":"requests","summary":"HTTP for Humans.","version":"2.32.3"}}`)
	})
	r, err := existsInPip(bg, "Requests")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/pypi/Requests/json" {
		t.Errorf("path = %q, want /pypi/Requests/json", gotPath)
	}
	if r == nil || r.Name != "requests" || r.Extra["version"] != "2.32.3" || r.Info != "HTTP for Humans." {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInPipNotFound(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if r, err := existsInPip(bg, "nope"); r != nil || err != nil {
		t.Fatalf("404 must be (nil, nil), got (%+v, %v)", r, err)
	}
}

func TestExistsInPipDecodeErrorNamesPackage(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"info":`) })
	_, err := existsInPip(bg, "requests")
	if err == nil || !strings.Contains(err.Error(), "pypi: decode requests") {
		t.Fatalf("err = %v, want it to name the registry and package", err)
	}
}

func TestStatusErrorWithEmptyBodyHasNoTrailingColon(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	_, err := existsInPip(bg, "requests")
	if err == nil || err.Error() != "pypi registry returned status 503" {
		t.Fatalf("err = %q, want %q", err, "pypi registry returned status 503")
	}
}

func TestExistsInComposerReturnsFirstHit(t *testing.T) {
	var gotQuery string
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		fmt.Fprint(w, `{"results":[{"name":"monolog/monolog","description":"Logging"},{"name":"x/monolog-ext"}]}`)
	})
	r, err := existsInComposer(bg, "monolog")
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "monolog" {
		t.Errorf("q = %q", gotQuery)
	}
	if r == nil || r.Name != "monolog/monolog" || r.Info != "Logging" {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInComposerNoHits(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"results":[]}`) })
	if r, err := existsInComposer(bg, "zzz"); r != nil || err != nil {
		t.Fatalf("no hits must be (nil, nil), got (%+v, %v)", r, err)
	}
}

func TestExistsInComposerDecodeErrorNamesPackage(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `<html>`) })
	_, err := existsInComposer(bg, "monolog")
	if err == nil || !strings.Contains(err.Error(), "packagist: decode monolog") {
		t.Fatalf("err = %v", err)
	}
}

func TestExistsInMavenBuildsCoordinates(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/maven/select" || r.URL.Query().Get("q") != "guava" {
			t.Errorf("unexpected request %s", r.URL)
		}
		fmt.Fprint(w, `{"response":{"docs":[{"id":"com.google.guava:guava","g":"com.google.guava","a":"guava","latestVersion":"33.3.1-jre"}]}}`)
	})
	r, err := existsInMaven(bg, "guava")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || r.Name != "com.google.guava:guava" || r.Extra["group"] != "com.google.guava" ||
		r.Extra["artifact"] != "guava" || r.Extra["version"] != "33.3.1-jre" {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInMavenNoDocs(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"response":{"docs":[]}}`) })
	if r, err := existsInMaven(bg, "zzz"); r != nil || err != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", r, err)
	}
}

func TestExistsInMavenDecodeErrorNamesPackage(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{`) })
	if _, err := existsInMaven(bg, "guava"); err == nil || !strings.Contains(err.Error(), "maven: decode guava") {
		t.Fatalf("err = %v", err)
	}
}

func TestMultiSearchSendsUserAgent(t *testing.T) {
	var uas []string
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		uas = append(uas, r.UserAgent())
		switch {
		case strings.HasPrefix(r.URL.Path, "/npm/"):
			fmt.Fprint(w, `{"objects":[{"package":{"name":"axios","version":"1.7.9"}}]}`)
		case strings.HasPrefix(r.URL.Path, "/crates-api/"):
			fmt.Fprint(w, `{"crates":[{"name":"serde","max_version":"2.0.0-rc.1","default_version":"1.0.215"}]}`)
		case strings.HasPrefix(r.URL.Path, "/packagist/"):
			fmt.Fprint(w, `{"results":[{"name":"monolog/monolog"}]}`)
		default:
			fmt.Fprint(w, `{"response":{"docs":[]}}`)
		}
	})
	npm, err := SearchNpmPackages(bg, "axios", 5)
	if err != nil || len(npm) != 1 || npm[0].Name != "axios" {
		t.Fatalf("npm: %+v %v", npm, err)
	}
	crates, err := SearchCratesIO(bg, "serde", 5)
	if err != nil || len(crates) != 1 || crates[0].Extra["version"] != "1.0.215" {
		t.Fatalf("crates: %+v %v (want the stable default_version)", crates, err)
	}
	if _, err := SearchPackagistPackages(bg, "monolog", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchMavenCentral(bg, "guava", 5); err != nil {
		t.Fatal(err)
	}
	for _, ua := range uas {
		if !strings.HasPrefix(ua, "xpm") {
			t.Fatalf("User-Agent = %q, want xpm/... (crates.io answers 403 to Go's default UA)", ua)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/search/ -run 'ExistsIn|StatusError|MultiSearch' -v`
Expected: FAIL. This is a build error: `undefined: SetEndpoints`, `undefined: Endpoints`, `too many arguments in call to existsInPip`.

- [ ] **Step 3: Add the endpoint variables and test hooks**

Create `internal/search/endpoints.go`:

```go
package search

// Registry base URLs. They are variables so tests (and SetEndpoints) can
// point them at an httptest server; production code never changes them.
var (
	npmRegistryURL = NpmRegistryURL
	pypiURL        = PyPIURL
	packagistURL   = PackagistURL
	cratesAPIURL   = CratesIOURL
	cratesIndexURL = CratesIndexURL
	mavenSearchURL = MavenSearchURL
)

// CratesIndexURL is the crates.io sparse index (served from a CDN).
const CratesIndexURL = "https://index.crates.io"

// Endpoints overrides registry base URLs. Empty fields keep the current value.
type Endpoints struct {
	Npm         string // default https://registry.npmjs.org
	PyPI        string // default https://pypi.org/pypi
	Packagist   string // default https://packagist.org
	CratesAPI   string // default https://crates.io/api/v1
	CratesIndex string // default https://index.crates.io
	MavenSearch string // default https://central.sonatype.com/solrsearch/select
}

// SetEndpoints points registry lookups at other base URLs and returns a
// function that restores the previous ones. It exists for tests in other
// packages (fake registries via httptest); it is not safe to call while
// lookups are running.
func SetEndpoints(e Endpoints) (restore func()) {
	old := Endpoints{npmRegistryURL, pypiURL, packagistURL, cratesAPIURL, cratesIndexURL, mavenSearchURL}
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&npmRegistryURL, e.Npm)
	set(&pypiURL, e.PyPI)
	set(&packagistURL, e.Packagist)
	set(&cratesAPIURL, e.CratesAPI)
	set(&cratesIndexURL, e.CratesIndex)
	set(&mavenSearchURL, e.MavenSearch)
	return func() {
		npmRegistryURL, pypiURL, packagistURL = old.Npm, old.PyPI, old.Packagist
		cratesAPIURL, cratesIndexURL, mavenSearchURL = old.CratesAPI, old.CratesIndex, old.MavenSearch
	}
}

// SetCacheDir replaces the on-disk lookup cache directory ("" disables the
// cache) and returns a function that restores the previous one. For tests in
// other packages; not safe to call while lookups are running.
func SetCacheDir(dir string) (restore func()) {
	old := lookupCacheDir
	lookupCacheDir = dir
	return func() { lookupCacheDir = old }
}
```

- [ ] **Step 4: Make `httpGet` context-aware and fix `statusError`**

In `internal/search/http.go`, replace `httpGet` and `statusError` (from the comment `// httpGet is the single entry point for registry GETs` to the end of the file) with the following. Then add `"context"` to the imports.

```go
// httpGet is the single entry point for registry GETs: shared client
// (with its timeout), ctx for cancellation, identifying User-Agent and a
// JSON Accept header.
func httpGet(ctx context.Context, rawURL string) (*http.Response, error) {
	return httpGetAccept(ctx, rawURL, "application/json")
}

// httpGetAccept is httpGet with a caller-chosen Accept header (the crates.io
// sparse index serves text/plain).
func httpGetAccept(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	return httpClient.Do(req)
}

// statusError builds an error for a non-2xx response, including at most
// 512 bytes of the body so HTML error pages don't flood the terminal.
// An empty body yields no trailing ": ".
func statusError(registry string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("%s registry returned status %d", registry, resp.StatusCode)
	}
	return fmt.Errorf("%s registry returned status %d: %s", registry, resp.StatusCode, msg)
}
```

- [ ] **Step 5: Replace `search.go`**

Replace the whole of `internal/search/search.go` with the version below. Compared with today:
- every `existsIn*` takes `ctx`, reads its base URL from the vars, and decodes through `decodeJSON`;
- pip returns PyPI's canonical name;
- Maven reuses `mavenSearchResponse`;
- `SearchEverywhere` creates a `context.WithTimeout(lookupDeadline)`, so stragglers are cancelled.

```go
// Package search provides functionality to search for packages across multiple registries.
//
// This package queries package registries (npm, PyPI, Packagist, crates.io, Maven Central)
// concurrently to find packages by name. Results include package metadata such as version and description.
//
// Example usage:
//
//	opts := search.Options{
//	    Enable: map[pm.ID]bool{
//	        pm.Npm: true,
//	        pm.Pip: true,
//	    },
//	}
//	results, err := search.SearchEverywhere("lodash", opts)
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// Result represents a package found in a registry search.
type Result struct {
	// Manager is the package manager that found this result.
	Manager pm.ID
	// Name is the package name (may differ from search query for fuzzy matches).
	Name string
	// Info is a short description of the package.
	Info string
	// Extra contains additional metadata like version, group (Maven), etc.
	Extra map[string]string
}

// Options controls search behavior.
type Options struct {
	// Enable specifies which package managers to include in the search.
	// If nil or a key is missing, that ecosystem is searched by default.
	Enable map[pm.ID]bool
}

// DefaultTimeout is the HTTP request timeout for registry queries.
const DefaultTimeout = 4 * time.Second

// httpClient is the shared HTTP client for all registry queries.
var httpClient = &http.Client{
	Timeout: DefaultTimeout,
}

// Enabled checks if a package manager is enabled in the given options.
// Returns true if the ecosystem should be searched.
func Enabled(opts Options, id pm.ID) bool {
	if opts.Enable == nil {
		return true
	}
	v, ok := opts.Enable[id]
	if !ok {
		return true
	}
	return v
}

// validatePackageNameForURL rejects names that are empty, too long, or could
// change the meaning of the registry URL they are spliced into
// (query/fragment/escape characters, whitespace, control characters, or
// "." / ".." path segments). Scoped names like "@types/node" and composer
// names like "vendor/pkg" are allowed.
func validatePackageNameForURL(pkg string) error {
	if len(pkg) == 0 {
		return fmt.Errorf("package name cannot be empty")
	}
	const maxPackageNameLength = 214 // npm's limit; the strictest registry
	if len(pkg) > maxPackageNameLength {
		return fmt.Errorf("package name too long (max %d characters)", maxPackageNameLength)
	}
	if strings.ContainsAny(pkg, "?#%\\") {
		return fmt.Errorf("invalid package name %q: contains a URL-reserved character", pkg)
	}
	for _, r := range pkg {
		if r <= ' ' || r == 0x7f {
			return fmt.Errorf("invalid package name %q: contains whitespace or control characters", pkg)
		}
	}
	for _, seg := range strings.Split(pkg, "/") {
		if seg == "." || seg == ".." {
			return fmt.Errorf("invalid package name %q: contains a relative path segment", pkg)
		}
	}
	return nil
}

// decodeJSON decodes at most maxMetadataBytes of body into v. Errors name
// the registry and package so "unexpected EOF" is never the whole message.
func decodeJSON(registry, pkg string, body io.Reader, v any) error {
	if err := json.NewDecoder(io.LimitReader(body, maxMetadataBytes)).Decode(v); err != nil {
		return fmt.Errorf("%s: decode %s: %w", registry, pkg, err)
	}
	return nil
}

// existsInNpm checks if a package exists in the npm registry.
// It fetches /<pkg>/latest (~2-4 KB) instead of the full packument, which is
// 15 MB+ for packages like typescript and regularly blew the 4s timeout.
// Returns (nil, nil) if the package is not found.
func existsInNpm(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s/%s/latest", npmRegistryURL, url.PathEscape(pkg))
	logx.Info("query npm: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("npm", resp)
	}
	var data struct {
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := decodeJSON("npm", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	return &Result{
		Manager: pm.Npm,
		Name:    pkg,
		Info:    data.Description,
		Extra:   map[string]string{"version": data.Version},
	}, nil
}

// existsInPip checks if a package exists in the Python Package Index (PyPI).
// Returns (nil, nil) if the package is not found.
func existsInPip(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s/%s/json", pypiURL, url.PathEscape(pkg))
	logx.Info("query pypi: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("pypi", resp)
	}
	var data struct {
		Info struct {
			Name    string `json:"name"`
			Summary string `json:"summary"`
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := decodeJSON("pypi", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	name := data.Info.Name
	if name == "" {
		name = pkg
	}
	return &Result{
		Manager: pm.Pip,
		Name:    name,
		Info:    data.Info.Summary,
		Extra:   map[string]string{"version": data.Info.Version},
	}, nil
}

// existsInComposer searches Packagist and returns the first hit, which may
// be a fuzzy match ("monolog" -> "monolog/monolog"). Returns (nil, nil) if
// there are no hits.
func existsInComposer(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s/search.json?q=%s", packagistURL, url.QueryEscape(pkg))
	logx.Info("query packagist: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("packagist", resp)
	}
	var data struct {
		Results []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"results"`
	}
	if err := decodeJSON("packagist", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	if len(data.Results) == 0 {
		return nil, nil
	}
	first := data.Results[0]
	return &Result{
		Manager: pm.Composer,
		Name:    first.Name,
		Info:    first.Description,
		Extra:   map[string]string{},
	}, nil
}

// existsInCrates checks if a crate exists in crates.io (Rust registry).
// Returns (nil, nil) if the crate is not found.
func existsInCrates(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s/crates/%s?include=default_version", cratesAPIURL, url.PathEscape(pkg))
	logx.Info("query crates.io: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("crates.io", resp)
	}
	var data struct {
		Crate struct {
			Description    string `json:"description"`
			MaxVersion     string `json:"max_version"`
			DefaultVersion string `json:"default_version"`
			Name           string `json:"name"`
		} `json:"crate"`
	}
	if err := decodeJSON("crates.io", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	version := data.Crate.DefaultVersion // newest stable; max_version can be a prerelease
	if version == "" {
		version = data.Crate.MaxVersion
	}
	return &Result{
		Manager: pm.Cargo,
		Name:    data.Crate.Name,
		Info:    data.Crate.Description,
		Extra:   map[string]string{"version": version},
	}, nil
}

// existsInMaven searches Maven Central and returns the first hit.
// Returns (nil, nil) if there are no hits.
func existsInMaven(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s?q=%s&rows=5&wt=json", mavenSearchURL, url.QueryEscape(pkg))
	logx.Info("query maven: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("maven", resp)
	}
	var data mavenSearchResponse
	if err := decodeJSON("maven", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	if len(data.Response.Docs) == 0 {
		return nil, nil
	}
	d := data.Response.Docs[0]
	return &Result{
		Manager: pm.Maven,
		Name:    d.Group + ":" + d.Artifact,
		Info:    "Maven artifact",
		Extra: map[string]string{
			"version":  d.LatestVersion,
			"id":       d.ID,
			"group":    d.Group,
			"artifact": d.Artifact,
		},
	}, nil
}

// lookup is one registry's exact-name check. fn must honour ctx.
type lookup struct {
	id pm.ID
	fn func(ctx context.Context, pkg string) (*Result, error)
}

// exactLookups is the registry table used by SearchEverywhere, in result order.
// Tests replace it with fakes.
var exactLookups = []lookup{
	{id: pm.Npm, fn: existsInNpm},
	{id: pm.Pip, fn: existsInPip},
	{id: pm.Composer, fn: existsInComposer},
	{id: pm.Cargo, fn: existsInCrates},
	{id: pm.Maven, fn: existsInMaven},
}

// lookupDeadline bounds the whole fan-out. Registry APIs have long tails
// (crates.io has been measured at 46s, Maven search stalls without ever
// answering), so a registry that misses the deadline is reported as timed
// out instead of holding up the answer.
var lookupDeadline = 2500 * time.Millisecond

var (
	// ErrAllRegistriesFailed is returned when every enabled registry errored
	// or timed out, so callers can tell "offline" from "no such package".
	ErrAllRegistriesFailed = errors.New("all registries failed")
	// ErrRegistryTimeout marks a registry that missed lookupDeadline.
	ErrRegistryTimeout = errors.New("registry did not answer in time")
)

// SearchEverywhere checks every enabled registry concurrently for an exact
// package name and returns within lookupDeadline. Results are returned in
// table order. A failing or slow registry is logged and skipped; only if all
// of them fail is an error (wrapping ErrAllRegistriesFailed) returned.
// Requests still running at the deadline are cancelled.
func SearchEverywhere(pkg string, opts Options) ([]Result, error) {
	var enabled []lookup
	for _, l := range exactLookups {
		if Enabled(opts, l.id) {
			enabled = append(enabled, l)
		}
	}
	if len(enabled) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), lookupDeadline)
	defer cancel()

	type outcome struct {
		i   int
		res *Result
		err error
	}
	// Buffered so goroutines that finish after the deadline never block.
	ch := make(chan outcome, len(enabled))
	cacheDir := lookupCacheDir
	for i, l := range enabled {
		go func(i int, l lookup) {
			res, err := cachedLookup(ctx, cacheDir, l, pkg)
			ch <- outcome{i: i, res: res, err: err}
		}(i, l)
	}

	outcomes := make([]outcome, len(enabled))
	for i := range outcomes {
		outcomes[i] = outcome{i: i, err: ErrRegistryTimeout}
	}
	timer := time.NewTimer(lookupDeadline)
	defer timer.Stop()
collect:
	for received := 0; received < len(enabled); received++ {
		select {
		case o := <-ch:
			outcomes[o.i] = o
		case <-timer.C:
			break collect
		}
	}

	var out []Result
	var errs []error
	for i, o := range outcomes {
		if o.err != nil {
			logx.Info("lookup %s in %s failed: %v", pkg, enabled[i].id, o.err)
			errs = append(errs, fmt.Errorf("%s: %w", enabled[i].id, o.err))
			continue
		}
		if o.res != nil {
			out = append(out, *o.res)
		}
	}
	if len(errs) == len(enabled) {
		return nil, fmt.Errorf("%w: %w", ErrAllRegistriesFailed, errors.Join(errs...))
	}
	return out, nil
}
```

- [ ] **Step 6: Route the multi-result searches through `httpGet`**

Today these use `httpClient.Get` with Go's default User-Agent, which crates.io rejects with 403. Their bodies are also uncapped.

Replace `internal/search/npm.go` with:

```go
package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// NpmRegistryURL is the base URL for the npm registry API.
const NpmRegistryURL = "https://registry.npmjs.org"

// SearchNpmPackages searches npm for packages matching a query.
// This uses the npm search API for broader results.
func SearchNpmPackages(ctx context.Context, query string, limit int) ([]Result, error) {
	u := fmt.Sprintf("%s/-/v1/search?text=%s&size=%d", npmRegistryURL, url.QueryEscape(query), limit)
	logx.Info("search npm: %s", u)

	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("npm search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("npm", resp)
	}

	var data struct {
		Objects []struct {
			Package struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Version     string `json:"version"`
			} `json:"package"`
		} `json:"objects"`
	}
	if err := decodeJSON("npm", query, resp.Body, &data); err != nil {
		return nil, err
	}

	var results []Result
	for _, obj := range data.Objects {
		results = append(results, Result{
			Manager: pm.Npm,
			Name:    obj.Package.Name,
			Info:    obj.Package.Description,
			Extra:   map[string]string{"version": obj.Package.Version},
		})
	}
	return results, nil
}
```

Replace `internal/search/pip.go` with:

```go
package search

// PyPIURL is the base URL for the PyPI JSON API. PyPI has no search API,
// so multi-result search uses the exact lookup (existsInPip).
const PyPIURL = "https://pypi.org/pypi"
```

Replace `internal/search/composer.go` with:

```go
package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// PackagistURL is the base URL for the Packagist API.
// Note: packagist.org hosts the public API endpoints (search.json, packages/list.json).
const PackagistURL = "https://packagist.org"

// packagistSearchResponse represents the Packagist search API response.
type packagistSearchResponse struct {
	Results []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Repository  string `json:"repository"`
		Downloads   int    `json:"downloads"`
		Favers      int    `json:"favers"`
	} `json:"results"`
	Total int `json:"total"`
}

// SearchPackagistPackages searches Packagist for up to limit results.
// Packagist API: https://packagist.org/apidoc#search-packages
func SearchPackagistPackages(ctx context.Context, query string, limit int) ([]Result, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	u := fmt.Sprintf("%s/search.json?q=%s", packagistURL, url.QueryEscape(query))
	logx.Info("search packagist: %s", u)

	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("packagist search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("packagist", resp)
	}

	var data packagistSearchResponse
	if err := decodeJSON("packagist", query, resp.Body, &data); err != nil {
		return nil, err
	}

	var results []Result
	for i, r := range data.Results {
		if i >= limit {
			break
		}
		results = append(results, Result{
			Manager: pm.Composer,
			Name:    r.Name,
			Info:    r.Description,
			Extra: map[string]string{
				"url":       r.URL,
				"downloads": fmt.Sprintf("%d", r.Downloads),
			},
		})
	}
	return results, nil
}
```

Replace `internal/search/cargo.go` with the following. It now prefers `default_version`, which is stable, over `max_version`, which can be a prerelease.

```go
package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// CratesIOURL is the base URL for the crates.io API.
const CratesIOURL = "https://crates.io/api/v1"

// SearchCratesIO searches crates.io for crates matching a query.
func SearchCratesIO(ctx context.Context, query string, limit int) ([]Result, error) {
	u := fmt.Sprintf("%s/crates?q=%s&per_page=%d", cratesAPIURL, url.QueryEscape(query), limit)
	logx.Info("search crates.io: %s", u)

	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("crates.io search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("crates.io", resp)
	}

	var data struct {
		Crates []struct {
			Name           string `json:"name"`
			Description    string `json:"description"`
			MaxVersion     string `json:"max_version"`
			DefaultVersion string `json:"default_version"`
			Downloads      int    `json:"downloads"`
		} `json:"crates"`
	}
	if err := decodeJSON("crates.io", query, resp.Body, &data); err != nil {
		return nil, err
	}

	var results []Result
	for _, c := range data.Crates {
		version := c.DefaultVersion
		if version == "" {
			version = c.MaxVersion
		}
		results = append(results, Result{
			Manager: pm.Cargo,
			Name:    c.Name,
			Info:    c.Description,
			Extra: map[string]string{
				"version":   version,
				"downloads": fmt.Sprintf("%d", c.Downloads),
			},
		})
	}
	return results, nil
}
```

In `internal/search/maven.go`:
- change the import block to `"context"`, `"fmt"`, `"net/http"`, `"net/url"` plus the two `internal/...` imports (`"encoding/json"` is no longer used);
- replace the whole `SearchMavenCentral` function (from `// SearchMavenCentral searches` to just before `// GetMavenDependencyXML`) with:

```go
// SearchMavenCentral searches Maven Central for up to limit artifacts.
func SearchMavenCentral(ctx context.Context, query string, limit int) ([]Result, error) {
	u := fmt.Sprintf("%s?q=%s&rows=%d&wt=json", mavenSearchURL, url.QueryEscape(query), limit)
	logx.Info("search maven: %s", u)

	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("maven search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("maven", resp)
	}

	var data mavenSearchResponse
	if err := decodeJSON("maven", query, resp.Body, &data); err != nil {
		return nil, err
	}

	var results []Result
	for _, doc := range data.Response.Docs {
		results = append(results, Result{
			Manager: pm.Maven,
			Name:    doc.Group + ":" + doc.Artifact,
			Info:    "Maven artifact",
			Extra: map[string]string{
				"version":  doc.LatestVersion,
				"id":       doc.ID,
				"group":    doc.Group,
				"artifact": doc.Artifact,
			},
		})
	}
	return results, nil
}
```

- [ ] **Step 7: Thread ctx through `parallel.go` and the cache**

`parallel.go` is rewritten in Task 3. For now, keep it compiling:
- Replace each of the three occurrences of `func(string) ([]Result, error)` with `func(context.Context, string) ([]Result, error)`. Two are struct fields; one is the goroutine's parameter type.
- Replace `results, err := fn(pkg)` with `results, err := fn(ctx, pkg)`.
- Replace the five wrappers (from `// Wrapper functions that use proper search APIs` to just before `// BatchSearch`) with:

```go
// Wrapper functions that use proper search APIs instead of existence checks
func searchNpmMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchNpmPackages(ctx, query, 20)
}

// searchPipMultiple uses the exact lookup: PyPI has no search API.
func searchPipMultiple(ctx context.Context, query string) ([]Result, error) {
	result, err := existsInPip(ctx, query)
	if err != nil || result == nil {
		return nil, err
	}
	return []Result{*result}, nil
}

func searchComposerMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchPackagistPackages(ctx, query, 20)
}

func searchCargoMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchCratesIO(ctx, query, 20)
}

func searchMavenMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchMavenCentral(ctx, query, 20)
}
```

In `internal/search/lookupcache.go`:
- add `"context"` to the imports;
- change the signature to `func cachedLookup(ctx context.Context, dir string, l lookup, pkg string) (*Result, error)`;
- change both calls `l.fn(pkg)` to `l.fn(ctx, pkg)`.

- [ ] **Step 8: Update the existing tests for the new signatures**

1. `internal/search/everywhere_test.go`: add `"context"` to the imports and make `fakeLookup` honour cancellation:

```go
func fakeLookup(id pm.ID, delay time.Duration, err error) lookup {
	return lookup{id: id, fn: func(ctx context.Context, pkg string) (*Result, error) {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, err
		}
		return &Result{Manager: id, Name: pkg}, nil
	}}
}
```

   In `TestSearchEverywhereSkipsDisabled`, the lookup becomes `{id: pm.Npm, fn: func(context.Context, string) (*Result, error) { called = true; return nil, nil }}`. In `BenchmarkSearchEverywhereFanout` the fake becomes `func(_ context.Context, p string) (*Result, error)`.
2. `internal/search/lookupcache_test.go`: add `"context"`. In `countingLookup` the fn becomes `func(context.Context, string) (*Result, error)`. Every `cachedLookup(dir, ...)` call becomes `cachedLookup(context.Background(), dir, ...)` (7 calls).
3. `internal/search/npm_test.go`: add `"context"`. Every `existsInNpm("...")` becomes `existsInNpm(context.Background(), "...")` (4 calls).
4. `internal/search/search_test.go`: delete `TestExistsInNpm`, `TestExistsInPip`, `TestExistsInComposer`, `TestExistsInCrates` and `TestExistsInMaven`. They start an httptest server but never call the code under test; `registry_test.go` replaces them. Then delete the now-unused imports `"net/http"` and `"net/http/httptest"`.

- [ ] **Step 9: Run the tests**

Run: `go test -race -count=2 ./internal/search/`
Expected: `ok  	github.com/crenspire/xpm/internal/search`. All new `TestExistsIn*`, `TestStatusErrorWithEmptyBodyHasNoTrailingColon` and `TestMultiSearchSendsUserAgent` PASS.

- [ ] **Step 10: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: all `ok`, no `gofmt` output, `0 issues.`

```bash
git add internal/search
git commit -m "search: injectable registry URLs, ctx-aware lookups, named decode errors, httptest coverage"
```

---

### Task 2: crates.io via the sparse index, with a best-effort description

**Files:**
- Create: `internal/search/crates_index.go`, `internal/search/crates_test.go`
- Modify: `internal/search/search.go`: delete `existsInCrates` (from `// existsInCrates checks if a crate exists` to just before `// existsInMaven`)

**Interfaces:**
- Consumes (Task 1): `httpGetAccept`, `httpGet`, `statusError`, `decodeJSON`, `cratesIndexURL`, `cratesAPIURL`, and `fakeRegistry`/`bg` from `registry_test.go`.
- Produces:
  - `func existsInCrates(ctx context.Context, pkg string) (*Result, error)` (same signature, new implementation);
  - `func cratesIndexPath(name string) string`;
  - `func latestFromIndex(r io.Reader) (name, version string, err error)`;
  - `func semverLess(a, b string) bool`;
  - `func isPrerelease(v string) bool`;
  - `var cratesDescriptionTimeout = 2 * time.Second`;
  - `const descriptionMargin = 200 * time.Millisecond`;
  - `func cratesDescriptionBudget(ctx context.Context) time.Duration`.

Design notes:
- **Existence and version come from the index.** `https://index.crates.io/<prefix>/<name>` is served from a CDN. Measured on 2026-10-07: 9–13 ms warm and 175 ms cold. A 404 means no such crate.
- **The description comes from the API, best-effort.** `/api/v1/crates/<name>?include=default_version` took 1.2–1.3 s in the same measurement.
  - It may run until 200 ms before the caller's deadline (at most 2 s), so it can never turn a found crate into a timeout.
- **Version choice deviates from the spec's "last non-yanked entry".** The version is the highest non-yanked **stable** version, and the highest prerelease only if nothing stable exists.
  - Index lines are in publish order, not version order. A `0.9.x` patch published after `1.0.0` would otherwise win.
  - The tests pin this.
- **Names that cannot be crates are "not found" with no request.** For example `@types/node` or `monolog/monolog`.

- [ ] **Step 1: Write the failing tests**

Create `internal/search/crates_test.go`:

```go
package search

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCratesIndexPath(t *testing.T) {
	for name, want := range map[string]string{
		"a": "1/a", "ab": "2/ab", "abc": "3/a/abc", "Serde": "se/rd/serde", "tokio": "to/ki/tokio",
	} {
		if got := cratesIndexPath(name); got != want {
			t.Errorf("cratesIndexPath(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestSemverLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.0.9", "1.0.10", true},
		{"0.9.0", "0.8.5", false},
		{"2.0.0-rc.1", "2.0.0", true},
		{"2.0.0", "2.0.0-rc.1", false},
		{"1.0.0+build.1", "1.0.0", false},
		{"1.2.3", "1.2.3", false},
	} {
		if got := semverLess(c.a, c.b); got != c.want {
			t.Errorf("semverLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

const serdeIndex = `{"name":"serde","vers":"1.0.214","deps":[],"cksum":"x","features":{},"yanked":false}
{"name":"serde","vers":"1.0.215","deps":[],"cksum":"x","features":{},"yanked":true}
{"name":"serde","vers":"2.0.0-rc.1","deps":[],"cksum":"x","features":{},"yanked":false}
{"name":"serde","vers":"0.9.16","deps":[],"cksum":"x","features":{},"yanked":false}
`

func TestExistsInCratesUsesSparseIndex(t *testing.T) {
	var indexPath string
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/crates-index/"):
			indexPath = r.URL.Path
			fmt.Fprint(w, serdeIndex)
		case r.URL.Path == "/crates-api/crates/serde":
			fmt.Fprint(w, `{"crate":{"name":"serde","description":"A serialization framework"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	r, err := existsInCrates(bg, "serde")
	if err != nil {
		t.Fatal(err)
	}
	if indexPath != "/crates-index/se/rd/serde" {
		t.Errorf("index path = %q", indexPath)
	}
	// 1.0.215 is yanked, 2.0.0-rc.1 is a prerelease, 0.9.16 was published last but is older.
	if r == nil || r.Name != "serde" || r.Extra["version"] != "1.0.214" || r.Info != "A serialization framework" {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInCratesNotFoundSkipsAPI(t *testing.T) {
	var apiCalls int32
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/crates-api/") {
			atomic.AddInt32(&apiCalls, 1)
		}
		http.NotFound(w, r)
	})
	if r, err := existsInCrates(bg, "no-such-crate"); r != nil || err != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", r, err)
	}
	if atomic.LoadInt32(&apiCalls) != 0 {
		t.Fatal("the API must not be called when the index says 404")
	}
}

func TestExistsInCratesAPIStallStillFound(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/crates-api/") {
			select { // stall like crates.io's 46 s tail
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return
		}
		fmt.Fprint(w, serdeIndex)
	})
	// The fan-out gives each registry a deadline; the crate must come back
	// before it, without a description, instead of timing out.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r, err := existsInCrates(ctx, "serde")
	if err != nil || r == nil || r.Extra["version"] != "1.0.214" {
		t.Fatalf("an API stall must not hide a found crate: (%+v, %v)", r, err)
	}
	if r.Info != "" {
		t.Errorf("Info = %q, want empty when the API stalls", r.Info)
	}
	if ctx.Err() != nil {
		t.Fatal("existsInCrates returned after the caller's deadline")
	}
}

func TestCratesDescriptionBudget(t *testing.T) {
	if got := cratesDescriptionBudget(context.Background()); got != cratesDescriptionTimeout {
		t.Errorf("no deadline: budget = %v, want %v", got, cratesDescriptionTimeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if got := cratesDescriptionBudget(ctx); got > time.Second-descriptionMargin {
		t.Errorf("budget %v ignores the caller's deadline", got)
	}
}

func TestExistsInCratesAllYankedIsNotFound(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"gone","vers":"0.1.0","yanked":true}`+"\n")
	})
	if r, err := existsInCrates(bg, "gone"); r != nil || err != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", r, err)
	}
}

func TestExistsInCratesInvalidNameMakesNoRequest(t *testing.T) {
	var calls int32
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&calls, 1) })
	for _, name := range []string{"@types/node", "monolog/monolog", "com.google.guava:guava", "1abc"} {
		if r, err := existsInCrates(bg, name); r != nil || err != nil {
			t.Errorf("%q: got (%+v, %v), want (nil, nil)", name, r, err)
		}
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("%d requests for names that cannot be crates", calls)
	}
}

func TestExistsInCratesBadIndexLineIsAnError(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "{not json\n") })
	if _, err := existsInCrates(bg, "serde"); err == nil || !strings.Contains(err.Error(), "crates.io index: serde") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/search/ -run 'Crates|Semver' -v`
Expected: FAIL. This is a build error: `undefined: cratesIndexPath`, `undefined: semverLess`, `undefined: cratesDescriptionBudget`.

- [ ] **Step 3: Implement**

Delete `existsInCrates` from `internal/search/search.go`, then create `internal/search/crates_index.go`:

```go
package search

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// cratesNamePattern is what crates.io accepts as a crate name. Anything else
// (e.g. "@types/node") cannot be a crate, so it is "not found" with no request.
var cratesNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// cratesDescriptionTimeout caps the best-effort API call for the
// description (the API's tail is long; 1.2 s is common). The index answers
// existence and version; an API stall must never turn a found crate into a
// timeout, so the call also stops descriptionMargin before the lookup's own
// deadline.
var cratesDescriptionTimeout = 2 * time.Second

const descriptionMargin = 200 * time.Millisecond

// maxIndexBytes caps an index file. The biggest real ones are a few MB; the
// file is streamed line by line, never held in memory.
const maxIndexBytes = 16 << 20

// cratesIndexPath returns the sparse-index path of a crate (lowercased):
// 1 char -> 1/a, 2 -> 2/ab, 3 -> 3/a/abc, else ab/cd/abcd...
func cratesIndexPath(name string) string {
	n := strings.ToLower(name)
	switch len(n) {
	case 1:
		return "1/" + n
	case 2:
		return "2/" + n
	case 3:
		return "3/" + n[:1] + "/" + n
	default:
		return n[:2] + "/" + n[2:4] + "/" + n
	}
}

// existsInCrates checks crates.io via the sparse index (CDN, ~50 ms warm;
// 404 = no such crate). The version is the highest non-yanked stable
// release (highest non-yanked prerelease if there is no stable one). The
// description comes from the API, best-effort. Returns (nil, nil) if the
// crate doesn't exist or every version is yanked.
func existsInCrates(ctx context.Context, pkg string) (*Result, error) {
	if !cratesNamePattern.MatchString(pkg) {
		return nil, nil
	}
	u := cratesIndexURL + "/" + cratesIndexPath(pkg)
	logx.Info("query crates.io index: %s", u)
	resp, err := httpGetAccept(ctx, u, "text/plain, application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("crates.io index", resp)
	}
	name, version, err := latestFromIndex(io.LimitReader(resp.Body, maxIndexBytes))
	if err != nil {
		return nil, fmt.Errorf("crates.io index: %s: %w", pkg, err)
	}
	if version == "" {
		return nil, nil
	}
	return &Result{
		Manager: pm.Cargo,
		Name:    name,
		Info:    cratesDescription(ctx, name),
		Extra:   map[string]string{"version": version},
	}, nil
}

// latestFromIndex reads newline-delimited index entries and returns the
// crate's canonical name and its best version. Publish order is not version
// order (0.8.x patches ship after 0.9.0), so versions are compared, not
// taken from the last line.
func latestFromIndex(r io.Reader) (name, version string, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	var bestStable, bestAny string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e struct {
			Name   string `json:"name"`
			Vers   string `json:"vers"`
			Yanked bool   `json:"yanked"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return "", "", fmt.Errorf("bad index entry: %w", err)
		}
		if e.Name != "" {
			name = e.Name
		}
		if e.Yanked || e.Vers == "" {
			continue
		}
		if bestAny == "" || semverLess(bestAny, e.Vers) {
			bestAny = e.Vers
		}
		if !isPrerelease(e.Vers) && (bestStable == "" || semverLess(bestStable, e.Vers)) {
			bestStable = e.Vers
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if bestStable != "" {
		return name, bestStable, nil
	}
	return name, bestAny, nil
}

// cratesDescriptionBudget is how long the description call may take: at
// most cratesDescriptionTimeout, and never past ctx's deadline minus
// descriptionMargin.
func cratesDescriptionBudget(ctx context.Context) time.Duration {
	budget := cratesDescriptionTimeout
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl) - descriptionMargin; left < budget {
			budget = left
		}
	}
	return budget
}

// cratesDescription fetches the crate's description from the API, giving
// up silently when cratesDescriptionBudget runs out.
func cratesDescription(ctx context.Context, name string) string {
	budget := cratesDescriptionBudget(ctx)
	if budget <= 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	u := fmt.Sprintf("%s/crates/%s?include=default_version", cratesAPIURL, url.PathEscape(name))
	resp, err := httpGet(ctx, u)
	if err != nil {
		logx.Info("crates.io description for %s unavailable: %v", name, err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var data struct {
		Crate struct {
			Description string `json:"description"`
		} `json:"crate"`
	}
	if err := decodeJSON("crates.io", name, resp.Body, &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.Crate.Description)
}

// isPrerelease reports whether a semver string has a pre-release part.
func isPrerelease(v string) bool {
	core, _, _ := strings.Cut(v, "+")
	return strings.Contains(core, "-")
}

// semverLess reports a < b for MAJOR.MINOR.PATCH[-pre][+build] versions.
// Non-numeric core parts count as 0; prereleases of the same core compare
// lexically and sort before the release.
func semverLess(a, b string) bool {
	ac, ap := splitSemver(a)
	bc, bp := splitSemver(b)
	for i := range ac {
		if ac[i] != bc[i] {
			return ac[i] < bc[i]
		}
	}
	if ap == "" || bp == "" {
		return ap != "" && bp == ""
	}
	return ap < bp
}

func splitSemver(v string) (core [3]int, pre string) {
	v, _, _ = strings.Cut(v, "+")
	v, pre, _ = strings.Cut(v, "-")
	for i, p := range strings.SplitN(v, ".", 3) {
		core[i], _ = strconv.Atoi(p)
	}
	return core, pre
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race -count=2 ./internal/search/`
Expected: `ok`. `TestExistsInCratesUsesSparseIndex` picks `1.0.214` (it skips the yanked `1.0.215`, the prerelease `2.0.0-rc.1` and the late-published `0.9.16`). `TestExistsInCratesAPIStallStillFound` returns before its 500 ms deadline.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/search
git commit -m "search(crates): sparse index for existence and version; API description is best-effort"
```

---

### Task 3: One deadline-bounded fan-out with a `Report` of unavailable registries (roadmap #13, search side)

**Files:**
- Create: `internal/search/fanout.go`, `internal/search/report_test.go`
- Replace: `internal/search/parallel.go` (whole file)
- Modify:
  - `internal/search/search.go`: delete everything from `// lookupDeadline bounds the whole fan-out` to the end of the file; it moves to `fanout.go`. Then remove the now-unused `"errors"` import.
  - `internal/search/lookupcache.go`: add `cachedSearch`; rename `writeCacheEntry` to `writeJSONAtomic`.

**Interfaces:**
- Consumes (Task 1): `lookup`, `exactLookups`, `cachedLookup(ctx, dir, l, pkg)`, `lookupCacheDir`, `cachePath`, the `search*Multiple(ctx, q)` wrappers.
- Produces:
  - `type RegistryFailure struct { Manager pm.ID; Err error }` with `func (f RegistryFailure) TimedOut() bool`.
  - `type Report struct { Results []Result; Unavailable []RegistryFailure }` with `func (r Report) UnavailableIDs() []pm.ID`.
  - `type registryCall struct { id pm.ID; timeout time.Duration; fn func(ctx context.Context) ([]Result, error) }`
  - `func fanOut(calls []registryCall) Report`: waits for the longest call timeout.
  - `func runReport(calls []registryCall) (Report, error)`: the error wraps `ErrAllRegistriesFailed` when every call failed, and the `Report` is still filled.
  - `func SearchEverywhereReport(pkg string, opts Options) (Report, error)`
  - `func SearchEverywhere(pkg string, opts Options) ([]Result, error)`: now a wrapper.
  - `type multiLookup struct { id pm.ID; fn func(ctx context.Context, query string) ([]Result, error) }` and `var multiLookups`.
  - `func SearchReport(query string, opts Options) (Report, error)`
  - `func SearchEverywhereParallel(query string, opts Options) ([]Result, error)`: same signature. It now uses `lookupDeadline` (2.5 s) instead of 10 s, uses the disk cache, and errors when every registry failed.
  - `func cachedSearch(ctx context.Context, dir string, m multiLookup, query string) ([]Result, error)`
  - `var searchTTL = 15 * time.Minute`
- Removed (no callers): `SearchResult`, `SearchFunc`, `ParallelSearchConfig`, `DefaultParallelSearchConfig`, `SearchEverywhereParallelWithConfig`, `SearchRegistriesParallel`, `BatchSearch`.

- [ ] **Step 1: Write the failing tests**

Create `internal/search/report_test.go`:

```go
package search

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

func TestSearchEverywhereReportListsUnavailable(t *testing.T) {
	withDeadline(t, 150*time.Millisecond)
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, nil),
		fakeLookup(pm.Pip, 0, errors.New("pypi registry returned status 503")),
		fakeLookup(pm.Maven, 5*time.Second, nil),
	})
	rep, err := SearchEverywhereReport("x", Options{})
	if err != nil {
		t.Fatalf("partial failure must not be an error, got %v", err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Manager != pm.Npm {
		t.Fatalf("Results = %+v, want only npm", rep.Results)
	}
	ids := rep.UnavailableIDs()
	if len(ids) != 2 || ids[0] != pm.Pip || ids[1] != pm.Maven {
		t.Fatalf("Unavailable = %v, want [pip maven] in table order", ids)
	}
	if rep.Unavailable[0].TimedOut() {
		t.Error("pip answered with an error; it must not be reported as timed out")
	}
	if !rep.Unavailable[1].TimedOut() {
		t.Error("maven missed the deadline; it must be reported as timed out")
	}
}

func TestSearchEverywhereReportAllFailedStillReports(t *testing.T) {
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, errors.New("offline")),
		fakeLookup(pm.Pip, 0, errors.New("offline")),
	})
	rep, err := SearchEverywhereReport("x", Options{})
	if !errors.Is(err, ErrAllRegistriesFailed) {
		t.Fatalf("err = %v, want ErrAllRegistriesFailed", err)
	}
	if len(rep.Unavailable) != 2 {
		t.Fatalf("Unavailable = %+v, want both registries", rep.Unavailable)
	}
}

// withMultiLookups swaps the multi-result search table and disables the cache.
func withMultiLookups(t *testing.T, ms []multiLookup) {
	t.Helper()
	orig, origDir := multiLookups, lookupCacheDir
	multiLookups, lookupCacheDir = ms, ""
	t.Cleanup(func() { multiLookups, lookupCacheDir = orig, origDir })
}

func fakeMulti(id pm.ID, delay time.Duration, names ...string) multiLookup {
	return multiLookup{id: id, fn: func(ctx context.Context, q string) ([]Result, error) {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		var out []Result
		for _, n := range names {
			out = append(out, Result{Manager: id, Name: n})
		}
		return out, nil
	}}
}

func TestSearchReportUsesTheLookupDeadline(t *testing.T) {
	withDeadline(t, 150*time.Millisecond)
	withMultiLookups(t, []multiLookup{
		fakeMulti(pm.Npm, 0, "axios", "axios-retry"),
		fakeMulti(pm.Maven, 10*time.Second, "never"),
	})
	start := time.Now()
	rep, err := SearchReport("axios", Options{})
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("took %v; the TUI search must not wait 10 s for a stalled registry", d)
	}
	if err != nil || len(rep.Results) != 2 {
		t.Fatalf("got (%+v, %v)", rep, err)
	}
	if ids := rep.UnavailableIDs(); len(ids) != 1 || ids[0] != pm.Maven {
		t.Fatalf("Unavailable = %v, want [maven]", ids)
	}
}

func TestSearchReportAllFailedIsAnError(t *testing.T) {
	withDeadline(t, 50*time.Millisecond)
	withMultiLookups(t, []multiLookup{fakeMulti(pm.Npm, time.Second)})
	if _, err := SearchEverywhereParallel("x", Options{}); !errors.Is(err, ErrAllRegistriesFailed) {
		t.Fatalf("err = %v, want ErrAllRegistriesFailed", err)
	}
}

func TestCachedSearchServesRepeatsFromDisk(t *testing.T) {
	var calls int32
	m := multiLookup{id: pm.Npm, fn: func(context.Context, string) ([]Result, error) {
		atomic.AddInt32(&calls, 1)
		return []Result{{Manager: pm.Npm, Name: "axios"}}, nil
	}}
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		got, err := cachedSearch(context.Background(), dir, m, "axios")
		if err != nil || len(got) != 1 || got[0].Name != "axios" {
			t.Fatalf("got (%+v, %v)", got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("registry searched %d times, want 1", calls)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/search/ -run 'Report|CachedSearch' -v`
Expected: FAIL. This is a build error: `undefined: SearchEverywhereReport`, `undefined: multiLookup`, `undefined: cachedSearch`.

- [ ] **Step 3: Implement the fan-out**

Delete the tail of `search.go` as described under **Files**, then create `internal/search/fanout.go`:

```go
package search

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// lookupDeadline bounds the whole fan-out. Registry APIs have long tails
// (crates.io has been measured at 46s, Maven search stalls without ever
// answering), so a registry that misses the deadline is reported as timed
// out instead of holding up the answer.
var lookupDeadline = 2500 * time.Millisecond

var (
	// ErrAllRegistriesFailed is returned when every enabled registry errored
	// or timed out, so callers can tell "offline" from "no such package".
	ErrAllRegistriesFailed = errors.New("all registries failed")
	// ErrRegistryTimeout marks a registry that missed the deadline.
	ErrRegistryTimeout = errors.New("registry did not answer in time")
)

// RegistryFailure is a registry that errored or did not answer in time.
type RegistryFailure struct {
	Manager pm.ID
	Err     error
}

// TimedOut reports whether the registry missed its deadline (as opposed to
// answering with an error).
func (f RegistryFailure) TimedOut() bool {
	return errors.Is(f.Err, ErrRegistryTimeout) || errors.Is(f.Err, context.DeadlineExceeded)
}

// Report is the outcome of asking several registries: what they found, and
// which ones could not answer. "Not in Unavailable and not in Results"
// means the registry answered "no such package".
type Report struct {
	Results     []Result
	Unavailable []RegistryFailure
}

// UnavailableIDs returns the managers of r.Unavailable in table order.
func (r Report) UnavailableIDs() []pm.ID {
	ids := make([]pm.ID, 0, len(r.Unavailable))
	for _, f := range r.Unavailable {
		ids = append(ids, f.Manager)
	}
	return ids
}

// registryCall is one registry's part of a fan-out. fn must honour ctx,
// which expires after timeout.
type registryCall struct {
	id      pm.ID
	timeout time.Duration
	fn      func(ctx context.Context) ([]Result, error)
}

// fanOut runs calls concurrently and waits at most for the longest call
// timeout. Results keep call order. A call that has not answered by then is
// reported as ErrRegistryTimeout and its request is cancelled.
func fanOut(calls []registryCall) Report {
	var deadline time.Duration
	for _, c := range calls {
		if c.timeout > deadline {
			deadline = c.timeout
		}
	}
	type outcome struct {
		res []Result
		err error
	}
	type indexed struct {
		i int
		outcome
	}
	// Buffered so goroutines that finish after the deadline never block.
	ch := make(chan indexed, len(calls))
	ctxs := make([]context.CancelFunc, 0, len(calls))
	defer func() {
		for _, cancel := range ctxs {
			cancel()
		}
	}()
	for i, c := range calls {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		ctxs = append(ctxs, cancel)
		go func(i int, c registryCall, ctx context.Context) {
			res, err := c.fn(ctx)
			ch <- indexed{i, outcome{res, err}}
		}(i, c, ctx)
	}

	outcomes := make([]outcome, len(calls))
	for i := range outcomes {
		outcomes[i] = outcome{err: ErrRegistryTimeout}
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()
collect:
	for received := 0; received < len(calls); received++ {
		select {
		case o := <-ch:
			outcomes[o.i] = o.outcome
		case <-timer.C:
			break collect
		}
	}

	var rep Report
	for i, o := range outcomes {
		if o.err != nil {
			logx.Info("registry %s failed: %v", calls[i].id, o.err)
			rep.Unavailable = append(rep.Unavailable, RegistryFailure{Manager: calls[i].id, Err: o.err})
			continue
		}
		rep.Results = append(rep.Results, o.res...)
	}
	return rep
}

// runReport fans out calls and turns "every registry failed" into an error
// wrapping ErrAllRegistriesFailed. The Report is returned either way.
func runReport(calls []registryCall) (Report, error) {
	if len(calls) == 0 {
		return Report{}, nil
	}
	rep := fanOut(calls)
	if len(rep.Unavailable) == len(calls) {
		errs := make([]error, 0, len(rep.Unavailable))
		for _, f := range rep.Unavailable {
			errs = append(errs, fmt.Errorf("%s: %w", f.Manager, f.Err))
		}
		return rep, fmt.Errorf("%w: %w", ErrAllRegistriesFailed, errors.Join(errs...))
	}
	return rep, nil
}

// SearchEverywhereReport checks every enabled registry concurrently for an
// exact package name and returns within lookupDeadline, reporting which
// registries could not answer. If all of them fail the error wraps
// ErrAllRegistriesFailed.
func SearchEverywhereReport(pkg string, opts Options) (Report, error) {
	cacheDir := lookupCacheDir
	var calls []registryCall
	for _, l := range exactLookups {
		if !Enabled(opts, l.id) {
			continue
		}
		calls = append(calls, registryCall{id: l.id, timeout: lookupDeadline, fn: func(ctx context.Context) ([]Result, error) {
			res, err := cachedLookup(ctx, cacheDir, l, pkg)
			if err != nil || res == nil {
				return nil, err
			}
			return []Result{*res}, nil
		}})
	}
	return runReport(calls)
}

// SearchEverywhere is SearchEverywhereReport without the per-registry
// failures: results in table order, or an error if every registry failed.
func SearchEverywhere(pkg string, opts Options) ([]Result, error) {
	rep, err := SearchEverywhereReport(pkg, opts)
	if err != nil {
		return nil, err
	}
	return rep.Results, nil
}
```

Replace `internal/search/parallel.go` with:

```go
package search

import (
	"context"

	"github.com/crenspire/xpm/internal/pm"
)

// multiLookup is one registry's multi-result search. fn must honour ctx.
type multiLookup struct {
	id pm.ID
	fn func(ctx context.Context, query string) ([]Result, error)
}

// multiLookups is the registry table used by SearchReport (the TUI and
// `xpm search`), in result order. Tests replace it with fakes.
var multiLookups = []multiLookup{
	{id: pm.Npm, fn: searchNpmMultiple},
	{id: pm.Pip, fn: searchPipMultiple},
	{id: pm.Composer, fn: searchComposerMultiple},
	{id: pm.Cargo, fn: searchCargoMultiple},
	{id: pm.Maven, fn: searchMavenMultiple},
}

// SearchReport runs every enabled registry's search API concurrently, with
// the same deadline and disk cache as SearchEverywhereReport.
func SearchReport(query string, opts Options) (Report, error) {
	cacheDir := lookupCacheDir
	var calls []registryCall
	for _, m := range multiLookups {
		if !Enabled(opts, m.id) {
			continue
		}
		calls = append(calls, registryCall{id: m.id, timeout: lookupDeadline, fn: func(ctx context.Context) ([]Result, error) {
			return cachedSearch(ctx, cacheDir, m, query)
		}})
	}
	return runReport(calls)
}

// SearchEverywhereParallel returns SearchReport's results, or an error if
// every registry failed.
func SearchEverywhereParallel(query string, opts Options) ([]Result, error) {
	rep, err := SearchReport(query, opts)
	if err != nil {
		return nil, err
	}
	return rep.Results, nil
}

func searchNpmMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchNpmPackages(ctx, query, 20)
}

// searchPipMultiple uses the exact lookup: PyPI has no search API.
func searchPipMultiple(ctx context.Context, query string) ([]Result, error) {
	result, err := existsInPip(ctx, query)
	if err != nil || result == nil {
		return nil, err
	}
	return []Result{*result}, nil
}

func searchComposerMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchPackagistPackages(ctx, query, 20)
}

func searchCargoMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchCratesIO(ctx, query, 20)
}

func searchMavenMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchMavenCentral(ctx, query, 20)
}
```

In `internal/search/lookupcache.go`, replace the tail of `cachedLookup` and the whole `writeCacheEntry` function. The span runs from the line `writeCacheEntry(path, cacheEntry{Found: res != nil, Result: res, At: time.Now()})` to the end of the file. Use:

```go
	writeJSONAtomic(path, cacheEntry{Found: res != nil, Result: res, At: time.Now()})
	return res, nil
}

// searchTTL is how long multi-result searches (TUI, `xpm search`) are cached.
var searchTTL = 15 * time.Minute

type searchCacheEntry struct {
	Results []Result  `json:"results"`
	At      time.Time `json:"at"`
}

// cachedSearch is cachedLookup for multi-result searches. Entries live under
// <dir>/search/<id>/. Errors are never cached; an empty dir disables caching.
func cachedSearch(ctx context.Context, dir string, m multiLookup, query string) ([]Result, error) {
	if dir == "" {
		return m.fn(ctx, query)
	}
	path := cachePath(filepath.Join(dir, "search"), m.id, query)
	if data, err := os.ReadFile(path); err == nil {
		var e searchCacheEntry
		if json.Unmarshal(data, &e) == nil && !e.At.After(time.Now()) && time.Since(e.At) < searchTTL {
			return e.Results, nil
		}
	}
	res, err := m.fn(ctx, query)
	if err != nil {
		return nil, err
	}
	writeJSONAtomic(path, searchCacheEntry{Results: res, At: time.Now()})
	return res, nil
}

// writeJSONAtomic writes v as JSON atomically (temp file + rename) so
// concurrent xpm processes never read a half-written entry. Failures are
// ignored: the cache is an optimisation only.
func writeJSONAtomic(path string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(path), "*.tmp")
	if err != nil {
		return
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(f.Name())
		return
	}
	if err := os.Rename(f.Name(), path); err != nil {
		_ = os.Remove(f.Name())
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race -count=3 ./internal/search/`
Expected: `ok`. `TestSearchReportUsesTheLookupDeadline` finishes in about 150 ms, where it used to take 10 s.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green. The CLI and TUI still compile, because `SearchEverywhere`/`SearchEverywhereParallel` keep their signatures. `0 issues.`

```bash
git add internal/search
git commit -m "search: shared fan-out returns a Report with unavailable registries; TUI search gets the 2.5s deadline and disk cache"
```

---

### Task 4: Per-registry timeouts and `search.OptionsFromConfig` (roadmap #11, #12)

**Files:**
- Create: `internal/search/options.go`, `internal/search/options_test.go`
- Modify:
  - `internal/search/search.go`: the `Options` struct and `httpClient`.
  - `internal/search/fanout.go` and `internal/search/parallel.go`: one line each.
  - `internal/search/http.go`: delete `SetTimeoutForRegistry`.
  - `internal/config/config.go`: `TimeoutConfig` doc and default.
  - `internal/config/config_test.go`: one assertion.
  - `internal/cli/cli.go` (`cmdInstall`, `cmdWhich`), `internal/cli/commands.go` (`cmdInfo`), `internal/cli/search_cmd.go` (`cmdSearch`, `cmdSearchNonInteractive`, `installFromSearchResult`).

**Interfaces:**
- Consumes (Task 3): `registryCall.timeout`, `fanOut` (it waits for the longest timeout).
- Produces:
  - `Options` gains `Timeout time.Duration` and `RegistryTimeout map[pm.ID]time.Duration`.
  - `func (o Options) timeoutFor(id pm.ID) time.Duration`: per-registry, else `Timeout`, else `lookupDeadline`.
  - `var Registries = []pm.ID{pm.Npm, pm.Pip, pm.Composer, pm.Cargo, pm.Maven}`
  - `func OptionsFromConfig(c config.Config) Options`. `timeout.perRegistry` keys accept `npm`, `pypi`/`pip`, `packagist`/`composer`, `crates`/`cargo` and `maven`, case-insensitively. Unknown keys and values ≤ 0 are ignored.
- Config semantics change: `timeout.default` now defaults to **0**, which means the built-in 2.5 s deadline. Old default: 4. A stored value of exactly 4 (written by older `xpm config set`) is also treated as "built-in" — add `TestOptionsFromConfigTreatsLegacy4AsDefault` asserting `OptionsFromConfig(config.Config{Timeout: config.TimeoutConfig{Default: 4}}).Timeout == 0`, and keep a test that `Default: 3` and `Default: 6` are honoured. A configured value can lengthen as well as shorten the wait (the fan-out waits for the longest per-registry timeout).

> Known gotcha: `xpm config set` writes the whole config, so users who ran it before this change have `"timeout": {"default": 4}` in their file. Their wait ceiling becomes 4 s instead of 2.5 s. This is documented in the README (Task 16) with the fix: `"default": 0`.

- [ ] **Step 1: Write the failing tests**

Create `internal/search/options_test.go`:

```go
package search

import (
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
)

func TestOptionsFromConfig(t *testing.T) {
	var c config.Config
	c.Search = map[string]bool{"maven": false, "gomod": true}
	c.Timeout.Default = 3
	c.Timeout.PerRegistry = map[string]int{"crates": 1, "PyPI": 5, "bogus": 9, "npm": 0}

	o := OptionsFromConfig(c)
	if Enabled(o, pm.Maven) || !Enabled(o, pm.Npm) || !Enabled(o, pm.Cargo) {
		t.Fatalf("Enable = %v, want maven off and the rest on", o.Enable)
	}
	for id, want := range map[pm.ID]time.Duration{
		pm.Cargo: time.Second, pm.Pip: 5 * time.Second, pm.Npm: 3 * time.Second, pm.Maven: 3 * time.Second,
	} {
		if got := o.timeoutFor(id); got != want {
			t.Errorf("timeoutFor(%s) = %v, want %v", id, got, want)
		}
	}
}

func TestOptionsFromEmptyConfigUseTheBuiltInDeadline(t *testing.T) {
	o := OptionsFromConfig(config.Config{})
	if got := o.timeoutFor(pm.Npm); got != lookupDeadline {
		t.Fatalf("timeoutFor = %v, want lookupDeadline %v", got, lookupDeadline)
	}
	for _, id := range Registries {
		if !Enabled(o, id) {
			t.Errorf("%s disabled by an empty config", id)
		}
	}
}

func TestPerRegistryTimeoutCutsOnlyThatRegistry(t *testing.T) {
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 150*time.Millisecond, nil),
		fakeLookup(pm.Maven, 5*time.Second, nil),
	})
	opts := Options{Timeout: 400 * time.Millisecond, RegistryTimeout: map[pm.ID]time.Duration{pm.Maven: 50 * time.Millisecond}}
	start := time.Now()
	rep, err := SearchEverywhereReport("x", opts)
	if d := time.Since(start); d > 350*time.Millisecond {
		t.Fatalf("took %v; npm answered at 150 ms and maven was cut at 50 ms", d)
	}
	if err != nil || len(rep.Results) != 1 || rep.Results[0].Manager != pm.Npm {
		t.Fatalf("got (%+v, %v), want the npm result", rep, err)
	}
	if len(rep.Unavailable) != 1 || !rep.Unavailable[0].TimedOut() {
		t.Fatalf("Unavailable = %+v, want maven timed out", rep.Unavailable)
	}
}

func TestConfiguredTimeoutCanExceedTheDefaultDeadline(t *testing.T) {
	withDeadline(t, 50*time.Millisecond)
	withLookups(t, []lookup{fakeLookup(pm.Npm, 150*time.Millisecond, nil)})
	rep, err := SearchEverywhereReport("x", Options{Timeout: 500 * time.Millisecond})
	if err != nil || len(rep.Results) != 1 {
		t.Fatalf("a slow-network user who raised timeout.default must get the result: (%+v, %v)", rep, err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/search/ -run 'Options|Timeout' -v`
Expected: FAIL. This is a build error: `undefined: OptionsFromConfig`, `o.timeoutFor undefined`, `unknown field Timeout in struct literal of type Options`.

- [ ] **Step 3: Implement**

Create `internal/search/options.go`:

```go
package search

import (
	"strings"
	"time"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
)

// Registries are the registries xpm searches, in result order.
var Registries = []pm.ID{pm.Npm, pm.Pip, pm.Composer, pm.Cargo, pm.Maven}

// registryAliases maps timeout.perRegistry keys to registries. Both the
// registry names documented in config.TimeoutConfig and manager IDs work.
var registryAliases = map[string]pm.ID{
	"npm": pm.Npm, "pypi": pm.Pip, "pip": pm.Pip,
	"packagist": pm.Composer, "composer": pm.Composer,
	"crates": pm.Cargo, "cargo": pm.Cargo, "maven": pm.Maven,
}

// OptionsFromConfig builds search options from the user's config: which
// registries are enabled (search.<id>, default true) and how long each may
// take (timeout.default / timeout.perRegistry, in seconds; 0 or absent means
// the built-in 2.5 s deadline).
// legacyDefaultTimeoutSecs is the pre-P3 timeout.default value.
const legacyDefaultTimeoutSecs = 4

func OptionsFromConfig(c config.Config) Options {
	opts := Options{Enable: make(map[pm.ID]bool, len(Registries))}
	for _, id := range Registries {
		enabled := true
		if v, ok := c.Search[string(id)]; ok {
			enabled = v
		}
		opts.Enable[id] = enabled
	}
	// 4 was the old built-in default that `xpm config set` wrote into users'
	// files; treat it as "use the built-in deadline" so those configs don't
	// silently slow every lookup to 4 s (user decision 2026-10-07).
	if c.Timeout.Default > 0 && c.Timeout.Default != legacyDefaultTimeoutSecs {
		opts.Timeout = time.Duration(c.Timeout.Default) * time.Second
	}
	for key, secs := range c.Timeout.PerRegistry {
		id, ok := registryAliases[strings.ToLower(key)]
		if !ok || secs <= 0 {
			continue
		}
		if opts.RegistryTimeout == nil {
			opts.RegistryTimeout = make(map[pm.ID]time.Duration)
		}
		opts.RegistryTimeout[id] = time.Duration(secs) * time.Second
	}
	return opts
}

// timeoutFor is how long registry id may take: its own timeout, else the
// default timeout, else lookupDeadline.
func (o Options) timeoutFor(id pm.ID) time.Duration {
	if d := o.RegistryTimeout[id]; d > 0 {
		return d
	}
	if o.Timeout > 0 {
		return o.Timeout
	}
	return lookupDeadline
}
```

In `internal/search/search.go`, replace the `Options` struct and the `httpClient` var with:

```go
// Options controls search behavior. Build it with OptionsFromConfig.
type Options struct {
	// Enable specifies which package managers to include in the search.
	// If nil or a key is missing, that ecosystem is searched by default.
	Enable map[pm.ID]bool
	// Timeout bounds each registry and therefore the whole fan-out;
	// 0 means lookupDeadline (2.5 s).
	Timeout time.Duration
	// RegistryTimeout overrides Timeout for individual registries.
	RegistryTimeout map[pm.ID]time.Duration
}
```

```go
// httpClient is the shared HTTP client for all registry queries. Every
// request carries a context deadline (Options.timeoutFor); the client
// timeout is only a backstop for callers that pass context.Background().
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}
```

Keep `const DefaultTimeout = 4 * time.Second` (`TestDefaultTimeout` asserts it).

In `fanout.go` (`SearchEverywhereReport`), change `registryCall{id: l.id, timeout: lookupDeadline,` to `registryCall{id: l.id, timeout: opts.timeoutFor(l.id),`. In `parallel.go` (`SearchReport`), change `registryCall{id: m.id, timeout: lookupDeadline,` to `registryCall{id: m.id, timeout: opts.timeoutFor(m.id),`.

In `internal/search/http.go`, delete the function `SetTimeoutForRegistry` (with its doc comment). It had no callers, and despite its name it changed the global timeout.

In `internal/config/config.go`, replace the `TimeoutConfig` type with:

```go
// TimeoutConfig holds how long registry lookups may take.
type TimeoutConfig struct {
	// Default is the timeout for every registry, in seconds.
	// 0 means the built-in 2.5 s deadline.
	Default int `json:"default"`

	// PerRegistry overrides Default per registry, in seconds.
	// Keys: "npm", "pypi" (or "pip"), "packagist" (or "composer"),
	// "crates" (or "cargo"), "maven".
	PerRegistry map[string]int `json:"perRegistry"`
}
```

and in `defaultConfig()` change `Default:     4, // 4 seconds default` to `Default:     0, // 0 = built-in 2.5 s deadline`.

In `internal/config/config_test.go` (`TestLoadFromPartialConfigKeepsDefaults`), change `c.Timeout.Default != 4` to `c.Timeout.Default != 0`.

- [ ] **Step 4: Collapse the five option builders**

The following 13-line block appears in `cmdInstall` and `cmdWhich` (`cli.go`), `cmdInfo` (`commands.go`), and `cmdSearch` and `cmdSearchNonInteractive` (`search_cmd.go`):

```go
	searchOpts := search.Options{
		Enable: make(map[pm.ID]bool),
	}
	for id := range map[pm.ID]struct{}{
		pm.Npm: {}, pm.Pip: {}, pm.Composer: {}, pm.Cargo: {}, pm.Maven: {},
	} {
		name := string(id)
		enabled := true
		if v, ok := cfg.Search[name]; ok {
			enabled = v
		}
		searchOpts.Enable[id] = enabled
	}
```

Replace each occurrence with:

```go
	searchOpts := search.OptionsFromConfig(cfg)
```

`search_cmd.go` shadows the global `cfg` with `cfg := config.Load()` three times (in `cmdSearch`, `cmdSearchNonInteractive` and `installFromSearchResult`). Delete those three lines, plus the `// Build search options` comment, so the global config is used. Task 11 makes the global config TTY-aware. Then remove the now-unused `"github.com/crenspire/xpm/internal/config"` import from `search_cmd.go`.

Check: `grep -n "for id := range map\[pm.ID\]struct" internal/cli/*.go` prints nothing.

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/search/ ./internal/config/ ./internal/cli/`
Expected: three `ok` lines.

- [ ] **Step 6: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/search internal/config internal/cli
git commit -m "search: per-registry timeouts from config; OptionsFromConfig replaces five copies of the options builder"
```

---
### Task 5: `which`/`info`/`install`/`search` report unavailable registries; "no matches" exits 1 (roadmap #13 CLI side, #8 exit codes)

**Files:**
- Create: `internal/cli/lookup.go`, `internal/cli/helpers_test.go`, `internal/cli/lookup_test.go`
- Modify: `internal/cli/cli.go` (`cmdWhich` replaced; the search step of `cmdInstall`), `internal/cli/commands.go` (`cmdInfo`), `internal/cli/search_cmd.go` (`cmdSearch`, `cmdSearchNonInteractive`)

**Interfaces:**
- Consumes (Tasks 3–4): `search.Report`, `search.RegistryFailure.TimedOut()`, `search.SearchEverywhereReport`, `search.SearchReport`, `search.Registries`, `search.Enabled`, `search.OptionsFromConfig`, `search.ErrRegistryTimeout`, `search.ErrAllRegistriesFailed`.
- Produces (package `cli`):
  - Seams: `var lookupReport = search.SearchEverywhereReport`, `var searchReport = search.SearchReport`, `var stdoutIsTerminal func() bool`.
  - `type registryStatus struct { NotFound []pm.ID; Unavailable []search.RegistryFailure }`
  - `func classify(rep search.Report, opts search.Options) registryStatus`: only registries in `search.Registries` that are enabled count. Today's code lists yarn/pnpm/bun/poetry/pipenv/gomod/gradle as "Not found in" although they are never searched.
  - `func managerName(id pm.ID) string` returns e.g. `"pip (Python)"`.
  - `func describeFailure(f search.RegistryFailure) string` returns `"timed out"` or the error text.
  - `func formatAvailability(st registryStatus) string`
  - Test helpers (`helpers_test.go`): `captureStdout(t, fn) string`, `withConfig(t, config.Config)`, `withLookupReport(t, rep, err)`, `withSearchReport(t, rep, err)`.
- Behaviour:
  - `which`, `info`, `install <pkg>` and non-TUI `search` exit **1** when nothing matched (today: 0). They still exit 1 when every registry failed.
  - When some registries could not answer, a separate section is printed:

    ```
    Unavailable (results may be incomplete):
    - maven (Java): timed out
    ```

  - `xpm search` without a terminal on stdout now prints plain results through `cmdSearchNonInteractive`, so its exit code is honest.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/helpers_test.go`:

```go
package cli

import (
	"io"
	"os"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/search"
)

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	defer func() {
		os.Stdout = old
	}()
	fn()
	w.Close()
	return <-out
}

// withConfig replaces the package-level cfg for one test.
func withConfig(t *testing.T, c config.Config) {
	t.Helper()
	old := cfg
	cfg = c
	t.Cleanup(func() { cfg = old })
}

// withLookupReport fakes the exact-name registry fan-out.
func withLookupReport(t *testing.T, rep search.Report, err error) {
	t.Helper()
	old := lookupReport
	lookupReport = func(string, search.Options) (search.Report, error) { return rep, err }
	t.Cleanup(func() { lookupReport = old })
}

// withSearchReport fakes the multi-result registry fan-out.
func withSearchReport(t *testing.T, rep search.Report, err error) {
	t.Helper()
	old := searchReport
	searchReport = func(string, search.Options) (search.Report, error) { return rep, err }
	t.Cleanup(func() { searchReport = old })
}
```

Create `internal/cli/lookup_test.go`:

```go
package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

var timedOut = search.RegistryFailure{Manager: pm.Maven, Err: fmt.Errorf("x: %w", search.ErrRegistryTimeout)}

func TestClassifySeparatesNotFoundFromUnavailable(t *testing.T) {
	rep := search.Report{
		Results:     []search.Result{{Manager: pm.Npm, Name: "axios"}},
		Unavailable: []search.RegistryFailure{timedOut},
	}
	opts := search.Options{Enable: map[pm.ID]bool{pm.Composer: false}}
	st := classify(rep, opts)
	if fmt.Sprint(st.NotFound) != "[pip cargo]" {
		t.Fatalf("NotFound = %v, want [pip cargo] (npm found, composer disabled, maven unavailable)", st.NotFound)
	}
	if len(st.Unavailable) != 1 || st.Unavailable[0].Manager != pm.Maven {
		t.Fatalf("Unavailable = %+v", st.Unavailable)
	}
}

func TestWhichReportsUnavailableRegistriesSeparately(t *testing.T) {
	withConfig(t, config.Config{})
	withLookupReport(t, search.Report{
		Results: []search.Result{{Manager: pm.Npm, Name: "axios", Extra: map[string]string{"version": "1.7.9"}}},
		Unavailable: []search.RegistryFailure{
			{Manager: pm.Pip, Err: errors.New("pypi registry returned status 503")},
			timedOut,
		},
	}, nil)
	var code int
	out := captureStdout(t, func() { code = cmdWhich([]string{"axios"}) })
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	notFound := out[strings.Index(out, "Not found in:"):strings.Index(out, "Unavailable")]
	if strings.Contains(notFound, "maven") || strings.Contains(notFound, "pip") {
		t.Fatalf("unavailable registries listed as not found:\n%s", out)
	}
	for _, want := range []string{
		"- npm: axios @1.7.9",
		"- composer (PHP)\n- cargo (Rust)",
		"Unavailable (results may be incomplete):\n- pip (Python): pypi registry returned status 503\n- maven (Java): timed out",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestNoMatchExitsOne(t *testing.T) {
	withConfig(t, config.Config{})
	withLookupReport(t, search.Report{Unavailable: []search.RegistryFailure{timedOut}}, nil)
	withSearchReport(t, search.Report{}, nil)
	old := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdoutIsTerminal = old })

	for name, run := range map[string]func() int{
		"which":   func() int { return cmdWhich([]string{"nope"}) },
		"info":    func() int { return cmdInfo([]string{"nope"}) },
		"install": func() int { return cmdInstall([]string{"nope"}) },
		"search":  func() int { return cmdSearch([]string{"nope"}) },
	} {
		var code int
		out := captureStdout(t, func() { code = run() })
		if code != 1 {
			t.Errorf("%s: exit %d, want 1 for no matches\n%s", name, code, out)
		}
	}
}

func TestAllRegistriesFailedExitsOne(t *testing.T) {
	withConfig(t, config.Config{})
	withLookupReport(t, search.Report{}, fmt.Errorf("%w: offline", search.ErrAllRegistriesFailed))
	var code int
	captureStdout(t, func() { code = cmdWhich([]string{"axios"}) })
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ -run 'Classify|Which|NoMatch|AllRegistries' -v`
Expected: FAIL. This is a build error: `undefined: lookupReport`, `undefined: classify`, `undefined: stdoutIsTerminal`.

- [ ] **Step 3: Implement the classification and output**

Create `internal/cli/lookup.go`:

```go
package cli

import (
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// Seams for tests: the registry fan-outs and the terminal check.
var (
	lookupReport     = search.SearchEverywhereReport
	searchReport     = search.SearchReport
	stdoutIsTerminal = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
)

// registryStatus says, for the registries that did not return a result,
// which answered "not here" and which could not answer at all.
type registryStatus struct {
	NotFound    []pm.ID
	Unavailable []search.RegistryFailure
}

// classify sorts the enabled registries without a result into NotFound and
// Unavailable, in search.Registries order.
func classify(rep search.Report, opts search.Options) registryStatus {
	found := map[pm.ID]bool{}
	for _, r := range rep.Results {
		found[r.Manager] = true
	}
	failed := map[pm.ID]bool{}
	for _, f := range rep.Unavailable {
		failed[f.Manager] = true
	}
	st := registryStatus{Unavailable: rep.Unavailable}
	for _, id := range search.Registries {
		if search.Enabled(opts, id) && !found[id] && !failed[id] {
			st.NotFound = append(st.NotFound, id)
		}
	}
	return st
}

// managerName is the display name of a manager ("pip (Python)").
func managerName(id pm.ID) string {
	if meta, ok := pm.MetaFor(id); ok {
		return meta.Name
	}
	return string(id)
}

// describeFailure is the one-word-ish reason a registry is unavailable.
func describeFailure(f search.RegistryFailure) string {
	if f.TimedOut() {
		return "timed out"
	}
	return f.Err.Error()
}

// formatAvailability renders the "Not found in" and "Unavailable" sections
// (empty when there is nothing to say).
func formatAvailability(st registryStatus) string {
	var b strings.Builder
	if len(st.NotFound) > 0 {
		b.WriteString("\nNot found in:\n")
		for _, id := range st.NotFound {
			b.WriteString("- " + managerName(id) + "\n")
		}
	}
	if len(st.Unavailable) > 0 {
		b.WriteString("\nUnavailable (results may be incomplete):\n")
		for _, f := range st.Unavailable {
			b.WriteString("- " + managerName(f.Manager) + ": " + describeFailure(f) + "\n")
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Use it in `which`**

In `internal/cli/cli.go`, replace the whole `cmdWhich` function with the version below. Results print in registry order instead of alphabetical, and the Maven hint mentions Gradle.

```go
func cmdWhich(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: missing package name")
		fmt.Fprintln(os.Stderr)
		showCommandUsage("which")
		return 1
	}
	pkg := args[0]

	fmt.Printf("Searching for %q...\n\n", pkg)

	searchOpts := search.OptionsFromConfig(cfg)
	rep, err := lookupReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	st := classify(rep, searchOpts)
	if len(rep.Results) == 0 {
		fmt.Printf("No matches found for %s.\n", pkg)
		fmt.Print(formatAvailability(st))
		return 1
	}

	fmt.Println("Found in:")
	for _, r := range rep.Results {
		line := fmt.Sprintf("- %s: %s", r.Manager, r.Name)
		if v := r.Extra["version"]; v != "" {
			line += " @" + v
		}
		if r.Info != "" {
			line += " - " + r.Info
		}
		fmt.Println(line)

		switch r.Manager {
		case pm.Npm:
			fmt.Println("  → Also available via: yarn, pnpm, bun")
		case pm.Pip:
			fmt.Println("  → Also available via: poetry, pipenv")
		case pm.Maven:
			fmt.Println("  → Add to pom.xml or build.gradle as a dependency.")
		case pm.Cargo:
			fmt.Println("  → Add to Cargo.toml under [dependencies].")
		case pm.Composer:
			fmt.Println("  → Add to composer.json or run `composer require ...`.")
		}
	}
	fmt.Print(formatAvailability(st))
	return 0
}
```

- [ ] **Step 5: Use it in `install`, `info` and `search`**

In `cmdInstall` (`cli.go`), replace

```go
	results, err := search.SearchEverywhere(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	if len(results) == 0 {
		fmt.Println("No matches found for", pkg)
		return 0
	}
```

with

```go
	rep, err := lookupReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	results := rep.Results
	if len(results) == 0 {
		fmt.Println("No matches found for", pkg)
		fmt.Print(formatAvailability(classify(rep, searchOpts)))
		return 1
	}
	if st := classify(rep, searchOpts); len(st.Unavailable) > 0 {
		fmt.Print(formatAvailability(registryStatus{Unavailable: st.Unavailable}))
		fmt.Println()
	}
```

and delete the second, now-unreachable block right after `filterResultsByLockFiles`:

```go
	if len(results) == 0 {
		fmt.Println("No matches found for", pkg)
		return 0
	}
```

In `cmdInfo` (`commands.go`), replace

```go
	results, err := search.SearchEverywhere(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}

	if len(results) == 0 {
		fmt.Printf("No package found matching %q\n", pkg)
		return 0
	}
```

with

```go
	rep, err := lookupReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	results := rep.Results
	st := classify(rep, searchOpts)
	if len(results) == 0 {
		fmt.Printf("No package found matching %q\n", pkg)
		fmt.Print(formatAvailability(st))
		return 1
	}
```

and insert `fmt.Print(formatAvailability(registryStatus{Unavailable: st.Unavailable}))` just before the final `fmt.Println()` / `return 0` of `cmdInfo`.

In `internal/cli/search_cmd.go`:
- In `cmdSearch`, replace

```go
	// Check if TUI is enabled
	if !cfg.SearchUI.Enabled {
		// Fallback to non-interactive search
		return cmdSearchNonInteractive(args)
	}
```

with

```go
	// The TUI needs a terminal; pipes and CI get plain output.
	if !cfg.SearchUI.Enabled || !stdoutIsTerminal() {
		return cmdSearchNonInteractive(args)
	}
```

- In `cmdSearchNonInteractive`, replace

```go
	results, err := search.SearchEverywhereParallel(pkg, searchOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return 0
	}
```

with

```go
	rep, err := searchReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	results := rep.Results
	st := classify(rep, searchOpts)
	if len(results) == 0 {
		fmt.Println("No results found.")
		fmt.Print(formatAvailability(st))
		return 1
	}
```

and replace the blank line before its final `return 0` with `fmt.Print(formatAvailability(registryStatus{Unavailable: st.Unavailable}))`.

- [ ] **Step 6: Run the tests**

Run: `go test -race ./internal/cli/`
Expected: `ok`. `TestWhichReportsUnavailableRegistriesSeparately` passes, with pip (503) and maven (timed out) under "Unavailable", and composer and cargo under "Not found in".

Manual check (needs network): `go run ./cmd/xpm which zzzz-no-such-pkg-xpm; echo "exit=$?"`
Expected:

```
Searching for "zzzz-no-such-pkg-xpm"...

No matches found for zzzz-no-such-pkg-xpm.

Not found in:
- npm (Node.js)
- pip (Python)
- composer (PHP)
- cargo (Rust)
- maven (Java)
exit=1
```

- [ ] **Step 7: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/cli
git commit -m "cli: report unavailable registries separately from not-found; no matches exits 1"
```

---

### Task 6: Deterministic project detection — lockfiles (incl. `bun.lock`, `Pipfile`) and Java build files (roadmap #1 determinism, #5, #6)

**Files:**
- Replace: `internal/pm/lockfiles.go` (whole file)
- Create: `internal/pm/lockfiles_test.go`

**Interfaces:**
- Produces (package `pm`):
  - `const EcosystemJava Ecosystem = "java"`. `EcosystemForManager(Maven|Gradle)` returns it, and `ManagersInEcosystem(EcosystemJava)` returns `[maven gradle]`.
  - `type ProjectFile struct { Name string; Ecosystem Ecosystem; Manager ID }`
  - `func ProjectManagers(dir string) map[Ecosystem][]ProjectFile`: covers lockfiles and Java build files (`pom.xml`, `build.gradle`, `build.gradle.kts`). Results are in table order, with one entry per manager.
  - `func DetectLockFiles(dir string) map[Ecosystem][]ID`: same signature. Node and Python only, because `internal/workspace/install.go` (lane C) depends on that. It is now deterministic and deduplicated, and adds `bun.lock`, `Pipfile` → pipenv.
  - `DetectLockFilesForEcosystem`, `HasLockFile`, `GetLockFileName`: same signatures.
- Removed: the exported map `LockFileMapping` (no users outside this file). Map iteration order made detection random.

- [ ] **Step 1: Write the failing tests**

Create `internal/pm/lockfiles_test.go`:

```go
package pm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectLockFilesIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "bun.lock", "yarn.lock", "package-lock.json", "poetry.lock", "requirements.txt")
	for i := 0; i < 50; i++ { // map iteration order would vary across calls
		got := DetectLockFiles(dir)
		if fmt.Sprint(got[EcosystemNode]) != "[npm yarn bun]" || fmt.Sprint(got[EcosystemPython]) != "[poetry pip]" {
			t.Fatalf("call %d: %v", i, got)
		}
	}
}

func TestBunLockFormatsCountOnce(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "bun.lock", "bun.lockb")
	if got := DetectLockFiles(dir)[EcosystemNode]; fmt.Sprint(got) != "[bun]" {
		t.Fatalf("got %v, want [bun]", got)
	}
}

func TestPipfileWithoutLockImpliesPipenv(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "Pipfile")
	if got := DetectLockFiles(dir)[EcosystemPython]; fmt.Sprint(got) != "[pipenv]" {
		t.Fatalf("got %v, want [pipenv]", got)
	}
}

func TestProjectManagersIncludesJavaBuildFiles(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "build.gradle.kts", "pnpm-lock.yaml")
	got := ProjectManagers(dir)
	java := got[EcosystemJava]
	if len(java) != 1 || java[0].Manager != Gradle || java[0].Name != "build.gradle.kts" {
		t.Fatalf("java = %+v, want gradle via build.gradle.kts", java)
	}
	if node := got[EcosystemNode]; len(node) != 1 || node[0].Manager != Pnpm || node[0].Name != "pnpm-lock.yaml" {
		t.Fatalf("node = %+v", node)
	}
	if _, ok := DetectLockFiles(dir)[EcosystemJava]; ok {
		t.Fatal("DetectLockFiles must stay node/python only (workspace installs rely on it)")
	}
}

func TestJavaEcosystem(t *testing.T) {
	if EcosystemForManager(Gradle) != EcosystemJava || EcosystemForManager(Maven) != EcosystemJava {
		t.Fatal("maven and gradle belong to the java ecosystem")
	}
	if fmt.Sprint(ManagersInEcosystem(EcosystemJava)) != "[maven gradle]" {
		t.Fatal(ManagersInEcosystem(EcosystemJava))
	}
}

func TestDetectLockFilesRefusesTraversal(t *testing.T) {
	if got := DetectLockFiles("../.."); len(got) != 0 {
		t.Fatalf("got %v for a path with ..", got)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/pm/ -run 'Lock|Bun|Pipfile|ProjectManagers|Java' -v`
Expected: FAIL. This is a build error: `undefined: ProjectManagers`, `undefined: EcosystemJava`.

- [ ] **Step 3: Implement**

Replace `internal/pm/lockfiles.go` with:

```go
package pm

import (
	"os"
	"path/filepath"
	"strings"
)

// Ecosystem represents a group of package managers that share a registry.
type Ecosystem string

// Ecosystem identifiers.
const (
	EcosystemNode   Ecosystem = "node"
	EcosystemPython Ecosystem = "python"
	EcosystemJava   Ecosystem = "java"
)

// ProjectFile ties a file in a project directory to the package manager it
// implies.
type ProjectFile struct {
	Name      string
	Ecosystem Ecosystem
	Manager   ID
}

// lockFiles are checked in this order, which is also the order results are
// returned in: detection is deterministic.
var lockFiles = []ProjectFile{
	{"package-lock.json", EcosystemNode, Npm},
	{"yarn.lock", EcosystemNode, Yarn},
	{"pnpm-lock.yaml", EcosystemNode, Pnpm},
	{"bun.lock", EcosystemNode, Bun},  // bun >= 1.2 (text)
	{"bun.lockb", EcosystemNode, Bun}, // bun < 1.2 (binary)
	{"poetry.lock", EcosystemPython, Poetry},
	{"Pipfile.lock", EcosystemPython, Pipenv},
	{"Pipfile", EcosystemPython, Pipenv},
	{"requirements.txt", EcosystemPython, Pip}, // not a lock file, but implies pip
}

// buildFiles name a Java project's build tool. They lock nothing, but they
// narrow Maven-vs-Gradle the way lock files narrow npm-vs-yarn.
var buildFiles = []ProjectFile{
	{"pom.xml", EcosystemJava, Maven},
	{"build.gradle", EcosystemJava, Gradle},
	{"build.gradle.kts", EcosystemJava, Gradle},
}

// EcosystemForManager returns the ecosystem that a package manager belongs to.
func EcosystemForManager(id ID) Ecosystem {
	switch id {
	case Npm, Yarn, Pnpm, Bun:
		return EcosystemNode
	case Pip, Poetry, Pipenv:
		return EcosystemPython
	case Maven, Gradle:
		return EcosystemJava
	default:
		return ""
	}
}

// ManagersInEcosystem returns all package managers in a given ecosystem.
func ManagersInEcosystem(eco Ecosystem) []ID {
	switch eco {
	case EcosystemNode:
		return []ID{Npm, Yarn, Pnpm, Bun}
	case EcosystemPython:
		return []ID{Pip, Poetry, Pipenv}
	case EcosystemJava:
		return []ID{Maven, Gradle}
	default:
		return nil
	}
}

// safeDir cleans dir and makes it absolute. It reports false for paths
// containing "..", which are refused.
func safeDir(dir string) (string, bool) {
	clean := filepath.Clean(dir)
	if strings.Contains(clean, "..") {
		return "", false
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return clean, true
	}
	return abs, true
}

// present returns the entries of files that exist in dir, in table order,
// keeping only the first file per manager.
func present(dir string, files []ProjectFile) []ProjectFile {
	abs, ok := safeDir(dir)
	if !ok {
		return nil
	}
	var out []ProjectFile
	seen := map[ID]bool{}
	for _, f := range files {
		if seen[f.Manager] {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, f.Name)); err == nil {
			out = append(out, f)
			seen[f.Manager] = true
		}
	}
	return out
}

// ProjectManagers returns, per ecosystem, the managers that dir's lock files
// and Java build files point at, in table order, with the file that implied
// each. Ecosystems with no such files are absent.
func ProjectManagers(dir string) map[Ecosystem][]ProjectFile {
	out := map[Ecosystem][]ProjectFile{}
	for _, f := range append(present(dir, lockFiles), present(dir, buildFiles)...) {
		out[f.Ecosystem] = append(out[f.Ecosystem], f)
	}
	return out
}

// DetectLockFiles returns the managers implied by lock files in dir, grouped
// by ecosystem (node and python only), in a fixed order. Paths containing
// ".." return an empty map.
func DetectLockFiles(dir string) map[Ecosystem][]ID {
	result := make(map[Ecosystem][]ID)
	for _, f := range present(dir, lockFiles) {
		result[f.Ecosystem] = append(result[f.Ecosystem], f.Manager)
	}
	return result
}

// DetectLockFilesForEcosystem returns the managers with lock files in the
// given ecosystem, or nil.
func DetectLockFilesForEcosystem(dir string, eco Ecosystem) []ID {
	return DetectLockFiles(dir)[eco]
}

// HasLockFile checks if any lock file exists for the given ecosystem in the directory.
func HasLockFile(dir string, eco Ecosystem) bool {
	return len(DetectLockFilesForEcosystem(dir, eco)) > 0
}

// GetLockFileName returns the first lock file name for a package manager.
func GetLockFileName(id ID) string {
	for _, f := range lockFiles {
		if f.Manager == id {
			return f.Name
		}
	}
	return ""
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race -count=3 ./internal/pm/ ./internal/cli/ ./internal/workspace/`
Expected: `ok` for pm and cli; `[no test files]` for workspace.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/pm/lockfiles.go internal/pm/lockfiles_test.go
git commit -m "pm: ordered, deterministic lockfile detection; bun.lock, Pipfile, Java build files"
```

---

### Task 7: Install candidates — lockfiles narrow within an ecosystem only, install the registry's name, no implicit version pin (roadmap #1, #2, #3; deferred validation items)

**Files:**
- Create: `internal/cli/install_plan.go`, `internal/cli/install.go`, `internal/cli/install_plan_test.go`
- Modify:
  - `internal/cli/cli.go`: delete `filterResultsByLockFiles` and `cmdInstall`; replace the "is the PM installed" block in `installForTarget`; drop the `"flag"` and `"sort"` imports.
  - `internal/cli/search_cmd.go`: `installFromSearchResult`.
  - `internal/pm/validation.go`: `rejectLeadingDash` helper and the Composer pattern.
  - `internal/pm/validation_test.go`

**Interfaces:**
- Consumes:
  - from Task 5: `lookupReport`, `classify`, `formatAvailability`, `registryStatus`, `managerName`;
  - from Task 6: `pm.ProjectManagers`, `pm.ProjectFile`, `pm.EcosystemJava`;
  - from `cli.go`: `preferOrderMap`, `parsePackageVersion`, `askYesNo`, `showCommandUsage`.
- Produces (package `cli`):
  - `type candidate struct { Result search.Result; Via string }`. `Result.Manager` is the tool that will run; `Via` is the file that chose it.
  - `func buildCandidates(results []search.Result, project map[pm.Ecosystem][]pm.ProjectFile) []candidate`
  - `func sortCandidates(cands []candidate, prefer []string)`: a stable sort by prefer order.
  - `func candidateLabel(c candidate) string`
  - `func installSpec(c candidate, requestedVersion string) (name string, extra map[string]string)`
  - `func installOne(spec string, global bool) int`
  - `func chooseCandidate(cands []candidate) (candidate, bool)`. Task 11 adds an `unavailable []pm.ID` parameter.
  - `func installCandidate(c candidate, query, requestedVersion string, global bool) int`
  - `func ensureManager(id pm.ID) error`, plus the seam `var ensurePM = ensureManager`.
- Produces (package `pm`):
  - `func rejectLeadingDash(field, value string) error`, used by `ValidatePackageName`, `ValidateGenericPackageName` and `ValidateVersion`;
  - `ValidatePackageName(name, Composer)` now enforces `composerPackagePattern` (vendor/package, lowercase).
- Behaviour:
  - A lockfile only replaces the tool **inside its ecosystem**:
    - `yarn.lock` plus an npm hit becomes yarn;
    - with a PyPI hit too, the user is still asked;
    - build files do the same for Java (`build.gradle` turns a Maven hit into Gradle).
  - An automatic pick happens only when exactly one candidate remains.
  - xpm installs the registry's name (`monolog` → `monolog/monolog`) and says so: `"monolog" matched monolog/monolog.`, then `Will install monolog/monolog via composer (PHP).`
  - A version is passed only when the user typed `@version`. Maven and Gradle keep the registry version because their snippet needs one.
  - The registry-supplied name is validated **before** the "install the package manager?" prompt, in both the CLI and the TUI path.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/install_plan_test.go`:

```go
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
```

Append to `internal/pm/validation_test.go`:

```go
func TestComposerNamesMustBeVendorPackage(t *testing.T) {
	for _, ok := range []string{"monolog/monolog", "symfony/http-kernel", "doctrine/dbal"} {
		if err := ValidatePackageName(ok, Composer); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"monolog", "Monolog/Monolog", "a/b/c", "vendor/"} {
		if ValidatePackageName(bad, Composer) == nil {
			t.Errorf("%q accepted for composer", bad)
		}
	}
}

func TestRejectLeadingDash(t *testing.T) {
	if rejectLeadingDash("version", "1.0.0") != nil || rejectLeadingDash("version", "-1") == nil {
		t.Fatal("rejectLeadingDash must reject exactly the values that start with '-'")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ ./internal/pm/ -run 'Lockfile|Candidate|Gradle|Registry|InstallSpec|TUIName|Composer|RejectLeadingDash' -v`
Expected: FAIL. This is a build error: `undefined: buildCandidates`, `undefined: candidate`, `undefined: ensurePM`, `undefined: rejectLeadingDash`.

- [ ] **Step 3: Factor the validation helper and enforce Composer names**

In `internal/pm/validation.go`:
1. Remove the line `//lint:ignore U1000 used by P3` above `composerPackagePattern`.
2. Add, above `containsDangerousChars`:

```go
// rejectLeadingDash refuses values a package manager would parse as an
// option (`-g`, `--registry=...`, `--working-dir=/etc`).
func rejectLeadingDash(field, value string) error {
	if strings.HasPrefix(value, "-") {
		return NewValidationError(field, value, "cannot start with '-'")
	}
	return nil
}
```

3. In `ValidatePackageName` and `ValidateGenericPackageName`, replace

```go
	// A leading dash would be parsed as an option by npm/composer/pip/cargo.
	if strings.HasPrefix(pkg, "-") {
		return NewValidationError("package name", pkg, "cannot start with '-'")
	}
```

   with

```go
	if err := rejectLeadingDash("package name", pkg); err != nil {
		return err
	}
```

   and in `ValidateVersion` replace the `strings.HasPrefix(version, "-")` block with

```go
	if err := rejectLeadingDash("version", version); err != nil {
		return err
	}
```

4. In `ValidatePackageName`, replace the `case Composer:` body with:

```go
	case Composer:
		// Install-time names are full vendor/package names; partial names
		// only reach the search, which uses ValidateGenericPackageName.
		if !composerPackagePattern.MatchString(pkg) {
			return NewValidationError("package name", pkg, "invalid Composer package name (want vendor/package, lowercase)")
		}
```

- [ ] **Step 4: Add the candidate logic**

Create `internal/cli/install_plan.go`:

```go
package cli

import (
	"fmt"
	"sort"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// candidate is one way to satisfy `xpm install <query>`: a registry hit and
// the tool that would install it (Result.Manager).
type candidate struct {
	Result search.Result
	// Via is the project file that selected the tool ("yarn.lock"), or "".
	Via string
}

// buildCandidates turns registry hits into install choices. Inside an
// ecosystem the project's lock/build files narrow the tool: with yarn.lock,
// the npm hit is installed with yarn. Across ecosystems nothing is narrowed:
// a name found on npm and on PyPI is always the user's choice.
func buildCandidates(results []search.Result, project map[pm.Ecosystem][]pm.ProjectFile) []candidate {
	var out []candidate
	for _, r := range results {
		files := project[pm.EcosystemForManager(r.Manager)]
		if len(files) == 0 {
			out = append(out, candidate{Result: r})
			continue
		}
		for _, f := range files {
			c := candidate{Result: r, Via: f.Name}
			c.Result.Manager = f.Manager
			out = append(out, c)
		}
	}
	return out
}

// sortCandidates orders candidates by the user's prefer list. Ties keep
// their registry order, so the result is deterministic.
func sortCandidates(cands []candidate, prefer []string) {
	order := preferOrderMap(prefer)
	sort.SliceStable(cands, func(i, j int) bool {
		return order[string(cands[i].Result.Manager)] < order[string(cands[j].Result.Manager)]
	})
}

// candidateLabel is how a candidate appears in the selection prompt.
func candidateLabel(c candidate) string {
	label := fmt.Sprintf("%s (%s)", c.Result.Name, c.Result.Manager)
	if c.Via != "" {
		label = fmt.Sprintf("%s (%s, from %s)", c.Result.Name, c.Result.Manager, c.Via)
	}
	if v := c.Result.Extra["version"]; v != "" {
		label += " @ " + v
	}
	if c.Result.Info != "" {
		label += " - " + c.Result.Info
	}
	return label
}

// installSpec returns what to hand the adapter: the registry's name for the
// package (Composer/Maven hits can differ from the query) and a copy of its
// extra info. A version is passed only when the user asked for one; Maven
// and Gradle keep the registry's latest because they print a snippet that
// needs a concrete version.
func installSpec(c candidate, requestedVersion string) (name string, extra map[string]string) {
	extra = make(map[string]string, len(c.Result.Extra)+1)
	for k, v := range c.Result.Extra {
		extra[k] = v
	}
	switch {
	case requestedVersion != "":
		extra["version"] = requestedVersion
	case c.Result.Manager == pm.Maven || c.Result.Manager == pm.Gradle:
	default:
		delete(extra, "version")
	}
	return c.Result.Name, extra
}
```

- [ ] **Step 5: Move the install flow into `install.go`**

In `internal/cli/cli.go`:
- delete `filterResultsByLockFiles` (from its doc comment `// filterResultsByLockFiles filters search results` to its closing brace);
- delete the whole `cmdInstall` function;
- in `installForTarget`, replace the block that starts `if !pm.Exists(meta.Binary) {` and ends just before `fmt.Printf("Running dependency install for %s using %s...\n", t.Label, meta.Name)` with:

```go
	if err := ensurePM(chosenPM); err != nil {
		return err
	}

```

- remove the imports `"flag"` and `"sort"`, which are now unused.

Create `internal/cli/install.go`:

```go
package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// ensurePM is ensureManager; tests replace it to observe ordering.
var ensurePM = ensureManager

func cmdInstall(args []string) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	global := fs.Bool("global", false, "install globally")
	gShort := fs.Bool("g", false, "install globally (shorthand)")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return 1
	}
	pkgArgs := fs.Args()
	glob := *global || *gShort

	if len(pkgArgs) == 0 {
		return autoInstallDetected(glob)
	}
	return installOne(pkgArgs[0], glob)
}

// installOne resolves one "name[@version]" argument against the registries
// and installs it with the chosen tool.
func installOne(spec string, global bool) int {
	pkg, requestedVersion := parsePackageVersion(spec)
	if err := pm.ValidateGenericPackageName(pkg); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid package name: %v\n\n", err)
		showCommandUsage("install")
		return 1
	}
	if requestedVersion != "" {
		if err := pm.ValidateVersion(requestedVersion); err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid version: %v\n\n", err)
			showCommandUsage("install")
			return 1
		}
		fmt.Printf("Searching for %q (version %s) across ecosystems...\n\n", pkg, requestedVersion)
	} else {
		fmt.Printf("Searching for %q across ecosystems...\n\n", pkg)
	}

	searchOpts := search.OptionsFromConfig(cfg)
	rep, err := lookupReport(pkg, searchOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search error:", err)
		return 1
	}
	st := classify(rep, searchOpts)
	if len(rep.Results) == 0 {
		fmt.Println("No matches found for", pkg)
		fmt.Print(formatAvailability(st))
		return 1
	}
	if len(st.Unavailable) > 0 {
		fmt.Print(formatAvailability(registryStatus{Unavailable: st.Unavailable}))
		fmt.Println()
	}

	cwd, _ := os.Getwd()
	cands := buildCandidates(rep.Results, pm.ProjectManagers(cwd))
	sortCandidates(cands, cfg.Prefer)
	chosen, ok := chooseCandidate(cands)
	if !ok {
		return 1
	}
	return installCandidate(chosen, pkg, requestedVersion, global)
}

// chooseCandidate picks automatically when there is one candidate, and
// otherwise prompts (or, non-interactively, takes the first).
func chooseCandidate(cands []candidate) (candidate, bool) {
	if len(cands) == 1 {
		if c := cands[0]; c.Via != "" {
			fmt.Printf("Detected %s - using %s\n\n", c.Via, c.Result.Manager)
		}
		return cands[0], true
	}
	labels := make([]string, len(cands))
	for i, c := range cands {
		labels[i] = candidateLabel(c)
	}
	if !cfg.Interactive {
		fmt.Println("Non-interactive mode: picking", labels[0])
		return cands[0], true
	}
	prompt := promptui.Select{Label: "Select package manager to install from", Items: labels}
	idx, _, err := prompt.Run()
	if err != nil {
		fmt.Println("Cancelled.")
		return candidate{}, false
	}
	return cands[idx], true
}

// installCandidate installs (or prints the snippet for) one candidate.
// query is what the user typed; it is only used to explain a renamed match.
func installCandidate(c candidate, query, requestedVersion string, global bool) int {
	id := c.Result.Manager
	meta, ok := pm.MetaFor(id)
	if !ok {
		fmt.Fprintln(os.Stderr, "Unsupported package manager:", id)
		return 1
	}
	name, extra := installSpec(c, requestedVersion)

	// The name comes from a registry response, not the user: validate it
	// before anything else happens (including offering to install the tool).
	if err := pm.ValidatePackageName(name, id); err != nil {
		fmt.Fprintf(os.Stderr, "Refusing to install %q: %v\n", name, err)
		return 1
	}

	if name != query {
		fmt.Printf("%q matched %s.\n", query, name)
	}
	target := name
	if requestedVersion != "" {
		target += "@" + requestedVersion
	}
	fmt.Printf("Will install %s via %s.\n\n", target, meta.Name)

	if global && !meta.SupportsGlobal {
		fmt.Printf("%s does not support global installs in the same way. Ignoring --global.\n\n", meta.Name)
		global = false
	}
	if global && id == pm.Pip {
		fmt.Println("Global pip installs often require sudo and can affect system Python.")
		yes, err := askYesNo("Show recommended sudo command instead of running pip?")
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		if yes {
			fmt.Printf("\nRun this manually:\n  sudo pip install %s\n", name)
			return 0
		}
		fmt.Println("Proceeding without sudo (may fail if permissions are insufficient)...")
	}

	if err := ensurePM(id); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	adapter, err := pm.NewAdapter(id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "adapter error:", err)
		return 1
	}
	if err := adapter.InstallPackage(name, global, nil, extra); err != nil {
		fmt.Fprintln(os.Stderr, "package install failed:", err)
		return 1
	}
	fmt.Println("\nDone ✅")
	return 0
}

// ensureManager makes sure id's binary is on PATH, offering to install it
// when config allows. It returns an error the caller should print.
func ensureManager(id pm.ID) error {
	meta, ok := pm.MetaFor(id)
	if !ok {
		return fmt.Errorf("unknown package manager %s", id)
	}
	if pm.Exists(meta.Binary) {
		return nil
	}
	fmt.Printf("%s (%s) is not installed on this system.\n", meta.Name, meta.Binary)
	if !cfg.AutoInstallPM {
		if hint := strings.TrimSpace(pm.InstallHint(id)); hint != "" {
			fmt.Println("Hint:", hint)
		}
		return fmt.Errorf("%s is not installed (auto-install is disabled in config)", meta.Name)
	}
	yes, err := askYesNo(fmt.Sprintf("Attempt to install %s now?", meta.Name))
	if err != nil {
		return fmt.Errorf("cancelled")
	}
	if !yes {
		return fmt.Errorf("%s is not installed", meta.Name)
	}
	if err := pm.InstallPM(id); err != nil {
		return fmt.Errorf("failed to install %s: %w", meta.Name, err)
	}
	return nil
}
```

In `internal/cli/search_cmd.go`, replace the whole `installFromSearchResult` function with:

```go
// installFromSearchResult installs the package picked in the TUI with the
// tool picked there. The registry-supplied name is validated before anything
// else, and no version is pinned (the tool resolves its own latest).
func installFromSearchResult(result search.Result, pmID pm.ID) int {
	c := candidate{Result: result}
	c.Result.Manager = pmID
	return installCandidate(c, result.Name, "", false)
}
```

- [ ] **Step 6: Run the tests**

Run: `go test -race ./internal/cli/ ./internal/pm/`
Expected: two `ok` lines. Two tests matter most:
- `TestLockfileNarrowsOnlyItsOwnEcosystem` gives `yarn:yarn.lock,pip:`;
- `TestTUINameIsValidatedBeforeOfferingToInstallTheTool` shows `ensurePM` is never called for `--registry=http://evil`.

- [ ] **Step 7: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/cli internal/pm/validation.go internal/pm/validation_test.go
git commit -m "install: lockfiles narrow within an ecosystem only; install the registry's name; no implicit version pin; validate names before the PM prompt"
```

---

### Task 8: Go module paths go straight to `go get`; Maven hits can be installed with Gradle in the TUI (roadmap #6)

**Files:**
- Modify: `internal/cli/install_plan.go` (add `isGoModulePath`, `goModuleCandidate`, and the `"strings"` import), `internal/cli/install.go` (`installOne`), `internal/tui/search/update.go` (`handleInstallSelectMsg`, new `installManagersFor`), `internal/tui/search/view.go` (`renderInstallSelector`)
- Test: `internal/cli/install_plan_test.go` (append), `internal/tui/search/update_test.go` (new)

**Interfaces:**
- Consumes (Task 6, 7): `pm.ManagersInEcosystem(pm.EcosystemJava)`, `installCandidate`.
- Produces:
  - `func isGoModulePath(s string) bool`: the first path element contains a dot, there is at least one more element, there is no `@` prefix and no `:`.
  - `func goModuleCandidate(path string) candidate` (Manager `gomod`, `Extra["module"]`).
  - `func installManagersFor(id pm.ID) []pm.ID` (package `tui/search`): every manager in the hit's ecosystem.
- Behaviour:
  - `xpm install github.com/gin-gonic/gin[@v]` skips the registries entirely: it prints `github.com/gin-gonic/gin is a Go module path; using go modules.` and runs `go get github.com/gin-gonic/gin@latest` (or `@v`).
  - Gradle is offered for Maven hits in the CLI (via Task 6 build files) and in the TUI (`[maven gradle]`).

- [ ] **Step 1: Write the failing tests**

Append to `internal/cli/install_plan_test.go`:

```go
func TestIsGoModulePath(t *testing.T) {
	for s, want := range map[string]bool{
		"github.com/gin-gonic/gin": true,
		"golang.org/x/term":        true,
		"gopkg.in/yaml.v3":         true,
		"axios":                    false,
		"@types/node":              false,
		"monolog/monolog":          false,
		"com.google.guava:guava":   false,
		"example.com":              false,
		"lodash.merge":             false,
	} {
		if got := isGoModulePath(s); got != want {
			t.Errorf("isGoModulePath(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestGoModulePathSkipsRegistries(t *testing.T) {
	withConfig(t, config.Config{})
	old := lookupReport
	lookupReport = func(string, search.Options) (search.Report, error) {
		t.Error("registries were queried for a Go module path")
		return search.Report{}, nil
	}
	t.Cleanup(func() { lookupReport = old })
	oldEnsure := ensurePM
	ensurePM = func(id pm.ID) error {
		if id != pm.GoMod {
			t.Errorf("tool = %s, want gomod", id)
		}
		return errors.New("stop before running go")
	}
	t.Cleanup(func() { ensurePM = oldEnsure })

	out := captureStdout(t, func() { installOne("github.com/gin-gonic/gin@v1.10.0", false) })
	if !strings.Contains(out, "Will install github.com/gin-gonic/gin@v1.10.0 via go modules (Go).") {
		t.Fatalf("output:\n%s", out)
	}
}
```

Create `internal/tui/search/update_test.go`:

```go
package search

import (
	"fmt"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
)

func TestInstallManagersFor(t *testing.T) {
	for id, want := range map[pm.ID]string{
		pm.Npm:   "[npm yarn pnpm bun]",
		pm.Pip:   "[pip poetry pipenv]",
		pm.Maven: "[maven gradle]",
		pm.Cargo: "[cargo]",
	} {
		if got := fmt.Sprint(installManagersFor(id)); got != want {
			t.Errorf("installManagersFor(%s) = %s, want %s", id, got, want)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ ./internal/tui/... -run 'GoModule|InstallManagersFor' -v`
Expected: FAIL. This is a build error: `undefined: isGoModulePath`, `undefined: installManagersFor`.

- [ ] **Step 3: Implement**

In `internal/cli/install_plan.go`, add `"strings"` to the imports and append:

```go
// isGoModulePath reports whether s looks like a Go module path
// (github.com/gin-gonic/gin, golang.org/x/term, gopkg.in/yaml.v3): a first
// element containing a dot (a domain) followed by at least one more element.
// npm scopes (@a/b), Composer names (vendor/pkg) and Maven coordinates
// (g:a) never match.
func isGoModulePath(s string) bool {
	first, rest, ok := strings.Cut(s, "/")
	return ok && rest != "" && !strings.HasPrefix(s, "@") &&
		strings.Contains(first, ".") && !strings.Contains(s, ":")
}

// goModuleCandidate installs a module path with go modules; no registry is
// consulted (`go get` resolves and verifies it via the module proxy).
func goModuleCandidate(path string) candidate {
	return candidate{Result: search.Result{Manager: pm.GoMod, Name: path, Extra: map[string]string{"module": path}}}
}
```

In `internal/cli/install.go` (`installOne`), replace

```go
		fmt.Printf("Searching for %q (version %s) across ecosystems...\n\n", pkg, requestedVersion)
	} else {
		fmt.Printf("Searching for %q across ecosystems...\n\n", pkg)
	}
```

with

```go
	}
	if isGoModulePath(pkg) {
		fmt.Printf("%s is a Go module path; using go modules.\n\n", pkg)
		return installCandidate(goModuleCandidate(pkg), pkg, requestedVersion, global)
	}
	if requestedVersion != "" {
		fmt.Printf("Searching for %q (version %s) across ecosystems...\n\n", pkg, requestedVersion)
	} else {
		fmt.Printf("Searching for %q across ecosystems...\n\n", pkg)
	}
```

After the edit, the version branch reads: validate the version, close the `if`, check for a Go module path, then print the search line.

In `internal/tui/search/update.go`, `handleInstallSelectMsg`: replace everything from `// Get available package managers for this ecosystem` through `m.installPMs = availablePMs` with

```go
	m.installPMs = installManagersFor(msg.result.Manager)
```

and append to the file:

```go
// installManagersFor lists the tools that can install a hit from registry
// id: every manager of its ecosystem (npm hit: npm, yarn, pnpm, bun; Maven
// hit: maven, gradle), or just id for single-tool ecosystems.
func installManagersFor(id pm.ID) []pm.ID {
	if ids := pm.ManagersInEcosystem(pm.EcosystemForManager(id)); len(ids) > 0 {
		return ids
	}
	return []pm.ID{id}
}
```

In `internal/tui/search/view.go`, `renderInstallSelector`: delete the block from `// Get available package managers for this ecosystem` through the closing `}` of the `else` branch, and change `for i, pmID := range availablePMs {` to `for i, pmID := range m.installPMs {`. The view then shows exactly the list the key handler navigates.

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/cli/ ./internal/tui/...`
Expected: `ok` for both.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/cli internal/tui
git commit -m "install: route Go module paths to go get; offer Gradle for Maven hits in the TUI"
```

---
### Task 9: `xpm install a b c`, flags anywhere, errors on unused arguments (roadmap #4)

**Files:**
- Create: `internal/cli/args.go`, `internal/cli/args_test.go`
- Modify: `internal/cli/install.go` (`cmdInstall`, seams), `internal/cli/cli.go` (`cmdWhich`), `internal/cli/commands.go` (`cmdInfo`, `cmdRemove`, `cmdUpdate`, `cmdList`), `internal/cli/search_cmd.go` (`cmdSearch`), `internal/cli/helpers_test.go` (append `withInstallOne`)

**Interfaces:**
- Consumes (Task 7): `installOne(spec string, global bool) int`, `autoInstallDetected(global bool) int`. Task 10 drops the parameter.
- Produces:
  - `type installArgs struct { Global bool; Packages []string }`
  - `func parseInstallArgs(args []string) (installArgs, error)`: accepts `-g`, `--global` and `-global` anywhere; `--` ends flag parsing; any other dash argument is an error.
  - `func exactlyOneArg(cmd string, args []string) (string, bool)`
  - `func atMostOneArg(cmd string, args []string) (string, bool)`
  - Seam `var installPkg = installOne`, declared in the same `var (...)` block as `ensurePM`.
  - Test helper `withInstallOne(t, fn)`.
- Behaviour:
  - `xpm install a b c` installs each package in order and stops at the first failure. It prints `[i/n] <pkg>` headers and `Stopped at b; 1 remaining package(s) were not installed.`
  - `which`, `info` and `remove` take exactly one argument; `update` and `search` take at most one; `list` takes none. Extra arguments print an error and exit 1. Today they are silently ignored.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/args_test.go`:

```go
package cli

import (
	"reflect"
	"testing"

	"github.com/crenspire/xpm/internal/config"
)

func TestParseInstallArgs(t *testing.T) {
	for _, c := range []struct {
		args    []string
		global  bool
		pkgs    []string
		wantErr bool
	}{
		{[]string{"axios"}, false, []string{"axios"}, false},
		{[]string{"axios", "-g"}, true, []string{"axios"}, false},
		{[]string{"-g", "typescript"}, true, []string{"typescript"}, false},
		{[]string{"a", "--global", "b", "c"}, true, []string{"a", "b", "c"}, false},
		{[]string{"axios", "--save-dev"}, false, nil, true},
		{[]string{"--", "-weird"}, false, []string{"-weird"}, false},
		{nil, false, nil, false},
	} {
		got, err := parseInstallArgs(c.args)
		if (err != nil) != c.wantErr {
			t.Errorf("%v: err = %v, wantErr %v", c.args, err, c.wantErr)
			continue
		}
		if !c.wantErr && (got.Global != c.global || !reflect.DeepEqual(got.Packages, c.pkgs)) {
			t.Errorf("%v: got %+v, want global=%v pkgs=%v", c.args, got, c.global, c.pkgs)
		}
	}
}

func TestInstallsEveryPackageInOrderAndStopsAtFirstFailure(t *testing.T) {
	withConfig(t, config.Config{})
	var seen []string
	withInstallOne(t, func(spec string, global bool) int {
		seen = append(seen, spec)
		if spec == "b" {
			return 1
		}
		return 0
	})
	var code int
	captureStdout(t, func() { code = cmdInstall([]string{"a", "b", "c", "-g"}) })
	if code != 1 || !reflect.DeepEqual(seen, []string{"a", "b"}) {
		t.Fatalf("code=%d seen=%v, want 1 and [a b]", code, seen)
	}
}

func TestExtraArgumentsAreAnError(t *testing.T) {
	withConfig(t, config.Config{})
	for name, run := range map[string]func() int{
		"which":  func() int { return cmdWhich([]string{"axios", "lodash"}) },
		"info":   func() int { return cmdInfo([]string{"axios", "lodash"}) },
		"remove": func() int { return cmdRemove([]string{"axios", "lodash"}) },
		"update": func() int { return cmdUpdate([]string{"axios", "lodash"}) },
		"list":   func() int { return cmdList([]string{"axios"}) },
		"search": func() int { return cmdSearch([]string{"axios", "lodash"}) },
	} {
		var code int
		captureStdout(t, func() { code = run() })
		if code != 1 {
			t.Errorf("%s with an extra argument exited %d, want 1", name, code)
		}
	}
}
```

Append to `internal/cli/helpers_test.go`:

```go

// withInstallOne fakes the per-package install step of cmdInstall.
func withInstallOne(t *testing.T, fn func(spec string, global bool) int) {
	t.Helper()
	old := installPkg
	installPkg = fn
	t.Cleanup(func() { installPkg = old })
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ -run 'ParseInstallArgs|InstallsEveryPackage|ExtraArguments' -v`
Expected: FAIL. This is a build error: `undefined: parseInstallArgs`, `undefined: installPkg`.

- [ ] **Step 3: Implement the argument helpers**

Create `internal/cli/args.go`:

```go
package cli

import (
	"fmt"
	"os"
	"strings"
)

// installArgs is `xpm install`'s command line.
type installArgs struct {
	Global   bool
	Packages []string
}

// parseInstallArgs accepts -g/--global anywhere (`xpm install axios -g`).
// "--" ends flag parsing; any other dash argument is an error rather than
// being passed on as a package name.
func parseInstallArgs(args []string) (installArgs, error) {
	var out installArgs
	for i, a := range args {
		switch {
		case a == "--":
			out.Packages = append(out.Packages, args[i+1:]...)
			return out, nil
		case a == "-g" || a == "--global" || a == "-global":
			out.Global = true
		case strings.HasPrefix(a, "-"):
			return installArgs{}, fmt.Errorf("unknown flag %q (install accepts -g/--global)", a)
		default:
			out.Packages = append(out.Packages, a)
		}
	}
	return out, nil
}

// exactlyOneArg returns the only argument of cmd. Missing or extra
// arguments print an error and the command's usage instead of being
// silently ignored.
func exactlyOneArg(cmd string, args []string) (string, bool) {
	switch len(args) {
	case 1:
		return args[0], true
	case 0:
		fmt.Fprintln(os.Stderr, "error: missing package name")
	default:
		fmt.Fprintf(os.Stderr, "error: %s takes one package name, got %d: %s\n", cmd, len(args), strings.Join(args, " "))
	}
	fmt.Fprintln(os.Stderr)
	showCommandUsage(cmd)
	return "", false
}

// atMostOneArg is exactlyOneArg for commands whose argument is optional.
func atMostOneArg(cmd string, args []string) (string, bool) {
	if len(args) == 0 {
		return "", true
	}
	return exactlyOneArg(cmd, args)
}
```

- [ ] **Step 4: Use them**

In `internal/cli/install.go`:
- remove the `"flag"` import;
- replace `// ensurePM is ensureManager; tests replace it to observe ordering.` and `var ensurePM = ensureManager` with:

```go
// Seams for tests: ensurePM is ensureManager, installPkg is installOne.
var (
	ensurePM   = ensureManager
	installPkg = installOne
)
```

- replace `cmdInstall` with:

```go
func cmdInstall(args []string) int {
	ia, err := parseInstallArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		fmt.Fprintln(os.Stderr)
		showCommandUsage("install")
		return 1
	}
	if len(ia.Packages) == 0 {
		return autoInstallDetected(ia.Global)
	}
	for i, p := range ia.Packages {
		if len(ia.Packages) > 1 {
			fmt.Printf("[%d/%d] %s\n", i+1, len(ia.Packages), p)
		}
		if code := installPkg(p, ia.Global); code != 0 {
			if left := len(ia.Packages) - i - 1; left > 0 {
				fmt.Fprintf(os.Stderr, "Stopped at %s; %d remaining package(s) were not installed.\n", p, left)
			}
			return code
		}
	}
	return 0
}
```

In `cmdWhich` (`cli.go`), `cmdInfo` and `cmdRemove` (`commands.go`), replace the opening argument check and the `pkg := args[0]` line that follows it:

```go
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: missing package name")
		fmt.Fprintln(os.Stderr)
		showCommandUsage("which")
		return 1
	}
	pkg := args[0]
```

Use each command's own name. In `cmdWhich` the new code is:

```go
	pkg, ok := exactlyOneArg("which", args)
	if !ok {
		return 1
	}
```

In `cmdInfo` use `exactlyOneArg("info", args)`, and in `cmdRemove` use `exactlyOneArg("remove", args)`. Otherwise the code is identical. In those two functions a blank line sits between the `if` block and `pkg := args[0]`; delete it as well.

At the top of `cmdUpdate`, insert:

```go
	if _, ok := atMostOneArg("update", args); !ok {
		return 1
	}
```

At the top of `cmdList`, insert:

```go
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "error: list takes no arguments, got: %s\n", strings.Join(args, " "))
		return 1
	}
```

At the top of `cmdSearch` (`search_cmd.go`), insert:

```go
	if _, ok := atMostOneArg("search", args); !ok {
		return 1
	}
```

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/cli/`
Expected: `ok`.

- [ ] **Step 6: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/cli
git commit -m "cli: install several packages, accept flags after them, reject unused arguments"
```

---

### Task 10: Project commands run the project's own tool; `xpm ci` is a native frozen install (roadmap #5, #7)

**Files:**
- Create: `internal/cli/project.go`, `internal/cli/project_test.go`
- Modify:
  - `internal/cli/cli.go`:
    - delete `type projectTarget`, `detectProjectTargets`, `autoInstallDetected`, `installForTarget` and `runBinary` (everything from `type projectTarget struct {` to just before `func pickPreferredPM(`);
    - delete `cmdCleanInstall` (from `// cmdCleanInstall performs a clean install` to just before `func cmdWhich(`);
    - remove the `"os/exec"` import.
  - `internal/cli/commands.go`: delete `cmdList`, `cmdUpdate` and `cmdRemove` (from `// cmdList lists installed packages` to just before `// cmdInfo shows detailed information`), and remove the `promptui` import.
  - `internal/cli/install.go`: the no-argument branch of `cmdInstall`.

**Interfaces:**
- Consumes:
  - from Task 6: `pm.ProjectManagers`, `pm.ProjectFile`;
  - from Task 7: `ensurePM`;
  - from Task 9: `exactlyOneArg`, `atMostOneArg`;
  - unchanged: `pickPreferredPM`, `fileExists`, `askYesNo`, `runBinaryWithCode` (`commands.go`).
- Produces (package `cli`):
  - Seams `var runTool = runBinaryWithCode` and `var pmExists = pm.Exists`.
  - `type projectTarget struct { Label, Kind string; PMs []pm.ID }`, the same shape. The new kinds are `poetry` and `pipenv`.
  - `func detectProjectTargets() []projectTarget`. For Python:
    - `Pipfile` or `Pipfile.lock` → pipenv;
    - otherwise `poetry.lock` → poetry;
    - otherwise `requirements.txt` → `pip-req` and/or `pyproject.toml` → `pip-pyproject`.
  - `type pmChoice struct { PM pm.ID; Via string; Options []pm.ID }`
  - `func resolveTargetPM(t projectTarget, files []pm.ProjectFile, prefer []string) pmChoice`:
    - one lockfile decides;
    - with several lockfiles, `prefer` narrows them, else the user picks;
    - with none, `prefer` decides, else npm.
  - `type projectCmd struct { Kind string; PM pm.ID; YarnBerry bool }` with `func (c projectCmd) args(action, pkg string) ([]string, error)`. The actions are `install`, `ci`, `list`, `update` and `remove`.
  - `type manualError struct{ msg string }`: no such command for this tool; print the message and exit 0.
  - `func cleanDirs(c projectCmd) []string`:
    - `node_modules` for yarn, pnpm and bun (`npm ci` clears it itself);
    - `vendor` for Composer only;
    - never Go's `vendor/`.
  - Other helpers: `selectTarget`, `projectCmdFor`, `runProject(t, action, pkg) int`, `runEach`.
  - Commands: `autoInstallDetected() int`, `cmdCleanInstall`, `cmdList`, `cmdUpdate`, `cmdRemove`.
- `xpm ci` command table:

| Project | Tool | Command |
|---|---|---|
| node | npm | `npm ci` |
| node | pnpm | `pnpm install --frozen-lockfile` |
| node | yarn 1 | `yarn install --frozen-lockfile` |
| node | yarn 2+ (`.yarnrc.yml`) | `yarn install --immutable` |
| node | bun | `bun install --frozen-lockfile` |
| PHP | composer | `composer install` |
| Python | pip (`requirements.txt`) | `pip install -r requirements.txt` |
| Python | pip (`pyproject.toml`) | `pip install .` |
| Python | poetry | `poetry install` |
| Python | pipenv | `pipenv install --deploy` |
| Rust | cargo | `cargo build --locked` |
| Go | go modules | `go mod download` |
| Java | maven | `mvn install` |
| Java | gradle | `gradle build` |

  Lockfiles are never deleted. `node_modules/` and `vendor/` are deleted only after a "yes" (`askYesNo`, which answers "no" without a terminal).
- Also changes:
  - `xpm install` (no args) in a Go project keeps running `go mod tidy` (what Go users expect; user decision 2026-10-07). Only `xpm ci` uses `go mod download`.
  - `cargo` list uses `cargo tree --depth 1`; `update <crate>` uses `cargo update -p <crate>`.
  - `xpm install -g` with no package is an error.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/project_test.go`:

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
)

// inProject chdirs into a temp dir containing files (dirs end in "/").
func inProject(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		p := filepath.Join(dir, f)
		if strings.HasSuffix(f, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	return dir
}

// recordTools replaces tool execution with a recorder; every tool "exists".
func recordTools(t *testing.T) *[]string {
	t.Helper()
	var ran []string
	oldRun, oldExists, oldEnsure := runTool, pmExists, ensurePM
	runTool = func(bin string, args []string) int {
		ran = append(ran, bin+" "+strings.Join(args, " "))
		return 0
	}
	pmExists = func(string) bool { return true }
	ensurePM = func(pm.ID) error { return nil }
	t.Cleanup(func() { runTool, pmExists, ensurePM = oldRun, oldExists, oldEnsure })
	return &ran
}

func TestDetectPythonTool(t *testing.T) {
	for files, want := range map[string]string{
		"Pipfile":                      "pipenv",
		"poetry.lock,pyproject.toml":   "poetry",
		"poetry.lock,requirements.txt": "poetry",
		"requirements.txt":             "pip-req",
	} {
		inProject(t, strings.Split(files, ",")...)
		var kinds []string
		for _, tg := range detectProjectTargets() {
			kinds = append(kinds, tg.Kind)
		}
		if strings.Join(kinds, ",") != want {
			t.Errorf("%s: kinds = %v, want %s", files, kinds, want)
		}
	}
}

func TestResolveTargetPM(t *testing.T) {
	node := projectTarget{Kind: "node", PMs: []pm.ID{pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun}}
	lock := func(name string, id pm.ID) pm.ProjectFile {
		return pm.ProjectFile{Name: name, Ecosystem: pm.EcosystemNode, Manager: id}
	}
	for _, c := range []struct {
		name   string
		files  []pm.ProjectFile
		prefer []string
		want   string
	}{
		{"one lock file decides", []pm.ProjectFile{lock("bun.lock", pm.Bun)}, []string{"pnpm"}, "{bun bun.lock []}"},
		{"two lock files, prefer narrows", []pm.ProjectFile{lock("package-lock.json", pm.Npm), lock("yarn.lock", pm.Yarn)}, []string{"yarn"}, "{yarn prefer []}"},
		{"two lock files, user picks", []pm.ProjectFile{lock("package-lock.json", pm.Npm), lock("yarn.lock", pm.Yarn)}, nil, "{  [npm yarn]}"},
		{"no lock file, prefer", nil, []string{"pnpm"}, "{pnpm prefer []}"},
		{"no lock file, default npm", nil, nil, "{npm  []}"},
	} {
		if got := fmt.Sprint(resolveTargetPM(node, c.files, c.prefer)); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	if got := resolveTargetPM(projectTarget{Kind: "cargo", PMs: []pm.ID{pm.Cargo}}, nil, nil); got.PM != pm.Cargo {
		t.Errorf("single-tool project: %+v", got)
	}
}

func TestProjectArgs(t *testing.T) {
	for _, c := range []struct {
		cmd         projectCmd
		action, pkg string
		want        string
	}{
		{projectCmd{Kind: "node", PM: pm.Npm}, "ci", "", "ci"},
		{projectCmd{Kind: "node", PM: pm.Pnpm}, "ci", "", "install --frozen-lockfile"},
		{projectCmd{Kind: "node", PM: pm.Yarn}, "ci", "", "install --frozen-lockfile"},
		{projectCmd{Kind: "node", PM: pm.Yarn, YarnBerry: true}, "ci", "", "install --immutable"},
		{projectCmd{Kind: "node", PM: pm.Bun}, "ci", "", "install --frozen-lockfile"},
		{projectCmd{Kind: "node", PM: pm.Yarn}, "update", "axios", "upgrade axios"},
		{projectCmd{Kind: "node", PM: pm.Yarn, YarnBerry: true}, "update", "", "up *"},
		{projectCmd{Kind: "node", PM: pm.Npm}, "remove", "axios", "uninstall axios"},
		{projectCmd{Kind: "node", PM: pm.Bun}, "list", "", "pm ls"},
		{projectCmd{Kind: "composer", PM: pm.Composer}, "ci", "", "install"},
		{projectCmd{Kind: "pip-req", PM: pm.Pip}, "ci", "", "install -r requirements.txt"},
		{projectCmd{Kind: "pipenv", PM: pm.Pipenv}, "ci", "", "install --deploy"},
		{projectCmd{Kind: "poetry", PM: pm.Poetry}, "remove", "requests", "remove requests"},
		{projectCmd{Kind: "cargo", PM: pm.Cargo}, "ci", "", "build --locked"},
		{projectCmd{Kind: "cargo", PM: pm.Cargo}, "update", "serde", "update -p serde"},
		{projectCmd{Kind: "gomod", PM: pm.GoMod}, "install", "", "mod tidy"},
		{projectCmd{Kind: "gomod", PM: pm.GoMod}, "ci", "", "mod download"},
	} {
		got, err := c.cmd.args(c.action, c.pkg)
		if err != nil || strings.Join(got, " ") != c.want {
			t.Errorf("%+v %s %q: got %q (%v), want %q", c.cmd, c.action, c.pkg, strings.Join(got, " "), err, c.want)
		}
	}
	if _, err := (projectCmd{Kind: "gomod", PM: pm.GoMod}).args("remove", "x"); !strings.Contains(fmt.Sprint(err), "go mod tidy") {
		t.Errorf("go remove must explain the manual step, got %v", err)
	}
}

func TestCleanDirsNeverTouchGoVendor(t *testing.T) {
	for c, want := range map[projectCmd][]string{
		{Kind: "node", PM: pm.Npm}:          nil,
		{Kind: "node", PM: pm.Pnpm}:         {"node_modules"},
		{Kind: "composer", PM: pm.Composer}: {"vendor"},
		{Kind: "gomod", PM: pm.GoMod}:       nil,
	} {
		if got := cleanDirs(c); !reflect.DeepEqual(got, want) {
			t.Errorf("cleanDirs(%+v) = %v, want %v", c, got, want)
		}
	}
}

func TestProjectCommandsUseTheLockfileTool(t *testing.T) {
	withConfig(t, config.Config{})
	inProject(t, "package.json", "pnpm-lock.yaml")
	ran := recordTools(t)
	captureStdout(t, func() {
		cmdInstall(nil)
		cmdList(nil)
		cmdUpdate([]string{"axios"})
		cmdRemove([]string{"axios"})
		cmdCleanInstall(nil)
	})
	want := []string{"pnpm install", "pnpm list --depth=0", "pnpm update axios", "pnpm remove axios", "pnpm install --frozen-lockfile"}
	if !reflect.DeepEqual(*ran, want) {
		t.Fatalf("ran %q\nwant %q", *ran, want)
	}
}

func TestPoetryAndPipenvProjects(t *testing.T) {
	withConfig(t, config.Config{})
	ran := recordTools(t)
	inProject(t, "pyproject.toml", "poetry.lock")
	captureStdout(t, func() { cmdInstall(nil) })
	inProject(t, "Pipfile", "Pipfile.lock")
	captureStdout(t, func() { cmdCleanInstall(nil) })
	if want := []string{"poetry install", "pipenv install --deploy"}; !reflect.DeepEqual(*ran, want) {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
}

func TestCIDeletesNothingWithoutConfirmation(t *testing.T) {
	withConfig(t, config.Config{Interactive: false})
	dir := inProject(t, "composer.json", "composer.lock", "vendor/", "package.json", "yarn.lock", ".yarnrc.yml", "node_modules/")
	ran := recordTools(t)
	var code int
	captureStdout(t, func() { code = cmdCleanInstall(nil) })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, f := range []string{"composer.lock", "yarn.lock", "vendor", "node_modules"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s was deleted without confirmation", f)
		}
	}
	if want := []string{"yarn install --immutable", "composer install"}; !reflect.DeepEqual(*ran, want) {
		t.Fatalf("ran %q, want %q", *ran, want)
	}
}

func TestGlobalWithoutPackagesIsAnError(t *testing.T) {
	withConfig(t, config.Config{})
	inProject(t, "package.json")
	ran := recordTools(t)
	if code := cmdInstall([]string{"-g"}); code != 1 || len(*ran) != 0 {
		t.Fatalf("code=%d ran=%v", code, *ran)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ -run 'DetectPython|ResolveTargetPM|ProjectArgs|CleanDirs|ProjectCommands|PoetryAndPipenv|CIDeletes|GlobalWithout' -v`
Expected: FAIL. This is a build error: `undefined: runTool`, `undefined: pmExists`, `undefined: resolveTargetPM`, `undefined: projectCmd`, `undefined: cleanDirs`.

- [ ] **Step 3: Implement**

Make the deletions listed under **Files**, then create `internal/cli/project.go`:

```go
package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/crenspire/xpm/internal/pm"
)

// Seams for tests: running a project tool and checking it is on PATH.
var (
	runTool  = runBinaryWithCode
	pmExists = pm.Exists
)

// projectTarget is one project type detected in the current directory.
type projectTarget struct {
	Label string
	Kind  string
	PMs   []pm.ID
}

// detectProjectTargets lists the project types in the current directory.
// Python projects resolve to one tool: Pipfile -> pipenv, poetry.lock ->
// poetry, otherwise pip (requirements.txt and/or pyproject.toml).
func detectProjectTargets() []projectTarget {
	var targets []projectTarget
	add := func(file bool, label, kind string, pms ...pm.ID) {
		if file {
			targets = append(targets, projectTarget{Label: label, Kind: kind, PMs: pms})
		}
	}
	add(fileExists("package.json"), "Node (package.json)", "node", pm.Npm, pm.Yarn, pm.Pnpm, pm.Bun)
	add(fileExists("composer.json"), "PHP (composer.json)", "composer", pm.Composer)
	add(fileExists("Cargo.toml"), "Rust (Cargo.toml)", "cargo", pm.Cargo)
	add(fileExists("go.mod"), "Go (go.mod)", "gomod", pm.GoMod)
	add(fileExists("pom.xml"), "Java (pom.xml)", "maven", pm.Maven)
	add(fileExists("build.gradle") || fileExists("build.gradle.kts"), "Java (Gradle build.gradle)", "gradle", pm.Gradle)
	switch {
	case fileExists("Pipfile") || fileExists("Pipfile.lock"):
		add(true, "Python (Pipfile)", "pipenv", pm.Pipenv)
	case fileExists("poetry.lock"):
		add(true, "Python (poetry.lock)", "poetry", pm.Poetry)
	default:
		add(fileExists("requirements.txt"), "Python (requirements.txt)", "pip-req", pm.Pip)
		add(fileExists("pyproject.toml"), "Python (pyproject.toml)", "pip-pyproject", pm.Pip)
	}
	return targets
}

// pmChoice is how a project's tool was decided: PM is set when it is
// decided (Via names the file or "prefer"), Options when the user must pick.
type pmChoice struct {
	PM      pm.ID
	Via     string
	Options []pm.ID
}

// resolveTargetPM picks the tool for t from the project's lock files
// (files, for t's ecosystem) and the prefer list. One lock file decides;
// several are narrowed by prefer or left to the user; none falls back to
// prefer, then to the ecosystem default (npm).
func resolveTargetPM(t projectTarget, files []pm.ProjectFile, prefer []string) pmChoice {
	if len(t.PMs) == 1 {
		return pmChoice{PM: t.PMs[0]}
	}
	allowed := map[pm.ID]bool{}
	for _, id := range t.PMs {
		allowed[id] = true
	}
	var locked []pm.ProjectFile
	for _, f := range files {
		if allowed[f.Manager] {
			locked = append(locked, f)
		}
	}
	if len(locked) == 1 {
		return pmChoice{PM: locked[0].Manager, Via: locked[0].Name}
	}
	options := t.PMs
	if len(locked) > 1 {
		options = nil
		for _, f := range locked {
			options = append(options, f.Manager)
		}
	}
	if p := pickPreferredPM(options, prefer); p != "" {
		return pmChoice{PM: p, Via: "prefer"}
	}
	if len(locked) > 1 {
		return pmChoice{Options: options}
	}
	return pmChoice{PM: options[0]}
}

// projectCmd is a resolved project: its kind and the tool that runs for it.
type projectCmd struct {
	Kind      string
	PM        pm.ID
	YarnBerry bool // yarn >= 2 (.yarnrc.yml): different flags
}

// manualError means the tool has no command for an action; its message
// tells the user what to do instead. It is not a failure (exit 0).
type manualError struct{ msg string }

func (e manualError) Error() string { return e.msg }

// withPkg appends pkg when it is set.
func withPkg(args []string, pkg string) []string {
	if pkg == "" {
		return args
	}
	return append(args, pkg)
}

// args returns the native command line for action ("install", "ci",
// "list", "update", "remove"); pkg is the package for update/remove.
// "ci" is a frozen install that fails instead of changing the lock file.
func (c projectCmd) args(action, pkg string) ([]string, error) {
	switch c.Kind {
	case "node":
		return c.nodeArgs(action, pkg)
	case "composer":
		return pick(action, pkg, []string{"install"}, []string{"install"}, []string{"show"},
			withPkg([]string{"update"}, pkg), []string{"remove", pkg})
	case "cargo":
		update := []string{"update"}
		if pkg != "" {
			update = []string{"update", "-p", pkg}
		}
		return pick(action, pkg, []string{"build"}, []string{"build", "--locked"}, []string{"tree", "--depth", "1"},
			update, []string{"remove", pkg})
	case "gomod":
		if action == "remove" {
			return nil, manualError{"To remove a Go dependency, delete its imports and run: go mod tidy"}
		}
		update := []string{"get", "-u", "./..."}
		if pkg != "" {
			update = []string{"get", "-u", pkg}
		}
		return pick(action, pkg, []string{"mod", "tidy"}, []string{"mod", "download"}, []string{"list", "-m", "all"}, update, nil)
	case "maven":
		if action == "remove" {
			return nil, manualError{fmt.Sprintf("Remove the dependency for %s from pom.xml.", pkg)}
		}
		return pick(action, pkg, []string{"install"}, []string{"install"}, []string{"dependency:list"},
			[]string{"versions:use-latest-releases"}, nil)
	case "gradle":
		switch action {
		case "update":
			return nil, manualError{"Gradle has no built-in update command; consider the Gradle Versions Plugin."}
		case "remove":
			return nil, manualError{fmt.Sprintf("Remove the dependency for %s from build.gradle.", pkg)}
		}
		return pick(action, pkg, []string{"build"}, []string{"build"},
			[]string{"dependencies", "--configuration", "implementation"}, nil, nil)
	case "pip-req":
		update := []string{"install", "--upgrade", "-r", "requirements.txt"}
		if pkg != "" {
			update = []string{"install", "--upgrade", pkg}
		}
		return pick(action, pkg, []string{"install", "-r", "requirements.txt"}, []string{"install", "-r", "requirements.txt"},
			[]string{"list"}, update, []string{"uninstall", "-y", pkg})
	case "pip-pyproject":
		update := []string{"install", "--upgrade", "."}
		if pkg != "" {
			update = []string{"install", "--upgrade", pkg}
		}
		return pick(action, pkg, []string{"install", "."}, []string{"install", "."}, []string{"list"}, update, []string{"uninstall", "-y", pkg})
	case "poetry":
		return pick(action, pkg, []string{"install"}, []string{"install"}, []string{"show"},
			withPkg([]string{"update"}, pkg), []string{"remove", pkg})
	case "pipenv":
		return pick(action, pkg, []string{"install"}, []string{"install", "--deploy"}, []string{"graph"},
			withPkg([]string{"update"}, pkg), []string{"uninstall", pkg})
	}
	return nil, fmt.Errorf("unknown project kind %q", c.Kind)
}

func (c projectCmd) nodeArgs(action, pkg string) ([]string, error) {
	switch action {
	case "install":
		return []string{"install"}, nil
	case "ci":
		switch {
		case c.PM == pm.Npm:
			return []string{"ci"}, nil
		case c.PM == pm.Yarn && c.YarnBerry:
			return []string{"install", "--immutable"}, nil
		default: // yarn 1, pnpm, bun
			return []string{"install", "--frozen-lockfile"}, nil
		}
	case "list":
		switch {
		case c.PM == pm.Bun:
			return []string{"pm", "ls"}, nil
		case c.PM == pm.Yarn && c.YarnBerry:
			return []string{"info", "--name-only"}, nil
		default:
			return []string{"list", "--depth=0"}, nil
		}
	case "update":
		switch {
		case c.PM == pm.Yarn && c.YarnBerry:
			if pkg == "" {
				pkg = "*"
			}
			return []string{"up", pkg}, nil
		case c.PM == pm.Yarn:
			return withPkg([]string{"upgrade"}, pkg), nil
		default:
			return withPkg([]string{"update"}, pkg), nil
		}
	case "remove":
		if c.PM == pm.Npm {
			return []string{"uninstall", pkg}, nil
		}
		return []string{"remove", pkg}, nil
	}
	return nil, fmt.Errorf("unknown action %q", action)
}

// pick returns the args for action from the per-action lists. A nil list
// means the tool has no such command.
func pick(action, pkg string, install, ci, list, update, remove []string) ([]string, error) {
	var out []string
	switch action {
	case "install":
		out = install
	case "ci":
		out = ci
	case "list":
		out = list
	case "update":
		out = update
	case "remove":
		if pkg == "" {
			return nil, errors.New("remove needs a package name")
		}
		out = remove
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
	if out == nil {
		return nil, fmt.Errorf("%s is not supported for this project", action)
	}
	return out, nil
}

// cleanDirs are the directories `xpm ci` offers to delete before a frozen
// install: node_modules for yarn/pnpm/bun (npm ci clears it itself) and
// vendor/ only for Composer (Go's vendor/ is source and is never touched).
func cleanDirs(c projectCmd) []string {
	switch {
	case c.Kind == "node" && c.PM != pm.Npm:
		return []string{"node_modules"}
	case c.Kind == "composer":
		return []string{"vendor"}
	}
	return nil
}

// selectTarget picks the project to act on: the only one, a prompt, or
// (non-interactive) the first.
func selectTarget(targets []projectTarget, label string) (projectTarget, bool) {
	if len(targets) == 1 || !cfg.Interactive {
		return targets[0], true
	}
	items := make([]string, len(targets))
	for i, t := range targets {
		items[i] = t.Label
	}
	idx, _, err := (&promptui.Select{Label: label, Items: items}).Run()
	if err != nil {
		fmt.Println("Cancelled.")
		return projectTarget{}, false
	}
	return targets[idx], true
}

// projectCmdFor decides which tool runs for t in the current directory.
func projectCmdFor(t projectTarget) (projectCmd, error) {
	files := pm.ProjectManagers(".")[pm.EcosystemForManager(t.PMs[0])]
	choice := resolveTargetPM(t, files, cfg.Prefer)
	id := choice.PM
	switch {
	case id == "" && !cfg.Interactive:
		id = choice.Options[0]
		fmt.Printf("Non-interactive mode: several lock files found, using %s\n", id)
	case id == "":
		items := make([]string, len(choice.Options))
		for i, o := range choice.Options {
			items[i] = string(o)
		}
		idx, _, err := (&promptui.Select{Label: "Several lock files found; package manager for " + t.Label, Items: items}).Run()
		if err != nil {
			return projectCmd{}, errors.New("cancelled")
		}
		id = choice.Options[idx]
	case choice.Via != "" && choice.Via != "prefer":
		fmt.Printf("Detected %s - using %s\n", choice.Via, id)
	}
	return projectCmd{Kind: t.Kind, PM: id, YarnBerry: id == pm.Yarn && fileExists(".yarnrc.yml")}, nil
}

// runProject runs action for t with its native tool and returns the exit code.
func runProject(t projectTarget, action, pkg string) int {
	pc, err := projectCmdFor(t)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	meta, _ := pm.MetaFor(pc.PM)
	if pkg != "" {
		if err := pm.ValidatePackageName(pkg, pc.PM); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid package name: %v\n", err)
			return 1
		}
	}
	args, err := pc.args(action, pkg)
	var manual manualError
	if errors.As(err, &manual) {
		fmt.Println(manual.msg)
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if action == "install" || action == "ci" {
		if err := ensurePM(pc.PM); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	} else if !pmExists(meta.Binary) {
		fmt.Printf("%s (%s) is not installed.\n", meta.Name, meta.Binary)
		return 1
	}
	fmt.Printf("Running: %s %s\n\n", meta.Binary, strings.Join(args, " "))
	return runTool(meta.Binary, args)
}

// noProjectFiles is printed when no project type is detected.
const noProjectFiles = "No known dependency files found (package.json, composer.json, etc)."

// autoInstallDetected installs dependencies for the detected projects:
// all of them, or the one the user picks.
func autoInstallDetected() int {
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	if len(targets) > 1 && cfg.Interactive {
		items := make([]string, 0, len(targets)+1)
		for _, t := range targets {
			items = append(items, t.Label)
		}
		items = append(items, "Run All")
		idx, _, err := (&promptui.Select{Label: "Detected multiple project types", Items: items}).Run()
		if err != nil {
			fmt.Println("Cancelled.")
			return 1
		}
		if idx < len(targets) {
			targets = targets[idx : idx+1]
		}
	} else if len(targets) > 1 {
		fmt.Println("Non-interactive mode: running installs for all detected project types.")
	}
	return runEach(targets, "install")
}

// runEach runs action for every target, stopping at the first failure.
func runEach(targets []projectTarget, action string) int {
	for _, t := range targets {
		fmt.Printf("Detected: %s\n", t.Label)
		if code := runProject(t, action, ""); code != 0 {
			return code
		}
	}
	return 0
}

// cmdCleanInstall (`xpm ci`) runs each detected project's frozen install
// (npm ci, pnpm/yarn/bun --frozen-lockfile, yarn --immutable, composer
// install, pip install -r, pipenv install --deploy, cargo build --locked,
// go mod download). It never deletes lock files, and deletes
// node_modules/ or vendor/ only after the user confirms.
func cmdCleanInstall(args []string) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "error: ci takes no arguments, got: %s\n", strings.Join(args, " "))
		return 1
	}
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	for _, t := range targets {
		fmt.Printf("Detected: %s\n", t.Label)
		pc, err := projectCmdFor(t)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		for _, dir := range cleanDirs(pc) {
			if !fileExists(dir) {
				continue
			}
			if yes, err := askYesNo(fmt.Sprintf("Delete %s/ before the frozen install?", dir)); err == nil && yes {
				fmt.Printf("Removing %s/\n", dir)
				if err := os.RemoveAll(dir); err != nil {
					fmt.Fprintln(os.Stderr, "error:", err)
					return 1
				}
			}
		}
		if code := runProject(t, "ci", ""); code != 0 {
			return code
		}
	}
	return 0
}

// cmdList lists installed packages for the detected project.
func cmdList(args []string) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "error: list takes no arguments, got: %s\n", strings.Join(args, " "))
		return 1
	}
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	t, ok := selectTarget(targets, "Select project to list packages for")
	if !ok {
		return 1
	}
	fmt.Printf("Listing packages for %s\n\n", t.Label)
	return runProject(t, "list", "")
}

// cmdUpdate updates one package, or all of them, in the detected project.
func cmdUpdate(args []string) int {
	pkg, ok := atMostOneArg("update", args)
	if !ok {
		return 1
	}
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	t, ok := selectTarget(targets, "Select project to update")
	if !ok {
		return 1
	}
	fmt.Printf("Updating packages for %s\n\n", t.Label)
	return runProject(t, "update", pkg)
}

// cmdRemove removes a package from the detected project.
func cmdRemove(args []string) int {
	pkg, ok := exactlyOneArg("remove", args)
	if !ok {
		return 1
	}
	targets := detectProjectTargets()
	if len(targets) == 0 {
		fmt.Println(noProjectFiles)
		return 1
	}
	t, ok := selectTarget(targets, "Select project to remove package from")
	if !ok {
		return 1
	}
	fmt.Printf("Removing %s from %s\n\n", pkg, t.Label)
	return runProject(t, "remove", pkg)
}
```

In `internal/cli/install.go` (`cmdInstall`), replace

```go
	if len(ia.Packages) == 0 {
		return autoInstallDetected(ia.Global)
	}
```

with

```go
	if len(ia.Packages) == 0 {
		if ia.Global {
			fmt.Fprintln(os.Stderr, "error: -g/--global needs a package name (project dependencies are never global)")
			return 1
		}
		return autoInstallDetected()
	}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/cli/`
Expected: `ok`. `TestProjectCommandsUseTheLockfileTool` records `pnpm install`, `pnpm list --depth=0`, `pnpm update axios`, `pnpm remove axios` and `pnpm install --frozen-lockfile`. `TestCIDeletesNothingWithoutConfirmation` leaves `vendor/`, `node_modules/` and both lockfiles in place. The existing `TestDetectProjectTargets` still passes.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/cli
git commit -m "cli: list/update/remove/install use the project's lockfile tool; xpm ci runs native frozen installs and deletes nothing unasked"
```

---

### Task 11: No TTY means non-interactive, and non-interactive never guesses when a registry was down (roadmap #8)

> Ordering: this task depends on Task 5 (roadmap #13). The non-interactive pick refuses when `Report.Unavailable` is non-empty. Without #13 a timed-out registry would look like "not found" and xpm would silently install the wrong ecosystem's package.

**Files:**
- Create: `internal/cli/tty.go`, `internal/cli/tty_test.go`
- Modify: `internal/cli/cli.go` (`Run`), `internal/cli/install.go` (`chooseCandidate` and its call in `installOne`)

**Interfaces:**
- Consumes:
  - from Task 3: `search.Report.UnavailableIDs()`;
  - from Task 5: `managerName`;
  - from Task 7: `candidate`;
  - `golang.org/x/term`.
- Produces:
  - `var isInteractiveTerminal func() bool`: true when stdin and stdout are both terminals. It is a test seam.
  - `func effectiveInteractive(configured, tty bool) bool`
  - `func nonInteractivePick(cands []candidate, unavailable []pm.ID) (candidate, error)`
  - `func chooseCandidate(cands []candidate, unavailable []pm.ID) (candidate, bool)`. **Signature change.**
- Behaviour:
  - `Run` sets `cfg.Interactive = effectiveInteractive(cfg.Interactive, isInteractiveTerminal())`. Pipes, CI and `| cat` never block on a prompt.
  - With one candidate and every registry answered, xpm picks automatically.
  - With an unavailable registry:
    - interactive: always prompt, even for one candidate;
    - non-interactive: refuse and exit 1, with `error: not choosing automatically because maven (Java) did not answer and the list of matches may be incomplete; retry, or turn that registry off with `xpm config set search.maven false``.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/tty_test.go`:

```go
package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

func TestEffectiveInteractive(t *testing.T) {
	if effectiveInteractive(true, false) || effectiveInteractive(false, true) || !effectiveInteractive(true, true) {
		t.Fatal("prompts need both the config and a terminal")
	}
}

func TestRunWithoutATerminalIsNonInteractive(t *testing.T) {
	withConfig(t, cfg)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	oldArgs, oldTTY := os.Args, isInteractiveTerminal
	t.Cleanup(func() { os.Args, isInteractiveTerminal = oldArgs, oldTTY })
	os.Args = []string{"xpm", "version"}

	for _, tty := range []bool{false, true} {
		isInteractiveTerminal = func() bool { return tty }
		captureStdout(t, func() { Run() })
		if cfg.Interactive != tty {
			t.Errorf("tty=%v: cfg.Interactive = %v (default config says interactive)", tty, cfg.Interactive)
		}
	}
}

func TestNonInteractiveRefusesToGuessWhenARegistryWasDown(t *testing.T) {
	cands := []candidate{{Result: npmAxios}, {Result: pipAxios}}
	if c, err := nonInteractivePick(cands, nil); err != nil || c.Result.Manager != pm.Npm {
		t.Fatalf("all registries answered: got (%+v, %v), want the first candidate", c, err)
	}
	_, err := nonInteractivePick(cands[:1], []pm.ID{pm.Maven})
	if err == nil || !strings.Contains(err.Error(), "maven (Java) did not answer") {
		t.Fatalf("err = %v", err)
	}
}

func TestInstallWithoutTerminalAndMissingRegistryExitsOne(t *testing.T) {
	withConfig(t, config.Config{Interactive: false})
	inProject(t, "package.json", "package-lock.json")
	ran := recordTools(t)
	withLookupReport(t, search.Report{
		Results:     []search.Result{npmAxios},
		Unavailable: []search.RegistryFailure{timedOut},
	}, nil)
	var code int
	captureStdout(t, func() { code = cmdInstall([]string{"axios"}) })
	if code != 1 || len(*ran) != 0 {
		t.Fatalf("code=%d ran=%v: must not install a guess while maven was down", code, *ran)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ -run 'EffectiveInteractive|WithoutATerminal|NonInteractiveRefuses|InstallWithoutTerminal' -v`
Expected: FAIL. This is a build error: `undefined: effectiveInteractive`, `undefined: isInteractiveTerminal`, `undefined: nonInteractivePick`.

- [ ] **Step 3: Implement**

Create `internal/cli/tty.go`:

```go
package cli

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/crenspire/xpm/internal/pm"
)

// isInteractiveTerminal reports whether prompts can work: both stdin and
// stdout are terminals. A seam for tests.
var isInteractiveTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// effectiveInteractive is whether xpm may prompt: the config allows it and
// a terminal is attached. Pipes, CI and `xpm ... | cat` never hang on a prompt.
func effectiveInteractive(configured, tty bool) bool {
	return configured && tty
}

// nonInteractivePick chooses a candidate without asking. It refuses when a
// registry did not answer: the missing hit might have been the right one.
func nonInteractivePick(cands []candidate, unavailable []pm.ID) (candidate, error) {
	if len(unavailable) > 0 {
		names := make([]string, len(unavailable))
		for i, id := range unavailable {
			names[i] = managerName(id)
		}
		return candidate{}, fmt.Errorf("not choosing automatically because %s did not answer and the list of matches may be incomplete; retry, or turn that registry off with `xpm config set search.%s false`",
			strings.Join(names, ", "), unavailable[0])
	}
	return cands[0], nil
}
```

In `Run` (`cli.go`), directly after `cfg = config.Load()`, add:

```go
	cfg.Interactive = effectiveInteractive(cfg.Interactive, isInteractiveTerminal())
```

In `installOne` (`install.go`), change `chosen, ok := chooseCandidate(cands)` to `chosen, ok := chooseCandidate(cands, rep.UnavailableIDs())`, and replace `chooseCandidate` with:

```go
// chooseCandidate picks automatically when there is exactly one candidate
// and every registry answered; otherwise it prompts or, without a terminal,
// uses nonInteractivePick.
func chooseCandidate(cands []candidate, unavailable []pm.ID) (candidate, bool) {
	if len(cands) == 1 && len(unavailable) == 0 {
		if c := cands[0]; c.Via != "" {
			fmt.Printf("Detected %s - using %s\n\n", c.Via, c.Result.Manager)
		}
		return cands[0], true
	}
	labels := make([]string, len(cands))
	for i, c := range cands {
		labels[i] = candidateLabel(c)
	}
	if !cfg.Interactive {
		c, err := nonInteractivePick(cands, unavailable)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return candidate{}, false
		}
		fmt.Println("Non-interactive mode: picking", labels[0])
		return c, true
	}
	prompt := promptui.Select{Label: "Select package manager to install from", Items: labels}
	idx, _, err := prompt.Run()
	if err != nil {
		fmt.Println("Cancelled.")
		return candidate{}, false
	}
	return cands[idx], true
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/cli/`
Expected: `ok`. The existing `TestRun*` tests still pass; `go test` has no TTY, so they now run non-interactively.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/cli
git commit -m "cli: without a terminal xpm is non-interactive and refuses to guess when a registry did not answer"
```

---

### Task 12: `InstallPM` never pipes a remote script into a shell; re-check `PATH` after installing

**Files:**
- Modify: `internal/pm/pm.go`: replace everything from `// runCommand executes a command with the given binary and arguments.` to the end of the file; the imports change `"runtime"` → `"strings"`. Also `internal/cli/install.go` (`ensureManager`).
- Create: `internal/pm/install_test.go`

**Decision — Composer: print the official instructions; don't verify and run the installer.**
- **Same rule for every tool.** Bun and rustup are also script installers. "xpm never runs a remote install script" is one rule users and reviewers can check.
- **It wouldn't work anyway.** The verify-and-run path would still need `sudo mv composer.phar /usr/local/bin`. That prompts for a password in the middle of an install and breaks on Windows. Today's script already does exactly that.
- **No new network code to maintain.** Verifying `composer-setup.php` against `https://composer.github.io/installer.sig` (SHA-384) would add a second network-and-crypto path in `pm` with no `httptest` seam. Composer's own instructions already include that check.

**Interfaces:**
- Produces (package `pm`):
  - `func SetCommandRunner(fn func(bin string, args ...string) error) (restore func())`: every `Wrap` (all adapters) and `InstallPM` go through it.
  - `func SetLookPath(fn func(file string) (string, error)) (restore func())`: used by `Exists` and `InstallPM`.
  - `type ManualInstallError struct { Manager ID; Steps string }`, returned for bun, cargo (rustup), composer, gomod, maven, gradle and npm.
  - `func InstallPM(id ID) error`:
    - runs pnpm/yarn via `npm install -g`;
    - runs pip via `python3`/`python -m ensurepip --upgrade`;
    - runs poetry via `pipx`, and pipenv via `pipx`, falling back to `python3 -m pip install --user`;
    - uses `exec` with argv, never `sh -c`, then re-checks `lookPath(meta.Binary)`.
  - `func InstallHint(id ID) string`: never contains `| sh` or `| bash`.
- Removed: `runShell`. It is gone along with its Windows branch, which printed a command and returned `nil`, so it reported success when nothing ran.
- `cli.ensureManager` turns a `*pm.ManualInstallError` into: `bun (bun) is not installed; xpm does not run remote install scripts. To install it, see https://bun.sh/docs/installation, then re-run xpm`.

- [ ] **Step 1: Write the failing tests**

Create `internal/pm/install_test.go`:

```go
package pm

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// fakeTools makes exactly the binaries in onPath "exist" and records every
// command InstallPM runs. Running a command named in `installs` puts that
// binary on PATH afterwards.
func fakeTools(t *testing.T, onPath []string, installs map[string]string) *[]string {
	t.Helper()
	path := map[string]bool{}
	for _, b := range onPath {
		path[b] = true
	}
	var ran []string
	restoreRun := SetCommandRunner(func(bin string, args ...string) error {
		cmd := bin + " " + strings.Join(args, " ")
		ran = append(ran, cmd)
		if b, ok := installs[cmd]; ok {
			path[b] = true
		}
		return nil
	})
	restoreLook := SetLookPath(func(file string) (string, error) {
		if path[file] {
			return "/usr/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	})
	t.Cleanup(func() { restoreRun(); restoreLook() })
	return &ran
}

func TestInstallPMNeverRunsRemoteScripts(t *testing.T) {
	ran := fakeTools(t, []string{"curl", "sh", "bash", "php"}, nil)
	for id, url := range map[ID]string{Bun: "https://bun.sh", Cargo: "https://rustup.rs", Composer: "https://getcomposer.org"} {
		err := InstallPM(id)
		var manual *ManualInstallError
		if !errors.As(err, &manual) || !strings.Contains(manual.Steps, url) {
			t.Errorf("InstallPM(%s) = %v, want a ManualInstallError pointing at %s", id, err, url)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %v; nothing may be executed for script-installed tools", *ran)
	}
}

func TestInstallPMRechecksPath(t *testing.T) {
	ran := fakeTools(t, []string{"npm"}, nil) // npm "succeeds" but pnpm never appears
	err := InstallPM(Pnpm)
	if err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("err = %v, want a not-on-PATH error", err)
	}
	if len(*ran) != 1 || (*ran)[0] != "npm install -g pnpm" {
		t.Fatalf("ran %v", *ran)
	}
}

func TestInstallPMSucceedsWhenBinaryAppears(t *testing.T) {
	fakeTools(t, []string{"npm"}, map[string]string{"npm install -g yarn": "yarn"})
	if err := InstallPM(Yarn); err != nil {
		t.Fatal(err)
	}
}

func TestInstallPMFallsBackToTheNextInstaller(t *testing.T) {
	ran := fakeTools(t, []string{"python"}, map[string]string{"python -m ensurepip --upgrade": "pip"})
	if err := InstallPM(Pip); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 1 || (*ran)[0] != "python -m ensurepip --upgrade" {
		t.Fatalf("ran %v, want only the python fallback (python3 is missing)", *ran)
	}
}

func TestInstallPMWithoutAnyInstallerFails(t *testing.T) {
	ran := fakeTools(t, nil, nil)
	if err := InstallPM(Poetry); err == nil || !strings.Contains(err.Error(), "pipx is not installed") {
		t.Fatalf("err = %v", err)
	}
	if len(*ran) != 0 {
		t.Fatalf("ran %v", *ran)
	}
}

func TestInstallHintsNeverPipeToAShell(t *testing.T) {
	for _, m := range AllMetas() {
		if h := InstallHint(m.ID); strings.Contains(h, "| sh") || strings.Contains(h, "| bash") {
			t.Errorf("InstallHint(%s) = %q suggests piping a download into a shell", m.ID, h)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/pm/ -run 'InstallPM|InstallHints' -v`
Expected: FAIL. This is a build error: `undefined: SetCommandRunner`, `undefined: SetLookPath`, `undefined: ManualInstallError`.

- [ ] **Step 3: Implement**

In `internal/pm/pm.go`, change the import `"runtime"` to `"strings"`, and replace everything from `// runCommand executes a command with the given binary and arguments.` to the end of the file with:

```go
// Seams: tests (here and, via SetCommandRunner/SetLookPath, in other
// packages) replace how commands run and how PATH is searched.
var (
	execRunner = runCommand
	lookPath   = exec.LookPath
)

// SetCommandRunner replaces how adapters and InstallPM run commands and
// returns a function that restores the real runner. For tests: with a fake
// runner nothing is executed. Not safe to call concurrently with installs.
func SetCommandRunner(fn func(bin string, args ...string) error) (restore func()) {
	old := execRunner
	execRunner = fn
	return func() { execRunner = old }
}

// SetLookPath replaces the PATH lookup used by Exists and InstallPM and
// returns a function that restores exec.LookPath. For tests.
func SetLookPath(fn func(file string) (string, error)) (restore func()) {
	old := lookPath
	lookPath = fn
	return func() { lookPath = old }
}

// runCommand executes a command with the given binary and arguments.
// stdout, stderr, and stdin are connected to the current process.
func runCommand(bin string, args ...string) error {
	logx.Info("running command: %s %v", bin, args)
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// Wrap executes a command and wraps any error with context.
// This is a convenience function used by adapters.
func Wrap(bin string, args []string) error {
	if err := execRunner(bin, args...); err != nil {
		return fmt.Errorf("%s %v failed: %w", bin, args, err)
	}
	return nil
}

// Exists checks if a binary is available in the system PATH.
func Exists(binary string) bool {
	_, err := lookPath(binary)
	return err == nil
}

// ManualInstallError means xpm will not install a tool itself (its official
// installer is a remote script or a whole toolchain). Steps says how.
type ManualInstallError struct {
	Manager ID
	Steps   string
}

func (e *ManualInstallError) Error() string {
	return fmt.Sprintf("%s must be installed manually: %s", e.Manager, e.Steps)
}

// installers are tools xpm installs with another tool that is already
// present, trying each command in order. Nothing is piped to a shell.
var installers = map[ID][][]string{
	Pnpm:   {{"npm", "install", "-g", "pnpm"}},
	Yarn:   {{"npm", "install", "-g", "yarn"}},
	Pip:    {{"python3", "-m", "ensurepip", "--upgrade"}, {"python", "-m", "ensurepip", "--upgrade"}},
	Poetry: {{"pipx", "install", "poetry"}},
	Pipenv: {{"pipx", "install", "pipenv"}, {"python3", "-m", "pip", "install", "--user", "pipenv"}},
}

// manualSteps are the official instructions for tools xpm won't install.
var manualSteps = map[ID]string{
	Bun:      "see https://bun.sh/docs/installation",
	Cargo:    "install Rust with rustup, see https://rustup.rs",
	Composer: "follow https://getcomposer.org/download/ (its installer is verified against https://composer.github.io/installer.sig)",
	GoMod:    "install Go from https://go.dev/dl/",
	Maven:    "install Maven from https://maven.apache.org/download.cgi",
	Gradle:   "install Gradle from https://gradle.org/install/",
	Npm:      "npm comes with Node.js; install Node from https://nodejs.org/",
}

// InstallPM installs a package manager using a tool that is already on
// PATH (npm for pnpm/yarn, python for pip, pipx for poetry/pipenv), then
// checks that the new binary is on PATH. Tools whose official installer is
// a remote script (bun, rustup, composer) are never run: a
// *ManualInstallError carries the official instructions instead.
func InstallPM(id ID) error {
	if steps, ok := manualSteps[id]; ok {
		return &ManualInstallError{Manager: id, Steps: steps}
	}
	attempts, ok := installers[id]
	if !ok {
		return fmt.Errorf("auto-install not implemented for %s", id)
	}
	meta, _ := MetaFor(id)
	var lastErr error
	for _, a := range attempts {
		if _, err := lookPath(a[0]); err != nil {
			lastErr = fmt.Errorf("%s is not installed", a[0])
			continue
		}
		fmt.Printf("Installing %s: %s\n", meta.Name, strings.Join(a, " "))
		if err := execRunner(a[0], a[1:]...); err != nil {
			lastErr = fmt.Errorf("%s failed: %w", strings.Join(a, " "), err)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return fmt.Errorf("could not install %s: %w", meta.Name, lastErr)
	}
	if _, err := lookPath(meta.Binary); err != nil {
		return fmt.Errorf("%s was installed but %q is not on PATH yet; open a new shell or add its bin directory to PATH", meta.Name, meta.Binary)
	}
	return nil
}

// InstallHint returns how to install the specified package manager, or "".
func InstallHint(id ID) string {
	if steps, ok := manualSteps[id]; ok {
		return steps
	}
	if attempts, ok := installers[id]; ok {
		return strings.Join(attempts[0], " ")
	}
	return ""
}
```

In `internal/cli/install.go`, add `"errors"` to the imports, and in `ensureManager` replace

```go
	if err := pm.InstallPM(id); err != nil {
		return fmt.Errorf("failed to install %s: %w", meta.Name, err)
	}
```

with

```go
	if err := pm.InstallPM(id); err != nil {
		var manual *pm.ManualInstallError
		if errors.As(err, &manual) {
			return fmt.Errorf("%s is not installed; xpm does not run remote install scripts. To install it, %s, then re-run xpm", meta.Name, manual.Steps)
		}
		return fmt.Errorf("failed to install %s: %w", meta.Name, err)
	}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/pm/ ./internal/cli/`
Expected: two `ok` lines. `TestInstallPMNeverRunsRemoteScripts` runs nothing even with `curl`, `sh`, `bash` and `php` "on PATH". The existing `TestWrap` (real `echo`/`false`) and `TestInstallHint` still pass.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/pm internal/cli/install.go
git commit -m "security(pm): never pipe remote install scripts to a shell; print official steps; re-check PATH after installing"
```

---
### Task 13: TUI — `q`/`j`/`k` type, real debounce with sequence IDs, rune-safe text, `searchUI.*` honoured (roadmap #9)

**Files:**
- Replace: `internal/tui/search/model.go`, `internal/tui/search/commands.go`, `internal/tui/search/run.go`, `internal/tui/search/update.go`, `internal/tui/search/update_test.go` (whole files)
- Modify: `internal/tui/search/view.go` (5 edits), `internal/cli/search_cmd.go` (`cmdSearch`)

**Interfaces:**
- Consumes:
  - from Task 3: `search.SearchEverywhereParallel` (deadline + cache), `search.Registries`, `search.Enabled`;
  - from Task 8: `installManagersFor`.
- Produces (package `tui/search`):
  - `type UIOptions struct { DebounceMs int; PageSize int }`
  - `func NewModel(initialQuery string, opts search.Options, ui UIOptions) model`
  - `func Run(initialQuery string, opts search.Options, ui UIOptions) (*SearchResult, error)`. It returns an error without a terminal; the plain-text fallback `runNonInteractive` is deleted, because the CLI handles non-TTY in Task 5.
  - `func (m model) visibleRows() int`
  - `type debounceMsg struct{ seq int; query string }` and `type searchMsg struct{ seq int; results []search.Result; err error }`. `errMsg` and `handleErrMsg` are deleted.
  - `var runSearch = search.SearchEverywhereParallel` (test seam).
  - `func debounceCmd(seq int, query string, d time.Duration) tea.Cmd`
  - `func searchCmd(seq int, query string, opts search.Options) tea.Cmd`
  - `func queryChanged(m model) (model, tea.Cmd)`
  - `func dropLastRune(s string) string`
  - `var registryChoices`
  - `func applyRegistrySelection(m model) model`
  - `func truncate(s string, max int) string`: now counts runes.
- Behaviour:
  - **Keys.**
    - In query mode and on the registry screen, only arrows, Ctrl+P/N, PgUp/PgDn, Ctrl+U/D, Enter and Esc/Ctrl+C act; every printable key is typed. Install mode keeps `q`/`j`/`k`, because nothing is typed there.
    - Backspace removes a whole character.
  - **Debounce and stale results.**
    - Every query change bumps `seq` and starts one debounce timer. Only the timer whose `seq` is still current starts a search.
    - Results whose `seq` is old are dropped.
    - An in-flight older search is not cancelled, but it is bounded by the 2.5 s deadline (Task 3) and its result is ignored.
  - **Config.** `searchUI.debounceMs` (0 → 200 ms) and `searchUI.pageSize` (0 → fit the terminal) are used.

- [ ] **Step 1: Write the failing tests**

Replace `internal/tui/search/update_test.go` with the following. It keeps `TestInstallManagersFor` from Task 8.

```go
package search

import (
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

func TestInstallManagersFor(t *testing.T) {
	for id, want := range map[pm.ID]string{
		pm.Npm:   "[npm yarn pnpm bun]",
		pm.Pip:   "[pip poetry pipenv]",
		pm.Maven: "[maven gradle]",
		pm.Cargo: "[cargo]",
	} {
		if got := fmt.Sprint(installManagersFor(id)); got != want {
			t.Errorf("installManagersFor(%s) = %s, want %s", id, got, want)
		}
	}
}

func queryModel(t *testing.T) model {
	t.Helper()
	m := NewModel("", search.Options{}, UIOptions{DebounceMs: 1})
	m.registryMode = false
	return m
}

func press(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(model), cmd
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestQueryModeTypesQJK(t *testing.T) {
	m := queryModel(t)
	for _, k := range []string{"q", "j", "k"} {
		var cmd tea.Cmd
		m, cmd = press(t, m, runes(k))
		if isQuit(cmd) {
			t.Fatalf("typing %q quit the search", k)
		}
	}
	if m.query != "qjk" {
		t.Fatalf("query = %q, want qjk", m.query)
	}
}

func TestRegistryModeTypingQStartsTheQuery(t *testing.T) {
	m := NewModel("", search.Options{}, UIOptions{DebounceMs: 1})
	m, cmd := press(t, m, runes("q"))
	if isQuit(cmd) || m.registryMode || m.query != "q" {
		t.Fatalf("query=%q registryMode=%v quit=%v", m.query, m.registryMode, isQuit(cmd))
	}
}

func TestEscAndCtrlCQuit(t *testing.T) {
	for _, k := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		if _, cmd := press(t, queryModel(t), tea.KeyMsg{Type: k}); !isQuit(cmd) {
			t.Errorf("%v did not quit", k)
		}
	}
}

func TestArrowsAndCtrlKeysNavigate(t *testing.T) {
	m := queryModel(t)
	m.results = []search.Result{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlN})
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
}

func TestOnlyTheLatestKeystrokeSearches(t *testing.T) {
	var searched []string
	old := runSearch
	runSearch = func(q string, _ search.Options) ([]search.Result, error) {
		searched = append(searched, q)
		return []search.Result{{Manager: pm.Npm, Name: q}}, nil
	}
	t.Cleanup(func() { runSearch = old })

	m := queryModel(t)
	m, first := press(t, m, runes("a"))
	m, second := press(t, m, runes("x"))

	m, cmd := press(t, m, first().(debounceMsg))
	if cmd != nil {
		t.Fatal("the debounce of a superseded keystroke started a search")
	}
	m, cmd = press(t, m, second().(debounceMsg))
	if cmd == nil {
		t.Fatal("the latest keystroke did not start a search")
	}
	reply := cmd().(searchMsg)
	if len(searched) != 1 || searched[0] != "ax" {
		t.Fatalf("searched %v, want only [ax]", searched)
	}

	// A late reply for an older query must not overwrite newer results.
	stale := searchMsg{seq: reply.seq - 1, results: []search.Result{{Name: "a"}}}
	m, _ = press(t, m, stale)
	if len(m.results) != 0 || !m.loading {
		t.Fatalf("stale results applied: %+v", m.results)
	}
	m, _ = press(t, m, reply)
	if len(m.results) != 1 || m.results[0].Name != "ax" || m.loading {
		t.Fatalf("results = %+v loading=%v", m.results, m.loading)
	}
}

func TestBackspaceRemovesAWholeCharacter(t *testing.T) {
	m := queryModel(t)
	m.query = "café"
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.query != "caf" {
		t.Fatalf("query = %q, want caf", m.query)
	}
}

func TestTruncateByRune(t *testing.T) {
	got := truncate("日本語のパッケージ名", 5)
	if got != "日本..." || !utf8.ValidString(got) {
		t.Fatalf("truncate = %q", got)
	}
	if truncate("héllo", 10) != "héllo" || truncate("héllo", 0) != "" {
		t.Fatal("short strings must be unchanged and max 0 must give empty")
	}
}

func TestUIOptionsAreUsed(t *testing.T) {
	m := NewModel("", search.Options{}, UIOptions{DebounceMs: 50, PageSize: 10})
	m.height = 50
	if m.debounce != 50*time.Millisecond || m.visibleRows() != 10 {
		t.Fatalf("debounce=%v rows=%d, want 50ms and 10", m.debounce, m.visibleRows())
	}
	m = NewModel("", search.Options{}, UIOptions{})
	m.height = 50
	if m.debounce != defaultDebounce || m.visibleRows() != 45 {
		t.Fatalf("defaults: debounce=%v rows=%d", m.debounce, m.visibleRows())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/tui/... -v`
Expected: FAIL. This is a build error: `too many arguments in call to NewModel`, `undefined: UIOptions`, `undefined: debounceMsg`, `undefined: runSearch`, `undefined: defaultDebounce`.

- [ ] **Step 3: Implement the model, commands and runner**

Replace `internal/tui/search/model.go` with:

```go
// Package search provides an interactive TUI for package search.
package search

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// UIOptions are the searchUI.* config values.
type UIOptions struct {
	// DebounceMs is how long typing must pause before a search starts
	// (searchUI.debounceMs); 0 means 200 ms.
	DebounceMs int
	// PageSize caps the result rows shown at once (searchUI.pageSize);
	// 0 means as many as fit the terminal.
	PageSize int
}

// defaultDebounce is used when searchUI.debounceMs is unset.
const defaultDebounce = 200 * time.Millisecond

// model represents the TUI application state.
type model struct {
	query              string
	results            []search.Result
	cursor             int
	scrollOffset       int // Offset for pagination
	loading            bool
	err                error
	width              int
	height             int
	searchOpts         search.Options
	installMode        bool
	selectedResult     *search.Result
	installPMs         []pm.ID
	installCursor      int
	registryMode       bool           // Whether we're in registry selection mode
	registryCursor     int            // Cursor for registry selection
	selectedRegistries map[pm.ID]bool // Selected registries for search
	debounce           time.Duration  // pause before a query is searched
	pageSize           int            // max rows shown; 0 = fit the terminal
	seq                int            // bumped on every query change; older results are dropped
}

// NewModel creates a new TUI model with initial state.
func NewModel(initialQuery string, opts search.Options, ui UIOptions) model {
	selectedRegistries := make(map[pm.ID]bool)
	for _, reg := range search.Registries {
		selectedRegistries[reg] = search.Enabled(opts, reg)
	}
	debounce := time.Duration(ui.DebounceMs) * time.Millisecond
	if debounce <= 0 {
		debounce = defaultDebounce
	}
	return model{
		query:              initialQuery,
		searchOpts:         opts,
		results:            []search.Result{},
		width:              80,
		height:             24,
		registryMode:       true, // Start in registry selection mode
		selectedRegistries: selectedRegistries,
		debounce:           debounce,
		pageSize:           ui.PageSize,
	}
}

// Init returns the initial command to run.
func (m model) Init() tea.Cmd {
	if m.query != "" {
		return debounceCmd(m.seq, m.query, m.debounce)
	}
	return nil
}

// visibleRows is how many results fit on one page: the terminal height
// minus header, query, separator and footer, capped by pageSize.
func (m model) visibleRows() int {
	rows := m.height - 5
	if m.pageSize > 0 && m.pageSize < rows {
		rows = m.pageSize
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

// debounceMsg fires when typing has paused; seq identifies the keystroke.
type debounceMsg struct {
	seq   int
	query string
}

// searchMsg carries the results of the search started for seq.
type searchMsg struct {
	seq     int
	results []search.Result
	err     error
}

// installSelectMsg is sent when entering install mode.
type installSelectMsg struct {
	result search.Result
}
```

Replace `internal/tui/search/commands.go` with:

```go
package search

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crenspire/xpm/internal/search"
)

// runSearch is the registry search; tests replace it.
var runSearch = search.SearchEverywhereParallel

// debounceCmd waits d and then reports which query change (seq) it was
// for. Only the latest change starts a search.
func debounceCmd(seq int, query string, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return debounceMsg{seq: seq, query: query}
	})
}

// searchCmd searches for query; the reply carries seq so a slower, older
// search can never overwrite newer results.
func searchCmd(seq int, query string, opts search.Options) tea.Cmd {
	return func() tea.Msg {
		if query == "" {
			return searchMsg{seq: seq}
		}
		results, err := runSearch(query, opts)
		return searchMsg{seq: seq, results: results, err: err}
	}
}
```

Replace `internal/tui/search/run.go` with:

```go
package search

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// SearchResult holds the selected package and package manager for installation.
type SearchResult struct {
	Result search.Result
	PM     pm.ID
}

// Run starts the TUI search interface. It needs a terminal; callers print
// plain results otherwise. Returns the selected result and package manager,
// or nil if the user cancelled.
func Run(initialQuery string, opts search.Options, ui UIOptions) (*SearchResult, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, errors.New("the search UI needs a terminal")
	}

	p := tea.NewProgram(NewModel(initialQuery, opts, ui), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	final, ok := finalModel.(model)
	if !ok {
		return nil, fmt.Errorf("invalid model type")
	}
	if final.installMode && final.selectedResult != nil && final.installCursor < len(final.installPMs) {
		return &SearchResult{
			Result: *final.selectedResult,
			PM:     final.installPMs[final.installCursor],
		}, nil
	}
	return nil, nil // User cancelled
}
```

Replace `internal/tui/search/update.go` with:

```go
package search

import (
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// Update handles messages and updates the model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleKeyMsg(m, msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case debounceMsg:
		if msg.seq != m.seq {
			return m, nil // typing continued; a newer debounce is pending
		}
		return m, searchCmd(msg.seq, msg.query, m.searchOpts)
	case searchMsg:
		return handleSearchMsg(m, msg)
	case installSelectMsg:
		return handleInstallSelectMsg(m, msg)
	}
	return m, nil
}

// queryChanged starts a new debounce window for the current query.
func queryChanged(m model) (model, tea.Cmd) {
	m.seq++
	m.cursor = 0
	m.scrollOffset = 0
	m.loading = true
	return m, debounceCmd(m.seq, m.query, m.debounce)
}

// dropLastRune removes the last character (not byte) of s.
func dropLastRune(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

// handleKeyMsg handles keyboard input. In query mode only arrows, ctrl
// keys, Enter and Esc act; every printable key, including q/j/k, is typed.
func handleKeyMsg(m model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.installMode {
		return handleInstallKeyMsg(m, msg)
	}
	if m.registryMode {
		return handleRegistryKeyMsg(m, msg)
	}

	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyEnter:
		if m.cursor < len(m.results) {
			return enterInstallMode(m, m.results[m.cursor])
		}
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		if m.cursor > 0 {
			m.cursor--
			m = adjustScrollOffset(m)
		}
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		if m.cursor < len(m.results)-1 {
			m.cursor++
			m = adjustScrollOffset(m)
		}
		return m, nil
	case tea.KeyPgUp, tea.KeyCtrlU:
		return pageUp(m), nil
	case tea.KeyPgDown, tea.KeyCtrlD:
		return pageDown(m), nil
	case tea.KeyRunes:
		m.query += string(msg.Runes)
		return queryChanged(m)
	case tea.KeyBackspace, tea.KeyDelete:
		if m.query == "" {
			return m, nil
		}
		m.query = dropLastRune(m.query)
		return queryChanged(m)
	}
	return m, nil
}

// handleInstallKeyMsg handles keyboard input in install mode.
func handleInstallKeyMsg(m model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "esc":
		// Exit install mode
		m.installMode = false
		m.selectedResult = nil
		m.installPMs = nil
		m.installCursor = 0
		return m, nil
	case "enter":
		if len(m.installPMs) > 0 && m.installCursor < len(m.installPMs) {
			// Return selected result and PM for installation
			return m, tea.Quit
		}
		return m, nil
	case "up", "k":
		if m.installCursor > 0 {
			m.installCursor--
		}
		return m, nil
	case "down", "j":
		if m.installCursor < len(m.installPMs)-1 {
			m.installCursor++
		}
		return m, nil
	}
	return m, nil
}

// handleSearchMsg applies search results, dropping those of an older query.
func handleSearchMsg(m model, msg searchMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.seq {
		return m, nil
	}
	m.loading = false
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	m.err = nil
	m.results = msg.results
	// Reset cursor and scroll offset if out of bounds
	if m.cursor >= len(m.results) {
		m.cursor = 0
	}
	m.scrollOffset = 0
	return m, nil
}

// handleInstallSelectMsg handles entering install mode.
func handleInstallSelectMsg(m model, msg installSelectMsg) (tea.Model, tea.Cmd) {
	m.installMode = true
	m.selectedResult = &msg.result

	m.installPMs = installManagersFor(msg.result.Manager)
	m.installCursor = 0

	return m, nil
}

// enterInstallMode enters install selection mode.
func enterInstallMode(m model, result search.Result) (tea.Model, tea.Cmd) {
	return handleInstallSelectMsg(m, installSelectMsg{result: result})
}

// adjustScrollOffset ensures the cursor is visible by adjusting scroll offset.
func adjustScrollOffset(m model) model {
	if len(m.results) == 0 {
		return m
	}

	availableHeight := m.visibleRows()

	// Calculate visible range
	visibleCount := availableHeight
	if visibleCount > len(m.results) {
		visibleCount = len(m.results)
	}

	// Ensure cursor is within visible range
	startIdx := m.scrollOffset
	endIdx := startIdx + visibleCount - 1

	if m.cursor < startIdx {
		// Cursor is above visible area, scroll up
		m.scrollOffset = m.cursor
	} else if m.cursor > endIdx {
		// Cursor is below visible area, scroll down
		m.scrollOffset = m.cursor - visibleCount + 1
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	}

	return m
}

// pageUp scrolls up one page.
func pageUp(m model) model {
	if len(m.results) == 0 {
		return m
	}

	availableHeight := m.visibleRows()

	// Scroll up by one page
	m.scrollOffset -= availableHeight
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Adjust cursor to stay within visible range
	visibleCount := availableHeight
	if visibleCount > len(m.results) {
		visibleCount = len(m.results)
	}
	endIdx := m.scrollOffset + visibleCount - 1
	if m.cursor > endIdx {
		m.cursor = endIdx
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	return m
}

// pageDown scrolls down one page.
func pageDown(m model) model {
	if len(m.results) == 0 {
		return m
	}

	availableHeight := m.visibleRows()

	// Calculate max offset
	visibleCount := availableHeight
	if visibleCount > len(m.results) {
		visibleCount = len(m.results)
	}
	maxOffset := len(m.results) - visibleCount
	if maxOffset < 0 {
		maxOffset = 0
	}

	// Scroll down by one page
	m.scrollOffset += availableHeight
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}

	// Adjust cursor to stay within visible range
	startIdx := m.scrollOffset
	if m.cursor < startIdx {
		m.cursor = startIdx
	}
	if m.cursor >= len(m.results) {
		m.cursor = len(m.results) - 1
	}

	return m
}

// registryChoices are the rows of the registry selection screen.
var registryChoices = []struct {
	id   pm.ID
	name string
}{
	{pm.Npm, "npm (Node.js)"},
	{pm.Pip, "pip (Python)"},
	{pm.Composer, "composer (PHP)"},
	{pm.Cargo, "cargo (Rust)"},
	{pm.Maven, "maven (Java)"},
	{pm.ID("all"), "All Registries"},
}

// applyRegistrySelection copies the checked registries into searchOpts.
func applyRegistrySelection(m model) model {
	enable := make(map[pm.ID]bool, len(search.Registries))
	for _, id := range search.Registries {
		enable[id] = m.selectedRegistries[id]
	}
	m.searchOpts.Enable = enable
	m.registryMode = false
	return m
}

// handleRegistryKeyMsg handles keyboard input in registry selection mode.
// Typing a printable key (q/j/k included) starts the query.
func handleRegistryKeyMsg(m model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyEnter:
		m = applyRegistrySelection(m)
		if m.query != "" {
			return queryChanged(m)
		}
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		if m.registryCursor > 0 {
			m.registryCursor--
		}
		return m, nil
	case tea.KeyDown, tea.KeyCtrlN:
		if m.registryCursor < len(registryChoices)-1 {
			m.registryCursor++
		}
		return m, nil
	case tea.KeySpace:
		reg := registryChoices[m.registryCursor]
		if reg.id != "all" {
			m.selectedRegistries[reg.id] = !m.selectedRegistries[reg.id]
			return m, nil
		}
		allSelected := true
		for _, id := range search.Registries {
			allSelected = allSelected && m.selectedRegistries[id]
		}
		for _, id := range search.Registries {
			m.selectedRegistries[id] = !allSelected
		}
		return m, nil
	case tea.KeyRunes:
		m = applyRegistrySelection(m)
		m.query = string(msg.Runes)
		return queryChanged(m)
	case tea.KeyBackspace, tea.KeyDelete:
		m.registryMode = false
		return m, nil
	}
	return m, nil
}

// installManagersFor lists the tools that can install a hit from registry
// id: every manager of its ecosystem (npm hit: npm, yarn, pnpm, bun; Maven
// hit: maven, gradle), or just id for single-tool ecosystems.
func installManagersFor(id pm.ID) []pm.ID {
	if ids := pm.ManagersInEcosystem(pm.EcosystemForManager(id)); len(ids) > 0 {
		return ids
	}
	return []pm.ID{id}
}
```

- [ ] **Step 4: Update the view**

In `internal/tui/search/view.go`:
1. Add `"unicode/utf8"` to the imports.
2. In `View`, replace the block starting `// Calculate available height for results` and ending with the `if availableHeight < 1 { ... }` check with `availableHeight := m.visibleRows()`.
3. In `renderResult`, change `len(packageName)` to `utf8.RuneCountInString(packageName)` in the `actualUsedWidth` line.
4. In `renderFooter`:
   - change `hints := "↑/↓: Navigate"` to `hints := "↑/↓ or Ctrl+P/N: Navigate"`;
   - replace the two lines `visibleCount := m.height - 5 // Header + query + separator + footer` and `if len(m.results) > visibleCount {` with `if len(m.results) > m.visibleRows() {`.
5. In `renderRegistrySelector`, replace the local `availableRegistries := []struct{...}{...}` literal, including its `// Available registries` comment, with `availableRegistries := registryChoices`.
6. Replace `truncate` with:

```go
// truncate shortens s to at most max characters (runes, never splitting a
// multi-byte character), ending in "..." when it cut something.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}
```

In `internal/cli/search_cmd.go` (`cmdSearch`), replace `result, err := tuisearch.Run(initialQuery, searchOpts)` with:

```go
	ui := tuisearch.UIOptions{DebounceMs: cfg.SearchUI.DebounceMs, PageSize: cfg.SearchUI.PageSize}
	result, err := tuisearch.Run(initialQuery, searchOpts, ui)
```

- [ ] **Step 5: Run the tests**

Run: `go test -race -count=2 ./internal/tui/... ./internal/cli/`
Expected: two `ok` lines. In `TestOnlyTheLatestKeystrokeSearches`, only `ax` is searched and the stale `a` reply is ignored.

Manual check (needs a terminal and network): run `go run ./cmd/xpm search`, press Enter on the registry screen, and type `qjk`. The query shows `qjk`, nothing quits, and results appear about 200 ms after typing stops.

- [ ] **Step 6: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/tui internal/cli/search_cmd.go
git commit -m "tui: type q/j/k, debounce with sequence IDs and drop stale results, rune-safe truncation, honour searchUI settings"
```

---

### Task 14: Config — validated `config set`, whole-line prefer editor, `"search": null`, testable warning (roadmap #10; deferred config items)

**Files:**
- Modify: `internal/config/config.go` (`warnOut`, `loadFrom`), `internal/config/config_test.go`, `internal/pm/validation.go` (`ValidateConfig` IDs), `internal/cli/config_cmd.go` (imports, `editConfig` call, `editPreferList`, `setConfigValue`, new `parsePreferList`, `parseBool`)
- Create: `internal/cli/config_cmd_test.go`

**Interfaces:**
- Produces:
  - Package `config`: `var warnOut io.Writer = os.Stderr`. After a successful unmarshal, `loadFrom` replaces a nil `Search` (from `"search": null`) with the default map.
  - Package `pm`: `ValidateConfig` accepts `poetry` and `pipenv`.
  - Package `cli`:
    - `func editPreferList(in io.Reader, current []string) []string`: reads a whole line;
    - `func parsePreferList(s string) ([]string, error)`;
    - `func parseBool(s string) (bool, error)`: accepts true/false, yes/no, on/off and 1/0;
    - `setConfigValue`: validates the key, the IDs and the boolean, writes nothing on error, and requires exactly 2 arguments;
    - test helper `isolatedHome(t)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/config_cmd_test.go`:

```go
package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
)

// isolatedHome points the config file at a temp dir.
func isolatedHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
}

func TestConfigSetValidatesBeforeWriting(t *testing.T) {
	isolatedHome(t)
	for _, args := range [][]string{
		{"prefer", "npm,notapm"},
		{"interactive", "maybe"},
		{"search.npmx", "false"},
		{"search.maven", "nope"},
		{"colour", "blue"},
	} {
		var code int
		captureStdout(t, func() { code = setConfigValue(args) })
		if code != 1 {
			t.Errorf("config set %v exited %d, want 1", args, code)
		}
	}
	if _, err := os.Stat(getConfigPath()); err == nil {
		t.Fatal("an invalid `config set` wrote the config file")
	}
}

func TestConfigSetAcceptsValidValues(t *testing.T) {
	isolatedHome(t)
	captureStdout(t, func() {
		for _, args := range [][]string{
			{"prefer", " pnpm, poetry "},
			{"interactive", "off"},
			{"search.maven", "false"},
		} {
			if code := setConfigValue(args); code != 0 {
				t.Fatalf("config set %v exited %d", args, code)
			}
		}
	})
	c := config.Load()
	if !reflect.DeepEqual(c.Prefer, []string{"pnpm", "poetry"}) || c.Interactive || c.Search["maven"] || !c.Search["npm"] {
		t.Fatalf("saved config = prefer %v interactive %v search %v", c.Prefer, c.Interactive, c.Search)
	}
}

func TestEditPreferListReadsTheWholeLine(t *testing.T) {
	var got []string
	captureStdout(t, func() { got = editPreferList(strings.NewReader("pnpm, pip cargo\n"), nil) })
	if got != nil {
		t.Fatalf("got %v; \"pip cargo\" is not an ID, so the list must be kept", got)
	}
	captureStdout(t, func() { got = editPreferList(strings.NewReader("pnpm, pip, cargo\n"), nil) })
	if !reflect.DeepEqual(got, []string{"pnpm", "pip", "cargo"}) {
		t.Fatalf("got %v, want [pnpm pip cargo]", got)
	}
}
```

In `internal/config/config_test.go`:
- add `"bytes"` and `"strings"` to the imports;
- replace `TestLoadFromInvalidJSONReturnsDefaults` with:

```go
// captureWarnings sends config warnings to a buffer for one test.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := warnOut
	warnOut = &buf
	t.Cleanup(func() { warnOut = old })
	return &buf
}

func TestLoadFromInvalidJSONReturnsDefaultsAndWarns(t *testing.T) {
	warnings := captureWarnings(t)
	path := writeConfig(t, `not valid json{{{`)
	c := loadFrom(path)
	if !c.Interactive || !c.Search["npm"] {
		t.Errorf("invalid JSON must yield defaults, got %+v", c)
	}
	if !strings.Contains(warnings.String(), "xpm: ignoring invalid config "+path) {
		t.Errorf("warning = %q, want it to name %s", warnings.String(), path)
	}
}

func TestLoadFromNullSearchKeepsDefaults(t *testing.T) {
	warnings := captureWarnings(t)
	c := loadFrom(writeConfig(t, `{"search": null}`))
	if c.Search == nil || !c.Search["npm"] || !c.Search["maven"] {
		t.Fatalf(`"search": null must keep the default registries, got %v`, c.Search)
	}
	if warnings.Len() != 0 {
		t.Errorf("valid JSON produced a warning: %q", warnings.String())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/config/ ./internal/cli/ -run 'LoadFrom|ConfigSet|EditPreferList' -v`
Expected: FAIL. This is a build error: `undefined: warnOut` and `too many arguments in call to editPreferList`. Once those compile, `TestLoadFromNullSearchKeepsDefaults` fails with ``"search": null must keep the default registries, got map[]``, and `TestConfigSetValidatesBeforeWriting` fails with `config set [prefer npm,notapm] exited 0, want 1`.

- [ ] **Step 3: Implement**

In `internal/config/config.go`, add `"io"` to the imports and, below the import block:

```go
// warnOut receives warnings about an unusable config file. Tests replace it.
var warnOut io.Writer = os.Stderr
```

In `loadFrom`, replace the unmarshal block and the final `return c` with:

```go
	if err := json.Unmarshal(data, &c); err != nil {
		_, _ = fmt.Fprintf(warnOut, "xpm: ignoring invalid config %s: %v\n", path, err)
		return defaultConfig()
	}
	// An explicit null replaces a map with nil; treat it as "use the defaults".
	if c.Search == nil {
		c.Search = defaultConfig().Search
	}
	return c
```

In `internal/pm/validation.go` (`ValidateConfig`), replace the `validIDs` literal with:

```go
	validIDs := map[string]bool{
		"npm": true, "yarn": true, "pnpm": true, "bun": true,
		"pip": true, "poetry": true, "pipenv": true,
		"composer": true, "cargo": true,
		"gomod": true, "maven": true, "gradle": true,
	}
```

In `internal/cli/config_cmd.go`:
1. Replace the import block with:

```go
import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/manifoldco/promptui"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
)
```

2. In `editConfig`, change `currentCfg.Prefer = editPreferList(currentCfg.Prefer)` to `currentCfg.Prefer = editPreferList(os.Stdin, currentCfg.Prefer)`.
3. Replace `editPreferList` (from `// editPreferList allows editing the prefer list.` to just before `// editSearchSettings`) with:

```go
// editPreferList reads a whole line ("pnpm, pip") from in. Invalid IDs are
// reported and the current list is kept.
func editPreferList(in io.Reader, current []string) []string {
	fmt.Println("\nCurrent prefer list:", strings.Join(current, ", "))
	fmt.Println("Enter package manager IDs separated by commas (e.g., npm,pip,cargo)")
	fmt.Println("Leave empty to clear the list.")
	fmt.Print("> ")

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return current
	}
	list, err := parsePreferList(line)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return current
	}
	return list
}

// parsePreferList splits "pnpm, pip" into IDs and validates them.
func parsePreferList(s string) ([]string, error) {
	list := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			list = append(list, p)
		}
	}
	if errs := pm.ValidateConfig(list, nil); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return list, nil
}

// parseBool accepts true/false, yes/no, on/off and 1/0.
func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "on", "1":
		return true, nil
	case "false", "no", "off", "0":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q (use true or false)", s)
}
```

4. Replace `setConfigValue` (from `// setConfigValue sets a configuration value from the command line.` to just before `// resetConfig`) with:

```go
// setConfigValue sets a configuration value from the command line. Values
// are validated before anything is written.
func setConfigValue(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: xpm config set <key> <value>")
		fmt.Fprintln(os.Stderr, "Keys: prefer, autoInstallPM, interactive, search.<ecosystem>")
		return 1
	}
	key, value := args[0], args[1]
	currentCfg := config.Load()

	var err error
	switch {
	case key == "prefer":
		currentCfg.Prefer, err = parsePreferList(value)
	case key == "autoInstallPM":
		currentCfg.AutoInstallPM, err = parseBool(value)
	case key == "interactive":
		currentCfg.Interactive, err = parseBool(value)
	case strings.HasPrefix(key, "search."):
		eco := strings.TrimPrefix(key, "search.")
		var on bool
		if on, err = parseBool(value); err == nil {
			if errs := pm.ValidateConfig(nil, map[string]bool{eco: on}); len(errs) > 0 {
				err = errors.Join(errs...)
			}
		}
		if err == nil {
			if currentCfg.Search == nil {
				currentCfg.Search = map[string]bool{}
			}
			currentCfg.Search[eco] = on
		}
	default:
		err = fmt.Errorf("unknown config key %q", key)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	if err := saveConfig(currentCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save config: %v\n", err)
		return 1
	}
	fmt.Printf("Set %s = %s\n", key, value)
	return 0
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/config/ ./internal/cli/ ./internal/pm/`
Expected: three `ok` lines. The config tests no longer print `xpm: ignoring invalid config ...` to the terminal.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/config internal/pm/validation.go internal/cli/config_cmd.go internal/cli/config_cmd_test.go
git commit -m "config: validate config set before writing; whole-line prefer editor; null search keeps defaults; testable warning"
```

---

### Task 15: `perf.sh` reports failures, uses a private cache, cleans up; `XPM_CACHE_DIR`

**Files:**
- Modify: `internal/search/lookupcache.go` (`defaultLookupCacheDir` and the package comment), `internal/search/lookupcache_test.go` (append), `README.md` (environment variables table)
- Replace: `scripts/perf.sh`

**Interfaces:**
- Produces:
  - `XPM_CACHE_DIR=<dir>` puts lookup and search cache entries in `<dir>/lookups`. `XPM_NO_CACHE=1` still wins.
  - `perf.sh` rows:
    - when hyperfine fails, print `name  -  budget  FAIL (<last line of hyperfine's error>)` and continue;
    - the warm row uses a temp cache, and the user's cache is never touched;
    - every temp dir is removed by `trap ... EXIT`;
    - new row `search-axios-cold` (budget 1500 ms — user decision 2026-10-07: crates.io's search API alone takes ~1.2 s cold; roadmap budget updated to match).

- [ ] **Step 1: Write the failing test**

Append to `internal/search/lookupcache_test.go`:

```go

func TestCacheDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XPM_NO_CACHE", "")
	t.Setenv("XPM_CACHE_DIR", dir)
	if got, want := defaultLookupCacheDir(), filepath.Join(dir, "lookups"); got != want {
		t.Fatalf("defaultLookupCacheDir() = %q, want %q", got, want)
	}
	t.Setenv("XPM_NO_CACHE", "1")
	if got := defaultLookupCacheDir(); got != "" {
		t.Fatalf("XPM_NO_CACHE must win over XPM_CACHE_DIR, got %q", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/search/ -run CacheDirOverride -v`
Expected: FAIL. The message is `defaultLookupCacheDir() = "/Users/<you>/Library/Caches/xpm/lookups", want "/var/folders/.../lookups"` (paths vary by OS).

- [ ] **Step 3: Implement**

In `internal/search/lookupcache.go`:
- in `defaultLookupCacheDir`, insert this after the `XPM_NO_CACHE` check:

```go
	if dir := os.Getenv("XPM_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "lookups")
	}
```

- extend the comment above the `var (` block to:

```go
// Exact-lookup results are memoised on disk so repeat commands
// (`xpm which x` then `xpm install x`) skip the network entirely.
// Set XPM_NO_CACHE=1 to bypass, or XPM_CACHE_DIR=<dir> to move the cache
// (entries then live in <dir>/lookups).
```

Replace `scripts/perf.sh` with:

```bash
#!/usr/bin/env bash
# Measures xpm against the roadmap's performance budgets.
# Requires: hyperfine, jq. Network-dependent rows need internet access.
set -euo pipefail

command -v hyperfine >/dev/null || { echo "install hyperfine (brew install hyperfine)"; exit 2; }
command -v jq >/dev/null || { echo "install jq (brew install jq)"; exit 2; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$(mktemp -d)"
WORK="$(mktemp -d)"   # empty dir: no project files, nothing to detect
CACHE="$(mktemp -d)"  # private lookup cache: warm rows never touch the user's cache
trap 'rm -rf "$BIN_DIR" "$WORK" "$CACHE"' EXIT

BIN="$BIN_DIR/xpm"
go build -trimpath -ldflags "-s -w" -o "$BIN" "$ROOT/cmd/xpm"
cd "$WORK"

# name | budget_ms | command
BUDGETS=(
  "help|10|$BIN help"
  "version|10|$BIN --version"
  "which-axios-cold|1500|env XPM_NO_CACHE=1 $BIN which axios"
  "which-typescript-cold|1500|env XPM_NO_CACHE=1 $BIN which typescript"
  "which-axios-warm|100|env XPM_CACHE_DIR=$CACHE $BIN which axios"
  "search-axios-cold|1500|env XPM_NO_CACHE=1 $BIN search axios"
)

fail=0
printf "%-22s %10s %10s  %s\n" "benchmark" "mean(ms)" "budget" "result"
for row in "${BUDGETS[@]}"; do
  IFS='|' read -r name budget cmd <<<"$row"
  # A command that exits non-zero (offline, registry error) makes hyperfine
  # fail; report that as a FAIL row instead of letting set -e end the run.
  if ! out=$(hyperfine --warmup 2 --runs 10 --style none --export-json "$WORK/$name.json" "$cmd" 2>&1); then
    reason=$(printf '%s\n' "$out" | grep -v '^[[:space:]]*$' | tail -n 1)
    printf "%-22s %10s %10s  %s\n" "$name" "-" "$budget" "FAIL (${reason:-hyperfine failed})"
    fail=1
    continue
  fi
  mean_ms=$(jq '.results[0].mean * 1000 | floor' "$WORK/$name.json")
  if [ "$mean_ms" -le "$budget" ]; then res="PASS"; else res="FAIL"; fail=1; fi
  printf "%-22s %10s %10s  %s\n" "$name" "$mean_ms" "$budget" "$res"
done

size_kb=$(( $(wc -c <"$BIN") / 1024 ))
# Stripped size: ~9.8 MB darwin/arm64, ~10.9 MB linux/amd64 (Go 1.27); was 14.6 MB unstripped.
if [ "$size_kb" -le 12288 ]; then res="PASS"; else res="FAIL"; fail=1; fi
printf "%-22s %10s %10s  %s\n" "binary-size(KB)" "$size_kb" "12288" "$res"

# Startup must not write files.
[ ! -e "$WORK/.xpm-env" ] || { echo "FAIL: startup wrote .xpm-env"; fail=1; }
exit $fail
```

In `README.md`, add this row to the "Environment variables" table, below `XPM_NO_CACHE`:

```markdown
| `XPM_CACHE_DIR=<dir>` | Keep xpm's lookup cache in `<dir>/lookups` instead of the OS cache folder (used by `make perf`) |
```

- [ ] **Step 4: Run the tests and lint the script**

Run: `go test -race ./internal/search/ && bash -n scripts/perf.sh && echo syntax-ok`
Expected: `ok  	github.com/crenspire/xpm/internal/search` then `syntax-ok`. If `shellcheck` is installed, `shellcheck scripts/perf.sh` prints nothing.

Optional, needs `hyperfine`, `jq` and network: run `make perf`. Expected shape (numbers vary):

```
benchmark                mean(ms)     budget  result
help                            2         10  PASS
version                         2         10  PASS
which-axios-cold             1020       1500  PASS
which-typescript-cold         980       1500  PASS
which-axios-warm                9        100  PASS
search-axios-cold            1210       1500  PASS
binary-size(KB)             10240      12288  PASS
```

`search-axios-cold` measured about 1.2 s on 2026-10-07: crates.io's search API answers in 1.2–1.3 s. It is a network budget and the CI perf job is informational, so record the number rather than chase it here. Offline, each network row prints `FAIL (Command terminated with non-zero exit code ...)` instead of the script dying silently.

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: green, `0 issues.`

```bash
git add internal/search/lookupcache.go internal/search/lookupcache_test.go scripts/perf.sh README.md
git commit -m "perf: report hyperfine failures as FAIL rows, private warm cache via XPM_CACHE_DIR, trap cleanup, xpm search budget"
```

---

### Task 16: End-to-end table for every README install example; README and roadmap

**Files:**
- Create: `internal/cli/e2e_test.go`
- Modify: `README.md`, `docs/superpowers/plans/2026-10-06-xpm-roadmap.md`

**Interfaces:**
- Consumes:
  - from Task 1: `search.SetEndpoints`, `search.SetCacheDir`;
  - from Task 12: `pm.SetCommandRunner`, `pm.SetLookPath`;
  - from Tasks 5, 10, 11 and 14: `runTool`, `isInteractiveTerminal`, `withConfig`, `captureStdout`, `inProject`, `isolatedHome`, `chdir` (`startup_test.go`);
  - `Run()`.
- This is the roadmap's P3 exit criterion: "an e2e test table (fake registries via `httptest`) covers each README install example".

- [ ] **Step 1: Write the end-to-end table**

Create `internal/cli/e2e_test.go`:

```go
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
		args   string
		code   int
		ran    []string
		outHas string
	}{
		{"search then install with the lockfile's tool", []string{"package.json", "package-lock.json"}, "install axios", 0, []string{"npm install axios"}, "Detected package-lock.json - using npm"},
		{"pin a version", nil, "install axios@1.7.0", 0, []string{"npm install axios@1.7.0"}, "Will install axios@1.7.0 via npm (Node.js)."},
		{"no implicit pin", nil, "install axios", 0, []string{"npm install axios"}, ""},
		{"global install", nil, "install -g typescript", 0, []string{"npm install -g typescript"}, ""},
		{"flag after the package", nil, "install typescript -g", 0, []string{"npm install -g typescript"}, ""},
		{"several packages", nil, "install axios lodash", 0, []string{"npm install axios", "npm install lodash"}, "[2/2] lodash"},
		{"yarn.lock selects yarn", []string{"package.json", "yarn.lock"}, "install axios", 0, []string{"yarn add axios"}, ""},
		{"composer fuzzy hit installs the real name", []string{"composer.json"}, "install monolog", 0, []string{"composer require monolog/monolog"}, `"monolog" matched monolog/monolog.`},
		{"go module path", []string{"go.mod"}, "install github.com/gin-gonic/gin", 0, []string{"go get github.com/gin-gonic/gin@latest"}, "is a Go module path"},
		{"gradle build file prints a gradle snippet", []string{"build.gradle.kts"}, "install guava", 0, nil, `implementation("com.google.guava:guava:33.3.1-jre")`},
		{"no match exits 1", nil, "install nope-not-a-package", 1, nil, "No matches found for nope-not-a-package"},
		{"project install uses the lockfile's tool", []string{"package.json", "pnpm-lock.yaml"}, "install", 0, []string{"pnpm install"}, ""},
		{"ci is a frozen install", []string{"package.json", "package-lock.json"}, "ci", 0, []string{"npm ci"}, ""},
		{"which", nil, "which axios", 0, nil, "- npm: axios @1.7.9 - Promise based HTTP client"},
	} {
		t.Run(c.name, func(t *testing.T) {
			withConfig(t, cfg)
			isolatedHome(t)
			inProject(t, c.files...)
			fakeRegistries(t)
			ran := recordAllCommands(t)
			oldArgs, oldTTY := os.Args, isInteractiveTerminal
			t.Cleanup(func() { os.Args, isInteractiveTerminal = oldArgs, oldTTY })
			isInteractiveTerminal = func() bool { return false }
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
```

- [ ] **Step 2: Run it**

Run: `go test -race ./internal/cli/ -run READMEInstallExamples -v`
Expected: `--- PASS: TestREADMEInstallExamples` with 14 subtests passing:
- `search then install with the lockfile's tool`
- `pin a version`
- `no implicit pin`
- `global install`
- `flag after the package`
- `several packages`
- `yarn.lock selects yarn`
- `composer fuzzy hit installs the real name`
- `go module path`
- `gradle build file prints a gradle snippet`
- `no match exits 1`
- `project install uses the lockfile's tool`
- `ci is a frozen install`
- `which`

It passes on the first run because Tasks 1–15 already implement the behaviour. To confirm the table really checks something, temporarily change `"npm install axios@1.7.0"` to `"npm install axios"` in the `pin a version` row. Expected: `ran ["npm install axios@1.7.0"], want ["npm install axios"]`. Revert the change.

- [ ] **Step 3: Update the README**

In `README.md`:

1. Replace the "Respects your project" bullet in "Why xpm" with:

```markdown
- **Respects your project.** Lockfiles pick the tool inside their ecosystem (`yarn.lock` → yarn, `poetry.lock` → poetry, `build.gradle` → Gradle) for `install`, `list`, `update`, `remove` and `ci`. If a name exists in several ecosystems, xpm still asks which one you mean.
```

2. In the Status table, replace the "Install / update / remove" row with:

```markdown
| Install / update / remove | `install`, `ci`, `update`, `remove`, `list` | ✅ Stable (Go modules and Gradle included) |
```

3. In "Supported package managers", replace the Node.js, Python, Java and Go rows with:

```markdown
| Node.js | npm, yarn, pnpm, bun | `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lock` / `bun.lockb` | npm |
| Python | pip, poetry, pipenv | `requirements.txt`, `poetry.lock`, `Pipfile` / `Pipfile.lock` | PyPI |
| Java | Maven, Gradle | `pom.xml` / `build.gradle(.kts)` pick the tool (xpm prints the dependency snippet) | Maven Central |
| Go | Go modules | `go.mod` | — (`xpm install github.com/x/y` runs `go get` directly) |
```

4. Replace the "Find a package everywhere" paragraph below its code block with:

```markdown
If a registry doesn't answer within 2.5 s, xpm shows what the others found and lists that registry under **Unavailable**, separately from **Not found in**. If every registry fails (for example, you're offline), the command exits with an error rather than claiming "no matches".
```

5. Replace the "Install" section's code block and paragraph with:

````markdown
```bash
xpm install axios               # search, then install with the right tool
xpm install axios@1.7.0         # pin a version (universal @ syntax); without @ the tool picks its latest
xpm install axios lodash        # several packages, in order (stops at the first failure)
xpm install -g typescript       # global install where the tool supports it (-g may also come last)
xpm install github.com/gin-gonic/gin   # Go module path: runs go get, no registry search
xpm install                     # no args: install this project's dependencies with its own tool
xpm ci                          # frozen install: npm ci, pnpm/yarn/bun --frozen-lockfile, yarn --immutable,
                                # composer install, pip install -r, pipenv --deploy, cargo build --locked
```

xpm installs the registry's own name (`xpm install monolog` → `composer require monolog/monolog`) and tells you when it differs from what you typed. Lockfiles narrow the tool within an ecosystem; across ecosystems you choose. `xpm ci` never deletes lockfiles, and deletes `node_modules/` or Composer's `vendor/` only if you confirm.

xpm never pipes a remote install script into a shell. If a tool such as bun, rustup or Composer is missing, it prints the official install instructions instead.
````

6. Add a section after "Global flags":

```markdown
### Scripts and CI

Without a terminal (pipes, CI), xpm never prompts. It picks the first match in your `prefer` order. If a registry was unavailable it refuses to guess and exits 1, so retry or turn that registry off (`xpm config set search.maven false`).

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | Error, cancelled, invalid arguments, **no matches**, or a refused non-interactive guess |
| other | `list` / `update` / `remove` / `run` pass through the underlying tool's exit code |
```

7. In the Configuration table:
   - change the `interactive` row's description to `Prompt for choices (only when a terminal is attached); false picks the preferred option`;
   - add these rows:

```markdown
| `timeout.default` | `0` | Seconds each registry may take; `0` = built-in 2.5 s. Older configs written by `xpm config set` contain `4` — set it to `0` for the fast default |
| `timeout.perRegistry` | `{}` | Per-registry override in seconds, e.g. `{"crates": 4}` (`npm`, `pypi`, `packagist`, `crates`, `maven`) |
| `searchUI.debounceMs` / `searchUI.pageSize` | `200` / `20` | TUI typing pause before searching / max rows per page |
```

8. In the Performance table, add this row:

```markdown
| `xpm search axios` (plain), first lookup | 1.08 s | ~1.2 s (bounded by crates.io's search API) |
```

In `docs/superpowers/plans/2026-10-06-xpm-roadmap.md`, change the P3 row of the "Phases at a glance" table from `| P3 | Install/search correctness | ~4 days | write JIT |` to `| P3 | Install/search correctness | ~4 days | [Plan C](2026-10-07-xpm-p3-install-search.md) |`.

- [ ] **Step 4: Check the README commands against the binary**

Run:

```bash
go build -o /tmp/xpm ./cmd/xpm && cd "$(mktemp -d)" && /tmp/xpm install -g; echo "exit=$?"; /tmp/xpm which a b; echo "exit=$?"; /tmp/xpm ci; echo "exit=$?"
```

Expected:

```
error: -g/--global needs a package name (project dependencies are never global)
exit=1
error: which takes one package name, got 2: a b
...usage...
exit=1
No known dependency files found (package.json, composer.json, etc).
exit=1
```

- [ ] **Step 5: Full gate and commit**

Run: `go build ./... && go vet ./... && go test -race ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...`
Expected: all `ok`, no `gofmt` output, `0 issues.`

```bash
git add internal/cli/e2e_test.go README.md docs/superpowers/plans/2026-10-06-xpm-roadmap.md
git commit -m "test(cli): end-to-end table for every README install example; docs: P3 behaviour, exit codes, timeouts"
```

---

## Self-review notes

**Spec coverage** (roadmap P3 items → tasks):

| Item | Requirement | Task |
|---|---|---|
| #1 | Lockfiles narrow within an ecosystem; deterministic | 6 (ordered detection), 7 (`buildCandidates`, stable `sortCandidates`) |
| #2 | Install `chosen.Name`, show "will install", `composerPackagePattern` | 7 |
| #3 | No implicit pin | 7 (`installSpec`), also on the TUI path |
| #4 | Multi-package, flags after arguments, unused-argument errors | 9 |
| #5 | Detection in list/update/remove/install incl. `bun.lock`, `Pipfile`, `poetry.lock` | 6, 10 |
| #6 | Go module paths → GoMod; Gradle offered | 8, plus 6/7 for build files |
| #7 | Native `xpm ci`, no unconfirmed deletion, `vendor/` only for Composer | 10 |
| #8 | No matches exits 1; non-TTY non-interactive | 5 (exit codes), 11 (TTY) |
| #9 | TUI keys, debounce + sequence IDs, runes, `SearchUI` | 13 |
| #10 | `config set` validation with `ValidateConfig` (+poetry/pipenv); whole-line editor | 14 |
| #11 | Per-registry timeouts via `context.WithTimeout` | 4 (`registryCall.timeout`, `timeoutFor`) |
| #12 | `OptionsFromConfig` | 4 |
| #13 | Unavailable vs not found; TUI search deadline + cache | 3 (search), 5 (CLI) |

Deferred items:
- crates sparse index → 2;
- injectable URLs, httptest for pip/composer/crates/maven, decode errors naming the package, `statusError` without a trailing `": "` → 1, 2;
- `InstallPM` → 12 (Composer decision justified there);
- `rejectLeadingDash` and validating before the PM prompt → 7;
- config `null`/warning → 14;
- `perf.sh` and `XPM_CACHE_DIR` → 15;
- README → 15, 16.

Exit criterion: the e2e table is Task 16. The `xpm search` < 1 s budget has a `perf.sh` row (Task 15) but is network-bound (about 1.2 s measured); see the notes in Task 15.

**Ordering constraint:** #13 lands in Task 5, before #8's non-TTY default in Task 11. Task 11's `nonInteractivePick` consumes `Report.UnavailableIDs()` from Task 3 and Task 5. ✔

**Type consistency** (each name is defined once and used with the same signature everywhere):
- `lookup.fn(ctx, pkg)`, `cachedLookup(ctx, dir, l, pkg)`, `registryCall{id, timeout, fn}`
- `Report{Results, Unavailable}`, `RegistryFailure{Manager, Err}.TimedOut()`, `UnavailableIDs()`
- `SearchEverywhereReport`, `SearchReport`, `OptionsFromConfig`, `timeoutFor`, `Registries`
- `ProjectFile{Name, Ecosystem, Manager}`, `ProjectManagers`
- `candidate{Result, Via}`, `installSpec`, `installCandidate(c, query, requestedVersion, global)`, `chooseCandidate(cands, unavailable)` (gains its second parameter in Task 11; Task 7's single call site is updated there)
- `ensurePM`, `installPkg`, `runTool`, `pmExists`, `lookupReport`, `searchReport`, `stdoutIsTerminal`, `isInteractiveTerminal`
- `projectCmd{Kind, PM, YarnBerry}.args(action, pkg)`, `pmChoice{PM, Via, Options}`
- `UIOptions{DebounceMs, PageSize}`, `debounceMsg{seq, query}`, `searchMsg{seq, results, err}`
- `pm.SetCommandRunner`, `pm.SetLookPath`, `pm.ManualInstallError{Manager, Steps}`
- `search.SetEndpoints(Endpoints{...})`, `search.SetCacheDir`

**Placeholder scan:** every code step has the full code. The Review Focus tests are in Tasks 2, 7, 10, 11 and 13.

**Known limits, accepted:**
- `workspace/install.go` (lane C) still ranges over the `DetectLockFiles` map across ecosystems; within an ecosystem it is now ordered.
- Older in-flight TUI searches are dropped, not cancelled.
- The yarn-berry `list`/`update` flags (`yarn info --name-only`, `yarn up "*"`), `bun pm ls` and `pipenv graph` come from the tools' documentation, not from a run against those tools.
