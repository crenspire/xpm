# xpm Roadmap — from "ambitious prototype" to "fast, trustworthy tool"

> **For agentic workers:** This is the master roadmap. It does not contain step-level tasks.
> Phases 0–1 and 2 have executable plans (linked below). Write each later phase's plan
> *just-in-time* with `superpowers:writing-plans` when its lane starts, using the
> "Scope" and "Exit criteria" in this file as the spec.

**Source:** full-repo review of 2026-10-06 (3 parallel subsystem reviews, ~100 findings, key ones verified in code).

**North star:** every xpm command answers in well under a second, never surprises the user, and only advertises what works.

---

## 1. Performance budgets ("fast af" made measurable)

Baselines measured on 2026-10-06 (macOS arm64, warm cache, home broadband).

| Metric | Baseline | Target | Owner phase |
|---|---|---|---|
| `xpm help` / `xpm --version` wall time | ~2 ms, but **writes `.xpm-env` on every command** | < 10 ms, **zero file writes, zero network** | P1 |
| `xpm which axios`, cold | **3–6 s** (5 registries queried one after another) | **≤ 1.5 s** mean; validated 0.9–1.6 s | P1 |
| `xpm which axios`, repeat | same as cold | **≤ 100 ms**; validated **60 ms** (disk cache) | P1 |
| `xpm install <pkg>` time-to-prompt | ≈ same as `which` | same as `which` | P1 |
| npm metadata per lookup | `typescript`: **15.7 MB / 2.9 s** (full packument) | **3.5 KB / ~0.4 s** (`/<pkg>/latest`) | P1 |
| crates.io metadata per lookup | 441 KB (and **always 403**: no User-Agent) | 2.7 KB (`?include=default_version`) | P1 |
| Maven search reliability | `search.maven.org` stalled in **5 of 8** requests | `central.sonatype.com`: 8 of 8 in ~0.9 s | P1 |
| Worst case, a registry hangs | up to 5 × 4 s = 20 s | **hard ceiling 2.5 s** (`lookupDeadline`) | P1 |
| HTTP response body size | unbounded | ≤ 1 MiB per registry response | P1 |
| `xpm search` (non-TUI) | 1.08 s | ≤ 1.5 s (crates.io search API ~1.2 s cold) | P3 |
| TUI keystroke → results | 1 full search per keystroke, stale results can win | debounced, 1 in-flight query set, stale results dropped | P3 |
| Release binary size | 14.6 MB | < 12 MB (`-trimpath -s -w`; 9.8 MB darwin/arm64, 10.9 MB linux/amd64) | P0 |
| Shim exec overhead | needs a Go compiler at install time; `exec.Command` child | < 5 ms, no compiler, `syscall.Exec` (**measured 5.1–5.6 ms**: no compiler needed, budget missed by about 0.1–0.6 ms; no dedicated shim binary was added) | P5 |
| `xpm graph` on 1k-node lockfile | O(E²) edge insert, exponential tree print on diamonds | < 200 ms | P6 |

`scripts/perf.sh` (added in P1) runs these and prints PASS/FAIL against the budget. CI runs it as an informational job.

---

## 2. Phases at a glance

```
Day 1        Day 2-4              Week 2                     Weeks 3-4                    Week 5+
P0 Green ──► P1 Speed core ─────► P3 Install correctness ──► (lane A done)                P7 Release
          └► P2 Security ───────► P5 Runtime manager rework ────────────────────────────►    & product
          └──────────────────────► P6 Graph / lock / workspace correctness ─────────────►
          └──────────────────────► P4 Scope cut & docs truth (continuous) ──────────────►
```

| Phase | Name | Size | Plan | Status |
|---|---|---|---|---|
| P0 | Green build | ~0.5 day | [Plan A](2026-10-06-xpm-p0-p1-green-and-fast.md) Tasks 1–6 | Done (merged to develop) |
| P1 | Speed core | ~1 day | [Plan A](2026-10-06-xpm-p0-p1-green-and-fast.md) Tasks 7–11 | Done (merged to develop) |
| P2 | Security hardening | ~1.5 days | [Plan B](2026-10-06-xpm-p2-security.md) | Done (merged to develop) |
| P3 | Install/search correctness | ~4 days | [Plan C](2026-10-07-xpm-p3-install-search.md) | Done (merged to develop) |
| P4 | Scope cut & docs truth | ~1 day, spread | write JIT | Done (merged to develop) |
| P5 | Runtime manager rework | ~1.5 weeks | write JIT | Done (merged to develop) |
| P6 | Graph / lock / workspace correctness | ~1.5 weeks | write JIT | Done (merged to develop) |
| P7 | Release & product features | ongoing | write JIT | Release tooling, `outdated`, `audit`, `why`, `install --workspace` and completions done; `xpm add`/`rm` deferred; v0.1.0 tag pending (user) |

