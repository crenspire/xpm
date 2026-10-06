# Plan P7 — Release & product

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make xpm releasable with one command (goreleaser on a `v*` tag, checksums, SBOM, optional cosign signature, Homebrew tap ready but off until a token exists, a checksum-verifying `install.sh`), give it shell completions, and ship the roadmap's P7 product features: `xpm outdated`, `xpm audit` (OSV.dev), `xpm install --workspace`, `xpm why <pkg>`.

**Spec:** `docs/superpowers/plans/2026-10-06-xpm-roadmap.md`, section "P7 — Release & product", plus the phase-lead dispatch (deferred items: wire `install --workspace`; completions from the single command table in `internal/cli/man.go`).

**Out of scope / deferred:**
- `xpm add` / `xpm rm` manifest editing (roadmap priority 3): deferred to a later phase (see Rulings in the ledger).
- Creating the tag, the GitHub release, the Homebrew tap repo, publishing anything: the user does this in the morning following `docs/RELEASING.md`.
- `internal/env/**`, `internal/cli/env_cmd.go`, `cmd/xpm/main.go`, README env section (P5 owns them).

**Tech stack:** Go 1.22 floor, stdlib `net/http`, `httptest`, `golang.org/x/mod` (already a dependency: `semver`, `module.EscapePath`), goreleaser v2 (CI only), POSIX `sh` for `install.sh`.

## Global Constraints

