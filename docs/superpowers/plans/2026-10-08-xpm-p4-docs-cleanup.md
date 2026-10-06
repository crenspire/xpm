# Plan D — P4 Scope cut & docs truth

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** What xpm says about itself (usage, `xpm man`, generated man pages, README, config docs) matches what the code does; unfinished subsystems are labelled experimental; the upm→xpm rename is finished in owned files; registry-supplied text cannot inject terminal escape sequences; dead code and test-hygiene debt from P3 is paid in owned packages; CI is green on Windows again.

**Spec:** roadmap section "P4 — Scope cut & docs truth" in `docs/superpowers/plans/2026-10-06-xpm-roadmap.md`, plus the P4-tagged/deferred items of the P3 ledger (`.superpowers/overnight/p3-ledger.md`) listed in the phase dispatch.

**Architecture:** no new subsystems. One exported helper per concern:
- `config.Path()` — the single source of the config-file location (used by `config` itself, `internal/cli/config_cmd.go`, and test helpers).
- `search.SanitizeText(string) string` — strips terminal control characters and escape sequences; applied once where registry results leave the search package (`fanOut`), and defensively in the TUI renderer.
- One shared command table (`commandTable` in `internal/cli/man.go`) drives both `usage()` and `xpm man`'s listing, with an `experimental` flag.

**Tech Stack:** Go 1.22 (module floor), stdlib only, golangci-lint v2.5.0.

## Global Constraints (binding, verbatim from the phase instructions)

