# xpm post-P4/P6 cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pay down legacy lint debt, finish the cache removal, fix stale docs and `upm` leftovers, pass extra script args safely to `sh -c`, and make `xpm run -w task -- args` forward its args.

**Architecture:** Small, independent edits across `internal/cli`, `internal/config`, `internal/scripts`, `internal/workspace`, `internal/doctor`, `internal/logx`, `internal/tui/search`, README and `.gitignore`. No new packages, no new dependencies.

**Tech Stack:** Go 1.22, golangci-lint v2.5.0.

Worktree: `/opt/personal/upm/.worktrees/cleanup`, branch `chore/post-p4-p6-cleanup` (from develop 76099df).

## Global Constraints

- Go module floor `go 1.22`; no APIs newer than 1.22 (no t.Chdir, os.CopyFS, range-over-int/func). New module dependencies only if clearly justified and only from golang.org/x/* (record a Ruling).
- Commit messages must NOT contain Co-Authored-By, "Generated with", or Claude-Session lines (a local commit-msg hook rejects them).
- Every task ends with `go build ./... && go vet ./... && go test ./...` green and `gofmt -l .` empty; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...` reports 0 issues.
- No network in unit tests (httptest / fakes / seams). Never run real package managers or real installers in tests; never write to the real $HOME, ~/.xpm, or real user caches in tests (use t.TempDir + env/seams).
- Never `git push`, tag, release, publish, or create repos. Never touch the main checkout at /opt/personal/upm (except reading).
- Keep docs honest: README/man claims must match code; verify claims.
- CI must stay green on linux/macos/windows (Go 1.22 + stable; macOS skips 1.22): guard OS-specific tests with runtime.GOOS skips where needed.
- **Ownership (this lane):** do NOT edit `internal/env/**`, `internal/cli/env_cmd.go`, `cmd/xpm/main.go`, the README env section, or the `env` entries in `internal/cli/man.go` / `internal/cli/manpage.go` (P5 is rewriting them in parallel).
- **Do not** remove `new-from-rev` from `.golangci.yml` (the controller does that after P5 merges). Check the ratchet-free lint with a temp config: `grep -v new-from-rev .golangci.yml > $TMPDIR/golangci-noratchet.yml && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run --config $TMPDIR/golangci-noratchet.yml --max-issues-per-linter 0 --max-same-issues 0 ./...` (the temp config must be outside the repo; paths in its output are relative to the config dir).
- Implementers `git add` only their own files; one commit per task (more is fine), message style `area: summary` like existing history.

## Ratchet-free lint baseline (develop 76099df)

75 issues; outside `internal/env/**` and `env_cmd.go` they are:

```
internal/logx/logx.go:200:12, 212:12, 314:12  errcheck fmt.Fprint
internal/cli/commands.go:88:21               errorlint type assertion on error
internal/cli/run_cmd.go:91:21                errorlint type assertion on error
internal/cli/cli.go:13:1                     goimports
internal/scripts/loader.go:10:1              goimports
internal/tui/search/view.go:8:1              goimports
internal/config/config_test.go:226:5         staticcheck S1009
internal/cli/errors.go:14,44,62,77,100,112   unused (formatError, formatPackageManagerNotFound, formatPackageNotFound, formatInstallationFailed, formatValidationError, printError)
```

`internal/cli/env_cmd.go:111:2 QF1007` and the 59 `internal/env/**` issues belong to P5 (reported, not fixed here).

---

### Task 1: Remove cache command leftovers

**Files:**
- Delete: `internal/cli/cache_cmd.go`, `internal/cli/cache_cmd_test.go`
- Modify: `internal/cli/cli.go` (dispatch `cc`/`cg`/`cache` cases), `internal/cli/man.go` (command table row line ~83, `case "cache"` ~142, `showCacheHelp`), `internal/cli/manpage.go` (`case "cache"` ~240), `internal/cli/man_test.go` (lines ~28, 37, 63, 139), `internal/config/config.go` (`CacheConfig`, `Config.Cache`, default), `internal/config/config_test.go` (`!c.Cache.Enabled` at ~105), `README.md` line ~98.

- [ ] **Step 1: Tests first.** In `man_test.go` drop `cache`/`cc`/`cg` from the command lists, alias map and help-func map. Add a test that `Run([]string{"cache"})` (and `cc`, `cg`) behaves like any unknown command (find how an unknown command is handled in `cli.go` and assert the same exit code / message shape — e.g. compare with `Run([]string{"definitely-not-a-command"})`), and that `xpm help` output (the command table) no longer contains `cache`. Add a config test: a config file containing `"cache": {"enabled": true, "path": "~/.xpm/cache"}` still loads without error via the normal loader (use t.TempDir and whatever seam/env the existing config tests use for the config path). Run `go test ./internal/cli/ ./internal/config/` → expect the new cli assertions to FAIL.
- [ ] **Step 2: Implement.** Delete `cache_cmd.go` + test; remove the three dispatch cases, the table row, the man switch cases and `showCacheHelp` (and any `xpm cache` mention in other help text, e.g. a SEE ALSO); remove `CacheConfig`, the `Cache` field and its default from `config.go`; remove `|| !c.Cache.Enabled` from the test. Check `alias`/`canonical` maps for `cc`/`cg` and remove them. `encoding/json` ignores unknown keys, so old files keep loading — the new test proves it.
- [ ] **Step 3: README.** Remove the sentence "`cc` and `cg` are short for `cache clean` and `cache gc` (experimental)." from line ~98 (keep the rest of the paragraph grammatical). Keep the one-line note at ~179 ("The dependency cache (`xpm cache`) was removed: …") — see Ruling R2. `git grep -n -i "xpm cache\|\bcc\b\|\bcg\b"` outside docs/superpowers must show only that note.
- [ ] **Step 4: Verify** full gate (build, vet, test, gofmt, lint) and commit `cli: drop the removed cache command, its aliases and config.CacheConfig`.

### Task 2: Legacy lint debt outside internal/env

**Files:** `internal/logx/logx.go`, `internal/cli/commands.go`, `internal/cli/run_cmd.go`, `internal/cli/cli.go`, `internal/scripts/loader.go`, `internal/tui/search/view.go`, `internal/config/config_test.go`, `internal/cli/errors.go`.

- [ ] Fix every issue in the baseline list above except `env_cmd.go`:
  - logx: `_, _ = fmt.Fprint(...)` (these are best-effort writes to the log writer; do not change behaviour).
  - errorlint: `var exitErr *exec.ExitError; if errors.As(err, &exitErr) { return exitErr.ExitCode() }`.
  - goimports: run `go run golang.org/x/tools/cmd/goimports@latest -local github.com/crenspire/xpm -w <file>` or fix import grouping by hand (std, third-party, `github.com/crenspire/xpm/...`).
  - S1009: drop the redundant nil check.
  - unused: delete the six unused functions in `errors.go` (and the file if it becomes empty; remove imports that become unused). Make sure nothing else (incl. tests) referenced them.
- [ ] Run the ratchet-free lint (see Global Constraints). Expected: only `internal/env/**` issues and `internal/cli/env_cmd.go:111` remain. Paste that remaining list into your report.
- [ ] Gate + commit `lint: fix legacy errcheck/errorlint/goimports/staticcheck/unused issues outside internal/env`.

### Task 3: Docs follow-ups (config comments, graph/lock man pages, exit status)

**Files:** `internal/config/config.go` (Include/Exclude comments ~87-94), `internal/cli/man.go` (`showGraphHelp` ~440-461, lock help ~392-404), `internal/cli/manpage.go` (graph, lock entries; `.SH EXIT STATUS` ~30), `internal/cli/man_test.go`, `README.md` (exit-code table ~199-203).

- [ ] **Config comments:** Include/Exclude are honoured by workspace commands since P6 — check `internal/workspace/filter.go` and its callers for the exact semantics (glob against the project path relative to the workspace root, `**` allowed, exclude wins?) and describe them accurately; remove "Not yet honoured".
- [ ] **graph help (man.go + manpage.go):** document every flag from `parseGraphArgs` in `internal/cli/graph_cmd.go`: `--json`, `--svg` (needs GraphViz), `--depth N` (default from `graph.depth`, 0 = unlimited), `--exec` (runs mvn/gradle/go to resolve full trees; otherwise files only), `--workspace`/`-w`; flags may come before or after the package name; and that usage errors (unknown flag, negative `--depth`, `--json` with `--svg`, more than one package) exit 2. Verify each claim against the code (`return 2` at graph_cmd.go ~96).
- [ ] **lock help:** `xpm lock --verify` reports each recorded lockfile as unchanged/changed/missing, reports supported lockfiles on disk that `xpm-lock.yaml` does not record as "added", and reports unreadable entries or recorded paths outside the project as errors; exit 1 on anything but all-unchanged. Verify against `internal/cli/lock_cmd.go` and `internal/lock/writer.go` (`Verify`).
- [ ] **EXIT STATUS:** manpage.go `.SH EXIT STATUS` and README's exit-code table gain: `2` — `graph` usage error (bad flag or argument). Before writing, `git grep -n "return 2" internal/cli` to confirm graph is the only exit-2 path (if not, list them all).
- [ ] Extend `man_test.go`: rendered graph man page contains `--exec`, `--depth`, `--workspace`; rendered lock page mentions `added`; top-level page EXIT STATUS mentions `2`. Write these assertions first and see them fail.
- [ ] Gate + commit `docs: graph/lock man pages, exit status 2, workspace include/exclude comments`.

### Task 4: upm leftovers

**Files:** `internal/doctor/output.go`, `internal/doctor/*_test.go`, `.gitignore`, `internal/lock/*.go` (comments only, if any).

- [ ] Test first: a doctor output test asserting the report header is `xpm Doctor+ Report`… use the existing project casing for the product name in other doctor output (check `internal/doctor` for how "xpm" is written elsewhere and match it) and that the output contains no `UPM`. Use the existing test seam for capturing stdout in that package (or `os.Pipe`). Fail, then fix line 60.
- [ ] `.gitignore`: remove `/upm` (Ruling R3: nothing builds a `upm` binary — Makefile builds `$(BINARY_NAME)` from `./cmd/xpm`; there is no `cmd/upm`). Also update the Makefile header comment "Universal Package Manager" only if it names the old product — leave it if it is just a description.
- [ ] `git grep -n -i "upm" internal/lock` is already empty on develop; scan `internal/lock/*.go` comments for statements that are stale after P6 (e.g. claims about what is hashed, timestamps, added/removed detection) and fix only clear inaccuracies. If none, say so in the report.
- [ ] Gate + commit `doctor: say xpm in the report header; drop /upm from .gitignore`.

### Task 5: Pass extra script args safely to sh -c

**Files:** `internal/scripts/runner.go`, `internal/scripts/runner_test.go` (create if absent), `internal/cli/manpage.go` + `internal/cli/man.go` run section only if they describe arg passing (one sentence max).

Today `buildPythonCommand`, `buildCargoCommand`, `buildShellCommand` concatenate extra args into the shell string (`cmd + " " + arg`), so `xpm run test -- "a b" '$(rm -rf x)'` re-splits and evaluates them.

- [ ] **Step 1: failing tests.** Table test on the three builders: with `extraArgs = []string{"a b", "$(echo pwned)", "c;d"}` the returned `*exec.Cmd` has `Args == []string{"sh", "-c", script.Command + ` "$@"`, "sh", "a b", "$(echo pwned)", "c;d"}`; with no extra args `Args == []string{"sh", "-c", script.Command}` (unchanged). Plus an execution test (skip on `runtime.GOOS == "windows"`, skip if `exec.LookPath("sh")` fails): build via `buildShellCommand(ScriptDefinition{Command: "printf '%s\\n'"}, []string{"a b", "$(echo pwned)", "c;d"})`, set `cmd.Stdout` to a buffer, `Run`, assert output is exactly `"a b\n$(echo pwned)\nc;d\n"`. Run → FAIL.
- [ ] **Step 2: implement** one helper used by all three builders:

```go
// shellCommand runs command through `sh -c`. Extra arguments are passed as
// positional parameters and appended as "$@", so the shell never re-splits
// or evaluates them. On Windows this needs an `sh` on PATH (Git for Windows,
// MSYS2 or WSL), exactly as before; xpm does not translate scripts to cmd.exe.
func shellCommand(command string, extraArgs []string) *exec.Cmd {
	if len(extraArgs) == 0 {
		return exec.Command("sh", "-c", command)
	}
	args := append([]string{"-c", command + ` "$@"`, "sh"}, extraArgs...)
	return exec.Command("sh", args...)
}
```

Keep `buildPythonCommand`/`buildCargoCommand`/`buildShellCommand` as thin wrappers (or replace their bodies) so call sites stay.
- [ ] **Step 3:** if the run man page (manpage.go `case "run"` / man.go run help) says scripts run with `sh -c`, add that extra args after `--` are passed as separate arguments, unexpanded. Gate + commit `scripts: pass extra args to sh -c as positional parameters`.

### Task 6: `xpm run -w task -- args` forwards the args

**Files:** `internal/workspace/run.go`, `internal/workspace/run_test.go`, `internal/cli/run_cmd.go`, `internal/cli/workspace_cmd.go`, `internal/cli/workspace_cmd_test.go` (or the existing run test file), README run/workspace line if it shows workspace run usage, man run page.

Today `cmdRun` calls `cmdRunWorkspace(runArgs[0])`, dropping everything after the task, and `workspace.Run` re-executes `<exe> run -- <task>`.

- [ ] **Step 1: failing tests.**
  - workspace: `Run(..., RunOptions{Args: []string{"--watch", "a b"}, Runner: fake})` makes each step's Command `Args == []string{"run", "--", task, "--", "--watch", "a b"}`; with no Args it stays `[]string{"run", "--", task}`. Follow the existing `run_test.go` fake-runner pattern.
  - cli: with the `workspaceRunner` / `workspaceExecutable` seams (see `workspace_cmd_test.go`), `cmdRun([]string{"-w", "test", "--", "x", "y z"})` produces child args ending in `"--", "x", "y z"`; `cmdRun([]string{"-w", "test"})` produces none.
  - Child side: a cli test that `cmdRun`'s argument splitting turns `[]string{"--", "test", "--", "x"}` into task `test`, extra `["x"]` (this is what the re-executed child receives). Extract the split into a helper `splitRunArgs(runArgs []string) (task string, extra []string)` used by both the normal and workspace paths, and unit-test it (cases: `["t"]`, `["t","--","a"]`, `["t","--"]` → no extra, `["t","--","--","a"]` → extra `["--","a"]`).
- [ ] **Step 2: implement.** Add `Args []string // extra args passed to the task after "--"` to `RunOptions`; in `Run` build `[]string{"run", "--", task}` and append `"--"` + `opts.Args...` when non-empty. `cmdRunWorkspace(task string, extra []string)` passes `Args: extra`. Update the `Run` doc comment.
- [ ] **Step 3: docs.** Man page run section (man.go + manpage.go) and README's `xpm run --workspace test` example line: mention `-- args` are passed to the task in every project. Gate + commit `run: forward extra args to every project in run --workspace`.

---

## Rulings

- R1: Lint fixes for `internal/env/**` and `internal/cli/env_cmd.go` are left to P5 (owner) — those files are being rewritten there — cost if wrong: controller fixes ~60 issues after the P5 merge before removing the ratchet.
- R2: README keeps the single line "The dependency cache (`xpm cache`) was removed: …" while removing the `cc`/`cg` usage line — users upgrading from a release that had `xpm cache` get an explanation now that the command itself returns "unknown command" — cost if wrong: one stale-looking sentence to delete later.
- R3: `.gitignore` `/upm` is removed — no target builds a `upm` binary (Makefile builds `./cmd/xpm`; no `cmd/upm`) — cost if wrong: a stale local `upm` binary shows up as untracked.
- R4: `env` sections of man.go/manpage.go are not touched even where they mention cache — P5 owns env docs — cost if wrong: a follow-up doc tweak.
- R5: Extra script args are only taken after `--` (existing behaviour, both normal and workspace run); positional args after the task without `--` stay ignored — changing that is a behaviour change outside this lane — cost if wrong: users still need `--`.