---

## 3. How to execute fast

1. **P0 is a hard gate.** Nothing else starts until `go test ./...` is green and CI passes on `develop`. A red baseline hides regressions.
2. **After P0, split into 4 lanes, each in its own git worktree** (`superpowers:using-git-worktrees`). Lanes own disjoint packages, so they merge without conflicts:

   | Lane | Owns | Phases |
   |---|---|---|
   | A — Core CLI | `internal/search`, `internal/pm`, `internal/tui`, `internal/config`, `internal/cli/cli.go`, `commands.go`, `search_cmd.go` | P1 → P3 |
   | B — Runtimes | `internal/env/**`, `internal/cli/env_cmd.go`, `internal/scripts` | P2 → P5 |
   | C — Analysis | `internal/graph`, `internal/lock`, `internal/workspace`, `internal/cache`, `internal/doctor`, their `*_cmd.go` | P6 |
   | D — Docs | `README.md`, `internal/cli/man*.go`, `.github/` | P4, P7 |

   **Shared-file rule:** only lane A edits `internal/cli/cli.go`. Other lanes add or modify their own `*_cmd.go` files. Plan B's one-line `cli.go` touch is already done in Plan A Task 9.
3. **Execution mode:** subagent-driven (`superpowers:subagent-driven-development`), one fresh implementer per task plus a reviewer. Lanes run concurrently. Each lane merges to `develop` at the end of every phase, behind a green CI.
4. **Small PRs:** one task = one commit; one phase = one PR to `develop`. `main` only receives tagged releases (P7).
5. **TDD everywhere:** every bug in this roadmap gets a failing test first. Coverage target: ≥ 60% for `search`, `pm`, `config`, `env`, `graph`, `lock` by the end of P6 (today they range from 0% to 43%).
6. **Ratchet linting:** golangci-lint v2 runs with `new-from-merge-base`, so new code must be clean while old debt is paid down per phase. P6 exit removes the ratchet.

---

## 4. Phase details

### P0 — Green build (Plan A, Tasks 1–6)
**Scope:**
- `.gitignore` swallows `cmd/xpm/`. Fix it and commit the entrypoint.
- Config merges onto the defaults. A partial config currently disables most features.
- URL-safe package names (makes the failing tests pass).
- Global flags are parsed only before the subcommand (`xpm run test -- --version` currently exits 0 without running the tests).
- gofmt plus dead-code removal (~30 symbols, ~600 lines).
- Fix CI: binary name, golangci v2 config, `develop` branch, Go matrix.
- Makefile ldflags plus `-trimpath -s -w`.

**Exit criteria:**
- A fresh clone builds with `go build ./cmd/xpm`.
- `go test -race ./...` is green.
- `staticcheck ./...` reports no U1000 findings.
- CI is green on `develop`.