- Go module floor `go 1.22`; no APIs newer than 1.22 (no t.Chdir, os.CopyFS, range-over-int/func). New module dependencies only if clearly justified and only from golang.org/x/* (record a Ruling).
- Commit messages must NOT contain Co-Authored-By, "Generated with", or Claude-Session lines (a local commit-msg hook rejects them).
- Every task ends with `go build ./... && go vet ./... && go test ./...` green and `gofmt -l .` empty; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...` reports 0 issues.
- No network in unit tests (httptest / fakes / seams). Never run real package managers or real installers in tests; never write to the real $HOME, ~/.xpm, or real user caches in tests (use t.TempDir + env/seams).
- Never `git push`, tag, release, publish, or create repos. Never touch the main checkout at /opt/personal/upm (except reading).
- Keep docs honest: README/man claims must match code; verify claims.
- CI must stay green on linux/macos/windows (Go 1.22 + stable; macOS skips 1.22): guard OS-specific tests with runtime.GOOS skips where needed.

**File ownership (this phase may edit ONLY):** `internal/cli/**` except `env_cmd.go`, `graph_cmd.go`, `lock_cmd.go`, `workspace_cmd.go`, `cache_cmd.go` and `cmdDoctor` in `cli.go`; `internal/search/**`, `internal/pm/**`, `internal/config/**`, `internal/tui/**`, `internal/scripts/**`, `internal/logx/**`; `README.md` except its env/graph/lock/workspace/cache/doctor sections; this plan. Other phases (P5: `internal/env`; P6: graph/lock/workspace/cache/doctor) run in parallel.

Work in `/opt/personal/upm/.worktrees/p4` (branch `feat/p4-docs-cleanup`). Implementers `git add` only the files they changed.

## Phase-level rulings (made before execution)

- **R1 `--global=false` / `-g=true`:** stay rejected. A boolean that is false is the same as omitting it; accepting `=value` forms adds parsing surface for no user value. The unknown-flag error must say so: `unknown flag "--global=false" (install accepts -g/--global; omit it for a local install)`. README already documents the rejection. Cost if wrong: a script using `--global=false` must drop the flag.
- **R2 unread config keys:** remove `lock.autoGenerate`, `lock.autoVerify` (the whole `LockConfig`/`Config.Lock`), `workspace.enabled`, and `env.default` from `config.Config` — no code reads them, and `json.Unmarshal` ignores unknown keys, so old config files still load. Keep `workspace.include`/`workspace.exclude` because roadmap P6 item 6 ("honour include/exclude") wires them in the parallel P6 branch; their doc comment says they are not yet honoured. Cost if wrong: P6 must re-add a field (compile error on merge, trivially fixed).
- **R3 hint wording:** every "here is how to install the missing tool" line uses the prefix `To install it:`; the word `Hint:` is not printed by xpm. Cost if wrong: cosmetic.
- **R4 upm fallback:** `[tool.xpm.scripts]` / `[package.metadata.xpm.scripts]` are read first; `[tool.upm.scripts]` / `[package.metadata.upm.scripts]` are read only when the xpm table is absent or empty (whole-table fallback, not per-key merge). Cost if wrong: a project with both tables loses upm-only names (unlikely; documented).
- **R5 upm remnants outside owned files** (`internal/doctor/output.go` "UPM Doctor+ Report", `internal/cache/object.go` and `internal/lock/lock.go` comments, `.gitignore` `/upm`): not edited here; listed in the phase report for P6 / controller. Cost if wrong: roadmap's `grep -ri upm` exit criterion is met only after P6 merges.
- **R6 README smoke script** (`scripts/readme-smoke.sh`): out of owned files; README install examples are already covered by `TestREADMEInstallExamples`. Deferred to P7. Cost if wrong: non-install README examples stay untested.

---

### Task 1: Config path seam — fix Windows CI

**Why:** CI on `develop` (f747c56) fails on `windows-latest` only: `internal/cli` `TestREADMEInstallExamples` rows that set `prefer` via config ("global install, flag after the package", "empty dir: prefer settles npm vs pip", "empty dir: prefer settles express", "global install in a python project follows prefer") exit 1 with `ran []`. The test helper `writeConfig` (internal/cli/helpers_test.go) writes `os.UserHomeDir()/.config/xpm/xpmrc.json`; on Windows `os.UserHomeDir()` reads `%USERPROFILE%` (not `$HOME`) and `config.configPath()` reads `%APPDATA%\xpm\xpmrc.json`, so the config is never seen.

**Files:**
- Modify: `internal/config/config.go` (export `Path()`)
- Modify: `internal/config/config_test.go`
- Modify: `internal/cli/config_cmd.go` (`getConfigPath` delegates to `config.Path`)
- Modify: `internal/cli/helpers_test.go` (`writeConfig`)
- Modify: `internal/cli/config_cmd_test.go` (`isolatedHome`), and any other test in `internal/cli` that sets HOME/APPDATA by hand (`startup_test.go`, `tty_test.go`) — make them call `isolatedHome`.

- [ ] **Step 1:** In `internal/config/config.go` rename `configPath` to exported `Path` with doc comment `// Path returns the platform-specific config file path, or "" if it cannot be determined. Windows: %APPDATA%\xpm\xpmrc.json; elsewhere: $HOME/.config/xpm/xpmrc.json.` Update `Load()` and `TestConfigPath`.
- [ ] **Step 2:** `internal/cli/config_cmd.go`: delete the body of `getConfigPath` and make it `return config.Path()` (or replace all its call sites with `config.Path()` and delete it; drop now-unused imports).
- [ ] **Step 3:** `isolatedHome(t)` sets `HOME`, `USERPROFILE` and `APPDATA` to the same `t.TempDir()` and returns that dir. `writeConfig(t, data)` writes to `config.Path()` (MkdirAll its parent) and fails the test if it is empty. Grep `internal/cli/*_test.go` for `Setenv("HOME"`; every hand-rolled HOME/APPDATA pair becomes `isolatedHome(t)`.
- [ ] **Step 4:** Add `TestWriteConfigIsWhatLoadReads` in `internal/cli/config_cmd_test.go`: `isolatedHome(t)`; `writeConfig(t, `{"prefer":["pip"]}`)`; `config.Load().Prefer` must equal `[]string{"pip"}`. This test runs on every OS and is the regression guard (on Windows it failed before the fix).
- [ ] **Step 5:** Add `TestPathFollowsEnv` in `internal/config/config_test.go`: set `HOME`, `USERPROFILE`, `APPDATA` to a temp dir; `Path()` must have that dir as prefix, on every OS (on Windows: `filepath.Join(dir, "xpm", "xpmrc.json")`; elsewhere `filepath.Join(dir, ".config", "xpm", "xpmrc.json")`).
- [ ] **Step 6:** Replace the two no-op tests `TestLoadValidJSON` / `TestLoadInvalidJSON` (they write to `.config/upm/upmrc.json` and assert nothing) with real ones using the existing `loadFrom(path)` + `writeConfig` test helper in `config_test.go`: valid JSON → fields applied; invalid JSON → defaults returned AND the warning written to `warnOut` (capture by swapping `warnOut` with a `bytes.Buffer`, restore with `t.Cleanup`) contains `ignoring invalid config`. If equivalent tests already exist, just delete the no-op ones.
- [ ] **Step 7:** Gates (build/vet/test/gofmt/lint). Commit: `config: one exported Path(); tests write the config where Load reads it (fixes Windows CI)`.

### Task 2: Config truth — remove unread keys, deterministic show, atomic save

**Files:** `internal/config/config.go`, `internal/config/config_test.go`, `internal/cli/config_cmd.go`, `internal/cli/config_cmd_test.go`.

- [ ] **Step 1 (test first):** in `config_test.go`, `TestOldKeysStillLoad`: a config file containing `{"lock":{"autoGenerate":false},"workspace":{"enabled":false,"parallel":false},"env":{"default":{"node":"20"}},"prefer":["npm"]}` loads without warning and yields `Prefer == ["npm"]`, `Workspace.Parallel == false`.
- [ ] **Step 2:** Per Ruling R2, delete `LockConfig`, `Config.Lock`, `WorkspaceConfig.Enabled`, `EnvConfig.Default` and their defaults. Doc comment on `Include`/`Exclude`: `// Not yet honoured by any command (reserved for the workspaces rework).` Check `go build ./...` — no other package may reference the removed fields (grep showed none).
- [ ] **Step 3:** `showConfig` in `config_cmd.go` prints `Search settings` sorted by key (it ranges a map today). Test: two calls produce identical output and `cargo` precedes `npm`.
- [ ] **Step 4 (atomic save):** `saveConfig` writes to a temp file in the same directory (`os.CreateTemp(dir, ".xpmrc-*.json")`), writes, `Close`s, `os.Chmod(tmp, 0o644)`, then `os.Rename(tmp, path)`; on any error removes the temp file. Test `TestSaveConfigLeavesNoTempFiles`: `isolatedHome`, `saveConfig(defaults)`, directory contains only `xpmrc.json`; `config.Load()` round-trips a changed `Prefer`. (Windows: `os.Rename` over an existing file works since Go 1.5 via MoveFileEx.)
- [ ] **Step 5 (stderr capture):** config-command tests that expect an error (`TestConfigSetValidatesBeforeWriting` etc.) capture stderr with the existing `captureStderr` helper in `helpers_test.go` and assert the error text is non-empty and names the bad key/value, instead of letting it print to the test log.
- [ ] **Step 6:** README `## Configuration` table: verify every listed key is read by code (grep) and none of the removed keys is listed. Gates. Commit: `config: drop keys nothing reads, sorted show, atomic save`.

### Task 3: Finish the upm→xpm rename (scripts tables + leftovers)

**Files:** `internal/scripts/loader.go`, new `internal/scripts/loader_test.go`, `internal/cli/run_cmd.go`, `internal/logx/logx.go`, `internal/cli/cli_test.go`, `README.md` (Run project scripts section only).

- [ ] **Step 1 (tests first):** `internal/scripts/loader_test.go`, table test with `t.Run` per row, each writing a file into `t.TempDir()` and calling `loadPyprojectTOML` / `loadCargoTOML`:
  - pyproject with only `[tool.xpm.scripts] test = "pytest"` → one script `test`.
  - pyproject with only `[tool.upm.scripts] test = "pytest"` → one script `test` (fallback).
  - pyproject with both `[tool.xpm.scripts] a = "x"` and `[tool.upm.scripts] b = "y"` → only `a` (Ruling R4).
  - pyproject with only `[tool.poetry.scripts]` → poetry scripts (unchanged behaviour); with only `[project.scripts]` → those.
  - Cargo.toml with `[package.metadata.xpm.scripts]`, with only `[package.metadata.upm.scripts]`, with both (xpm wins, upm ignored).
- [ ] **Step 2:** Add `XPM` struct fields (`toml:"xpm"`) beside the existing `UPM` ones; load xpm first, upm only if xpm produced no scripts. Doc comments: `[tool.xpm.scripts] (legacy [tool.upm.scripts] is read when the xpm table is absent)`.
- [ ] **Step 3:** `run_cmd.go` hint lines: `pyproject.toml ([tool.xpm.scripts] section)` and `Cargo.toml ([package.metadata.xpm.scripts] section)`. `logx.go` package comment: `xpm`. `cli_test.go`: `os.Args = []string{"xpm", ...}`.
- [ ] **Step 4:** README "Run project scripts": one sentence naming the sources: `package.json` `scripts`, `composer.json` `scripts`, `pyproject.toml` `[tool.xpm.scripts]` (falls back to `[tool.upm.scripts]`, then `[tool.poetry.scripts]`, then `[project.scripts]`), `Cargo.toml` `[package.metadata.xpm.scripts]`; and that scripts run through `sh -c` (on Windows `sh` must be on PATH, e.g. Git for Windows). Verify each claim against `internal/scripts`.
- [ ] **Step 5:** `grep -rniI upm` over owned files returns only the documented fallback lines (loader.go struct tags/comments, README fallback mention). Gates. Commit: `scripts: read [tool.xpm.scripts] with upm fallback; finish upm→xpm rename in cli/logx`.

### Task 4: Strip terminal control sequences from registry text

**Files:** new `internal/search/sanitize.go`, new `internal/search/sanitize_test.go`, `internal/search/fanout.go`, `internal/tui/search/view.go`, a test in `internal/search` for the fan-out path.

- [ ] **Step 1 (tests first):** `TestSanitizeText` table (`t.Run` per row):
  - `"plain"` → `"plain"`; `"héllo 世界"` → unchanged (printable Unicode kept).
  - `"\x1b[31mred\x1b[0m"` → `"red"` (CSI removed whole).
  - `"a\x1b]0;title\x07b"` → `"ab"` (OSC terminated by BEL); `"a\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\b"` → `"alinkb"` (OSC terminated by ST).
  - `"a\x1bPdata\x1b\\b"` → `"ab"` (DCS); lone `"a\x1bcd"` → `"ad"` (ESC plus the one rune after it are dropped).
  - `"\u009b31mX"` → `"X"` (8-bit CSI, C1).
  - `"line1\nline2\ttab\r"` → `"line1 line2 tab"` (each `\n`, `\r`, `\t` becomes one space; the result is passed through `strings.TrimSpace`).
  - `"a\x00b\x7fc\x08d"` → `"abcd"` (other C0, DEL, backspace removed).
  - invalid UTF-8 byte `"a\xffb"` → `"a�b"` (use `strings.ToValidUTF8(s, "�")` first).
  Rules: ESC `[` … final byte in 0x40–0x7E = CSI, removed; ESC `]` … BEL or ESC `\` = OSC, removed; ESC `P`/`X`/`^`/`_` … ESC `\` = string sequences, removed; ESC followed by any other single rune: both removed; C1 0x9B (CSI) handled like ESC `[`, 0x9D like ESC `]`; every other rune with `unicode.IsControl` removed (except \n \r \t → space). An unterminated sequence removes to end of string.
- [ ] **Step 2:** Implement `SanitizeText` in `sanitize.go` (single pass over runes, no regexp needed). Exported because the TUI uses it.
- [ ] **Step 3:** In `fanOut` (fanout.go), where each registry's results are appended to `rep.Results`, sanitize `Name`, `Info` and every value in `Extra` (copy the map; do not mutate a cached map shared with the lookup cache). Test `TestFanOutSanitizesRegistryText`: a fake `registryCall` returning a Result with `Info: "\x1b[2Jevil"` and `Extra{"version":"1.0\x1b[0m"}` → report has `Info == "evil"`, `Extra["version"] == "1.0"`. Use the existing fan-out test seams (see `everywhere_test.go` / `timeout_sentinel_test.go` for how calls are faked).
- [ ] **Step 4:** `internal/tui/search/view.go` `renderResult`: apply `search.SanitizeText` to `result.Name` and `result.Info` before `truncate` (defence in depth; the TUI may receive results via paths other than fanOut in future). Unit test in `internal/tui/search` that `renderResult` output for a result with `Name: "x\x1b]0;t\x07y"` contains `xy` and no `\x1b]`.
- [ ] **Step 5:** Check `internal/cli` print sites of registry text (`cmdWhich`, `cmdInfo`, `cmdSearchNonInteractive`, install candidate menus/messages, `describeFailure`): they all print fields of `search.Report.Results` from fanOut, so they are covered; registry *error* strings (`RegistryFailure.Err`) can contain response text — in `describeFailure` (or wherever a failure's error text is printed) wrap with `search.SanitizeText`. Gates. Commit: `search: strip terminal escape sequences from registry-supplied text`.

### Task 5: Search package dead code and test hygiene

**Files:** `internal/search/http.go`, `internal/search/http_test.go`, `internal/search/search.go`, `internal/search/search_test.go`, `internal/search/fanout.go` (tests only), `internal/search/timeout_sentinel_test.go`.

- [ ] **Step 1:** Delete `isRetryableError`, `contains`, `containsAt`, `toLower` and their test(s) — they are referenced only by tests. Delete `SetHTTPConfig`, and also `GetHTTPConfig`, `HTTPConfig`, `DefaultHTTPConfig`, `httpConfig` if, after `grep -rn` across the whole repo (all packages, including other phases' packages in this branch), nothing outside `internal/search` tests references them; keep whatever `httpClient` setup still needs (e.g. if `httpClient`'s timeout is initialised from `DefaultHTTPConfig().Timeout`, inline the constant). Delete `DefaultTimeout` (search.go) and `TestDefaultTimeout` if no non-test code uses it.
- [ ] **Step 2:** Add `TestAsTimeout` table test (`t.Run` per row) for `asTimeout` in fanout.go: nil → nil; `context.DeadlineExceeded` → `errors.Is(err, ErrRegistryTimeout)`; a wrapped DeadlineExceeded (`fmt.Errorf("x: %w", context.DeadlineExceeded)`) → is timeout; a `net.Error` whose `Timeout()` is true (small fake type) → is timeout; `errors.New("boom")` → returned unchanged, not a timeout. Read `asTimeout` first and match its actual contract.
- [ ] **Step 3:** Rename `TestFanOutCtxHonouringTimeoutIsSentinel` to a name that says what it asserts (read it; e.g. `TestFanOutCtxDeadlineReportedAsTimeout`). Any table test in `internal/search` that ranges over a map without `t.Run` gets `t.Run(name, ...)`.
- [ ] **Step 4:** Gates (including lint: unused imports). Commit: `search: delete dead retry/http-config helpers; asTimeout table test`.

### Task 6: CLI small fixes — trimmed search query, --global=value message, hint wording, test hygiene

**Files:** `internal/cli/search_cmd.go`, `internal/cli/args.go`, `internal/cli/install.go`, `internal/pm/errors.go` (only if it prints "Hint:"), tests in `internal/cli` (`lookup_test.go`, `args_test.go`/`flags_test.go`, new tests as needed), `internal/cli/helpers_test.go`.

- [ ] **Step 1 (tests first):**
  - `TestSearchWhitespaceQueryIsMissing`: `cfg.Interactive=false` path — `cmdSearch([]string{"  ", "\t"})` returns 1 and stderr contains `missing package name` (no registry call: assert via the `searchReport` seam that it was not called). Also `cmdSearch([]string{" axios "})` calls the seam with `"axios"`.
  - `TestParseInstallArgsRejectsValuedGlobal`: `--global=false`, `--global=true`, `-g=true`, `-g=false` each return an error whose text contains `omit it for a local install` (Ruling R1).
  - Hint wording: with `cfg.AutoInstallPM=false` and a missing tool that has an install hint (use existing seams in `install.go`/`pm` — `pmExists`/`ensurePM` tests in `install_decision_test.go` show how), stdout contains `To install it:` and not `Hint:`.
- [ ] **Step 2:** `cmdSearch`: `query := strings.TrimSpace(strings.Join(args, " "))`. `parseInstallArgs`: for `--global=…`/`-g=…`/`-global=…` return `fmt.Errorf("unknown flag %q (install accepts -g/--global; omit it for a local install)", a)`; keep the generic message for other flags. `install.go`: `fmt.Println("Hint:", hint)` → `fmt.Println("To install it:", hint)` (Ruling R3). Grep owned code for any other user-facing `Hint:` print and align it; a struct field named `Hint` is fine.
- [ ] **Step 3 (hygiene):** `lookup_test.go` `TestNoMatchExitsOne` (ranges a map) → `t.Run(name, …)` per entry. Any other map-ranged table in `internal/cli` tests without `t.Run` → same. `captureStdout`/`captureStderr` in helpers_test.go restore `os.Stdout`/`os.Stderr` with `defer` before calling `fn` (so a panic in `fn` restores them) — check the current order and fix if the restore is not deferred before `fn()` runs. Any test that restores a package seam without `defer`/`t.Cleanup` → use `t.Cleanup`.
- [ ] **Step 4:** Gates. Commit: `cli: trim search query, clearer --global=value error, one install-hint wording, test hygiene`.

### Task 7: Usage — one command table, experimental labels, ci/cc/cg listed

**Files:** `internal/cli/cli.go` (`usage()` only), `internal/cli/man.go` (`listCommands`, `normalizeCommand`), new test in `internal/cli/man_test.go`.

- [ ] **Step 1 (tests first):** `man_test.go`:
  - `TestUsageListsEveryDispatchedCommand`: capture `usage()` stdout; it contains each of `install`, `ci`, `run`, `which`, `list`, `update`, `remove`, `info`, `search`, `lock`, `cache`, `cc`, `cg`, `graph`, `workspaces`, `env`, `doctor`, `config`, `version`, `help`, `man`.
  - `TestUsageMarksExperimental`: for each of `env`, `graph`, `lock`, `workspaces`, `cache`, the usage line containing that command name contains `(experimental)`; the lines for `install`, `which`, `search` do not.
  - `TestManListMatchesUsage`: `listCommands()` output contains the same command lines (both are generated from one table).
  - `TestNormalizeCommandAliases`: `cc`→`cache`, `cg`→`cache`, `i`→`install`, `-V`→`version`, `--version`→`version`.
- [ ] **Step 2:** In `man.go` define
  ```go
  type commandInfo struct {
  	name         string
  	aliases      []string
  	description  string
  	experimental bool
  }

  // commandTable lists every command Run dispatches, in help order.
  var commandTable = []commandInfo{
  	{"install", []string{"i"}, "Install packages (several at once) or this project's dependencies", false},
  	{"ci", nil, "Frozen install from lockfiles; deletes nothing unasked", false},
  	{"run", []string{"r"}, "Run project scripts (package.json, composer.json, pyproject.toml, Cargo.toml)", false},
  	{"which", []string{"w"}, "Check which ecosystems have a package", false},
  	{"search", []string{"s"}, "Search all registries (TUI on a terminal, plain output otherwise)", false},
  	{"info", nil, "Show detailed package information", false},
  	{"list", []string{"l"}, "List installed packages for the current project", false},
  	{"update", []string{"u"}, "Update packages in the current project", false},
  	{"remove", []string{"rm"}, "Remove a package from the current project", false},
  	{"doctor", []string{"d"}, "Environment & project diagnostics", false},
  	{"config", nil, "View or edit configuration", false},
  	{"env", nil, "Manage runtime versions (node, go, ...)", true},
  	{"graph", []string{"g"}, "Dependency graph across ecosystems", true},
  	{"lock", nil, "Generate or verify the unified lockfile (xpm-lock.yaml)", true},
  	{"workspaces", nil, "List detected workspaces/monorepos", true},
  	{"cache", []string{"cc", "cg"}, "Manage the dependency cache (cc = cache clean, cg = cache gc)", true},
  	{"version", []string{"-v", "-V", "--version"}, "Show version information", false},
  	{"help", []string{"-h", "--help"}, "Show this help message", false},
  	{"man", nil, "Show the detailed manual for a command", false},
  }
  ```
  and `func printCommandTable()` that prints each row as today (`%-20s` name column — widen to fit `version, -v, -V, --version` if needed) with ` (experimental)` appended to the description when `experimental`. `usage()` and `listCommands()` both call it. Remove the two duplicated inline tables. After the table, usage prints one line: `Experimental commands are being reworked; their behaviour and output may change.`
- [ ] **Step 3:** `normalizeCommand` derives aliases from `commandTable` (build the map from it, plus no extras) so `cc`/`cg` → `cache`. Note `xpm -v` alone is version but `-v` before a command is verbose — the table text stays "Show version information" (matches `xpm -v` alone).
- [ ] **Step 4:** Gates. Commit: `cli: one command table for help and man; label experimental commands; list ci/cc/cg`.

### Task 8: `xpm man` and generated man pages match the CLI

**Files:** `internal/cli/man.go` (`show*Help`), `internal/cli/manpage.go`, `internal/cli/man_test.go`. Verify every claim against the code (`args.go`, `install.go`, `project.go`, `search_cmd.go`, `config_cmd.go`, `run_cmd.go`, `lookup.go`, `internal/search/lookupcache.go` for the env vars).

- [ ] **Step 1 (tests first):** in `man_test.go`:
  - `TestEveryCommandHasManPage`: for each `commandTable` entry, `getManPageData(name).Description != "xpm command"` (no default fallthrough) and `GenerateManPage(name)` succeeds; also `ci` has its own page.
  - `TestGenerateAllManPagesCoversTable`: `GenerateAllManPages(t.TempDir())` writes `xpm-<name>.1` for every `commandTable` name (silence stdout via `captureStdout`).
  - `TestInstallManDescribesCurrentFlags`: captured `showInstallHelp()` output AND `GenerateManPage("install")` each contain `--` (end of flags), `several`/multiple-package wording, `-g` and that it may come after packages, and do NOT contain `--workspace`; the install page mentions exit status `1` for no matches.
  - `TestManPagesShareExitAndEnvSections`: every generated page contains `.SH EXIT STATUS` and `.SH ENVIRONMENT` with `XPM_NO_CACHE` and `XPM_CACHE_DIR`.
  - `TestExperimentalManPagesSayExperimental`: `showEnvHelp`, `showGraphHelp`, `showLockHelp`, `showWorkspacesHelp`, `showCacheHelp` outputs and their generated pages contain `experimental`.
  - `TestWorkspacesManHasNoInstallWorkspace`: `showWorkspacesHelp()` output does not contain `install --workspace` (the flag does not exist; `run -w` does).
- [ ] **Step 2:** Content updates (both `man.go` and `manpage.go`):
  - **install:** synopsis `xpm install [-g|--global] [package[@version] ...] [--] `; text: several packages are installed in order and xpm stops at the first failure; `-g` may come before or after packages; `--` ends flags (say "`--` ends flag parsing"; names starting with `-` are still rejected by name validation — verify in `pm.ValidateGenericPackageName` before claiming it); `--global=false` and other flags are rejected; without packages installs the project's dependencies (`-g` without packages is an error); a short "how xpm picks a tool" paragraph (project ecosystem first; outside a project/-g: single ecosystem, else `prefer`, else menu/refusal without a terminal); Go module paths run `go get` (`go install …@latest` with -g); Maven/Gradle print a snippet. Examples: add `xpm install axios lodash`, `xpm install typescript -g`, `xpm install github.com/gin-gonic/gin`.
  - **ci (new `showCiHelp` + manpage case):** native frozen installs per tool (copy the list from README "How xpm picks a tool" last paragraph, verified against `project.go`), checks every project's tool before running any, deletes nothing unasked (asks before removing `node_modules/` or Composer `vendor/`), takes no arguments. Add `"ci"` to `showCommandHelp` switch.
  - **which / search / info:** registries that don't answer within 2.5 s are listed as Unavailable; exit 1 when nothing matches or every registry fails; search: several words are one query, TUI only when stdin and stdout are terminals and `interactive`/`searchUI.enabled` are true, plain output otherwise.
  - **config:** `set` accepts `prefer` (comma-separated manager IDs), `autoInstallPM`, `interactive` (true/false) and `search.<id>`; validated before writing; unknown keys rejected. Read `setConfigValue` for the exact accepted keys/values and state exactly those.
  - **run:** sources (`[tool.xpm.scripts]` with upm fallback etc., Task 3), `-w/--workspace` runs in all workspace projects (experimental).
  - **env, graph, lock, workspaces, cache:** first DESCRIPTION line ends with ` (experimental)`, plus one sentence: `This command is experimental and is being reworked; its behaviour and output may change.` Do not otherwise rewrite these pages (other phases change those commands); remove only the false `xpm install --workspace` reference in workspaces.
  - **version/help:** aliases `-v` (alone), `-V`, `--version`; global flags `-v`/`--verbose` go before the command.
  - **manpage template:** add after EXAMPLES:
    ```
    .SH EXIT STATUS
    0 on success; 1 on errors, invalid arguments, cancelled prompts, no matches, or a refused non-interactive choice. install (without packages), ci, list, update, remove and run pass through the underlying tool's exit code.
    .SH ENVIRONMENT
    .TP
    \fBXPM_NO_CACHE=1\fR
    Skip the on-disk registry lookup cache.
    .TP
    \fBXPM_CACHE_DIR\fR=\fIdir\fR
    Keep the lookup cache in \fIdir\fR/lookups instead of the OS cache folder.
    .SH FILES
    ~/.config/xpm/xpmrc.json (Windows: %APPDATA%\\xpm\\xpmrc.json)
    ```
    (escape backslashes correctly for roff; verify by reading generated output). Default-case description `Universal Package Manager for multiple ecosystems.` → `Cross-ecosystem package manager front end.` (only if you touch it). `GenerateAllManPages` iterates `commandTable` names instead of its own list.
  - Leave the `.TH` date as is (out of scope).
- [ ] **Step 3:** Gates. Commit: `man: bring xpm man and generated pages up to date with the CLI`.

### Task 9: README truth pass — supported platforms, experimental labels, owned sections

**Files:** `README.md` only (NOT the env/graph/lock/workspace/cache/doctor sections — the Status table rows for those areas may be read but not rewritten).

- [ ] **Step 1:** Add `## Supported platforms` after `## Supported package managers`: a table with rows Linux, macOS, Windows and columns "Core commands (which/search/info/install/ci/list/update/remove/config)", "`run`", "`search` TUI", "`env` (experimental)". Fill each cell only with what is verified: CI (`.github/workflows/*.yml`, read only) runs tests on linux/macos/windows; `run` uses `sh -c` (Windows needs `sh` on PATH, e.g. Git for Windows / WSL); the TUI is bubbletea (works in Windows Terminal / modern consoles); for `env` on Windows write "not supported yet (see roadmap P5)" unless `internal/env` code (read only) clearly supports it. Add a one-line note that Windows support for the core commands is covered by CI but less used in practice.
- [ ] **Step 2:** Usage overview: mention that `xpm help` marks experimental commands and `xpm man <command>` has details; ensure `ci`, `cc`/`cg` are mentioned where commands are listed (owned sections only).
- [ ] **Step 3:** Truth check of owned sections: every flag, key, env var, exit code and example in Install / Find / How xpm picks a tool / Run / Global flags / Scripts and CI / Changes in this release / Configuration / Environment variables matches the code after Tasks 1–8 (`--global=false` rejected wording, `To install it:` if quoted, config keys list after Task 2, `[tool.xpm.scripts]` from Task 3). Fix discrepancies; record any claim in a non-owned section that is false in the phase report instead of editing it.
- [ ] **Step 4:** Gates (README-only change: `go test ./...` still runs `TestREADMEInstallExamples`). Commit: `README: supported platforms, experimental labels, claims match the code`.