- Go module floor `go 1.22`; no APIs newer than 1.22 (no t.Chdir, os.CopyFS, range-over-int/func). New module dependencies only if clearly justified and only from golang.org/x/* (record a Ruling).
- Commit messages must NOT contain Co-Authored-By, "Generated with", or Claude-Session lines (a local commit-msg hook rejects them).
- Every task ends with `go build ./... && go vet ./... && go test ./...` green and `gofmt -l .` empty; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...` reports 0 issues.
- No network in unit tests (httptest / fakes / seams). Never run real package managers or real installers in tests; never write to the real $HOME, ~/.xpm, or real user caches in tests (use t.TempDir + env/seams).
- Never `git push`, tag, release, publish, or create repos. Never touch the main checkout at /opt/personal/upm (except reading).
- Keep docs honest: README/man claims must match code; verify claims.
- CI must stay green on linux/macos/windows (Go 1.22 + stable; macOS skips 1.22): guard OS-specific tests with runtime.GOOS skips where needed.

House rules (from `.golangci.yml` and earlier phases):
- `github.com/crenspire/xpm/...` imports go in their own last import group (goimports local-prefixes).
- Wrap errors with `%w`; compare with `errors.Is/As` (errorlint). Close response bodies (bodyclose).
- Writes to an `io.Writer` other than os.Stdout/os.Stderr: `_, _ =` or a `strings.Builder` (errcheck).
- Every command in `commandTable` (internal/cli/man.go) must have `showXHelp` in `showCommandHelp` and a `getManPageData` case in manpage.go (existing tests `TestEveryCommandHasManPage`, `TestGenerateAllManPagesCoversTable` enforce this) and a `case` in `Run`'s switch in cli.go (`TestUsageListsEveryDispatchedCommand`).
- New commands are NOT marked experimental unless stated.
- Text that reaches the terminal from a registry or OSV goes through `search.SanitizeText`.
- Tests that change the working directory use the existing pattern in the package (os.Chdir + t.Cleanup restore); never t.Chdir.

## Exit-status convention for the new report commands (binding)

`outdated`, `audit` and `why` share one convention, documented in their man pages, the man EXIT STATUS section and the README exit-code table:
- `0` — nothing to report (all current / no known vulnerabilities / for `why`: package found and paths printed).
- `1` — something to report (`outdated`: at least one dependency is outdated; `audit`: at least one vulnerability; `why`: the package is not in the dependency graph).
- `2` — usage error (bad flag/argument), or the check could not be completed (project unreadable, OSV/registries unreachable so nothing could be checked, or — for `outdated`/`audit` — some lookups failed and nothing was found to report).

## File map

| File | Change | Task |
|---|---|---|
| `internal/cli/args.go`, `args_test.go` | `installArgs.Workspace` (`-w`, `--workspace`) | 1 |
| `internal/cli/install.go`, `workspace_cmd_test.go` | route `install --workspace` to `cmdInstallWorkspace` | 1 |
| `internal/cli/man.go`, `manpage.go`, `man_test.go` | docs for every new command/flag | 1,2,5,7,8 |
| `internal/cli/completion.go`, `completion_test.go` | new: `xpm completion bash|zsh|fish` | 2 |
| `internal/cli/cli.go` | dispatch new commands; version from build info | 2,5,7,8,9 |
| `internal/search/latest.go`, `latest_test.go`, `endpoints.go` | new: `LatestVersions`, Go proxy lookup | 3 |
| `internal/deps/deps.go`, `version.go`, tests | new: deps from graph, ecosystem mapping, version compare | 4 |
| `internal/cli/outdated_cmd.go`, test | new: `xpm outdated` | 5 |
| `internal/osv/osv.go`, test | new: OSV.dev batch client | 6 |
| `internal/cli/audit_cmd.go`, test | new: `xpm audit` | 7 |
| `internal/graph/paths.go`, test; `internal/cli/why_cmd.go`, test | new: `PathsTo`, `xpm why` | 8 |
| `.goreleaser.yaml`, `.github/workflows/release.yml`, `scripts/install.sh`, `scripts/install_test.go`, `Makefile`, `.gitignore` | release tooling | 9 |
| `README.md`, `docs/RELEASING.md`, roadmap | docs | 10 |

Order: Tasks 1–8 are Go code, sequential (several touch `internal/cli`). Task 9 (release tooling) touches only non-`internal/cli` files except a small `cli.go` version change, and may run in parallel with Tasks 3, 4 or 6 (packages disjoint). Task 10 last.

---

### Task 1: `xpm install --workspace`

**Files:** `internal/cli/args.go`, `internal/cli/args_test.go`, `internal/cli/install.go`, `internal/cli/workspace_cmd_test.go`, `internal/cli/man.go`, `internal/cli/manpage.go`, `internal/cli/man_test.go`.

P6 exposed `cmdInstallWorkspace(global bool) int` in `workspace_cmd.go` (it rejects `global`, installs every detected workspace via `workspace.Install` with seams `workspaceRunner`, `workspaceLookPath`).

- [ ] Add `Workspace bool` to `installArgs`. `parseInstallArgs` accepts `-w`, `--workspace`, `-workspace` anywhere before `--` (like `-g`). `--workspace=…`/`-w=…` are unknown-flag errors.
- [ ] `cmdInstall`: when `ia.Workspace`:
  - with packages → error `error: --workspace installs project dependencies; it cannot be combined with package names` and exit 1;
  - otherwise `return cmdInstallWorkspace(ia.Global)` (which already errors on `-g`).
- [ ] Tests (TDD, table-driven in args_test.go): `-w`, `--workspace`, `-workspace`, `-w -g`, `--workspace=x` (error), `axios -w` (parsed; cmdInstall rejects). In workspace_cmd_test.go: `TestInstallWorkspaceFlagRunsWorkspaceInstall` — a temp dir with an npm workspace (reuse the fixtures/helpers already in workspace_cmd_test.go, fake `workspaceRunner` and `workspaceLookPath`) and `cmdInstall([]string{"--workspace"})` returns 0 and the fake runner saw the install in the root; `TestInstallWorkspaceWithPackagesIsError` (exit 1, runner never called); `TestInstallWorkspaceGlobalIsError` (exit 1).
- [ ] Docs: install help/man synopsis `xpm install [-g|--global] [-w|--workspace] [package[@version] ...] [--]`, option line "`-w, --workspace` Install dependencies in every workspace project (honours workspace.include/exclude and workspace.parallel); cannot be combined with packages or -g". Replace `TestWorkspacesManHasNoInstallWorkspace` with a test asserting the install help mentions `--workspace` and the workspaces help's related commands list `xpm install --workspace`. Add `xpm install --workspace` to `showWorkspacesHelp` RELATED COMMANDS and the workspaces man page SEE ALSO/description where it lists workspace commands.
- [ ] Gates; commit `cli: wire install --workspace`.

### Task 2: Shell completions — `xpm completion bash|zsh|fish`

**Files:** new `internal/cli/completion.go`, `internal/cli/completion_test.go`; `internal/cli/man.go`, `internal/cli/manpage.go`, `internal/cli/cli.go`.

- [ ] Add the row `{"completion", nil, "Print a shell completion script (bash, zsh, fish)", false}` to `commandTable` (place it just before `version`), dispatch `case "completion": return cmdCompletion(rest)` in `Run`.
- [ ] `completion.go`:
  - `var completionFlags = map[string][]string{...}` — flags offered per canonical command name. Initial content: `install: {"-g","--global","-w","--workspace","--"}`, `run: {"-w","--workspace"}`, `graph: {"--json","--svg","--depth","--exec","--workspace","-w"}`, `lock: {"--verify"}`, `man: {"--generate"}`. Later tasks add their commands' flags here.
  - `completionWords() []string` — every command name and every alias from `commandTable` that does not start with `-`, in table order, de-duplicated.
  - `func writeCompletion(w io.Writer, shell string) error` — generates the script for `bash`, `zsh` or `fish` from `commandTable` and `completionFlags`; unknown shell → error `unsupported shell %q (want bash, zsh or fish)`.
  - Behaviour of every script: first word completes to `completionWords()`; after `man` completes command names (canonical names only); after `completion` completes `bash zsh fish`; after `config` completes `show path edit set reset`; after `env` completes `install use list ls-remote current remove`; for a word starting with `-` completes the flags of the canonical command (resolve aliases with the same alias data); otherwise falls back to file completion (bash `-o default`, zsh `_files`, fish default).
  - bash: a `_xpm()` function using `COMPREPLY`/`compgen -W`, registered with `complete -o default -F _xpm xpm`. Must work in bash 3.2 (macOS): no associative arrays, no `mapfile`; use a `case` on the canonical command.
  - zsh: `#compdef xpm` header, a `_xpm` function using `compadd`/`_files`; ends with `compdef _xpm xpm` guarded so sourcing works (`if [ "$funcstack[1]" = "_xpm" ]; then _xpm "$@"; else compdef _xpm xpm; fi`).
  - fish: `complete -c xpm -f -n __fish_use_subcommand -a '<cmd>' -d '<description>'` per command (descriptions from the table, single quotes escaped), plus `-n '__fish_seen_subcommand_from <cmd and aliases>'` lines for flags/subcommands; long flags as `-l name`, short as `-s x`; `--` is skipped for fish.
  - `cmdCompletion(args []string) int`: exactly one argument (the shell); missing/extra/unknown → message on stderr plus `showCommandUsage("completion")`, exit 2 (usage error, consistent with graph). Writes the script to stdout, exit 0.
- [ ] Tests (TDD):
  - `TestCompletionScriptsListEveryCommand`: for each shell, every `completionWords()` entry appears in the script.
  - `TestCompletionFlagsKeysAreCommands`: every `completionFlags` key is a canonical `commandTable` name.
  - `TestCompletionBashSyntax` / `Zsh` / `Fish`: write the script to t.TempDir and run `bash -n` / `zsh -n` / `fish --no-execute` if the shell is on PATH (`exec.LookPath`), else `t.Skip`; skip on windows.
  - `TestCompletionBashCompletes`: if bash is on PATH (skip on windows), source the script in `bash --norc --noprofile -c` and simulate `COMP_WORDS=(xpm ins) COMP_CWORD=1; _xpm; echo "${COMPREPLY[@]}"` → contains `install`; `COMP_WORDS=(xpm i --w) COMP_CWORD=2` → contains `--workspace`; `COMP_WORDS=(xpm man gr) COMP_CWORD=2` → contains `graph`.
  - `TestCompletionUsageErrors`: no args, two args, `powershell` → exit 2.
- [ ] Help + man: `showCompletionHelp` (synopsis `xpm completion <bash|zsh|fish>`, install lines: bash `source <(xpm completion bash)` or save to `/usr/local/etc/bash_completion.d/xpm` / `~/.local/share/bash-completion/completions/xpm`; zsh `xpm completion zsh > "${fpath[1]}/_xpm"`; fish `xpm completion fish > ~/.config/fish/completions/xpm.fish`), man page data case, EXIT STATUS man line "2 on a graph or completion usage error" — update the shared EXIT STATUS line in the man template accordingly (keep `TestGraphAndLockDocsCoverFlagsAndExitStatus` passing).
- [ ] Gates; commit `cli: add xpm completion for bash, zsh and fish`.

### Task 3: `search.LatestVersions` — newest version of many packages, plus Go modules

**Files:** new `internal/search/latest.go`, `internal/search/latest_test.go`; `internal/search/endpoints.go`.

- [ ] Add `goProxyURL = GoProxyURL` (const `GoProxyURL = "https://proxy.golang.org"`) to the endpoint vars, and `GoProxy string` to `Endpoints` / `SetEndpoints` (restore too).
- [ ] `latestGoModule(ctx, module string) (*Result, error)`: `module.EscapePath` (golang.org/x/mod/module) → GET `<goProxyURL>/<escaped>/@latest` via `httpGet`; 404/410 → `(nil, nil)` (not found); other non-200 → `statusError("go proxy", resp)`; body capped at `maxMetadataBytes`, JSON `{"Version":"v1.2.3"}` → `&Result{Manager: pm.GoMod, Name: module, Extra: {"version": v}}`; empty Version → not found.
- [ ] API:

```go
// LatestQuery is one package to look up in one registry.
type LatestQuery struct {
	Manager pm.ID // pm.Npm, pm.Pip, pm.Composer, pm.Cargo, pm.Maven or pm.GoMod
	Name    string
}

// LatestResult is the newest version a registry reports for a query.
// Found=false with Err=nil means the registry answered "no such package".
type LatestResult struct {
	LatestQuery
	Version string
	Found   bool
	Err     error
}

// ErrRegistryDisabled marks a query whose registry is turned off in the config.
var ErrRegistryDisabled = errors.New("registry disabled in config")

// LatestVersions looks up every query concurrently (at most latestConcurrency
// in flight), each bounded by opts' timeout for its registry, through the
// on-disk lookup cache. Results are in query order.
func LatestVersions(queries []LatestQuery, opts Options) []LatestResult
```

  - `latestConcurrency = 8`.
  - Lookup function per manager: the matching entry of `exactLookups` (by id) or `latestGoModule` for `pm.GoMod` (as a `lookup{id: pm.GoMod, fn: latestGoModule}`); unknown manager → `Err: fmt.Errorf("no registry lookup for %s", m)`.
  - `pm.GoMod` is not in `Registries` and has no `search.<id>` switch: it is always enabled. Others: `!Enabled(opts, id)` → `Err: ErrRegistryDisabled` without a request.
  - Each call: `ctx, cancel := context.WithTimeout(context.Background(), opts.timeoutFor(id))`, `cachedLookup(ctx, lookupCacheDir, l, name)`, errors through `asTimeout` and wrapped in `sanitizedError` like `fanOut` does.
  - Exactness: a result counts as found only if its name matches the query: Maven `Result.Name == query`; PyPI compare PEP 503-normalized lowercase (`[-_.]+` → `-`); Composer and npm case-insensitive equal; crates.io case-insensitive with `-`/`_` equivalent; Go exact. A non-matching result → `Found=false`. `Version` comes from `Extra["version"]`; empty → `Found=false`.
- [ ] Tests (httptest, cache disabled via `SetCacheDir("")` restore, endpoints via `SetEndpoints`):
  - `TestLatestVersionsPerRegistry` — fake npm `/left-pad/latest`, PyPI `/requests/json`, Packagist, crates sparse index, Maven solr, Go proxy `/github.com/!burnt!sushi/toml/@latest` (assert the escaped path); each returns a version; results in query order.
  - `TestLatestVersionsNotFoundAndErrors` — 404 → Found=false Err=nil; 500 → Err != nil; disabled registry → `errors.Is(err, ErrRegistryDisabled)` and no request made.
  - `TestLatestVersionsMavenRequiresExactCoordinates` — solr returns `other:artifact` for `g:artifact` → Found=false.
  - `TestLatestVersionsConcurrencyCap` — 30 queries against a handler that tracks max in-flight → max ≤ 8, all answered.
  - `TestLatestVersionsTimeout` — handler blocks until request ctx done; `Options{Timeout: 50ms}` → Err wraps `ErrRegistryTimeout` and the call returns within 2 s.
- [ ] Gates (also `go test -race ./internal/search/`); commit `search: LatestVersions for many packages, Go proxy lookups`.

### Task 4: `internal/deps` — dependencies from the graph, ecosystem mapping, version comparison

**Files:** new `internal/deps/deps.go`, `internal/deps/version.go`, `internal/deps/deps_test.go`, `internal/deps/version_test.go`.

```go
// Package deps turns a dependency graph into the package lists that
// outdated and audit check, and compares versions.
package deps

// Dep is one package at one version in one ecosystem.
type Dep struct {
	Ecosystem string // graph ecosystem: node, python, php, rust, go, java
	Name      string
	Version   string // "" when the project files pin no exact version
}

// Direct returns the direct dependencies of g's projects: children of g.Root
// that are not themselves projects (Metadata["project"] == "true" or roots),
// de-duplicated by (ecosystem, name, version), sorted by ecosystem, name, version.
func Direct(g *graph.DepGraph) []Dep

// All returns every non-project node of g, de-duplicated and sorted the same way.
func All(g *graph.DepGraph) []Dep

// Manager maps a graph ecosystem to the registry that serves it:
// node→pm.Npm, python→pm.Pip, php→pm.Composer, rust→pm.Cargo, java→pm.Maven,
// go→pm.GoMod; ok=false otherwise.
func Manager(ecosystem string) (pm.ID, bool)

// OSVEcosystem maps a graph ecosystem to its OSV.dev name:
// node→"npm", python→"PyPI", php→"Packagist", rust→"crates.io", go→"Go",
// java→"Maven"; ok=false otherwise.
func OSVEcosystem(ecosystem string) (string, bool)

// Pinned reports whether v is a concrete version usable for lookups:
// non-empty, contains a digit, and contains none of "${", "*", "^", "~",
// ">", "<", "=", " ", "||", "x.", ".x", "workspace:", "file:", "link:", "git".
func Pinned(v string) bool
```

`version.go`:

```go
// Compare compares two versions of a package in ecosystem: -1, 0 or +1.
// Go versions use golang.org/x/mod/semver (a missing "v" is added).
// Everything else uses a loose semver-like order: an optional leading "v"
// is dropped, the release part is split on "." into numeric fields compared
// numerically (missing fields are 0, so 1.2 == 1.2.0; a non-numeric field
// compares as a string after all numeric ones), and a version with a
// pre-release suffix (after the first "-", or a PEP 440 "a"/"b"/"rc"/".dev"
// marker directly after a numeric field) sorts before the same release
// without it. Pre-release identifiers compare dot-separated: numeric
// identifiers numerically (rc.2 < rc.10), others lexically. Build metadata
// after "+" is ignored.
func Compare(ecosystem, a, b string) int
```

- [ ] Tests (TDD), table-driven: `Compare` cases incl. `1.2.3<1.10.0`, `1.2==1.2.0`, `v1.2.3==1.2.3`, `1.0.0-rc.2<1.0.0-rc.10`, `1.0.0-beta<1.0.0`, `2.0.0rc1<2.0.0` (python), `1.0.0+build==1.0.0`, go `v0.0.0-2023…-abc < v0.1.0`, go `v2.0.0+incompatible`; Java `5.3.20 < 6.0.0`, `1.0-SNAPSHOT < 1.0`. `Direct`/`All` on graphs built with `graph.NewGraph/AddNode/AddEdge/AddRoot`: project roots excluded, a workspace member node marked project excluded, duplicates removed, sorted. `Manager`, `OSVEcosystem` tables. `Pinned` table (`^1.2.0`, `${ver}`, `""`, `latest`, `1.2.3`, `v1.2.3` true, `workspace:*`).
- [ ] Gates; commit `deps: direct/all dependencies, ecosystem maps, version compare`.

### Task 5: `xpm outdated`

**Files:** new `internal/cli/outdated_cmd.go`, `internal/cli/outdated_cmd_test.go`; `cli.go` (dispatch), `man.go`, `manpage.go`, `completion.go` (flags).

- [ ] Seam: `var latestVersions = search.LatestVersions`.
- [ ] Flags (stdlib `flag`, flags anywhere like `parseGraphArgs`): `--json`, `--workspace`/`-w` (combine all workspace projects via the existing `workspaceGraph`), `--all` (check every locked package, not only direct dependencies). No positional arguments (any → usage error exit 2).
- [ ] Flow: extract the graph (`extractGraph(cwd, opts)` with `Warn` to stderr, or `workspaceGraph`); extraction error → stderr, exit 2. `deps.Direct(g)` (or `deps.All` with `--all`). Split deps into: checkable (`deps.Manager` ok and `deps.Pinned(version)`) and unchecked. Query `latestVersions` once with every checkable dep (de-duplicated by manager+name). Row status: `outdated` if `deps.Compare(eco, current, latest) < 0`; `current` otherwise; `unavailable` on Err (message sanitized); `not found` when Found=false.
- [ ] Human output (stdout): when there are outdated/unavailable/not-found rows, a table with header `ECOSYSTEM  PACKAGE  CURRENT  LATEST  STATUS` (columns aligned with `text/tabwriter`), rows sorted by ecosystem then package, outdated first is NOT required — keep sorted order; only non-current rows are listed. Then a summary line: `N outdated, M up to date` (+ `, K could not be checked` when unavailable/not found > 0). When nothing is outdated and every lookup answered: `All N dependencies are up to date.` When there are no dependencies: `No dependencies found.` (exit 0). Unchecked deps (no pinned version or unsupported ecosystem): a final stderr note `note: K dependencies have no locked version and were not checked (add a lockfile)`. Ecosystem column shows the registry display name from `pm.MetaFor(manager).Name`? — No: show the graph ecosystem (`node`, `python`, …) for stability.
- [ ] `--json` (stdout only JSON, even when empty):

```json
{"dependencies":[{"ecosystem":"node","name":"left-pad","current":"1.0.0","latest":"1.3.0","status":"outdated"}],
 "unchecked":[{"ecosystem":"python","name":"requests","current":""}]}
```
  `dependencies` lists every checked dep (all statuses, incl. `current`; `latest` omitted when unknown; `error` string field for unavailable), sorted; `unchecked` lists the skipped ones. Both arrays are `[]` not `null` when empty.
- [ ] Exit status (binding convention): 1 if any outdated; else 2 if any unavailable (could not be checked); else 0. `not found` alone does not change the exit status (a private package is normal) but appears in the table.
- [ ] Tests: temp project dir with a `package-lock.json` (v3, two direct deps) + `go.mod` (one require) — fake `latestVersions` returning versions; assert table rows, summary, exit 1; all current → message and exit 0; one Err → exit 2 with `unavailable`; `--json` decodes into the shape above with `[]` arrays; requirements.txt with an unpinned dep → listed in JSON `unchecked`, stderr note; unknown flag / positional arg → exit 2; `--all` includes a transitive dep. The fake must assert it was called once with de-duplicated queries (manager `npm` for node, `gomod` for go).
- [ ] Docs: `commandTable` row `{"outdated", nil, "Show dependencies with newer versions, across ecosystems", false}` (after `list`); help + man page (registries used: npm, PyPI, Packagist, crates.io sparse index, Maven Central, proxy.golang.org; uses the lookup cache and the registry timeouts; reads lockfiles; exit status per convention); `completionFlags["outdated"] = {"--json","--all","--workspace","-w"}`.
- [ ] Gates; commit `cli: add xpm outdated`.

### Task 6: `internal/osv` — OSV.dev batch client

**Files:** new `internal/osv/osv.go`, `internal/osv/osv_test.go`.

```go
// Package osv queries the OSV.dev vulnerability database.
package osv

// DefaultBaseURL is the OSV.dev API.
const DefaultBaseURL = "https://api.osv.dev"

// Package is one package version to check.
type Package struct {
	Ecosystem string // OSV ecosystem name: npm, PyPI, Packagist, crates.io, Go, Maven
	Name      string
	Version   string
}

// Vuln is one vulnerability, with details when they could be fetched.
type Vuln struct {
	ID       string   `json:"id"`
	Aliases  []string `json:"aliases,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Severity string   `json:"severity,omitempty"` // database_specific.severity (e.g. HIGH), else ""
	Fixed    []string `json:"fixed,omitempty"`    // "fixed" events for this package's ecosystem+name, sorted, de-duplicated
}

// Client talks to OSV.dev.
type Client struct {
	BaseURL string       // "" means DefaultBaseURL
	HTTP    *http.Client // nil means a client without its own timeout (ctx bounds every call)
}

// QueryBatch returns, for each package (same order), the vulnerabilities
// affecting it. It sends POST /v1/querybatch in chunks of at most 1000
// queries, follows per-query next_page_token pages, then fetches each
// distinct vulnerability's details with GET /v1/vulns/{id} (at most 8 in
// flight). A failed details fetch keeps the vulnerability with its ID only
// (detailsErr counts them); a failed batch query returns an error.
func (c *Client) QueryBatch(ctx context.Context, pkgs []Package) (results [][]Vuln, detailsErrs int, err error)
```

- [ ] Request body: `{"queries":[{"package":{"ecosystem":E,"name":N},"version":V}, …]}`; Go versions are sent without a leading `v` (OSV's Go convention). Headers: `Content-Type: application/json`, `Accept: application/json`, `User-Agent: xpm (+https://github.com/crenspire/xpm)`. Response `{"results":[{"vulns":[{"id":"GHSA-…","modified":"…"}],"next_page_token":"…"}]}`; a result with a `next_page_token` is re-queried alone with `"page_token"` in its query (loop up to 10 pages per package). Non-200 → error with status and ≤512 bytes of body; response bodies capped (batch 16 MiB, details 4 MiB); `len(results) != len(queries)` → error.
- [ ] Details: `{"id","summary","aliases","database_specific":{"severity":…},"affected":[{"package":{"ecosystem","name"},"ranges":[{"events":[{"introduced":…},{"fixed":…}]}]}]}`. `Fixed` collects `fixed` events of affected entries whose package ecosystem and name equal the queried package (Go: compare without leading `v`). `Severity` only when `database_specific.severity` is a string. Summary passes through `search.SanitizeText`? — no: osv must not import search; the CLI sanitizes when printing. Results' vulns sorted by ID.
- [ ] Tests (httptest): single batch with two packages (one vulnerable), details fetched; Go version sent without `v`; chunking (2500 packages → 3 POSTs of 1000/1000/500; results stitched in order); pagination (`next_page_token` → second request carries `page_token`, vulns merged); batch 500 → error; results length mismatch → error; details 404 → vuln kept with ID only and `detailsErrs == 1`; ctx cancelled → error; empty input → no request, empty result.
- [ ] Gates (+ `-race` for the package); commit `osv: OSV.dev batch client with details`.

### Task 7: `xpm audit`

**Files:** new `internal/cli/audit_cmd.go`, `internal/cli/audit_cmd_test.go`; `cli.go`, `man.go`, `manpage.go`, `completion.go`.

- [ ] Seam: `var osvBaseURL = osv.DefaultBaseURL` (tests point it at httptest).
- [ ] Flags (anywhere): `--json`, `--timeout <duration>` (default `30s`; must be > 0; parse with `time.ParseDuration`; a bare number like `20` is accepted as seconds), `--workspace`/`-w`. No positional args (usage error, exit 2).
- [ ] Flow: extract graph (like outdated); `deps.All(g)`; keep deps with `deps.OSVEcosystem` ok and `deps.Pinned`; the rest are "unchecked". One `osv.Client{BaseURL: osvBaseURL}.QueryBatch` under `context.WithTimeout(timeout)`. Batch error → stderr `error: OSV.dev query failed: …` exit 2. `detailsErrs > 0` → stderr warning `warning: details for K vulnerabilities could not be fetched; IDs are listed`.
- [ ] Human output: for each vulnerable package (sorted by ecosystem, name, version): `name@version (ecosystem)` then one indented line per vuln: `ID  [SEVERITY]  summary  (fixed in: a, b)` — severity bracket and fixed part omitted when empty; all OSV text through `search.SanitizeText`; aliases shown after ID in parentheses if present (first 3). Then `Found V vulnerabilities in P packages (N packages scanned).` or `No known vulnerabilities in N packages.`; unchecked note on stderr like outdated. No dependencies → `No dependencies found.` exit 0.
- [ ] `--json`:

```json
{"scanned":12,"vulnerable":[{"ecosystem":"node","name":"lodash","version":"4.17.15","vulns":[{"id":"GHSA-…","aliases":["CVE-…"],"summary":"…","severity":"HIGH","fixed":["4.17.21"]}]}],
 "unchecked":[{"ecosystem":"python","name":"requests","version":""}]}
```
  arrays never `null`.
- [ ] Exit: 1 if any vuln; 0 otherwise; 2 on usage error/extraction error/OSV failure.
- [ ] Tests: httptest OSV fake (batch + details) with an npm lockfile project: vulnerable → exit 1, output lists ID, severity, fixed; clean → exit 0 message; OSV 500 → exit 2; `--timeout 50ms` against a hanging server → exit 2 within 2 s; `--timeout 0` / `--timeout abc` → exit 2; `--json` shape; control characters in an OSV summary are stripped from human output; the batch request contains only pinned packages and maps node→npm.
- [ ] Docs: `commandTable` row `{"audit", nil, "Check locked dependencies for known vulnerabilities (OSV.dev)", false}` (after `outdated`); help + man page: sends package names and versions from the lockfiles to api.osv.dev (privacy note), one batch request per 1000 packages plus one details request per distinct vulnerability; `xpm doctor` keeps running the ecosystems' own audit tools (npm audit, pip-audit, …) offline-from-xpm's-view; exit convention. `completionFlags["audit"] = {"--json","--timeout","--workspace","-w"}`.
- [ ] Gates; commit `cli: add xpm audit (OSV.dev)`.

### Task 8: `xpm why <pkg>`

**Files:** new `internal/graph/paths.go`, `internal/graph/paths_test.go`, `internal/cli/why_cmd.go`, `internal/cli/why_cmd_test.go`; `cli.go`, `man.go`, `manpage.go`, `completion.go`.

```go
// PathsTo returns dependency paths from g's roots to every node named name
// (all versions), each path a list of node IDs from a root to the target.
// It walks parents depth-first from each target (parents in sorted order),
// never revisiting a node within one path (cycles are cut), and stops after
// limit paths (limit <= 0 means no limit). truncated reports whether more
// paths exist. Paths are returned sorted by length, then lexically by IDs.
// A target that is itself a root yields the one-node path [target].
func PathsTo(g *DepGraph, name string, limit int) (paths [][]string, truncated bool)
```

- [ ] Root test: a node is a path start if it is in `g.Root` (use the root set) or has no parents.
- [ ] `xpm why <package> [--json] [--limit N] [--workspace|-w]`, flags anywhere, exactly one positional; `--limit` default 10, `0` = all, negative → usage error. Extract graph like `graph` (no `--exec`); `graph.NormalizeGraph(g)` first like cmdGraph.
- [ ] Not in graph → stderr `error: package %q not found in the dependency graph`, exit 1. Found → stdout:

```
lodash@4.17.21 (node)
  my-app > express@4.18.2 > body-parser@1.20.1 > lodash@4.17.21
  my-app > lodash@4.17.21
```
  one block per target version (sorted by node ID); each path line joins node `ShortString()` (name@version, or name) with ` > `; when truncated a last line `  … more paths (use --limit 0 to show all)`. Exit 0.
- [ ] `--json`: `{"package":"lodash","paths":[["node:my-app@1.0.0","node:express@4.18.2",…]],"truncated":false}` (IDs).
- [ ] Tests: graph package — diamond (two paths, sorted by length), cycle (terminates), limit truncation, target is root, missing name → nil; perf guard: a 30-level layered graph with 2 nodes per layer fully connected (2^30 paths) with limit 10 returns within 1 s. CLI — package-lock fixture with nested dep, path printed, exit 0; missing → exit 1; no args / two args / `--limit -1` → exit 2; `--json` shape.
- [ ] Docs: `commandTable` row `{"why", nil, "Show why a package is in the dependency graph", false}` (after `graph`); help + man; `completionFlags["why"] = {"--json","--limit","--workspace","-w"}`.
- [ ] Gates; commit `cli: add xpm why`.

### Task 9: Release tooling — goreleaser, release workflow, install.sh

**Files:** new `.goreleaser.yaml`, `scripts/install.sh`, `scripts/install_test.go`; rewrite `.github/workflows/release.yml`; `Makefile`, `.gitignore`; `internal/cli/cli.go` + test (version from build info).

- [ ] `.goreleaser.yaml` (`version: 2`, project_name `xpm`):
  - `before.hooks`: `go mod download`; `sh -c 'rm -rf completions manpages && mkdir -p completions manpages'`; `sh -c 'go run ./cmd/xpm completion bash > completions/xpm.bash'`, same for `zsh` (`completions/_xpm`) and `fish` (`completions/xpm.fish`); `go run ./cmd/xpm man --generate manpages`. (Task 2's `completion` command must exist; if Task 9 runs before Task 2 lands, the snapshot validation is re-run after Task 2.)
  - `builds`: id xpm, main `./cmd/xpm`, binary `xpm`, `env: [CGO_ENABLED=0]`, goos linux/darwin/windows, goarch amd64/arm64, `flags: [-trimpath]`, `ldflags: ["-s -w -X github.com/crenspire/xpm/internal/cli.Version={{.Version}}"]`, `mod_timestamp: '{{ .CommitTimestamp }}'`.
  - `archives`: id default, `formats: [tar.gz]`, `format_overrides: [{goos: windows, formats: [zip]}]`, `name_template: '{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}'`, `files: [README.md, LICENSE, completions/*, manpages/*]`.
  - `checksum.name_template: checksums.txt`, algorithm sha256.
  - `sboms: [{artifacts: archive}]` (needs `syft` on PATH; CI installs it).
  - `snapshot.version_template: '{{ incpatch .Version }}-next'`.
  - `changelog`: sort asc, exclude `^docs:`, `^test:`, `^chore:`, merge commits.
  - `release`: github owner `crenspire`, name `xpm`, `prerelease: auto`, `draft: false`.
  - Homebrew: `brews` (or `homebrew_casks` if the installed goreleaser marks `brews` deprecated and `check` fails on it — record which) for repository `crenspire/homebrew-tap`, token `'{{ envOrDefault "HOMEBREW_TAP_GITHUB_TOKEN" "" }}'`, `skip_upload: '{{ if eq (envOrDefault "HOMEBREW_TAP_GITHUB_TOKEN" "") "" }}true{{ else }}auto{{ end }}'`, homepage, description, license MIT (check LICENSE), install installs the binary plus bash/zsh/fish completions and man pages, `test: system "#{bin}/xpm", "version"`.
- [ ] `.github/workflows/release.yml`: on `push: tags: ['v*']`; `permissions: contents: write, id-token: write`; job `test` (ubuntu, Go stable, `go test -race ./...`); job `release` needs test: checkout `fetch-depth: 0`, setup-go stable, `anchore/sbom-action/download-syft@v0`, `goreleaser/goreleaser-action@v6` with `distribution: goreleaser`, `version: '~> v2'`, `args: release --clean`, env `GITHUB_TOKEN`, `HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}`; then signing steps all `continue-on-error: true`: `sigstore/cosign-installer@v3`, `cosign sign-blob --yes --output-signature dist/checksums.txt.sig --output-certificate dist/checksums.txt.pem dist/checksums.txt`, `gh release upload "$GITHUB_REF_NAME" dist/checksums.txt.sig dist/checksums.txt.pem --clobber` (env `GH_TOKEN`). A comment explains the signature is best-effort and how to verify (`cosign verify-blob --certificate-identity-regexp 'https://github.com/crenspire/xpm/.*' --certificate-oidc-issuer https://token.actions.githubusercontent.com --signature checksums.txt.sig --certificate checksums.txt.pem checksums.txt`).
- [ ] `scripts/install.sh` (POSIX sh, `set -eu`), usage via env vars and flags:
  - `XPM_VERSION` / `--version vX.Y.Z` (default: latest, resolved from `https://api.github.com/repos/crenspire/xpm/releases/latest` `"tag_name"` with sed; a version without `v` gets one);
  - `XPM_INSTALL_DIR` / `--dir` (default `/usr/local/bin` if writable, else `$HOME/.local/bin`);
  - `XPM_DOWNLOAD_URL` (base, default `https://github.com/crenspire/xpm/releases/download`; files at `$base/$tag/xpm_${ver}_${os}_${arch}.tar.gz` and `$base/$tag/checksums.txt`) — for mirrors and tests;
  - OS from `uname -s` (Linux→linux, Darwin→darwin, else error — Windows users download the zip), arch from `uname -m` (x86_64/amd64→amd64, arm64/aarch64→arm64, else error);
  - downloader: curl (`-fsSL`) or wget (`-qO`); sha256: `sha256sum` or `shasum -a 256`; none → error;
  - in a `mktemp -d` dir (trap removes it): download archive + checksums; the archive's expected sum is the line whose second field equals the archive name exactly; missing → `error: checksums.txt has no entry for <archive>` exit 1; mismatch → `error: checksum mismatch for <archive> (expected X, got Y); not installing` exit 1, nothing installed; then `tar -xzf`, `install -m 0755` (fallback cp+chmod) `xpm` into the dir (mkdir -p), print `xpm <tag> installed to <dir>/xpm` and a PATH hint if the dir is not in PATH.
  - `--help` prints usage.
- [ ] `scripts/install_test.go` (package `scripts_test`; skip on windows; skip if `sh`, `tar` or a sha256 tool is missing): builds a fake release in t.TempDir (`xpm` shell script that echoes `fake xpm`, tarred as `xpm_1.2.3_<goos>_<goarch>.tar.gz` for the test machine's `runtime.GOOS/GOARCH`, and a matching `checksums.txt` with also a decoy line for another archive), serves it with `httptest.NewServer(http.FileServer)` under `/v1.2.3/`, runs `sh scripts/install.sh` with `XPM_VERSION=v1.2.3`, `XPM_DOWNLOAD_URL=<server>`, `XPM_INSTALL_DIR=<tmp>/bin`, `HOME=<tmp>` → exit 0 and `<tmp>/bin/xpm` exists and runs; tampered archive (checksums.txt sum of different bytes) → non-zero, output contains `checksum mismatch`, `<tmp>/bin/xpm` absent; archive missing from checksums.txt → non-zero, `no entry`. Unsupported arch can't be faked easily — skip. Locate the script via the test's working directory (`scripts/`): `filepath.Abs("install.sh")`.
- [ ] Version from build info: in `cli.go`, `Version` default stays as is; add `func versionString() string` that returns `Version` unless it equals the default `"0.0.1"`… — rule: if `Version == "0.0.1"` (not set by ldflags) and `debug.ReadBuildInfo()` reports a main module version other than `""`/`"(devel)"`, return that version without the leading `v`. `printBanner` uses `versionString()`. Test `TestVersionStringPrefersLdflags` / `TestVersionStringFromBuildInfo` via a seam `var readBuildInfo = debug.ReadBuildInfo`. This makes `go install github.com/crenspire/xpm/cmd/xpm@v0.1.0` report 0.1.0.
- [ ] `Makefile`: `snapshot` target → `go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=publish,sbom`; `release-check` → `… check`. Keep existing targets. `.gitignore`: `/completions/`, `/manpages/`, `/man/` (dist/ already ignored).
- [ ] Validate: `goreleaser check` and `goreleaser release --snapshot --clean --skip=publish` (add `--skip=sbom` if syft is not installed locally — record it) using an installed goreleaser v2 binary or `go run github.com/goreleaser/goreleaser/v2@latest`; inspect `dist/` (6 archives + checksums.txt; one archive lists README.md, LICENSE, completions, manpages, xpm); run `sh scripts/install.sh` against `dist/` via a local `python3 -m http.server` or the Go test approach with the real snapshot archive for the host platform; record outputs in the report; then `rm -rf dist completions manpages`.
- [ ] Gates; commit `release: goreleaser, release workflow, checksum-verifying install.sh`.

### Task 10: Documentation — README, RELEASING.md, roadmap status, honesty pass

**Files:** `README.md` (not the env section), new `docs/RELEASING.md`, `docs/superpowers/plans/2026-10-06-xpm-roadmap.md`, `internal/cli/man.go`/`manpage.go` only for corrections found by the honesty pass.

- [ ] README:
  - Install: `curl -fsSL https://raw.githubusercontent.com/crenspire/xpm/main/scripts/install.sh | sh` (verifies SHA-256 against the release's checksums.txt; `XPM_VERSION`, `XPM_INSTALL_DIR`), Homebrew (`brew install crenspire/tap/xpm`, available once the tap is published — say so), `go install …@latest`, prebuilt archives on the Releases page, build from source. State honestly that the first release (v0.1.0) is pending if no tag exists yet.
  - Shell completions section.
  - Status table: add rows for `outdated`, `audit`, `why`; Monorepos row: `install --workspace` now wired.
  - Usage sections for outdated / audit / why with examples verified against the code (flags, exit codes, JSON shapes).
  - Exit-code table: the `0/1/2` convention for outdated/audit/why; `2` also for `completion` usage errors.
  - Workspace paragraph: `workspace.parallel` applies to `install --workspace` now (remove "once it is wired").
- [ ] `docs/RELEASING.md`: prerequisites (green CI on `main`/`develop`, push rights); exact morning steps: merge develop → main (`git checkout main && git merge --ff-only develop` or PR), `git tag -a v0.1.0 -m "xpm v0.1.0"`, `git push origin v0.1.0` — ONE command to release once main is ready: `git tag -a v0.1.0 -m v0.1.0 && git push origin v0.1.0`; what CI does (tests, goreleaser builds 6 targets, archives with completions + man pages, checksums.txt, SBOMs, GitHub release with changelog, best-effort cosign signature of checksums.txt, Homebrew formula only when the secret exists); local dry run (`make snapshot`); enabling the tap (create `crenspire/homebrew-tap` repo, fine-grained PAT with contents:write on it, add repo secret `HOMEBREW_TAP_GITHUB_TOKEN`, re-run or next tag); verifying a release (`sha256sum -c`, cosign verify-blob command); rollback (delete release + tag).
- [ ] Roadmap: add a `Status` column to the "Phases at a glance" table: P0–P4, P6 `Done (merged to develop)`; P5 `In progress` (or `Done` if merged by then — check `git log develop`); P7 `Release tooling + outdated/audit/why/completions done; v0.1.0 tag pending (user)`; mention add/rm deferred.
- [ ] Honesty pass: for each README/man claim about the new commands, check the code (flags exist, exit statuses, JSON field names, defaults like `--limit 10`, `--timeout 30s`). Fix docs, not code, unless the code is wrong (then report).
- [ ] Gates; commit `docs: release guide, install options, outdated/audit/why, roadmap status`.

## Self-review notes

- Spec coverage: goreleaser multi-arch + checksums + SBOM + cosign (guarded) + tap (configured, off) → Task 9; install.sh with checksum → Task 9; completions → Task 2; outdated → Tasks 3–5; audit → Tasks 4, 6, 7; add/rm → deferred (ruling); why → Task 8; install --workspace → Task 1; docs → Task 10.
- The completion map is extended by Tasks 5, 7, 8; `TestCompletionFlagsKeysAreCommands` keeps it honest.
- Tasks 5, 7, 8 each add a `commandTable` row, help, man page and dispatch; existing man tests enforce consistency.

## Addendum (controller update, 2026-10-08)

- Task 9 also adds a CI job to `.github/workflows/ci.yml`: `env-smoke`, matrix ubuntu-latest + macos-latest (not Windows), `timeout-minutes: 10`, `continue-on-error: false`: builds xpm (`go build -o "$RUNNER_TEMP/bin/xpm" ./cmd/xpm`), sets `HOME` to a fresh temp dir, runs `xpm env install node@20`, prepends `$HOME/.xpm/env/shims` to PATH and runs `node -v` (must print `v20.`), and `xpm env current`.
- Task 10 records the measured shim overhead (5.1–5.6 ms vs the < 5 ms budget) in the roadmap budget table and README performance notes instead of adding a dedicated shim binary.
