# Plan A — P0 Green Build + P1 Speed Core

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make xpm build from a fresh clone with green tests and CI. Then cut lookups from 3–6 s to ~1 s cold and ~0.06 s warm, and make startup side-effect-free.

**Architecture:**
- P0 (Tasks 1–6) is hygiene: commit the entrypoint, fix config merging and flag parsing, delete dead code, repair CI.
- P1 (Tasks 7–11):
  - replaces the sequential registry loop with a deadline-bounded parallel fan-out over an injectable lookup table;
  - moves npm/Maven/crates.io to small, reliable endpoints and routes every registry GET through one helper (User-Agent, capped bodies);
  - caches lookups on disk;
  - removes the auto-activation that writes `.xpm-env` on every command;
  - adds a perf harness with budgets.

All P1 code was validated in a scratch worktree on 2026-10-06 (`go test -race -count=3` green; cold `which` 0.9–1.6 s, warm 0.06 s).

**Tech Stack:** Go 1.22 (module floor), stdlib `net/http`, `httptest`, golangci-lint v2, GitHub Actions, hyperfine (perf only).

**Spec:** `docs/superpowers/plans/2026-10-06-xpm-roadmap.md` — sections "Performance budgets", "P0" and "P1".

## Global Constraints

- The module's Go version stays `go 1.22`. Don't use APIs newer than 1.22 (no `t.Chdir`, no `os.CopyFS`), and don't add dependencies.
- The binary name is `xpm`. The module path is `github.com/crenspire/xpm`.
- Only this plan edits `internal/cli/cli.go` (lane A owns it).
- Commit messages must not contain AI co-author trailers or "Generated with" lines (user's global rule).
- Every task ends with `go build ./... && go test ./...` green before its commit.
- Work on a branch off `develop` (e.g. `p0-p1-green-fast`), never directly on `main`.

## Review Focus

1. **A config file with only a `search` section** (e.g. `{"search":{"maven":false}}`) should keep every other default and every other search key `true`. Test in Task 2.
2. **A config file with invalid JSON** should fall back to the defaults and print one warning to stderr, not exit non-zero. Test in Task 2.
3. **Arguments after the subcommand that look like xpm flags** (`xpm run build -- -v`, `xpm run t -- --version`) must reach the subcommand untouched. Test in Task 4.
4. **One registry hanging (accepting the connection but never answering) while others succeed:** the healthy registries' results must come back by `lookupDeadline`, and the straggler must not race with later code. Tests in Task 7 (`TestSearchEverywhereDeadlineDropsStragglers`, run with `-race -count=3`).
5. **Scoped npm packages** (`@types/node`) must hit `/@types%2Fnode/latest` and parse correctly; a 404 means "not found", not an error. Test in Task 8.

---

## File map

| File | Change | Task |
|---|---|---|
| `.gitignore` | anchor binary ignores to repo root | 1 |
| `cmd/xpm/main.go` | first commit (already on disk) | 1 |
| `internal/config/config.go` | `Load` → `loadFrom(path)` merging onto defaults | 2 |
| `internal/config/config_test.go` | fix filename test; add merge tests | 2 |
| `internal/search/search.go` | URL-safe name validation | 3 |
| `internal/cli/cli.go` | `splitGlobalFlags`; remove auto-activation | 4, 9 |
| `internal/cli/flags_test.go` | new | 4 |
| many | gofmt + dead code deletion | 5 |
| `.github/workflows/ci.yml`, `.golangci.yml`, `.pre-commit-config.yaml`, `Makefile` | CI repair | 6 |
| `internal/search/search.go`, `internal/search/everywhere_test.go` | parallel `SearchEverywhere` with deadline | 7 |
| `internal/search/http.go`, `search.go`, `maven.go`, `npm_test.go`, `internal/cli/commands.go` | `httpGet`, npm `/latest`, Sonatype Central, slim crates.io, body caps | 8 |
| `internal/cli/startup_test.go` | new | 9 |
| `internal/search/lookupcache.go`, `lookupcache_test.go`, `search.go`, `everywhere_test.go` | on-disk lookup cache | 10 |
| `scripts/perf.sh`, `Makefile`, `internal/search/everywhere_test.go` | perf harness + benchmark | 11 |

---

### Task 1: Commit the entrypoint (fix `.gitignore`)

**Files:**
- Modify: `.gitignore:1-3`
- Add to git: `cmd/xpm/main.go`
- Delete: empty directory `cmd/upm/`

**Interfaces:** Produces: a tracked `cmd/xpm/main.go`, which every later task's build relies on.

- [ ] **Step 1: Prove the bug**

Run: `git check-ignore -v cmd/xpm/main.go`
Expected: `.gitignore:2:xpm	cmd/xpm/main.go` (the file is ignored)

- [ ] **Step 2: Anchor the binary patterns to the repo root**

In `.gitignore`, replace

```gitignore
# Binaries
xpm
upm
```

with

```gitignore
# Binaries (anchored so cmd/xpm/ is NOT ignored)
/xpm
/upm
```

- [ ] **Step 3: Verify and clean up**

Run: `git check-ignore -v cmd/xpm/main.go; echo "exit=$?"` → Expected: `exit=1` (no longer ignored)
Run: `git check-ignore -v xpm` → Expected: `.gitignore:3:/xpm	xpm` (the root binary is still ignored)
Run: `rmdir cmd/upm`

- [ ] **Step 4: Commit**

```bash
git add .gitignore cmd/xpm/main.go
git commit -m "build: track cmd/xpm entrypoint; anchor binary ignores to repo root"
```

---

### Task 2: Config merges onto defaults

**Files:**
- Modify: `internal/config/config.go` (`Load`, at the end of the file)
- Test: `internal/config/config_test.go` (`TestConfigPath` around line 108; replace `TestConfigMergeWithDefaults`)

**Interfaces:**
- Produces: `func loadFrom(path string) Config` (unexported; used by `Load` and tests). `Load()` keeps its signature.

- [ ] **Step 1: Write the failing tests**

In `internal/config/config_test.go`, in `TestConfigPath`, replace both occurrences of `upmrc.json` with `xpmrc.json`.

Replace the whole `TestConfigMergeWithDefaults` function with:

```go
// writeConfig writes content to a temp xpmrc.json and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xpmrc.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadFromPartialConfigKeepsDefaults(t *testing.T) {
	c := loadFrom(writeConfig(t, `{"prefer":["yarn"],"search":{"maven":false}}`))

	if len(c.Prefer) != 1 || c.Prefer[0] != "yarn" {
		t.Errorf("Prefer = %v, want [yarn]", c.Prefer)
	}
	if c.Search["maven"] {
		t.Error("Search[maven] should be false (set in file)")
	}
	if !c.Search["npm"] || !c.Search["pip"] {
		t.Errorf("unset search keys must keep default true, got %v", c.Search)
	}
	if !c.Interactive || !c.AutoInstallPM || !c.SearchUI.Enabled || !c.Env.Enabled || !c.Cache.Enabled {
		t.Errorf("unset booleans must keep defaults, got %+v", c)
	}
	if c.Graph.Depth != 5 || c.Timeout.Default != 4 {
		t.Errorf("unset numbers must keep defaults, got depth=%d timeout=%d", c.Graph.Depth, c.Timeout.Default)
	}
}

func TestLoadFromExplicitFalseOverridesDefault(t *testing.T) {
	c := loadFrom(writeConfig(t, `{"interactive":false}`))
	if c.Interactive {
		t.Error("explicit interactive:false must win over default")
	}
	if !c.AutoInstallPM {
		t.Error("autoInstallPM must keep its default")
	}
}

func TestLoadFromInvalidJSONReturnsDefaults(t *testing.T) {
	c := loadFrom(writeConfig(t, `not valid json{{{`))
	if !c.Interactive || !c.Search["npm"] {
		t.Errorf("invalid JSON must yield defaults, got %+v", c)
	}
}

func TestLoadFromMissingFileReturnsDefaults(t *testing.T) {
	c := loadFrom(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if !c.Interactive {
		t.Error("missing file must yield defaults")
	}
}
```

Make sure the test file imports `os` and `path/filepath` (it already uses both).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run 'LoadFrom|ConfigPath' -v`
Expected: build failure `undefined: loadFrom`. (`TestConfigPath` passes once the filename is fixed.)

- [ ] **Step 3: Implement**

In `internal/config/config.go`, add `"fmt"` to the imports and replace the whole `Load` function with:

```go
// Load reads the configuration file and returns the Config.
// Fields missing from the file keep their default values.
// If the file doesn't exist, defaults are returned; if it is invalid,
// a warning is printed to stderr and defaults are returned.
func Load() Config {
	return loadFrom(configPath())
}

func loadFrom(path string) Config {
	c := defaultConfig()
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	// Unmarshal on top of the defaults: absent keys keep their default,
	// and map fields (Search) merge key-by-key.
	if err := json.Unmarshal(data, &c); err != nil {
		fmt.Fprintf(os.Stderr, "xpm: ignoring invalid config %s: %v\n", path, err)
		return defaultConfig()
	}
	return c
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "fix(config): merge config file onto defaults instead of zero values"
```

---

### Task 3: URL-safe package names for registry lookups

**Files:**
- Modify: `internal/search/search.go` (`validatePackageNameForURL`, lines 69–91)
- Test: `internal/search/search_test.go` (`TestURLInjectionPrevention` already exists and currently fails)

**Interfaces:** `func validatePackageNameForURL(pkg string) error`. The signature is unchanged; the error text contains `invalid package name` or `too long`.

- [ ] **Step 1: Confirm the existing tests fail**

Run: `go test ./internal/search/ -run TestURLInjectionPrevention -v`
Expected: FAIL for `path_traversal_attempt`, `query_injection_attempt`, `fragment_injection`

- [ ] **Step 2: Add the scoped-name cases that must keep working**

In `internal/search/search_test.go`, add these entries to the `tests` slice of `TestURLInjectionPrevention`:

```go
		{
			name:    "npm scoped name",
			pkg:     "@types/node",
			wantErr: false,
		},
		{
			name:    "composer vendor/name",
			pkg:     "monolog/monolog",
			wantErr: false,
		},
		{
			name:     "embedded whitespace",
			pkg:      "foo bar",
			wantErr:  true,
			contains: "invalid package name",
		},
```

- [ ] **Step 3: Implement**

Add `"strings"` to the imports of `internal/search/search.go`. Replace `validatePackageNameForURL` (including its doc comment) with:

```go
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
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/search/ -run TestURLInjectionPrevention -v`
Expected: PASS (all 8 cases)

Run: `go test ./...`
Expected: PASS for every package (this is the first fully green run).

- [ ] **Step 5: Commit**

```bash
git add internal/search/search.go internal/search/search_test.go
git commit -m "fix(search): reject URL-altering characters in registry lookups"
```

---

### Task 4: Global flags only before the subcommand

**Files:**
- Modify: `internal/cli/cli.go:122-151` (inside `Run`)
- Create: `internal/cli/flags_test.go`

**Interfaces:**
- Produces: `func splitGlobalFlags(raw []string) (args []string, verbose, showVersion bool)`.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/flags_test.go`:

```go
package cli

import (
	"reflect"
	"testing"
)

func TestSplitGlobalFlags(t *testing.T) {
	tests := []struct {
		name        string
		raw         []string
		wantArgs    []string
		wantVerbose bool
		wantVersion bool
	}{
		{"no args", []string{}, nil, false, false},
		{"long version", []string{"--version"}, nil, false, true},
		{"short V", []string{"-V"}, nil, false, true},
		{"lone -v is version", []string{"-v"}, nil, false, true},
		{"-v before command is verbose", []string{"-v", "install", "axios"}, []string{"install", "axios"}, true, false},
		{"--verbose before command", []string{"--verbose", "which", "x"}, []string{"which", "x"}, true, false},
		{"script args keep --version", []string{"run", "test", "--", "--version"}, []string{"run", "test", "--", "--version"}, false, false},
		{"script args keep -v", []string{"run", "build", "--", "-v"}, []string{"run", "build", "--", "-v"}, false, false},
		{"flag after command is not global", []string{"graph", "-V"}, []string{"graph", "-V"}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args, verbose, version := splitGlobalFlags(tc.raw)
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tc.wantArgs)
			}
			if verbose != tc.wantVerbose {
				t.Errorf("verbose = %v, want %v", verbose, tc.wantVerbose)
			}
			if version != tc.wantVersion {
				t.Errorf("showVersion = %v, want %v", version, tc.wantVersion)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/cli/ -run TestSplitGlobalFlags -v`
Expected: build failure `undefined: splitGlobalFlags`

- [ ] **Step 3: Implement**

In `internal/cli/cli.go`, add this function directly above `func Run() int`:

```go
// splitGlobalFlags consumes xpm's own flags (-v, --verbose, --version, -V)
// only while they appear BEFORE the subcommand. Everything from the
// subcommand onward is returned untouched, so `xpm run test -- --version`
// passes --version to the script instead of printing xpm's version.
// A lone `-v` means version; `-v` followed by a command means verbose.
func splitGlobalFlags(raw []string) (args []string, verbose, showVersion bool) {
	for i, a := range raw {
		switch a {
		case "--version", "-V":
			showVersion = true
		case "-v":
			if len(raw) == 1 {
				showVersion = true
			} else {
				verbose = true
			}
		case "--verbose":
			verbose = true
		default:
			return raw[i:], verbose, showVersion
		}
	}
	return nil, verbose, showVersion
}
```

Then, in `Run`, replace everything from `var verbose bool` through the end of the second `for _, a := range rawArgs { ... }` loop (the old lines 125–151) with:

```go
	args, verbose, showVersion := splitGlobalFlags(rawArgs)
```

Leave the following lines (`logx.Verbose = verbose`, …) unchanged.

- [ ] **Step 4: Run the tests and a manual check**

Run: `go test ./internal/cli/ -v -run 'TestSplitGlobalFlags'` → Expected: PASS
Run: `go build -o /tmp/xpm-t ./cmd/xpm && /tmp/xpm-t --version | tail -1 && /tmp/xpm-t -v | tail -1`
Expected: both print the version line.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/cli.go internal/cli/flags_test.go
git commit -m "fix(cli): only parse global flags before the subcommand"
```

---

### Task 5: gofmt + delete dead code

**Files:** Modify (deletions only): the files listed in Step 2.

**Interfaces:** None. This task is behaviour-preserving, and later tasks must not depend on any deleted symbol.

- [ ] **Step 1: Format and commit the formatting on its own (pure noise, easy to review)**

Run: `gofmt -w . && gofmt -l . && go build ./... && go test ./...`
Expected: no output from `gofmt -l`; build and tests pass.

```bash
git add -A
git commit -m "style: gofmt entire tree"
```

- [ ] **Step 2: Delete the unused symbols reported by staticcheck (U1000)**

| File | Delete |
|---|---|
| `internal/cli/errors.go` | `printErrorf`, `printWarning`, `printSuccess`, `printInfo`, `suggestSimilarPackages`, `formatNetworkError` |
| `internal/env/runtimes/php.go` | `fixMacOSLibraryPaths`, `fixBinaryLibraryPaths`, `fixLibraryDependencies`, `fixLoaderPathsToRpath` |
| `internal/env/runtimes/python.go` | `pyenvExists` |
| `internal/pm/pm_test.go` | `var commandExecutor` |
| `internal/pm/validation.go` | `mavenCoordinatePattern` (**keep** `composerPackagePattern`; P3 uses it) |
| `internal/search/cargo.go` | `cratesIOResponse`, `searchCargo` |
| `internal/search/composer.go` | `searchComposer` |
| `internal/search/maven.go` | `searchMaven` |
| `internal/search/npm.go` | `npmPackageResponse`, `searchNpm` |
| `internal/search/http.go` | `retryableGet`, `withTimeout` |
| `internal/search/parallel.go` | `searchWithRetry` |
| `internal/tui/search/commands.go` | `resizeCmd`, `installSelectCmd` |
| `internal/tui/search/model.go` | `resizeMsg` |

Do **not** delete `cmdInstallWorkspace` in `internal/cli/workspace_cmd.go`; P6 wires it up. Add `//nolint:unused // wired in P6 (--workspace)` on its line instead.

In `internal/search/search_test.go`, replace every call `contains(` with `strings.Contains(` (add the `"strings"` import). That lets the hand-rolled `contains`/`containsAt`/`toLower` helpers in `http.go` become deletable once nothing else uses them.

- [ ] **Step 3: Repeat until staticcheck finds no unused code**

Run: `go build ./... && staticcheck ./... 2>&1 | grep U1000`
Deleting a symbol can make its helpers unused. Delete each newly reported U1000 symbol and re-run until the command prints nothing. (If `staticcheck` is missing: `go install honnef.co/go/tools/cmd/staticcheck@latest`.)

Also fix SA4031 at `internal/cli/cli.go` (`if autoSelect != nil {` right after `autoSelect = &pm`, around line 346). Remove the always-true `if` and keep its body. Leave SA4009/SA4010 in `node.go`; the P5 rewrite removes that code.

- [ ] **Step 4: Verify behaviour is unchanged**

Run: `go vet ./... && go test -race ./...`
Expected: PASS
Run: `git diff --stat | tail -1`
Expected: roughly 600+ deletions, almost no insertions beyond gofmt.

- [ ] **Step 5: Commit the deletions**

```bash
gofmt -l .   # must print nothing
git add -A
git commit -m "refactor: delete unused code reported by staticcheck"
```

---

### Task 6: Repair CI, lint config, pre-commit and Makefile

**Files:**
- Replace: `.github/workflows/ci.yml`
- Replace: `.golangci.yml`
- Modify: `.pre-commit-config.yaml:1,25`
- Modify: `Makefile:9`

**Interfaces:** Produces: `make build` emits a stripped `./xpm`; the CI job names `test`, `lint`, `build` (branch-protection targets).

- [ ] **Step 1: Replace `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main, develop]

permissions:
  contents: read

jobs:
  test:
    name: Test
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
        go-version: ['1.22', 'stable']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}
          cache: true
      - run: go mod verify
      - run: go test -race -coverprofile=coverage.out ./...
      - if: matrix.os == 'ubuntu-latest' && matrix.go-version == 'stable'
        uses: codecov/codecov-action@v4
        with:
          files: coverage.out
          fail_ci_if_error: false

  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0 # needed for new-from-merge-base
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - uses: golangci/golangci-lint-action@v8
        with:
          version: v2.5.0

  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [test, lint]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: make build
      - run: ./xpm version
```

- [ ] **Step 2: Replace `.golangci.yml` (v2 format, ratcheted to new code)**

```yaml
version: "2"

run:
  timeout: 5m

linters:
  default: standard # errcheck, govet, ineffassign, staticcheck, unused
  enable:
    - bodyclose
    - errorlint
    - gosec
    - misspell
    - unconvert
  settings:
    errcheck:
      exclude-functions:
        - (*os.File).Close
        - (io.Closer).Close
        - (io.ReadCloser).Close
        - os.Remove
        - os.RemoveAll
    gosec:
      excludes:
        - G107 # HTTP GET with variable URL: registry/runtime URLs are built from validated names
        - G110 # decompression bomb: runtime archives are SHA-256 verified before extraction (P2)
        - G204 # subprocess with variable args: running package managers is xpm's job
        - G301 # 0755 dirs: runtime install dirs must be traversable
        - G302 # 0644/0755 files: installed binaries must be executable
        - G304 # file path from variable: xpm reads the user's project files by design
        - G306 # WriteFile 0644: config/lock files are not secrets
  exclusions:
    rules:
      - path: _test\.go
        linters: [errcheck, gosec]

formatters:
  enable:
    - gofmt
    - goimports
  settings:
    goimports:
      local-prefixes:
        - github.com/crenspire/xpm

issues:
  # Ratchet: only code changed since main must be clean. Removed at the end of P6.
  new-from-merge-base: main
```

- [ ] **Step 3: Fix the pre-commit config and Makefile**

In `.pre-commit-config.yaml`, change the header comment `# Pre-commit hooks for upm` to `# Pre-commit hooks for xpm`, and `args: ['-local', 'github.com/crenspire/upm']` to `args: ['-local', 'github.com/crenspire/xpm']`.

In `Makefile`, replace line 9 (`LDFLAGS=-ldflags "... -X main.BuildTime=$(BUILD_TIME)"`) with:

```make
LDFLAGS=-trimpath -ldflags "-s -w -X github.com/crenspire/xpm/internal/cli.Version=$(VERSION)"
```

(`main.BuildTime` doesn't exist, so that `-X` flag did nothing.) Delete the now-unused `BUILD_TIME=` line.

- [ ] **Step 4: Verify locally**

Run: `make build && ./xpm version && ls -la xpm`
Expected: the banner prints, and the binary is noticeably smaller than 14.6 MB (target < 10 MB).
Run: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 config verify`
Expected: no output (the config is valid).

- [ ] **Step 5: Commit, push, confirm CI**

```bash
git add .github/workflows/ci.yml .golangci.yml .pre-commit-config.yaml Makefile
git commit -m "ci: fix binary name, migrate golangci to v2, run on develop, strip release binary"
git push -u origin HEAD
gh run watch --exit-status
```

Expected: `test` (6 matrix legs), `lint` and `build` are green. If Windows `-race` fails because cgo is unavailable, add `if: runner.os != 'Windows'` to the race step and run plain `go test ./...` on Windows instead. **P0 gate ends here: do not start any other lane until this is green.**

---

### Task 7: Parallel exact-match lookup with a hard deadline

**Files:**
- Modify: `internal/search/search.go` (`SearchEverywhere`, at the end of the file)
- Create: `internal/search/everywhere_test.go`

**Interfaces:**
- Produces:
  - `type lookup struct { id pm.ID; fn func(pkg string) (*Result, error) }`
  - `var exactLookups []lookup`, a package-level table that tests swap out
  - `var lookupDeadline = 2500 * time.Millisecond`
  - `var ErrAllRegistriesFailed` and `var ErrRegistryTimeout`
  - `SearchEverywhere(pkg string, opts Options) ([]Result, error)`: same signature, now concurrent. It returns within `lookupDeadline` with results in table order (npm, pip, composer, cargo, maven). It returns an error wrapping `ErrAllRegistriesFailed` only when *every* enabled registry errored or timed out.
- Consumed by: `internal/cli/cli.go:515,714` and `internal/cli/commands.go:344` (unchanged callers; their existing `search error:` branches now actually fire). Task 10 changes one line inside the goroutine to add caching.

**Why a deadline as well as parallelism (measured 2026-10-06):** `search.maven.org` accepted the request and never sent a byte in 5 of 8 tries. One crates.io request took 46 s. With parallelism alone, the slowest registry's timeout still sets the wall time.

- [ ] **Step 1: Write the failing tests**

Create `internal/search/everywhere_test.go`:

```go
package search

import (
	"errors"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

// withLookups swaps in fake registries for one test.
func withLookups(t *testing.T, ls []lookup) {
	t.Helper()
	orig := exactLookups
	exactLookups = ls
	t.Cleanup(func() { exactLookups = orig })
}

func withDeadline(t *testing.T, d time.Duration) {
	t.Helper()
	orig := lookupDeadline
	lookupDeadline = d
	t.Cleanup(func() { lookupDeadline = orig })
}

func fakeLookup(id pm.ID, delay time.Duration, err error) lookup {
	return lookup{id: id, fn: func(pkg string) (*Result, error) {
		time.Sleep(delay)
		if err != nil {
			return nil, err
		}
		return &Result{Manager: id, Name: pkg}, nil
	}}
}

var allIDs = []pm.ID{pm.Npm, pm.Pip, pm.Composer, pm.Cargo, pm.Maven}

func TestSearchEverywhereIsParallelAndOrdered(t *testing.T) {
	var ls []lookup
	for _, id := range allIDs {
		ls = append(ls, fakeLookup(id, 200*time.Millisecond, nil))
	}
	withLookups(t, ls)

	start := time.Now()
	got, err := SearchEverywhere("x", Options{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > 600*time.Millisecond {
		t.Fatalf("took %v for 5×200ms lookups; registries were queried sequentially", elapsed)
	}
	if len(got) != len(allIDs) {
		t.Fatalf("got %d results, want %d", len(got), len(allIDs))
	}
	for i, r := range got {
		if r.Manager != allIDs[i] {
			t.Errorf("result %d = %s, want %s (order must be deterministic)", i, r.Manager, allIDs[i])
		}
	}
}

func TestSearchEverywhereDeadlineDropsStragglers(t *testing.T) {
	withDeadline(t, 150*time.Millisecond)
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, nil),
		fakeLookup(pm.Maven, 5*time.Second, nil), // stalls like search.maven.org
	})
	start := time.Now()
	got, err := SearchEverywhere("x", Options{})
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("took %v; a stalled registry held up the result", elapsed)
	}
	if err != nil {
		t.Fatalf("one straggler must not be an error, got %v", err)
	}
	if len(got) != 1 || got[0].Manager != pm.Npm {
		t.Fatalf("got %+v, want only the npm result", got)
	}
}

func TestSearchEverywhereAllTimedOutIsAnError(t *testing.T) {
	withDeadline(t, 50*time.Millisecond)
	withLookups(t, []lookup{fakeLookup(pm.Npm, time.Second, nil)})
	_, err := SearchEverywhere("x", Options{})
	if !errors.Is(err, ErrAllRegistriesFailed) || !errors.Is(err, ErrRegistryTimeout) {
		t.Fatalf("err = %v, want ErrAllRegistriesFailed wrapping ErrRegistryTimeout", err)
	}
}

func TestSearchEverywherePartialFailureKeepsResults(t *testing.T) {
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, errors.New("boom")),
		fakeLookup(pm.Pip, 0, nil),
	})
	got, err := SearchEverywhere("x", Options{})
	if err != nil {
		t.Fatalf("partial failure must not be an error, got %v", err)
	}
	if len(got) != 1 || got[0].Manager != pm.Pip {
		t.Fatalf("got %+v, want only the pip result", got)
	}
}

func TestSearchEverywhereAllFailedIsAnError(t *testing.T) {
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, errors.New("offline")),
		fakeLookup(pm.Pip, 0, errors.New("offline")),
	})
	_, err := SearchEverywhere("x", Options{})
	if !errors.Is(err, ErrAllRegistriesFailed) {
		t.Fatalf("err = %v, want ErrAllRegistriesFailed", err)
	}
}