### P1 — Speed core (Plan A, Tasks 7–11)
**Scope:**
- Query registries in parallel for `install`/`which`/`info`, with a 2.5 s hard deadline.
- npm uses `/<pkg>/latest`; Maven uses Sonatype Central; crates.io uses `?include=default_version`.
- One `httpGet` helper with User-Agent (fixes crates.io's 403 on every request), Accept and capped bodies.
- On-disk lookup cache: 1 h for found packages, 15 min for not-found, `XPM_NO_CACHE=1` to bypass.
- Error when every registry fails (no more silent "No matches").
- Remove the `.xpm-env`-writing auto-activation from startup.
- `scripts/perf.sh` with budgets, plus `make perf`.

**Exit criteria:** every P1 row of the budget table passes in `scripts/perf.sh`.

### P2 — Security hardening (Plan B)
**Scope:**
- Archive extraction that can't escape the install directory (paths and symlinks).
- Strict runtime/version validation for `install`/`remove`/`.xpm-env`/shims, so `env remove node@` no longer deletes everything.
- Reject package arguments that start with `-`; validate names coming from the TUI.
- SHA-256-verified downloads for Node and Go (the runtimes that work today), with a timeout-bounded client.
- Remove the broken unverified rustup bootstrap.

**Exit criteria:**
- The extraction path-traversal tests pass.
- A `.xpm-env` with `node=../../x` is ignored.
- A tampered Node/Go download is rejected.

### P3 — Install/search correctness (lane A)
**Scope (each item is a TDD task):**
1. Lockfiles narrow the package-manager choice *within* an ecosystem only. Always prompt across ecosystems. Pick deterministically (sorted slices, not map order).
2. Install `chosen.Name`, not the query (Composer/Maven fuzzy hits); show "will install X". Apply `composerPackagePattern`.
3. Pass a version only when the user asked for one (no implicit pin to latest).
4. `xpm install a b c` installs all packages. Allow flags after positional arguments (`xpm install axios -g`). Error on unused arguments.
5. Detect the Node/Python package manager from lockfiles in `list/update/remove/install` (no args), including `bun.lock`, `Pipfile` and `poetry.lock`.
6. Route Go module paths (`x.y/...`) to `GoMod`. Offer Gradle when `build.gradle*` exists.
7. `xpm ci` uses native frozen installs (`npm ci`, `pnpm i --frozen-lockfile`, `yarn --immutable`, `composer install`, `pip install -r`). Delete nothing without confirmation; touch `vendor/` only for composer.
8. Exit codes: "no matches" exits 1. With no TTY, default to non-interactive (`golang.org/x/term.IsTerminal`).
9. TUI:
   - query mode treats only arrows, ctrl keys and esc as navigation (so `q`/`j`/`k` can be typed);
   - real debounce, plus a request sequence ID so stale results are dropped;
   - truncate by rune, not byte;
   - use `SearchUI.DebounceMs`/`PageSize`.
10. `config set` validation using `pm.ValidateConfig` (add poetry/pipenv). The prefer-list editor reads a whole line.
11. Wire `timeout.default`/`perRegistry` into the `search` HTTP client (per-registry `context.WithTimeout`).
12. Collapse the five copies of the search-options builder into `search.OptionsFromConfig(cfg)`.
13. In `which`/`install`, report registries that timed out or errored as "unavailable", separate from "Not found in". Extend the deadline/cache treatment to the TUI's multi-result search (`parallel.go`), which still waits up to 10 s.

**Exit criteria:** an e2e test table (fake registries via `httptest`) covers each README install example. Budget: `xpm search` ≤ 1.5 s.

### P4 — Scope cut & docs truth (lane D, continuous)
**Scope:**
- Mark `env`, `cache`, `graph`, `lock` and `workspaces` as **experimental** in `usage()` and the README until P5/P6 land.
- Remove README claims and config keys that nothing reads (`lock.autoGenerate/autoVerify`, `workspace.include/exclude/enabled`, `env.default`, `--node=` precedence), or implement them in their lane.
- Finish the upm→xpm rename: `[tool.xpm.scripts]` with an `upm` fallback, doctor title, comments, `.pre-commit-config.yaml`.
- List the `ci`/`cc`/`cg` commands in `usage()`.
- Add a "Supported platforms" matrix that is honest about Windows.

**Exit criteria:**
- `grep -ri upm` only matches the documented fallback.
- Every README command example is covered by a test or by `scripts/readme-smoke.sh`.

### P5 — Runtime manager rework (lane B)
**Scope:**
1. **Busybox shims:** the `xpm` binary dispatches on `os.Args[0]` and is symlinked per tool (hardlink/copy on Windows). This deletes the Go-template shim compiler, its escaping bugs and the Go-toolchain requirement. Use `syscall.Exec` on Unix.
2. Fall back to the system binary (PATH without the shims dir) when no version resolves.
3. Atomic installs: stage in `runtimes/<rt>/.tmp-*`, verify, `os.Rename`. Per-runtime lock file. Atomic writes of `active.json`/`.xpm-env` (temp file + rename, stable key order, comments preserved).
4. Semver resolution: resolve partial versions (`node@20`) to an exact version *before* computing `dest`; component-wise prefix matching; semver sort in `ls`/`ls-remote`.
5. Fix the per-runtime installers:
   - Go `include=all` + Windows zip;
   - Bun tag prefix + Windows filename;
   - Deno root-level binary;
   - Java macOS `Contents/Home`;
   - PHP `latest` crash;
   - Rust via rustup with `RUSTUP_HOME`/`CARGO_HOME` inside xpm, `--no-modify-path`.
   - Python: python-build-standalone tarballs, or declare it unsupported.
6. Checksums for Bun/Deno/Java (Adoptium) using the P2 helper.
7. `setup-path`: create the fish dir, update the right profile per shell, append mode, exact PATH entry match.
8. Script runner: `cmd.Dir` instead of `os.Chdir`; pass args as `sh -c '<cmd> "$@"' sh args...`. Python entry points are run via `python -m`/console scripts, not `sh`.
9. Windows: either fully supported (tests on the `windows-latest` CI runner) or `env` disabled on Windows with a clear message. Decide at P5 kickoff.

**Exit criteria:**
- `xpm env install node@20 && node -v` works on macOS/Linux CI with no Go toolchain present.
- Shim overhead < 5 ms.
- Ctrl-C mid-install leaves no partial version.

### P6 — Graph / lock / workspace correctness (lane C)
**Scope:**
1. Fixture-driven parser tests (`testdata/` with real lockfiles) for:
   - npm v1/v2/v3: name from the `node_modules/` path; hoisted resolution;
   - pnpm v6/v9: split on the last `@`; `snapshots`;
   - yarn v1 + berry;
   - Cargo: key by name+version;
   - go: full module path, `go mod graph`;
   - poetry, Maven tgf, Gradle.
2. Graph core:
   - adjacency index for O(1) edge dedupe;
   - `NormalizeVersions` re-keys nodes/edges/roots;
   - the tree printer tracks depth and marks repeated subtrees `(*)`.
3. Output hygiene:
   - warnings and status go to stderr;
   - `--svg` streams to stdout via `dot -Tsvg` on stdin (no fixed `/tmp/graph.dot`);
   - escape DOT labels.
4. `graph` doesn't run `mvn`/`gradle`/`go` unless `--exec` is passed (it parses files only by default).
5. `xpm-lock.yaml`:
   - key entries by relative path;
   - `--verify` detects new/removed lockfiles;
   - no `generatedAt`/mtime churn;
   - containment check on the `File` path.
6. Workspaces:
   - `cmd.Dir` per project (no `os.Chdir` from goroutines);
   - Node workspaces install once at the root;
   - `go.work` block form via `golang.org/x/mod/modfile`;
   - skip `node_modules`/`.git`/`.venv`/`vendor`;
   - wire `--workspace`;
   - honour include/exclude.
7. Cache: either wire `InjectForEcosystem` into adapter execution (pnpm `--store-dir`, `npm_config_cache`, `PIP_CACHE_DIR` — never override `CARGO_HOME`/`GRADLE_USER_HOME`), or delete the package. Recommended: delete it. Native package-manager caches already handle this, and keeping the feature costs more than it delivers.
8. Doctor:
   - an audit tool failure means "unavailable", not "OK";
   - support the pip-audit object format;
   - no false-positive "requirements.lock"/"poetry.lock missing" for PEP 621 projects;
   - drift check by hash, not mtime.

**Exit criteria:**
- Every fixture produces the expected node and edge counts.
- `xpm graph --json > g.json` is valid JSON.
- Budget: < 200 ms for a 1k-node graph.
- Remove the lint ratchet.

### P7 — Release & product (lane D, then everyone)
**Release:**
- goreleaser (multi-arch, checksums, SBOM, cosign signature) and a Homebrew tap;
- `install.sh` that verifies the checksum;
- shell completions (bash/zsh/fish).

**Product, prioritised:**
1. `xpm outdated`: one cross-ecosystem table, using the parallel registry layer.
2. `xpm audit`: unified vulnerability report built on the fixed doctor audits; OSV API batch query (one request for all lockfiles).
3. `xpm add`/`xpm rm` that edit manifests consistently. **Deferred** (ruling): `xpm install` and `xpm remove` cover this for now.
4. `xpm why <pkg>`: built on the fixed graph.

**Exit criteria:** `v0.1.0` tagged from `main`, installable via `brew install` and `go install`.

---

## 5. Finding → phase map (for traceability)

| Finding (review 2026-10-06) | Phase |
|---|---|
| `.gitignore` hides `cmd/xpm`, failing tests, CI binary name, golangci v1 config, gofmt, dead code, Makefile ldflags | P0 |
| Partial config disables features; global flags stripped from script args | P0 |
| Sequential registry lookup; full npm packument; unbounded bodies; dead retry/UA code; silent all-registries-failed; `.xpm-env` rewritten on every command | P1 |
| Zip-slip; unverified downloads; `.xpm-env` path injection; `env remove node@` wipes all; rustup fallback; leading-dash args; unvalidated TUI names | P2 |
| Cross-ecosystem lockfile autoselect; composer query vs name; implicit version pin; multi-package/flag order; PM detection in list/update/remove; Go/Gradle unreachable; `xpm ci` deletes lockfiles; TUI q/j/k + debounce; config editor bugs; timeouts unused | P3 |
| README/config keys unimplemented; upm leftovers; usage gaps; Windows claims | P4 |
| Shims need Go + template escaping; no system fallback; non-atomic installs; partial versions; string sort; broken Deno/Python/Java-mac/PHP/Bun/Go-list; setup-path; script runner chdir/arg quoting; Windows | P5 |
| npm/pnpm/yarn/cargo/go/maven/gradle graph parsers; NormalizeVersions IDs; flat tree; stdout corruption; DOT escaping; graph runs build tools; xpm-lock keyed by ecosystem; workspace chdir/root install/go.work/globs; dead cache; doctor fake-OK audits & false positives | P6 |
| Homebrew placeholder; no completions; product gaps | P7 |