func TestSearchEverywhereSkipsDisabled(t *testing.T) {
	called := false
	withLookups(t, []lookup{
		{id: pm.Npm, fn: func(string) (*Result, error) { called = true; return nil, nil }},
	})
	got, err := SearchEverywhere("x", Options{Enable: map[pm.ID]bool{pm.Npm: false}})
	if err != nil || len(got) != 0 || called {
		t.Fatalf("disabled registry was queried (called=%v got=%v err=%v)", called, got, err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/search/ -run SearchEverywhere -v`
Expected: build failure `undefined: lookup` / `exactLookups` / `lookupDeadline` / `ErrAllRegistriesFailed`

- [ ] **Step 3: Implement**

Add `"errors"` to the imports of `internal/search/search.go` (`"time"` is already imported). Replace the `SearchEverywhere` function and its doc comment with:

```go
// lookup is one registry's exact-name check.
type lookup struct {
	id pm.ID
	fn func(pkg string) (*Result, error)
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

	type outcome struct {
		i   int
		res *Result
		err error
	}
	// Buffered so goroutines that finish after the deadline never block.
	ch := make(chan outcome, len(enabled))
	for i, l := range enabled {
		go func(i int, l lookup) {
			res, err := l.fn(pkg)
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

Also update the package doc comment at the top of `search.go`: change "in sequence" to "concurrently" wherever it describes `SearchEverywhere`.

- [ ] **Step 4: Run the tests (race detector, repeated)**

Run: `go test -race -count=3 ./internal/search/ -v -run 'SearchEverywhere'`
Expected: PASS, including the existing `TestSearchEverywhereOptions`. `-count=3` matters: stragglers keep running after the deadline, and the race detector must stay quiet across runs.

- [ ] **Step 5: Commit**

```bash
git add internal/search/search.go internal/search/everywhere_test.go
git commit -m "perf(search): concurrent registry lookups with a 2.5s hard deadline"
```

---

### Task 8: One HTTP helper, npm `/latest`, capped bodies

**Files:**
- Modify: `internal/search/http.go` (add `httpGet`, `statusError`, `maxMetadataBytes`)
- Modify: `internal/search/search.go` (`existsInNpm`; switch `existsInPip`/`existsInComposer`/`existsInCrates`/`existsInMaven` to `httpGet` and capped decoding)
- Create: `internal/search/npm_test.go`

**Interfaces:**
- Produces:
  - `func httpGet(rawURL string) (*http.Response, error)`: sets `User-Agent: xpm (+https://github.com/crenspire/xpm)` and `Accept: application/json`.
  - `func statusError(registry string, resp *http.Response) error`: includes at most 512 bytes of the body.
  - `const maxMetadataBytes = 1 << 20`.
  - `var npmRegistryURL = "https://registry.npmjs.org"`.
- Consumes: `exactLookups` from Task 7 (unchanged).

- [ ] **Step 1: Write the failing tests**

Create `internal/search/npm_test.go`:

```go
package search

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withNpmServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	orig := npmRegistryURL
	npmRegistryURL = srv.URL
	t.Cleanup(func() { npmRegistryURL = orig })
}

func TestExistsInNpmUsesLatestEndpoint(t *testing.T) {
	var gotPath, gotUA string
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA = r.URL.EscapedPath(), r.UserAgent()
		fmt.Fprint(w, `{"name":"@types/node","version":"22.1.0","description":"TS defs"}`)
	})

	r, err := existsInNpm("@types/node")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/@types%2Fnode/latest" {
		t.Errorf("path = %q, want /@types%%2Fnode/latest", gotPath)
	}
	if !strings.HasPrefix(gotUA, "xpm") {
		t.Errorf("User-Agent = %q, want xpm/...", gotUA)
	}
	if r == nil || r.Extra["version"] != "22.1.0" || r.Info != "TS defs" || r.Name != "@types/node" {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInNpmNotFound(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `"Not Found"`, http.StatusNotFound)
	})
	r, err := existsInNpm("nope")
	if err != nil || r != nil {
		t.Fatalf("404 must be (nil, nil), got (%+v, %v)", r, err)
	}
}

func TestExistsInNpmServerErrorIsTruncated(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, strings.Repeat("x", 10_000))
	})
	_, err := existsInNpm("lodash")
	if err == nil || len(err.Error()) > 700 {
		t.Fatalf("want a short status error, got len=%d err=%v", len(fmt.Sprint(err)), err)
	}
}

func TestExistsInNpmCapsBodySize(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"1.0.0","description":"`+strings.Repeat("a", 2<<20)+`"}`)
	})
	if _, err := existsInNpm("huge"); err == nil {
		t.Fatal("a body over maxMetadataBytes must fail to decode, got nil error")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/search/ -run ExistsInNpm -v`
Expected: build failure `undefined: npmRegistryURL`

- [ ] **Step 3: Implement the helper**

Append to `internal/search/http.go` (add `"strings"` to its imports if missing):

```go
// maxMetadataBytes caps every registry response body xpm decodes.
const maxMetadataBytes = 1 << 20 // 1 MiB

const userAgent = "xpm (+https://github.com/crenspire/xpm)"

// httpGet is the single entry point for registry GETs: shared client
// (with its timeout), identifying User-Agent, JSON Accept header.
func httpGet(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	return httpClient.Do(req)
}

// statusError builds an error for a non-2xx response, including at most
// 512 bytes of the body so HTML error pages don't flood the terminal.
func statusError(registry string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("%s registry returned status %d: %s", registry, resp.StatusCode, strings.TrimSpace(string(body)))
}
```

If `http.go` still has a hard-coded `"xpm (https://github.com/crenspire/xpm)"` User-Agent string elsewhere, replace it with `userAgent`.

- [ ] **Step 4: Rewrite `existsInNpm`**

In `internal/search/search.go`, add next to `httpClient`:

```go
// npmRegistryURL is the npm registry base; tests point it at httptest.
var npmRegistryURL = "https://registry.npmjs.org"
```

Replace `existsInNpm` with:

```go
// existsInNpm checks if a package exists in the npm registry.
// It fetches /<pkg>/latest (~2–4 KB) instead of the full packument, which is
// 15 MB+ for packages like typescript and regularly blew the 4s timeout.
// Returns (nil, nil) if the package is not found.
func existsInNpm(pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s/%s/latest", npmRegistryURL, url.PathEscape(pkg))
	logx.Info("query npm: %s", u)
	resp, err := httpGet(u)
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxMetadataBytes)).Decode(&data); err != nil {
		return nil, fmt.Errorf("npm: decode %s: %w", pkg, err)
	}
	return &Result{
		Manager: pm.Npm,
		Name:    pkg,
		Info:    data.Description,
		Extra:   map[string]string{"version": data.Version},
	}, nil
}
```

- [ ] **Step 5: Apply the same hygiene to the other four lookups**

In `existsInPip`, `existsInComposer`, `existsInCrates` and `existsInMaven` (all in `internal/search/search.go`):
- replace `httpClient.Get(` with `httpGet(`;
- replace `json.NewDecoder(resp.Body)` with `json.NewDecoder(io.LimitReader(resp.Body, maxMetadataBytes))`;
- replace each `body, _ := io.ReadAll(resp.Body)` + `fmt.Errorf("... status %d: %s", ...)` pair with `return nil, statusError("<registry>", resp)`. Keep each function's existing `404 → return nil, nil` branch *before* it, and use the registry names `pypi`, `packagist`, `crates.io`, `maven`.

Run: `grep -n 'httpClient.Get\|io.ReadAll(resp.Body)\|NewDecoder(resp.Body)' internal/search/search.go`
Expected: no output.

- [ ] **Step 6: Move Maven and crates.io to endpoints that don't stall**

Both were measured on 2026-10-06:
- `search.maven.org` stalled with no response in 5 of 8 requests. `central.sonatype.com/solrsearch/select` is the same Solr API (same JSON shape, including `latestVersion`) and answered 8 of 8 in about 0.9 s.
- The crates.io crate endpoint returns 441 KB including every version. `?include=default_version` returns 2.7 KB, and its `default_version` field is the newest *stable* release (`max_version` can be a prerelease).

In `internal/search/maven.go`, change the constant to:

```go
const MavenSearchURL = "https://central.sonatype.com/solrsearch/select"
```

In `internal/search/search.go`, in `existsInMaven`, build the URL from the constant:

```go
	url := fmt.Sprintf("%s?q=%s&rows=5&wt=json", MavenSearchURL, url.QueryEscape(pkg))
```

In `existsInCrates`, change the URL, add the field and prefer it:

```go
	url := fmt.Sprintf("%s/crates/%s?include=default_version", CratesIOURL, url.PathEscape(pkg))
```

```go
	var data struct {
		Crate struct {
			Description    string `json:"description"`
			MaxVersion     string `json:"max_version"`
			DefaultVersion string `json:"default_version"`
			Name           string `json:"name"`
		} `json:"crate"`
	}
```

```go
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
```

In `internal/cli/commands.go` (around line 388), change the printed registry link `https://search.maven.org/` to `https://central.sonatype.com/`.

- [ ] **Step 7: Run the tests and verify live**

Run: `go test -race ./internal/search/ -v`
Expected: PASS
Run: `go build -o /tmp/xpm-t ./cmd/xpm && time /tmp/xpm-t which typescript && /tmp/xpm-t which tokio`
Expected:
- `typescript` finishes in about 1 s with npm listed (before this change, npm's part alone was 2.9 s).
- `tokio` lists **cargo**. Before this task every crates.io request returned **403**, because crates.io rejects requests without a User-Agent, so cargo results never appeared.

- [ ] **Step 8: Commit**

```bash
git add internal/search/ internal/cli/commands.go
git commit -m "perf(search): npm /latest, Sonatype Central search, slim crates.io; 1MiB body cap; User-Agent (fixes crates 403)"
```

---

### Task 9: Side-effect-free startup

**Files:**
- Modify: `internal/cli/cli.go` (delete the auto-activate block after `cfg = config.Load()`, around lines 158–163)
- Create: `internal/cli/startup_test.go`

**Interfaces:** None produced. Removes the only caller of `env.ActivateFromLocalEnv` from the hot path. Shims already resolve `.xpm-env` at exec time, and lane B (P5) owns the env package.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/startup_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// chdir is t.Chdir for Go < 1.24.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestRunHelpWritesNoFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)

	// A project with .xpm-env in the root and an installed matching version:
	// exactly the situation in which the old auto-activation rewrote files.
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".xpm-env"), []byte("node=20.11.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".xpm", "env", "runtimes", "node", "20.11.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(proj, "packages", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, sub)

	oldArgs := os.Args
	os.Args = []string{"xpm", "help"}
	t.Cleanup(func() { os.Args = oldArgs })

	if code := Run(); code != 0 {
		t.Fatalf("xpm help exited %d", code)
	}
	if _, err := os.Stat(filepath.Join(sub, ".xpm-env")); err == nil {
		t.Fatal("xpm help created .xpm-env in the current directory")
	}
	if _, err := os.Stat(filepath.Join(home, ".xpm", "env", "active.json")); err == nil {
		t.Fatal("xpm help wrote active.json")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/cli/ -run TestRunHelpWritesNoFiles -v`
Expected: FAIL `xpm help created .xpm-env in the current directory`

- [ ] **Step 3: Implement**

In `internal/cli/cli.go`'s `Run`, delete these lines:

```go
	// Auto-activate versions from .xpm-env
	if cfg.Env.Enabled {
		manager, err := env.NewManager(cfg)
		if err == nil {
			env.ActivateFromLocalEnv(manager)
		}
	}
```

If the `env` import in `cli.go` is now unused, remove it (`go build` will tell you).

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/cli/ -v` → Expected: PASS
Run: `go test ./...` → Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/
git commit -m "perf(cli): drop startup auto-activation that rewrote .xpm-env on every command"
```

---

### Task 10: On-disk lookup cache (repeat lookups in ~60 ms)

**Files:**
- Create: `internal/search/lookupcache.go`, `internal/search/lookupcache_test.go`
- Modify: `internal/search/search.go` (one line inside `SearchEverywhere`'s goroutine, plus the snapshot line above the loop)
- Modify: `internal/search/everywhere_test.go` (`withLookups` also disables the cache)

**Interfaces:**
- Consumes: `lookup`, `exactLookups`, `SearchEverywhere` from Task 7.
- Produces:
  - `var lookupCacheDir string`: `<os.UserCacheDir()>/xpm/lookups`, or `""` when `XPM_NO_CACHE` is set.
  - `var positiveTTL = time.Hour` and `var negativeTTL = 15 * time.Minute`.
  - `func cachedLookup(dir string, l lookup, pkg string) (*Result, error)`.
  - `func cachePath(dir string, id pm.ID, pkg string) string`.

Measured on 2026-10-06: `xpm which axios` takes 1.0–1.6 s cold and **0.06 s** warm.

- [ ] **Step 1: Write the failing tests**

Create `internal/search/lookupcache_test.go`:

```go
package search

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

func countingLookup(id pm.ID, res *Result, err error) (lookup, *int32) {
	var n int32
	return lookup{id: id, fn: func(string) (*Result, error) {
		atomic.AddInt32(&n, 1)
		return res, err
	}}, &n
}

func TestCachedLookupServesRepeatsFromDisk(t *testing.T) {
	dir := t.TempDir()
	l, calls := countingLookup(pm.Npm, &Result{Manager: pm.Npm, Name: "axios", Extra: map[string]string{"version": "1.2.3"}}, nil)

	first, err := cachedLookup(dir, l, "axios")
	if err != nil {
		t.Fatal(err)
	}
	second, err := cachedLookup(dir, l, "axios")
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("registry called %d times, want 1", *calls)
	}
	if second == nil || second.Extra["version"] != first.Extra["version"] {
		t.Fatalf("cached result %+v != original %+v", second, first)
	}
}

func TestCachedLookupCachesNotFound(t *testing.T) {
	dir := t.TempDir()
	l, calls := countingLookup(pm.Pip, nil, nil)
	for i := 0; i < 2; i++ {
		if r, err := cachedLookup(dir, l, "nope"); r != nil || err != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", r, err)
		}
	}
	if *calls != 1 {
		t.Fatalf("registry called %d times, want 1", *calls)
	}
}

func TestCachedLookupNeverCachesErrors(t *testing.T) {
	dir := t.TempDir()
	l, calls := countingLookup(pm.Cargo, nil, errors.New("timeout"))
	cachedLookup(dir, l, "serde")
	cachedLookup(dir, l, "serde")
	if *calls != 2 {
		t.Fatalf("registry called %d times, want 2 (errors must not be cached)", *calls)
	}
}

func TestCachedLookupRefetchesExpiredEntries(t *testing.T) {
	dir := t.TempDir()
	l, calls := countingLookup(pm.Npm, &Result{Manager: pm.Npm, Name: "x"}, nil)
	path := cachePath(dir, pm.Npm, "x")
	stale, _ := json.Marshal(cacheEntry{Found: true, Result: &Result{Name: "old"}, At: time.Now().Add(-2 * positiveTTL)})
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, stale, 0o644)

	r, _ := cachedLookup(dir, l, "x")
	if *calls != 1 || r == nil || r.Name != "x" {
		t.Fatalf("expired entry was served (calls=%d result=%+v)", *calls, r)
	}
}

func TestCacheDisabledByEnv(t *testing.T) {
	t.Setenv("XPM_NO_CACHE", "1")
	if dir := defaultLookupCacheDir(); dir != "" {
		t.Fatalf("XPM_NO_CACHE=1 must disable the cache, got dir %q", dir)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/search/ -run 'Cache' -v`
Expected: build failure `undefined: cachedLookup`

- [ ] **Step 3: Implement the cache**

Create `internal/search/lookupcache.go`:

```go
package search

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

// Exact-lookup results are memoised on disk so repeat commands
// (`xpm which x` then `xpm install x`) skip the network entirely.
// Set XPM_NO_CACHE=1 to bypass.
var (
	lookupCacheDir = defaultLookupCacheDir()
	positiveTTL    = time.Hour        // package found
	negativeTTL    = 15 * time.Minute // package not found (may be published soon)
)

func defaultLookupCacheDir() string {
	if os.Getenv("XPM_NO_CACHE") != "" {
		return ""
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "xpm", "lookups")
}

type cacheEntry struct {
	Found  bool      `json:"found"`
	Result *Result   `json:"result,omitempty"`
	At     time.Time `json:"at"`
}

func cachePath(dir string, id pm.ID, pkg string) string {
	sum := sha256.Sum256([]byte(pkg))
	return filepath.Join(dir, string(id), hex.EncodeToString(sum[:16])+".json")
}

// cachedLookup answers from the cache in dir when a fresh entry exists,
// otherwise calls l.fn and stores the outcome. Errors are never cached.
// dir is passed in (not read from lookupCacheDir) because lookups that miss
// the deadline keep running in the background after SearchEverywhere returns.
// An empty dir disables caching.
func cachedLookup(dir string, l lookup, pkg string) (*Result, error) {
	if dir == "" {
		return l.fn(pkg)
	}
	path := cachePath(dir, l.id, pkg)
	if data, err := os.ReadFile(path); err == nil {
		var e cacheEntry
		if json.Unmarshal(data, &e) == nil {
			ttl := negativeTTL
			if e.Found {
				ttl = positiveTTL
			}
			if time.Since(e.At) < ttl {
				return e.Result, nil
			}
		}
	}

	res, err := l.fn(pkg)
	if err != nil {
		return nil, err
	}
	writeCacheEntry(path, cacheEntry{Found: res != nil, Result: res, At: time.Now()})
	return res, nil
}

// writeCacheEntry writes atomically (temp file + rename) so concurrent xpm
// processes never read a half-written entry. Failures are ignored: the cache
// is an optimisation only.
func writeCacheEntry(path string, e cacheEntry) {
	data, err := json.Marshal(e)
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

- [ ] **Step 4: Use it from `SearchEverywhere`, and keep tests off the real cache**

In `internal/search/search.go` (`SearchEverywhere`), add the snapshot line right after `ch := make(chan outcome, len(enabled))` and change the goroutine's call:

```go
	cacheDir := lookupCacheDir
	for i, l := range enabled {
		go func(i int, l lookup) {
			res, err := cachedLookup(cacheDir, l, pkg)
			ch <- outcome{i: i, res: res, err: err}
		}(i, l)
	}
```

Pass the directory in as a snapshot rather than reading `lookupCacheDir` inside `cachedLookup`. Stragglers that missed the deadline are still running when tests restore the global, and reading it there is a real data race (the race detector caught it during plan validation).

In `internal/search/everywhere_test.go`, replace `withLookups` with:

```go
// withLookups swaps in fake registries and disables the on-disk cache so
// fakes never leak into the user's real cache directory.
func withLookups(t *testing.T, ls []lookup) {
	t.Helper()
	origLookups, origDir := exactLookups, lookupCacheDir
	exactLookups, lookupCacheDir = ls, ""
	t.Cleanup(func() { exactLookups, lookupCacheDir = origLookups, origDir })
}
```

- [ ] **Step 5: Run the tests and measure**

Run: `go test -race -count=3 ./internal/search/` → Expected: PASS
Run:

```bash
go build -o /tmp/xpm-t ./cmd/xpm
time XPM_NO_CACHE=1 /tmp/xpm-t which axios   # cold: ~1-1.6s
/tmp/xpm-t which axios >/dev/null            # prime
time /tmp/xpm-t which axios                  # warm: < 0.1s
```

- [ ] **Step 6: Commit**

```bash
git add internal/search/
git commit -m "perf(search): cache exact lookups on disk (1h found / 15m not-found; XPM_NO_CACHE=1 bypasses)"
```

---

### Task 11: Perf harness with budgets

**Files:**
- Create: `scripts/perf.sh`
- Modify: `Makefile` (add a `perf` target next to `bench`)
- Modify: `internal/search/everywhere_test.go` (add a benchmark)
- Modify: `.github/workflows/ci.yml` (add a non-blocking `perf` job)

**Interfaces:** Produces `make perf`, which exits non-zero if a budget fails. Later phases add rows to `BUDGETS` in `scripts/perf.sh`.

- [ ] **Step 1: Add the benchmark**

Append to `internal/search/everywhere_test.go`:

```go
// BenchmarkSearchEverywhereFanout measures fan-out overhead with instant fakes.
func BenchmarkSearchEverywhereFanout(b *testing.B) {
	origLookups, origDir := exactLookups, lookupCacheDir
	defer func() { exactLookups, lookupCacheDir = origLookups, origDir }()
	exactLookups, lookupCacheDir = nil, ""
	for _, id := range allIDs {
		id := id
		exactLookups = append(exactLookups, lookup{id: id, fn: func(p string) (*Result, error) {
			return &Result{Manager: id, Name: p}, nil
		}})
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := SearchEverywhere("x", Options{}); err != nil {
			b.Fatal(err)
		}
	}
}
```

Run: `go test ./internal/search/ -run '^$' -bench SearchEverywhereFanout -benchmem`
Expected: on the order of microseconds per op (the fan-out itself adds no meaningful latency).

- [ ] **Step 2: Create `scripts/perf.sh`**

```bash
#!/usr/bin/env bash
# Measures xpm against the roadmap's performance budgets.
# Requires: hyperfine, jq. Network-dependent rows need internet access.
set -euo pipefail

command -v hyperfine >/dev/null || { echo "install hyperfine (brew install hyperfine)"; exit 2; }
command -v jq >/dev/null || { echo "install jq (brew install jq)"; exit 2; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$(mktemp -d)/xpm"
go build -trimpath -ldflags "-s -w" -o "$BIN" "$ROOT/cmd/xpm"
WORK="$(mktemp -d)"; cd "$WORK"   # empty dir: no project files, nothing to detect

# name | budget_ms | command
BUDGETS=(
  "help|10|$BIN help"
  "version|10|$BIN --version"
  "which-axios-cold|1500|env XPM_NO_CACHE=1 $BIN which axios"
  "which-typescript-cold|1500|env XPM_NO_CACHE=1 $BIN which typescript"
  "which-axios-warm|100|$BIN which axios"
)

fail=0
printf "%-20s %10s %10s  %s\n" "benchmark" "mean(ms)" "budget" "result"
for row in "${BUDGETS[@]}"; do
  IFS='|' read -r name budget cmd <<<"$row"
  hyperfine --warmup 2 --runs 10 --style none --export-json "$WORK/$name.json" "$cmd" >/dev/null 2>&1
  mean_ms=$(jq '.results[0].mean * 1000 | floor' "$WORK/$name.json")
  if [ "$mean_ms" -le "$budget" ]; then res="PASS"; else res="FAIL"; fail=1; fi
  printf "%-20s %10s %10s  %s\n" "$name" "$mean_ms" "$budget" "$res"
done

size_kb=$(( $(wc -c <"$BIN") / 1024 ))
if [ "$size_kb" -le 10240 ]; then res="PASS"; else res="FAIL"; fail=1; fi
printf "%-20s %10s %10s  %s\n" "binary-size(KB)" "$size_kb" "10240" "$res"

# Startup must not write files.
[ ! -e "$WORK/.xpm-env" ] || { echo "FAIL: startup wrote .xpm-env"; fail=1; }
exit $fail
```

Run: `chmod +x scripts/perf.sh`

- [ ] **Step 3: Wire up `make perf` and a CI job**

In `Makefile`, after the `bench` target, add:

```make
# Check CLI latency/size against roadmap budgets (needs hyperfine + jq + network)
.PHONY: perf
perf:
	./scripts/perf.sh
```

In `.github/workflows/ci.yml`, add this job (network-dependent rows are noisy on shared runners, so it's informational):

```yaml
  perf:
    name: Perf budgets (informational)
    runs-on: ubuntu-latest
    needs: [build]
    continue-on-error: true
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: sudo apt-get update && sudo apt-get install -y hyperfine jq
      - run: ./scripts/perf.sh
```

- [ ] **Step 4: Run it**

Run: `make perf`
Expected: every row PASS:
- help/version ≤ 10 ms;
- cold lookups ≤ 1500 ms mean (hard ceiling 2.5 s from the deadline);
- warm lookups ≤ 100 ms;
- binary < 10 MB;
- no `.xpm-env` written.

If a cold row fails only because of a slow network, re-run before investigating.

- [ ] **Step 5: Commit, then open the phase PR**

```bash
git add scripts/perf.sh Makefile internal/search/everywhere_test.go .github/workflows/ci.yml
git commit -m "perf: add budget harness (make perf) and fan-out benchmark"
git push
gh pr create --base develop --title "P0+P1: green build and speed core" --body "Implements docs/superpowers/plans/2026-10-06-xpm-p0-p1-green-and-fast.md. \`make perf\` output: <paste table>."
```

(Per the user's global rule, the PR body has no AI attribution line.)

---

## Self-review notes

- **Spec coverage:** every P0/P1 scope bullet in the roadmap maps to Tasks 1–11. "Error when every registry fails" is Task 7 (`ErrAllRegistriesFailed`; the CLI's existing `search error:` branch exits 1). The exit code for "no matches", and showing timed-out registries separately from "Not found in", are deliberately left to P3 #8 and #13.
- **Type consistency:** `lookup{id, fn}`, `exactLookups`, `lookupDeadline`, `ErrAllRegistriesFailed`, `ErrRegistryTimeout`, `httpGet`, `statusError`, `maxMetadataBytes`, `npmRegistryURL`, `MavenSearchURL`, `lookupCacheDir`, `cachedLookup(dir, l, pkg)`, `cachePath(dir, id, pkg)`, `splitGlobalFlags` and `loadFrom` are each defined once and referenced with the same names everywhere.
- **Validation:** Plans A and B were applied together to a scratch worktree, and `go build`, `go vet` and `go test -race ./...` all pass. Validation found three issues, all now in the plan:
  - the unused `env` import in `cli.go` (Task 9);
  - the stale-global race (Task 10 Step 4);
  - crates.io returning 403 without a User-Agent (Task 8).
- **Ordering risk:** Task 5 deletes `contains` from `http.go`, so Task 5 Step 2 first switches `search_test.go` to `strings.Contains`. Task 8 adds `userAgent`; Task 5 has already deleted `retryableGet`, its only previous user.
