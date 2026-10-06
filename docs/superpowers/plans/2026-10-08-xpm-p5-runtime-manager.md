# P5 Runtime Manager Rework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `xpm env` a trustworthy runtime manager on macOS and Linux. Every download is checksum-verified, installs are atomic and survive Ctrl-C, there is one version resolver, shims are links to xpm itself with a system fallback, and `setup-path` edits exactly one shell file. Windows is turned off with a clear message.

**Architecture:**
- **Core (`internal/env`, Tasks 1, 3, 4, 5, 12).**
  - `version.go`: tolerant semver (`1.22rc1`, `21.0.12.1+1`, `8u422-b05`), component-wise prefix matching and newest-first sorting.
  - `state.go`: the single resolver `Manager.ActiveVersion`, shared by `current`/`list`/`remove` and every shim. Order: nearest `.xpm-env` (walking up) that names the runtime, then `active.json`. `defaults.json` is gone. State files are written atomically (temp file + rename), and `.xpm-env` keeps its comments.
  - `install.go`: the installer interface v2 (ctx, `InstallRequest`, optional `Resolver`/`LTSResolver`/`Remover`) and `InstallRuntime`. It resolves the spec first, takes a per-runtime flock, stages in `runtimes/<rt>/.tmp-*`, verifies, writes `.xpm-meta.json` and renames into place. It never writes `.xpm-env`.
  - `shims.go`: busybox shims. `<root>/shims/<name>` is a symlink to the xpm executable. `cmd/xpm/main.go` dispatches on `argv[0]` before `cli.Run()`, then `syscall.Exec`s the real binary with its `bin` dir first on `PATH`. When nothing is configured it falls back to the system binary. The Go-template shim compiler is deleted.
  - `path.go`: `setup-path` appends one line to one shell file, and `CheckPATH` matches the PATH entry exactly.
- **Installers (`internal/env/runtimes`, Tasks 2, 6–11).** `download.go` holds the shared helpers: ctx-aware `fetchSmall`/`fetchJSON`/`fetchGitHubReleases`, `downloadVerified`, `extractArchive`, `hoistDir`, and one symlink rule `checkRelativeLink`. Each installer is a small file built from pure, table-tested asset-name functions (`nodeAsset(goos, goarch, version)` …) plus an `Install` that is download → verify → extract → hoist. Every base URL is a package var, so the tests use httptest servers. rustup and brew run through the `runCmd`/`cmdOutput` seams.
- **CLI (`internal/cli/env_cmd.go`, Task 13).** Windows gate, `reshim`, `--global` anywhere, Ctrl-C → exit 130, `current` with sources. The README section is rewritten to match.

Every code block in this plan was compiled, tested and linted on 2026-10-07. The work was done in a scratch copy of the repo by replaying the tasks in order and committing after each one. After every task: `go build ./...`, `go vet ./...` (also with `GOOS=windows`), `go test ./internal/env/... ./internal/cli/` and `gofmt -l .` were clean, and golangci-lint v2.5.0 reported `0 issues.` on the diff. The final tree also passes `go test -race`, and every test binary compiles with `GOTOOLCHAIN=go1.22.12 GOOS=linux`. The `diff` blocks below were generated from those commits. A manual smoke run (Task 13, Step 6) installed node, python, java, bun, deno and go and ran them through the shims.

**Tech Stack:** Go 1.22 (module floor), stdlib only (`net/http`, `archive/tar`, `archive/zip`, `crypto/sha256`, `syscall.Flock`/`syscall.Exec`, `os/signal`), httptest, golangci-lint v2.5.0.

**Spec:** `docs/superpowers/plans/2026-10-06-xpm-roadmap.md` (P5 section) + design brief `.superpowers/sdd/2026-10-08-xpm-p5-runtime-manager/design.md` (phase-lead rulings, copied into this plan's "Design rulings" section)

**Out of scope:**
- the script runner (roadmap P5 item 8; ruling R14: `internal/scripts` belongs to P4);
- `usage()`/man-page text in `internal/cli/cli.go`, `man.go` and `manpage.go` (lane D). Their `env` line still omits php; P4 should add it;
- the README supported-platforms table row (README line ~56, P4);
- `config.EnvConfig.Default` (unused before and after; config is not owned here).

## Global Constraints

- Go module floor `go 1.22`; no APIs newer than 1.22 (no t.Chdir, os.CopyFS, range-over-int/func). New module dependencies only if clearly justified and only from golang.org/x/* (record a Ruling).
- Commit messages must NOT contain Co-Authored-By, "Generated with", or Claude-Session lines (a local commit-msg hook rejects them).
- Every task ends with `go build ./... && go vet ./... && go test ./...` green and `gofmt -l .` empty; `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...` reports 0 issues.
- No network in unit tests (httptest / fakes / seams). Never run real package managers or real installers in tests; never write to the real $HOME, ~/.xpm, or real user caches in tests (use t.TempDir + env/seams).
- Never `git push`, tag, release, publish, or create repos. Never touch the main checkout at /opt/personal/upm (except reading).
- Keep docs honest: README/man claims must match code; verify claims.
- CI must stay green on linux/macos/windows (Go 1.22 + stable; macOS skips 1.22): guard OS-specific tests with runtime.GOOS skips where needed.

House rules that follow from these and the repo:
- Owned files only: `internal/env/**`, `internal/cli/env_cmd.go` (+ its new test file, see the Ownership note), `cmd/xpm/main.go`, and the README "Runtime versions" section. Do not edit `internal/cli/startup_test.go` (it checks the `~/.xpm/env` layout and must keep passing).
- The repo root has a committed `.xpm-env`. Every test that resolves versions chdirs into a `t.TempDir()` (`isolate`, `envTest`) and points HOME at a temp dir.
- `internal/env` tests cannot import `internal/env/runtimes` (it imports `env`): they use the test-only runtime `xpmfake`. The CLI tests use `xpmclifake`.
- Tests that need symlinks, flock, exec bits or Unix paths start with `if runtime.GOOS == "windows" { t.Skip(...) }` (helpers `skipOnWindows`, `skipWindows`).
- Lint (`.golangci.yml`): `github.com/crenspire/xpm/...` imports go in their own last group; `unused` also checks test helpers, so each helper arrives with the task that first uses it; the ratchet is `new-from-rev` so only changed lines are judged.
- `go test ./...` on macOS with Go 1.22 aborts (`missing LC_UUID`) for every package, before and after this work; CI skips 1.22 on macOS. Locally use the default toolchain.

## Review Focus

1. **Two terminals install the same runtime at once, or the user presses Ctrl-C while waiting for the lock.** The second install says `Waiting for another xpm process installing <rt>...`, then finds the version already installed and does not download it again. Ctrl-C while waiting returns at once (the lock is polled against the context). Tests in Task 4: `TestInstallRuntimeWaitsForConcurrentInstall`, `TestLockFileWaitsAndHonoursCancel`.
2. **Upgrading from a pre-P5 `~/.xpm/env`:** compiled Go shims and `.go` leftovers in `shims/`, legacy `latest`/`lts` version dirs, an unused `defaults.json`. The first state-changing command prunes the shims dir to the new symlinks, and alias-named dirs are never mistaken for versions. Tests: `TestCreateShimsLinksEveryBinaryAndPrunes` (Task 5), `TestInstalledVersionsIgnoresLegacyAliasDirs` (Task 3).
3. **Scripts whose shebang is `#!/usr/bin/env node` / `python3` (npm, npx, pip) run under a shim.** They must get the same version without going through the shim again: the resolved `bin` dir comes first on `PATH`, `argv[0]` is the absolute real path and `XPM_SHIM_DEPTH` counts up. Test in Task 5: `TestRunShimExecsTheActiveVersion` (and `TestRunShimRecursionGuard`).
4. **A stray `.xpm-env` in a parent directory (home dir, `/tmp`) pins a version that is not installed.** The shim must refuse with exit 127, name the file that set it and give the install command, and must not silently run the system binary. `current` must show where the value came from. Planning hit exactly this: `/private/tmp/claude-501/.xpm-env` pinned `node=20.11.0`. Tests: `TestRunShimConfiguredButNotInstalled` (Task 5), `TestEnvCurrentListsSortedWithSourceAndWarns` (Task 13).
5. **The xpm binary moves or is upgraded (e.g. a Homebrew keg path changes).** Shims link the un-resolved `os.Executable()` path, so a stable symlink like `/opt/homebrew/bin/xpm` survives upgrades. Every `install`/`use`/`remove`/`reshim` re-points them atomically. Test in Task 5: `TestCreateShimsLinksEveryBinaryAndPrunes` (the re-point half).

## Design rulings (copied from the design brief; binding)

- **R1 Windows.** `xpm env` is disabled on Windows. `cmdEnv` prints to stderr `xpm env is not supported on Windows yet: runtime downloads, shims and PATH setup are Unix-only (macOS, Linux).` and returns 1. Shim dispatch in main.go is a no-op on Windows. The code still compiles and `go test ./...` passes on windows-latest: tests needing symlinks, flock, exec of shell scripts or Unix paths skip on Windows. The pure asset-name functions still produce the correct Windows names (Go `.zip`, Bun `bun-windows-x64.zip`) and are table-tested with Windows rows.
- **R2 Busybox shims.** `<root>/shims/<name>` is a symlink to the xpm executable path: `os.Executable()`, NOT EvalSymlinks'd. Every state-changing `xpm env` subcommand (install, use, remove, reshim) re-creates the shim set idempotently. New subcommand `xpm env reshim`. The shim set is the union, over installed runtimes, of the base names in `installer.BinaryPaths()`: node,npm,npx; go,gofmt; python,python3,pip,pip3; java,javac,jar,keytool; rustc,cargo; bun,bunx; deno; php. Anything else in the shims dir is removed. Tests inject the executable path; they never use `os.Executable` of the test binary. Creation: symlink to `<name>.tmp-<rand>`, then `os.Rename` over `<name>`.
- **R3 Shim dispatch.** `if name, ok := env.ShimName(os.Args[0]); ok { os.Exit(env.RunShim(name, os.Args[1:])) }` before `cli.Run()`. ShimName takes the base name of argv[0] (stripping `.exe`) and is ok only when a registered installer provides that name and it is not `xpm`. main.go blank-imports `internal/env/runtimes`. On Windows ShimName returns false. RunShim:
  1. `config.Load()`; `!cfg.Env.Enabled` → system fallback.
  2. Resolve the runtime's active version with the same resolver the CLI uses (R5). An invalid entry → stderr `xpm: <why>`, exit 1.
  3. Nothing configured → system fallback: search PATH, skipping entries equal (Clean+EvalSymlinks) to the shims dir and candidates whose EvalSymlinks equals the running xpm's. Found → exec it. Not found → `xpm: no <runtime> version is configured and no system <name> was found on PATH\nInstall one: xpm env install <runtime>@<version>`, exit 127.
  4. Configured but not installed → `xpm: <runtime>@<v> is not installed (set in <source>)\nRun: xpm env install <runtime>@<v>`, exit 127 (explicit pins never fall back).
  5. Binary = `<versionDir>/<rel>`; missing → `xpm: <name> is not provided by <runtime>@<v>`, exit 127.
  6. Child env: `PATH = <dir of binary> + ListSeparator + PATH`; `XPM_SHIM_DEPTH` incremented; depth > 4 → `xpm: shim recursion detected for <name>`, exit 1.
  7. `syscall.Exec(absPath, append([]string{absPath}, args...), env)`. Files `exec_unix.go` (`//go:build !windows`) / `exec_windows.go` (returns an error).
  Budget: < 5 ms, no network, no subprocess.
- **R4 Atomic installs.** `InstallRuntime(ctx, m, runtime, spec) (exact string, err)`: validate, then resolve (network allowed) BEFORE computing `dest = versionDir(rt, exact)`. If dest exists and verifies → "already installed". Otherwise take the lock `<root>/runtimes/<rt>/.lock` (flock LOCK_EX via `syscall.Flock`, build tag `//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly`; no-op elsewhere). If the non-blocking attempt fails, print `Waiting for another xpm process installing <rt>...`. Re-check dest under the lock, remove stale `.tmp-*`, `staging := os.MkdirTemp(runtimes/<rt>, ".tmp-<exact>-")`, `installer.Install(ctx, InstallRequest{Version: exact, Dest: staging, Root: m.GetEnvPath()})`, verify every BinaryPaths entry, write `.xpm-meta.json` `{"version", "alias"}`. A corrupt dest is renamed to `.tmp-old-*` and removed, then `os.Rename(staging, dest)`. Any failure or cancel → RemoveAll(staging). Listing ignores dot-names. Ctrl-C: `signal.NotifyContext(os.Interrupt, syscall.SIGTERM)`, exit 130.
- **R5 Active version.** `func (m *Manager) ActiveVersion(runtime string) (Active, error)`, `type Active struct{ Version, Source string; Global bool }`. Order: the nearest `.xpm-env` up from cwd that has the key, then `active.json`; `defaults.json` is dropped. An invalid entry → error `invalid <runtime> version %q in <path>` (fail closed; CLI `current` warns on stderr and continues). Values: exact, partial (component-wise prefix, highest installed match), `latest` (highest installed), `lts` (highest installed with meta alias `lts`). No installed match → `Active{Version: <raw>}` + an `ErrNotInstalled`-wrapped error. `ErrNoVersion` is a separate sentinel.
- **R6 State files.** `active.json` is written atomically: `json.MarshalIndent` of the map (sorted keys) plus a trailing newline. `.xpm-env` is written atomically too, keeping comments, blank lines and key order: the first `key=value` line is updated in place (its key text kept), or `runtime=version` is appended. `xpm env install` NEVER writes `.xpm-env`. After an install, if active.json has no version for the runtime, it is set there (`Set <rt>@<v> as the global default`); otherwise the hint `Use it here: xpm env use <rt>@<v>` is printed. `use <rt>@<spec> [--global]` resolves against INSTALLED versions and writes the exact version; not installed → error with an install hint.
- **R7 Semver.** `ParseVersion` accepts "1.22.3", "1.22", "1.22rc1", "3.15.0rc3", "21.0.12.1+1" and "20.11.0-beta.1". Numbers compare numerically, a prerelease sorts below its release, numeric prerelease identifiers compare numerically (rc.10 > rc.2), and build metadata breaks the last tie numerically. `MatchesSpec`: "20" matches 20.11.0 but not 200.1.0, and "1.2" does not match 1.20.3. Also `CompareVersions`, `SortVersionsDesc` and `HighestMatch(spec, versions, includePrerelease)`. `ls` and `ls-remote` use semver order; ls-remote shows the newest 20, newest first.
- **R8 Installer interface v2.** As in Task 4's Interfaces. `ResolveSpec`: a Resolver wins. Otherwise `latest` is the highest non-prerelease of ListRemote, `lts` needs an LTSResolver (else `<rt> has no lts alias`), and a spec with ≥3 numeric components and no Resolver is returned as-is without network. A partial spec takes the HighestMatch over ListRemote (prereleases only if the spec is one); no match → `no <rt> version matches <spec> (see: xpm env ls-remote <rt>)`. `ValidateVersion` is dropped. There is no bare `http.Get` anywhere in `internal/env`.
- **R9 Download helpers.** `fetchSmall` caps the body at 8 MiB and fails with `response from <url> exceeds 8 MiB` (it reads cap+1). Also `fetchJSON` and `fetchGitHubReleases(ctx, owner/repo)`, which calls `api.github.com/repos/<repo>/releases?per_page=100` with `Accept: application/vnd.github+json` and `Authorization: Bearer $GITHUB_TOKEN` when set. `downloadVerified(ctx, url, wantHex)`. All base URLs are package vars. `extractArchive` dispatches on the suffix (.zip/.tar.gz/.tgz). `hoistDir(dest, sub)` errors if sub is missing and refuses name collisions. The dangling-link fallback uses the same leading-`..` rule as `makeSymlink` (`checkRelativeLink`). `copyDirectory` is deleted once unused.
- **R10 Installers.**
  - node: `index.json` via fetchJSON; LatestLTS is the first lts-truthy entry; asset `node-v<v>-<os>-<arch>.tar.gz` (Windows `.zip`), checksum from `SHASUMS256.txt`, `hoistDir(node-v<v>-<os>-<arch>)`; BinaryPaths `bin/node`, `bin/npm`, `bin/npx` (no corepack: absent in node ≥ 25).
  - go: `include=all` stable versions; asset `go<v>.<os>-<arch>.tar.gz` (Windows `.zip`), checksum via `goReleaseChecksum`, `hoistDir("go")`; `bin/go`, `bin/gofmt`.
  - bun: tags `bun-v1.4.2` → `1.4.2` (skip others and canary); `bun-<darwin|linux|windows>-<x64|aarch64>.zip`; checksum from the release's `SHASUMS256.txt`; `bun-<os>-<arch>/bun` → `bin/bun`, plus `bin/bunx` → `bun`; BinaryPaths `bin/bun`, `bin/bunx`.
  - deno: tags `v2.9.7` → `2.9.7`; `deno-<x86_64|aarch64>-<apple-darwin|unknown-linux-gnu|pc-windows-msvc>.zip` holds a root-level `deno` → `bin/deno`; checksum `<asset-url>.sha256sum` (`<hex>  <name>`); a 404 → `deno <v> publishes no SHA-256 checksum for <asset>; xpm only installs verified downloads (Deno 2.0.6+ publish them)`.
  - java: Adoptium (`adoptiumAPI`). `latest` → `most_recent_feature_release`, `lts` → `most_recent_lts`; major `21` → `/v3/assets/latest/21/hotspot?architecture=<a>&image_type=jdk&os=<o>&vendor=eclipse` → release_name without `jdk-` (`21.0.12.1+1`; Java 8 `jdk8u422-b05` → `8u422-b05`); exact accepted as-is. Install: `/v3/assets/release_name/eclipse/jdk-<version>?architecture&image_type=jdk&os&heap_size=normal&jvm_impl=hotspot` (Java 8: `jdk`+version) → package link + checksum → downloadVerified. os mac|linux|windows, arch x64|aarch64; hoist the single top dir, then `Contents/Home` on macOS. ListRemote = available_releases. `bin/java`, `bin/javac`, `bin/jar`, `bin/keytool`.
  - python: python-build-standalone (`pbsLatestURL`, `<prefix>/SHA256SUMS`); triples aarch64/x86_64 apple-darwin and x86_64/aarch64 unknown-linux-gnu; asset `cpython-<v>+<tag>-<triple>-install_only.tar.gz` (exact suffix, no other flavours); Resolver: latest/partial → HighestMatch (stable); an exact version missing from the release → `python-build-standalone <tag> provides 3.10.22, 3.11.17, ...`; `hoistDir("python")`; `bin/python`, `bin/python3`, `bin/pip`, `bin/pip3`.
  - rust: `RUSTUP_HOME=<Root>/rustup`, `CARGO_HOME=<Root>/cargo`, rustup at `<Root>/cargo/bin/rustup`. Bootstrap once under the rust lock from `https://static.rust-lang.org/rustup/dist/<host>/rustup-init`, with the checksum from `rustup-init.sha256` (`<hex> *./rustup-init`), run as `-y --no-modify-path --default-toolchain none --profile minimal` with `RUSTUP_INIT_SKIP_PATH_CHECK=yes`. Resolver: `stable`/`latest` → `[pkg.rust] version` of `channel-rust-stable.toml`; `1.80` → `channel-rust-1.80.toml`; X.Y.Z as-is; beta/nightly → `rust channels other than stable are not supported; use an exact version or stable`. Install runs `rustup toolchain install <v> --profile minimal --no-self-update`, then `Dest/bin` → the absolute toolchain bin. Remover: `rustup toolchain uninstall <v>`. ListRemote = [stable]. Everything goes through the `runCmd` seam. `bin/rustc`, `bin/cargo`.
  - php: Homebrew on macOS only, versions MAJOR.MINOR. `latest`/major → `?json&version=<major>` → `"8.5.11"` → `8.5`; `8.3` as-is; `8.3.12` → `PHP is installed per minor version through Homebrew; use php@8.3`. Without brew, and on Linux: `PHP needs Homebrew on macOS (https://brew.sh); on Linux install PHP with your system package manager`. Install runs `brew install shivammathur/php/php@<X.Y>` and `brew --prefix ...`, then makes `Dest/bin/php` an absolute symlink to `<prefix>/bin/php` (no copy, no wrappers). ListRemote: `8.<m>` for m = latest minor down to 0, then `7.4`. `bin/php`.
- **R11 setup-path** (`internal/env/path.go`). The file is chosen from the `$SHELL` base name:
  - zsh → `${ZDOTDIR:-$HOME}/.zshrc`;
  - bash → `~/.bashrc` (linux) or `~/.bash_profile` (darwin);
  - fish → `${XDG_CONFIG_HOME:-~/.config}/fish/conf.d/xpm.fish` (MkdirAll);
  - anything else → `~/.profile`.

  The sh-family line is `export PATH="<shims>:$PATH"`, with `\ " $` and backtick escaped. For fish it is `fish_add_path --prepend '<shims>'`, with `'` and `\` escaped. "Configured" means an exact trimmed line match. xpm appends `"\n# Added by xpm\n<line>\n"` with O_APPEND|O_CREATE|O_WRONLY 0644. `CheckPATH` compares `filepath.Clean` of each entry exactly. The home dir comes from `os.UserHomeDir`. Output names the file and says `Restart your shell or run: source <file>`.
- **R12 Dead code deleted:**
  - `ActivateFromLocalEnv`, `ActivateVersions`, `DetectLocalEnv`, `defaults.json`, `getSystemPHPVersion`;
  - `generateShimCode`/`createShim`, `copyDirectory`;
  - PHP wrappers/phpbrew/prebuilt, python stubs;
  - node `removeString`/`resolveVersion`/`InstallWithAlias`;
  - the string helpers, PHP `sortVersions`/`compareVersions`.
- **R13 CLI.**
  - The Windows gate (R1), and a "disabled" message on stderr.
  - The usage lists install/use/list/ls-remote/current/remove/reshim/setup-path.
  - `--global`/`-g` is accepted anywhere in `use`, with exactly one runtime@version (extra args → exit 1).
  - install uses NotifyContext and returns 130 on cancel.
  - `current` prints `<rt> <version> (<source>)`, sorted, with warnings on stderr.
  - `list` is semver-sorted with an active marker.
  - The help lists node, go, python, java, rust, bun, deno, php.
- **R14** Script runner: P4 owns `internal/scripts`; not in this plan.
- **R15** README "Runtime versions (experimental)" section rewritten to match the code (Task 13); other sections untouched.

**Implementation choices within the rulings (flag in review if you disagree):**
- `lockFile` polls `LOCK_NB` every 100 ms instead of a blocking `LOCK_EX`, so Ctrl-C (which `signal.NotifyContext` turns into a cancelled context instead of killing the process) works while waiting. A blocking flock would hang until the other install finished.
- `fetchGitHubReleases` reads up to 32 MiB (`maxReleasesResponse`), not fetchSmall's 8 MiB: measured on 2026-10-07, Deno's 100-release page is 5.9 MB and Bun's 4.4 MB, too close to 8 MiB.
- `downloadVerified` keeps the URL's archive suffix on its temp file (so `extractArchive(archivePath, dest)` can dispatch on it). `downloadVerifiedTo(..., dir)` lets rustup-init be written and executed under `<root>` instead of a possibly `noexec` `/tmp`.
- `InstalledVersions` ignores directory names that are not versions (`latest`, `lts` left by the pre-P5 code).
- `RemoveVersion` also refuses the global default (not just the version active in the cwd), renames the dir to `.tmp-old-*` before deleting, and calls `Remover` first (rust).
- On an already-installed version, `install <rt>@lts` records the `lts` alias in its metadata so `.xpm-env` values of `lts` resolve.
- Exec failure in a shim exits 126; `xpm env help` prints the usage and exits 0.

**Ownership note.** `internal/cli/env_cmd_test.go` (new) is not on the owned-files list, but it tests only `env_cmd.go` and touches no other lane's file. If the lead rejects it, drop the file: `env_cmd.go` still builds and the gate stays green.

## File map

| File | Change | Task |
|---|---|---|
| `internal/env/version.go`, `version_test.go`, `remote_test.go` | new: semver | 1 |
| `internal/env/remote.go` | newest-first; ctx (Task 4) | 1, 4 |
| `internal/env/list.go` | semver sort (1); rewritten on `InstalledVersions`/`ActiveVersion` (3) | 1, 3 |
| `internal/env/runtimes/download.go` | ctx helpers, caps, GitHub, suffix-keeping download (2); `hostOS`/`hostArch` (6) | 2, 6 |
| `internal/env/runtimes/utils.go` | `checkRelativeLink`, `extractArchive`, `hoistDir` (2); `singleTopDir` (8); `copyDirectory` deleted (11) | 2, 8, 11 |
| `internal/env/runtimes/helpers_test.go`, `download_test.go` | httptest/archive helpers, helper tests | 2, 6 |
| `internal/env/state.go`, `state_test.go`, `fake_test.go` | new: resolver, state files, meta; fake runtime | 3, 4, 5 |
| `internal/env/manager.go` | state moved out (3); output writer (4); executable (5) | 3, 4, 5 |
| `internal/env/use.go`, `remove.go` | rewritten | 3, 4 |
| `internal/env/detect.go` | deleted | 3 |
| `internal/env/runtime.go`, `install.go`, `install_test.go`, `lock_unix.go`, `lock_other.go` | interface v2, install engine, lock | 4 |
| `internal/env/shims.go`, `shims_test.go`, `exec_unix.go`, `exec_windows.go` | busybox shims | 5 |
| `cmd/xpm/main.go` | shim dispatch | 5 |
| `internal/env/path.go`, `path_test.go` | moved (5); rewritten setup-path (12) | 5, 12 |
| `internal/env/validate_test.go` | isolate/chdir, fail-closed test, template test deleted | 3, 4, 5 |
| `internal/env/runtimes/{node,go}.go` + tests, `installer_test.go` | rewritten | 6 |
| `internal/env/runtimes/{github,bun,deno}.go` + tests | rewritten / new | 7 |
| `internal/env/runtimes/java.go` + test | rewritten | 8 |
| `internal/env/runtimes/python.go` + test | rewritten | 9 |
| `internal/env/runtimes/{cmd,rust}.go` + test | new seam, rewritten | 10 |
| `internal/env/runtimes/php.go` + test, `cmd.go` | rewritten | 11 |
| `internal/env/runtimes/{bun,deno,go,java,node,php,python,rust}.go` | v2 signatures only | 4 |
| `internal/cli/env_cmd.go` | callers kept building (3, 4, 12); rewritten (13) | 3, 4, 12, 13 |
| `internal/cli/env_cmd_test.go` | new | 13 |
| `README.md` (Runtime versions section) | rewritten | 13 |

---

### Task 1: Semantic versions (`internal/env/version.go`) and newest-first sorting (R7)

**Files:**
- Create: `internal/env/version.go`, `internal/env/version_test.go`, `internal/env/remote_test.go`
- Modify: `internal/env/list.go` (the version sort in `listVersionsForRuntime`), `internal/env/remote.go` (whole file)

**Interfaces:**
- Consumes: nothing new.
- Produces (used by Tasks 3, 4, 9 and every installer):
  - `type Version struct{ Nums []int; Pre []string; Build []string }` with `func (v Version) IsPrerelease() bool` and `func (v Version) Compare(o Version) int`
  - `func ParseVersion(s string) (Version, bool)`
  - `func CompareVersions(a, b string) int` (unparseable strings sort below parseable ones)
  - `func SortVersionsDesc(versions []string)` (in place, newest first)
  - `func MatchesSpec(spec, v string) bool` (`""`/`"latest"` match every parseable version)
  - `func HighestMatch(spec string, versions []string, includePrerelease bool) (string, bool)`
  - `FormatRemote` prints newest first, at most 20, with `(showing the newest 20 of N versions)`.

- [ ] **Step 1: Write the failing tests**

`internal/env/version_test.go`:
```go
package env

import (
	"reflect"
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in    string
		nums  []int
		pre   []string
		build []string
	}{
		{"1.22.3", []int{1, 22, 3}, nil, nil},
		{"v20.11.0", []int{20, 11, 0}, nil, nil},
		{"1.22", []int{1, 22}, nil, nil},
		{"1.22rc1", []int{1, 22}, []string{"rc", "1"}, nil},
		{"3.15.0rc3", []int{3, 15, 0}, []string{"rc", "3"}, nil},
		{"21.0.12.1+1", []int{21, 0, 12, 1}, nil, []string{"1"}},
		{"20.11.0-beta.1", []int{20, 11, 0}, []string{"beta", "1"}, nil},
		{"8u422-b05", []int{8}, []string{"u", "422", "b", "05"}, nil},
	}
	for _, c := range cases {
		v, ok := ParseVersion(c.in)
		if !ok {
			t.Errorf("ParseVersion(%q) failed", c.in)
			continue
		}
		if !reflect.DeepEqual(v.Nums, c.nums) || !reflect.DeepEqual(v.Pre, c.pre) || !reflect.DeepEqual(v.Build, c.build) {
			t.Errorf("ParseVersion(%q) = %+v", c.in, v)
		}
	}
	for _, bad := range []string{"", "lts", "latest", "stable", "1.", "1.2.3-", "1..2", "x1", "1.2.3+", "1.2.3_4", "1234567890"} {
		if _, ok := ParseVersion(bad); ok {
			t.Errorf("ParseVersion(%q) succeeded, want failure", bad)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	less := [][2]string{
		{"1.9.0", "1.10.0"},
		{"1.22rc1", "1.22.0"},
		{"1.22rc2", "1.22rc10"},
		{"3.15.0rc3", "3.15.0"},
		{"20.11.0-beta.1", "20.11.0"},
		{"20.11.0-rc.2", "20.11.0-rc.10"},
		{"20.11.0-alpha", "20.11.0-beta"},
		{"21.0.12+1", "21.0.12.1+1"},
		{"21.0.12.1+1", "21.0.12.1+2"},
		{"8u412-b08", "8u422-b05"},
		{"lts", "1.0.0"},
	}
	for _, p := range less {
		if CompareVersions(p[0], p[1]) >= 0 || CompareVersions(p[1], p[0]) <= 0 {
			t.Errorf("want %s < %s", p[0], p[1])
		}
	}
	if CompareVersions("20.11.0", "v20.11.0") != 0 {
		t.Error("a leading v must not matter")
	}
}

func TestSortVersionsDesc(t *testing.T) {
	got := []string{"1.9.0", "1.10.0", "1.22rc1", "1.22.0", "1.2.0"}
	SortVersionsDesc(got)
	want := []string{"1.22.0", "1.22rc1", "1.10.0", "1.9.0", "1.2.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMatchesSpec(t *testing.T) {
	yes := [][2]string{
		{"20", "20.11.0"}, {"20.11", "20.11.0"}, {"20.11.0", "20.11.0"},
		{"latest", "1.0.0"}, {"21", "21.0.12.1+1"}, {"8", "8u422-b05"},
		{"1.22rc1", "1.22rc1"},
	}
	no := [][2]string{
		{"20", "200.1.0"}, {"1.2", "1.20.3"}, {"20.11.0", "20.11"},
		{"1.22rc1", "1.22rc2"}, {"lts", "20.11.0"}, {"20", "lts"},
	}
	for _, p := range yes {
		if !MatchesSpec(p[0], p[1]) {
			t.Errorf("MatchesSpec(%q, %q) = false", p[0], p[1])
		}
	}
	for _, p := range no {
		if MatchesSpec(p[0], p[1]) {
			t.Errorf("MatchesSpec(%q, %q) = true", p[0], p[1])
		}
	}
}

func TestHighestMatch(t *testing.T) {
	versions := []string{"20.9.0", "20.11.1", "20.11.0", "200.1.0", "21.0.0-rc.1", "21.0.0-rc.2", "18.19.0"}
	cases := []struct {
		spec string
		pre  bool
		want string
		ok   bool
	}{
		{"20", false, "20.11.1", true},
		{"20.9", false, "20.9.0", true},
		{"latest", false, "200.1.0", true},
		{"21", false, "", false},
		{"21", true, "21.0.0-rc.2", true},
		{"19", false, "", false},
	}
	for _, c := range cases {
		got, ok := HighestMatch(c.spec, versions, c.pre)
		if got != c.want || ok != c.ok {
			t.Errorf("HighestMatch(%q, pre=%v) = %q, %v; want %q, %v", c.spec, c.pre, got, ok, c.want, c.ok)
		}
	}
}
```

`internal/env/remote_test.go`:
```go
package env

import (
	"fmt"
	"strings"
	"testing"
)

func TestFormatRemoteShowsNewestTwenty(t *testing.T) {
	var versions []string
	for i := 30; i >= 1; i-- {
		versions = append(versions, fmt.Sprintf("1.%d.0", i))
	}
	out := FormatRemote("go", versions)
	if !strings.Contains(out, "(showing the newest 20 of 30 versions)") {
		t.Fatalf("missing count line:\n%s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if first := strings.TrimSpace(lines[3]); first != "1.30.0" {
		t.Fatalf("first listed = %q, want 1.30.0", first)
	}
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "1.11.0" {
		t.Fatalf("last listed = %q, want 1.11.0", last)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/ -run 'Version|Matches|Highest|FormatRemote' -count=1`
Expected: build failure, `undefined: ParseVersion` (and `CompareVersions`, `SortVersionsDesc`, `MatchesSpec`, `HighestMatch`).

- [ ] **Step 3: Implement `internal/env/version.go`**

```go
package env

import (
	"sort"
	"strconv"
	"strings"
)

// Version is a parsed runtime version such as "1.22.3", "1.22rc1",
// "21.0.12.1+1" or "20.11.0-beta.1".
type Version struct {
	Nums  []int    // numeric release components, any count
	Pre   []string // prerelease identifiers; nil for a release
	Build []string // build metadata identifiers ("+1")
}

// IsPrerelease reports whether v carries prerelease identifiers.
func (v Version) IsPrerelease() bool { return len(v.Pre) > 0 }

// ParseVersion parses s tolerantly. A leading "v" is ignored. The numeric
// part is one or more dot-separated integers; a prerelease follows either
// "-" or directly a letter ("1.22rc1"); build metadata follows "+".
func ParseVersion(s string) (Version, bool) {
	var v Version
	s = strings.TrimPrefix(s, "v")
	i := 0
	for {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i || j-i > 9 {
			return Version{}, false
		}
		n, err := strconv.Atoi(s[i:j])
		if err != nil {
			return Version{}, false
		}
		v.Nums = append(v.Nums, n)
		i = j
		if i+1 < len(s) && s[i] == '.' && s[i+1] >= '0' && s[i+1] <= '9' {
			i++
			continue
		}
		break
	}
	rest := s[i:]
	pre, build, hasBuild := strings.Cut(rest, "+")
	if hasBuild {
		ids, ok := splitIdentifiers(build)
		if !ok {
			return Version{}, false
		}
		v.Build = ids
	}
	if pre != "" {
		if pre[0] == '-' {
			pre = pre[1:]
		} else if !isLetter(pre[0]) {
			return Version{}, false
		}
		ids, ok := splitIdentifiers(pre)
		if !ok {
			return Version{}, false
		}
		v.Pre = ids
	}
	return v, true
}

// splitIdentifiers splits "rc.10", "beta-1" or "rc1" into identifiers,
// breaking on '.', '-' and on letter/digit boundaries.
func splitIdentifiers(s string) ([]string, bool) {
	if s == "" {
		return nil, false
	}
	var ids []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '-' }) {
		start := 0
		for k := 1; k <= len(part); k++ {
			if k == len(part) || isDigit(part[k]) != isDigit(part[k-1]) {
				ids = append(ids, part[start:k])
				start = k
			}
		}
	}
	if len(ids) == 0 {
		return nil, false
	}
	for _, id := range ids {
		for k := 0; k < len(id); k++ {
			if !isDigit(id[k]) && !isLetter(id[k]) {
				return nil, false
			}
		}
	}
	return ids, true
}

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// compareIdentifiers compares identifier lists: numeric ones numerically,
// numeric < alphanumeric, otherwise lexically; a shorter equal prefix is lower.
func compareIdentifiers(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareIdentifier(a[i], b[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(a), len(b))
}

func compareIdentifier(a, b string) int {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	switch {
	case aerr == nil && berr == nil:
		return compareInt(an, bn)
	case aerr == nil:
		return -1
	case berr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Compare orders versions: numeric components (missing ones count as 0),
// then release > prerelease, then prerelease identifiers, then the number of
// components, then build metadata.
func (v Version) Compare(o Version) int {
	for i := 0; i < len(v.Nums) || i < len(o.Nums); i++ {
		var a, b int
		if i < len(v.Nums) {
			a = v.Nums[i]
		}
		if i < len(o.Nums) {
			b = o.Nums[i]
		}
		if c := compareInt(a, b); c != 0 {
			return c
		}
	}
	switch {
	case v.IsPrerelease() && !o.IsPrerelease():
		return -1
	case !v.IsPrerelease() && o.IsPrerelease():
		return 1
	}
	if c := compareIdentifiers(v.Pre, o.Pre); c != 0 {
		return c
	}
	if c := compareInt(len(v.Nums), len(o.Nums)); c != 0 {
		return c
	}
	return compareIdentifiers(v.Build, o.Build)
}

// CompareVersions compares two version strings. Unparseable strings sort
// below every parseable one and lexically among themselves.
func CompareVersions(a, b string) int {
	va, aok := ParseVersion(a)
	vb, bok := ParseVersion(b)
	switch {
	case aok && bok:
		return va.Compare(vb)
	case aok:
		return 1
	case bok:
		return -1
	}
	return strings.Compare(a, b)
}

// SortVersionsDesc sorts versions newest first, in place.
func SortVersionsDesc(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) > 0 })
}

// MatchesSpec reports whether version v satisfies spec. "latest" (and "")
// match everything. Otherwise spec's numeric components must be an exact
// prefix of v's ("20" matches 20.11.0 but not 200.1.0; "1.2" does not
// match 1.20.3). A spec with a prerelease or build part must match exactly.
func MatchesSpec(spec, v string) bool {
	if spec == "" || spec == "latest" {
		_, ok := ParseVersion(v)
		return ok
	}
	sv, ok := ParseVersion(spec)
	if !ok {
		return false
	}
	vv, ok := ParseVersion(v)
	if !ok || len(sv.Nums) > len(vv.Nums) {
		return false
	}
	for i, n := range sv.Nums {
		if vv.Nums[i] != n {
			return false
		}
	}
	if sv.IsPrerelease() || len(sv.Build) > 0 {
		return len(sv.Nums) == len(vv.Nums) &&
			compareIdentifiers(sv.Pre, vv.Pre) == 0 &&
			(len(sv.Build) == 0 || compareIdentifiers(sv.Build, vv.Build) == 0)
	}
	return true
}

// HighestMatch returns the highest version satisfying spec. Prereleases are
// considered only when includePrerelease is true.
func HighestMatch(spec string, versions []string, includePrerelease bool) (string, bool) {
	best := ""
	var bestV Version
	for _, s := range versions {
		if !MatchesSpec(spec, s) {
			continue
		}
		v, _ := ParseVersion(s)
		if v.IsPrerelease() && !includePrerelease {
			continue
		}
		if best == "" || v.Compare(bestV) > 0 {
			best, bestV = s, v
		}
	}
	return best, best != ""
}
```

- [ ] **Step 4: Sort `list` and `ls-remote` by semver**

Replace `internal/env/remote.go` entirely (the installer interface is still v1 here; Task 4 adds `ctx`):

```go
package env

import (
	"fmt"
	"strings"
)

// ListRemote returns available remote versions for a runtime, newest first.
func ListRemote(manager *Manager, runtime string) ([]string, error) {
	installer, err := GetInstaller(runtime)
	if err != nil {
		return nil, err
	}
	versions, err := installer.ListRemote()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote versions: %w", err)
	}
	SortVersionsDesc(versions)
	return versions, nil
}

// FormatRemote formats remote versions (newest first) for display.
func FormatRemote(runtime string, versions []string) string {
	if len(versions) == 0 {
		return fmt.Sprintf("No versions available for %s.\n", runtime)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Available %s versions:\n", runtime)
	shown := versions
	if len(shown) > 20 {
		shown = shown[:20]
		fmt.Fprintf(&b, "(showing the newest 20 of %d versions)\n\n", len(versions))
	}
	for _, v := range shown {
		fmt.Fprintf(&b, "  %s\n", v)
	}
	return b.String()
}
```

In `internal/env/list.go`, replace the string sort:

```diff
--- a/internal/env/list.go
+++ b/internal/env/list.go
@@ -123,9 +123,9 @@ func listVersionsForRuntime(runtimePath, runtime string, manager *Manager) ([]Ve
 		versions = append(versions, info)
 	}
 
-	// Sort versions (simple string sort for now)
-	sort.Slice(versions, func(i, j int) bool {
-		return versions[i].Version < versions[j].Version
+	// Newest first, by semantic version (1.10.0 above 1.9.0).
+	sort.SliceStable(versions, func(i, j int) bool {
+		return CompareVersions(versions[i].Version, versions[j].Version) > 0
 	})
 
 	return versions, nil
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/env/ -run 'Version|Matches|Highest|FormatRemote' -count=1`
Expected: `ok`.

- [ ] **Step 6: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 7: Commit (only this task's files)**

```bash
git add internal/env/version.go \
  internal/env/version_test.go \
  internal/env/remote.go \
  internal/env/remote_test.go \
  internal/env/list.go
git commit -m "env: semantic version parsing and newest-first sorting"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 2: Download and archive helpers (R9)

**Files:**
- Modify: `internal/env/runtimes/download.go` (whole file), `internal/env/runtimes/utils.go` (add `checkRelativeLink`, `extractArchive`, `hoistDir`; `makeSymlink` and `verifySymlinksWithin` use `checkRelativeLink`; keep `copyDirectory` until Task 11, its last user), `internal/env/runtimes/node.go` and `internal/env/runtimes/go.go` (pass `context.TODO()`; rewritten in Task 6), `internal/env/runtimes/download_test.go`
- Create: `internal/env/runtimes/helpers_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces (runtimes package, used by Tasks 6–11):
  - vars `nodeDistURL`, `goDLURL`, `githubAPI` (base URLs); `progress io.Writer` (download log, `io.Discard` in tests); `maxSmallResponse int64 = 8 << 20`; `maxReleasesResponse int64 = 32 << 20`
  - `type statusError struct{ URL string; Code int }`, `func isNotFound(err error) bool`
  - `func fetchSmall(ctx context.Context, url string) ([]byte, error)` — error `response from <url> exceeds 8 MiB` instead of truncating
  - `func fetchJSON(ctx context.Context, url string, v any) error`
  - `type githubRelease struct{ TagName string; Draft, Prerelease bool }`, `func fetchGitHubReleases(ctx context.Context, repo string) ([]githubRelease, error)` (`Accept: application/vnd.github+json`, `Authorization: Bearer $GITHUB_TOKEN` when set)
  - `func downloadVerified(ctx context.Context, url, wantHex string) (string, error)`; `func downloadVerifiedTo(ctx context.Context, url, wantHex, dir string) (string, error)`. The temp file keeps the URL's archive suffix (`.tar.gz`, `.tgz`, `.zip`) so `extractArchive` can dispatch on it.
  - `func extractArchive(archivePath, dest string) error`, `func hoistDir(dest, sub string) error` (sub is slash-separated, e.g. `"Contents/Home"`), `func checkRelativeLink(link string) error`
  - test helpers in `helpers_test.go`: `TestMain` (silences `progress`), `type route struct{ body []byte; status int }`, `newServer(t, map[string]route) (string, *[]*http.Request)`, `setVar[T any](t, *T, T)`, `zipBytes(t, map[string]string) []byte`, `reg(name, body string) tarEntry`

- [ ] **Step 1: Write the failing tests**

Create `internal/env/runtimes/helpers_test.go`:

```go
package runtimes

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	progress = io.Discard
	os.Exit(m.Run())
}

// route is one canned response of a fake server; status 0 means 200.
type route struct {
	body   []byte
	status int
}

// newServer serves routes keyed by "path" or "path?query" (query must
// match exactly when given). Unknown paths are 404s. It records requests.
func newServer(t *testing.T, routes map[string]route) (string, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r)
		rt, ok := routes[r.URL.Path+"?"+r.URL.RawQuery]
		if !ok {
			rt, ok = routes[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		if rt.status != 0 {
			w.WriteHeader(rt.status)
		}
		_, _ = w.Write(rt.body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &seen
}

// setVar swaps a package variable for one test.
func setVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// zipBytes builds a zip in memory; names ending in "/" are directories.
func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func reg(name, body string) tarEntry { return tarEntry{name: name, body: body, typ: tar.TypeReg} }
```

In `internal/env/runtimes/download_test.go`, pass a context to the existing `downloadVerified` calls and append the new tests (full diff):

```diff
--- a/internal/env/runtimes/download_test.go
+++ b/internal/env/runtimes/download_test.go
@@ -1,11 +1,16 @@
 package runtimes
 
 import (
+	"context"
 	"crypto/sha256"
 	"encoding/hex"
+	"errors"
 	"net/http"
 	"net/http/httptest"
 	"os"
+	"path/filepath"
+	"runtime"
+	"strings"
 	"testing"
 )
 
@@ -35,7 +40,7 @@ func isolateTemp(t *testing.T) string {
 func TestDownloadVerifiedAcceptsMatchingHash(t *testing.T) {
 	isolateTemp(t)
 	url := serve(t, "archive-bytes")
-	path, err := downloadVerified(url, sha("archive-bytes"))
+	path, err := downloadVerified(context.Background(), url, sha("archive-bytes"))
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -49,7 +54,7 @@ func TestDownloadVerifiedAcceptsMatchingHash(t *testing.T) {
 func TestDownloadVerifiedRejectsMismatchAndCleansUp(t *testing.T) {
 	tmp := isolateTemp(t)
 	url := serve(t, "tampered")
-	if _, err := downloadVerified(url, sha("original")); err == nil {
+	if _, err := downloadVerified(context.Background(), url, sha("original")); err == nil {
 		t.Fatal("checksum mismatch accepted")
 	}
 	entries, _ := os.ReadDir(tmp)
@@ -60,7 +65,7 @@ func TestDownloadVerifiedRejectsMismatchAndCleansUp(t *testing.T) {
 
 func TestDownloadVerifiedRefusesWithoutChecksum(t *testing.T) {
 	isolateTemp(t)
-	if _, err := downloadVerified(serve(t, "x"), ""); err == nil {
+	if _, err := downloadVerified(context.Background(), serve(t, "x"), ""); err == nil {
 		t.Fatal("download without checksum accepted")
 	}
 }
@@ -89,3 +94,186 @@ func TestGoChecksum(t *testing.T) {
 		t.Fatal("missing file must be an error")
 	}
 }
+
+func TestFetchSmallFailsInsteadOfTruncating(t *testing.T) {
+	setVar(t, &maxSmallResponse, 16)
+	url, _ := newServer(t, map[string]route{"/big": {body: []byte("0123456789abcdefX")}, "/ok": {body: []byte("0123456789abcdef")}})
+	if _, err := fetchSmall(context.Background(), url+"/big"); err == nil || !strings.Contains(err.Error(), "response from "+url+"/big exceeds") {
+		t.Fatalf("err = %v", err)
+	}
+	if b, err := fetchSmall(context.Background(), url+"/ok"); err != nil || len(b) != 16 {
+		t.Fatalf("exactly the cap: %q %v", b, err)
+	}
+	if _, err := fetchSmall(context.Background(), url+"/missing"); !isNotFound(err) {
+		t.Fatalf("404 not recognised: %v", err)
+	}
+}
+
+func TestFetchHonoursContext(t *testing.T) {
+	url := serve(t, "x")
+	ctx, cancel := context.WithCancel(context.Background())
+	cancel()
+	if _, err := fetchSmall(ctx, url); !errors.Is(err, context.Canceled) {
+		t.Fatalf("err = %v", err)
+	}
+	if _, err := downloadVerified(ctx, url, sha("x")); !errors.Is(err, context.Canceled) {
+		t.Fatalf("download err = %v", err)
+	}
+}
+
+func TestFetchGitHubReleasesSendsTokenAndAccept(t *testing.T) {
+	url, seen := newServer(t, map[string]route{"/repos/oven-sh/bun/releases?per_page=100": {body: []byte(`[{"tag_name":"bun-v1.4.2"}]`)}})
+	setVar(t, &githubAPI, url)
+	t.Setenv("GITHUB_TOKEN", "tok123")
+	rels, err := fetchGitHubReleases(context.Background(), "oven-sh/bun")
+	if err != nil || len(rels) != 1 || rels[0].TagName != "bun-v1.4.2" {
+		t.Fatalf("got %+v, %v", rels, err)
+	}
+	r := (*seen)[0]
+	if r.Header.Get("Authorization") != "Bearer tok123" || r.Header.Get("Accept") != "application/vnd.github+json" {
+		t.Fatalf("headers = %v", r.Header)
+	}
+	t.Setenv("GITHUB_TOKEN", "")
+	if _, err := fetchGitHubReleases(context.Background(), "oven-sh/bun"); err != nil {
+		t.Fatal(err)
+	}
+	if h := (*seen)[1].Header.Get("Authorization"); h != "" {
+		t.Fatalf("sent Authorization %q without a token", h)
+	}
+}
+
+func TestDownloadVerifiedKeepsArchiveSuffix(t *testing.T) {
+	isolateTemp(t)
+	url, _ := newServer(t, map[string]route{"/a/node.tar.gz": {body: []byte("z")}})
+	p, err := downloadVerified(context.Background(), url+"/a/node.tar.gz", sha("z"))
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer os.Remove(p)
+	if !strings.HasSuffix(p, ".tar.gz") {
+		t.Fatalf("temp file %s lost the archive suffix", p)
+	}
+}
+
+func TestExtractArchiveBySuffix(t *testing.T) {
+	dest := t.TempDir()
+	zipPath := filepath.Join(t.TempDir(), "a.zip")
+	if err := os.WriteFile(zipPath, zipBytes(t, map[string]string{"d/f": "zip"}), 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if err := extractArchive(zipPath, dest); err != nil {
+		t.Fatal(err)
+	}
+	if b, _ := os.ReadFile(filepath.Join(dest, "d", "f")); string(b) != "zip" {
+		t.Fatalf("zip content %q", b)
+	}
+	tgz := makeTarGz(t, []tarEntry{reg("t", "tar")})
+	if err := extractArchive(tgz, dest); err != nil {
+		t.Fatal(err)
+	}
+	if err := extractArchive(filepath.Join(t.TempDir(), "x.rar"), dest); err == nil {
+		t.Fatal("unknown archive type accepted")
+	}
+}
+
+func TestHoistDir(t *testing.T) {
+	dest := t.TempDir()
+	for _, p := range []string{"go/bin/go", "go/go/inner"} { // sub may contain its own name
+		if err := os.MkdirAll(filepath.Join(dest, filepath.Dir(p)), 0o755); err != nil {
+			t.Fatal(err)
+		}
+		if err := os.WriteFile(filepath.Join(dest, p), []byte(p), 0o755); err != nil {
+			t.Fatal(err)
+		}
+	}
+	if err := hoistDir(dest, "go"); err != nil {
+		t.Fatal(err)
+	}
+	if b, _ := os.ReadFile(filepath.Join(dest, "bin", "go")); string(b) != "go/bin/go" {
+		t.Fatalf("bin/go = %q", b)
+	}
+	if b, _ := os.ReadFile(filepath.Join(dest, "go", "inner")); string(b) != "go/go/inner" {
+		t.Fatalf("go/inner = %q", b)
+	}
+	entries, _ := os.ReadDir(dest)
+	if len(entries) != 2 {
+		t.Fatalf("leftovers: %v", entries)
+	}
+
+	if err := hoistDir(t.TempDir(), "missing"); err == nil || !strings.Contains(err.Error(), "archive has no missing directory") {
+		t.Fatalf("missing sub: %v", err)
+	}
+
+	clash := t.TempDir()
+	for _, p := range []string{"jdk/Contents/Home/bin/java", "jdk/bin/java"} {
+		if err := os.MkdirAll(filepath.Join(clash, filepath.Dir(p)), 0o755); err != nil {
+			t.Fatal(err)
+		}
+		if err := os.WriteFile(filepath.Join(clash, p), nil, 0o755); err != nil {
+			t.Fatal(err)
+		}
+	}
+	if err := os.Mkdir(filepath.Join(clash, "bin"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	if err := hoistDir(clash, "jdk"); err == nil || !strings.Contains(err.Error(), "bin already exists") {
+		t.Fatalf("collision: %v", err)
+	}
+}
+
+func TestHoistNestedSubDir(t *testing.T) {
+	dest := t.TempDir()
+	if err := os.MkdirAll(filepath.Join(dest, "Contents", "Home", "bin"), 0o755); err != nil {
+		t.Fatal(err)
+	}
+	if err := os.WriteFile(filepath.Join(dest, "Contents", "Info.plist"), nil, 0o644); err != nil {
+		t.Fatal(err)
+	}
+	if err := hoistDir(dest, "Contents/Home"); err != nil {
+		t.Fatal(err)
+	}
+	entries, _ := os.ReadDir(dest)
+	if len(entries) != 1 || entries[0].Name() != "bin" {
+		t.Fatalf("entries = %v", entries)
+	}
+}
+
+func TestCheckRelativeLink(t *testing.T) {
+	for _, ok := range []string{"bun", "../lib/x.js", "../../a/b", "./a"} {
+		if err := checkRelativeLink(ok); err != nil {
+			t.Errorf("%q: %v", ok, err)
+		}
+	}
+	for _, bad := range []string{"", "/etc", `\evil`, "a/../..", "s/..", "a/../b"} {
+		if err := checkRelativeLink(bad); err == nil {
+			t.Errorf("%q accepted", bad)
+		}
+	}
+}
+
+func TestVerifySymlinksWithinRejectsDanglingDotDotAfterName(t *testing.T) {
+	if runtime.GOOS == "windows" {
+		t.Skip("symlinks need privileges on Windows")
+	}
+	dest, _ := newDest(t)
+	// Dangling (y does not exist), lexically inside, but ".." follows a name.
+	if err := os.Symlink("y/../z", filepath.Join(dest, "x")); err != nil {
+		t.Fatal(err)
+	}
+	if err := verifySymlinksWithin(dest); err == nil {
+		t.Fatal("dangling link with a non-leading .. accepted")
+	}
+}
+
+func TestFetchJSON(t *testing.T) {
+	url, _ := newServer(t, map[string]route{"/ok": {body: []byte(`{"tag":"20261003"}`)}, "/bad": {body: []byte(`{`)}})
+	var v struct {
+		Tag string `json:"tag"`
+	}
+	if err := fetchJSON(context.Background(), url+"/ok", &v); err != nil || v.Tag != "20261003" {
+		t.Fatalf("got %+v, %v", v, err)
+	}
+	if err := fetchJSON(context.Background(), url+"/bad", &v); err == nil || !strings.Contains(err.Error(), "parse "+url+"/bad") {
+		t.Fatalf("bad JSON: %v", err)
+	}
+}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run '.' -count=1`
Expected: build failure: `not enough arguments in call to downloadVerified`, `undefined: fetchJSON`, `undefined: hoistDir`, `undefined: checkRelativeLink` ...

- [ ] **Step 3: Rewrite `internal/env/runtimes/download.go`**

```go
package runtimes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// Base URLs are variables so tests can point them at httptest servers.
var (
	nodeDistURL = "https://nodejs.org/dist"
	goDLURL     = "https://go.dev/dl"
	githubAPI   = "https://api.github.com"
)

// progress receives "Downloading ..." lines; tests silence it.
var progress io.Writer = os.Stdout

// maxSmallResponse caps metadata documents (fetchSmall/fetchJSON);
// maxReleasesResponse caps the GitHub releases list (Deno's is ~6 MB).
var (
	maxSmallResponse    int64 = 8 << 20
	maxReleasesResponse int64 = 32 << 20
)

// downloadClient bounds every runtime download: no more hanging forever on a
// stalled connection (the old code used http.Get with no timeout).
var downloadClient = &http.Client{
	Timeout: 15 * time.Minute, // whole JDKs are ~200 MB on slow links
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// statusError is a non-200 answer; isNotFound recognises 404s.
type statusError struct {
	URL  string
	Code int
}

func (e *statusError) Error() string { return fmt.Sprintf("GET %s: status %d", e.URL, e.Code) }

func isNotFound(err error) bool {
	var se *statusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

func logf(format string, args ...any) { _, _ = fmt.Fprintf(progress, format, args...) }

// get performs a ctx-bound GET and returns the response for 200 OK only.
func get(ctx context.Context, url string, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("User-Agent", "xpm")
	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, &statusError{URL: url, Code: resp.StatusCode}
	}
	return resp, nil
}

// fetchSmall GETs a metadata document (checksum list, release JSON). It
// fails, rather than truncating, when the body exceeds 8 MiB.
func fetchSmall(ctx context.Context, url string) ([]byte, error) {
	return fetchCapped(ctx, url, nil, maxSmallResponse)
}

func fetchCapped(ctx context.Context, url string, header http.Header, limit int64) ([]byte, error) {
	resp, err := get(ctx, url, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response from %s exceeds %d MiB", url, limit>>20)
	}
	return body, nil
}

// fetchJSON decodes a small JSON document into v.
func fetchJSON(ctx context.Context, url string, v any) error {
	body, err := fetchSmall(ctx, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("parse %s: %w", url, err)
	}
	return nil
}

// githubRelease is the part of a GitHub release xpm reads.
type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// fetchGitHubReleases lists the newest 100 releases of repo ("owner/name"),
// authenticating with $GITHUB_TOKEN when set (60 requests/hour otherwise).
func fetchGitHubReleases(ctx context.Context, repo string) ([]githubRelease, error) {
	h := http.Header{}
	h.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		h.Set("Authorization", "Bearer "+tok)
	}
	url := githubAPI + "/repos/" + repo + "/releases?per_page=100"
	body, err := fetchCapped(ctx, url, h, maxReleasesResponse)
	if err != nil {
		return nil, err
	}
	var rels []githubRelease
	if err := json.Unmarshal(body, &rels); err != nil {
		return nil, fmt.Errorf("parse %s: %w", url, err)
	}
	return rels, nil
}

// archiveExt keeps the archive suffix of a URL so extractArchive can
// dispatch on the downloaded file's name.
func archiveExt(url string) string {
	base := path.Base(url)
	for _, ext := range []string{".tar.gz", ".tgz", ".zip"} {
		if strings.HasSuffix(base, ext) {
			return ext
		}
	}
	return ""
}

// downloadVerified streams url to a temp file while hashing it, and returns
// the temp path only if its SHA-256 equals wantHex. On any failure the temp
// file is removed. The caller must os.Remove the returned path when done.
func downloadVerified(ctx context.Context, url, wantHex string) (string, error) {
	return downloadVerifiedTo(ctx, url, wantHex, "")
}

// downloadVerifiedTo is downloadVerified with the temp file in dir
// ("" = os.TempDir()), for files that must be executed (rustup-init).
func downloadVerifiedTo(ctx context.Context, url, wantHex, dir string) (string, error) {
	if len(wantHex) != sha256.Size*2 {
		return "", fmt.Errorf("refusing to download %s: no valid SHA-256 to verify against", url)
	}
	logf("Downloading %s\n", url)
	resp, err := get(ctx, url, nil)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp(dir, "xpm-dl-*"+archiveExt(url))
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp.Name())
		if copyErr != nil {
			return "", fmt.Errorf("download %s: %w", url, copyErr)
		}
		return "", closeErr
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, wantHex) {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s", url, got, wantHex)
	}
	return tmp.Name(), nil
}

// checksumFromSums finds filename in a SHASUMS256.txt-style document
// ("<hex>  <filename>" per line; a leading '*' on the name marks binary mode).
func checksumFromSums(sums, filename string) (string, error) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no published checksum for %s", filename)
}

// goChecksum finds filename's sha256 in go.dev/dl/?mode=json&include=all output.
func goChecksum(releasesJSON []byte, filename string) (string, error) {
	var releases []struct {
		Files []struct {
			Filename string `json:"filename"`
			SHA256   string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(releasesJSON, &releases); err != nil {
		return "", fmt.Errorf("parse Go release list: %w", err)
	}
	for _, r := range releases {
		for _, f := range r.Files {
			if f.Filename == filename {
				return f.SHA256, nil
			}
		}
	}
	return "", fmt.Errorf("no published checksum for %s", filename)
}
```

- [ ] **Step 4: Shared link rule, `extractArchive`, `hoistDir` in `internal/env/runtimes/utils.go`**

`checkRelativeLink` takes over the leading-`..` loop of `makeSymlink`, and `verifySymlinksWithin`'s dangling-link fallback now applies the same rule (it used to reject only absolute targets). `copyDirectory` stays for now.

```diff
--- a/internal/env/runtimes/utils.go
+++ b/internal/env/runtimes/utils.go
@@ -124,12 +124,35 @@ func writeEntry(root, target string, mode os.FileMode, r io.Reader) error {
 	return f.Close()
 }
 
+// checkRelativeLink accepts only relative link targets whose ".." parts
+// form a leading run. A ".." after a real component would be followed by the
+// kernel through whatever that component is (possibly another symlink), so
+// lexical containment checks would lie.
+func checkRelativeLink(link string) error {
+	if link == "" || filepath.IsAbs(link) || filepath.VolumeName(link) != "" ||
+		strings.HasPrefix(link, "/") || strings.HasPrefix(link, `\`) {
+		return fmt.Errorf("link target %q: only relative targets are allowed", link)
+	}
+	seenName := false
+	for _, part := range strings.Split(filepath.FromSlash(link), string(filepath.Separator)) {
+		switch part {
+		case "", ".":
+		case "..":
+			if seenName {
+				return fmt.Errorf("link target %q: \"..\" after a path component is not allowed", link)
+			}
+		default:
+			seenName = true
+		}
+	}
+	return nil
+}
+
 // makeSymlink creates target -> linkname only if linkname is relative and
 // resolves (lexically) inside root.
 func makeSymlink(root, target, linkname string) error {
-	if linkname == "" || filepath.IsAbs(linkname) || filepath.VolumeName(linkname) != "" ||
-		strings.HasPrefix(linkname, "/") || strings.HasPrefix(linkname, `\`) {
-		return fmt.Errorf("symlink %s -> %q: only relative targets are allowed", target, linkname)
+	if err := checkRelativeLink(linkname); err != nil {
+		return fmt.Errorf("symlink %s: %w", target, err)
 	}
 	if err := mkdirWithin(root, filepath.Dir(target)); err != nil {
 		return err
@@ -146,21 +169,6 @@ func makeSymlink(root, target, linkname string) error {
 		return err
 	}
 	native := filepath.Clean(filepath.FromSlash(linkname))
-	// ".." may only climb from the link's own directory (a leading run). A ".."
-	// after a real component would be followed by the kernel through whatever
-	// that component is (possibly another symlink), so lexical checks lie.
-	seenName := false
-	for _, part := range strings.Split(filepath.FromSlash(linkname), string(filepath.Separator)) {
-		switch part {
-		case "", ".":
-		case "..":
-			if seenName {
-				return fmt.Errorf("symlink %s -> %q: %q after a path component is not allowed", target, linkname, "..")
-			}
-		default:
-			seenName = true
-		}
-	}
 	if !within(realRoot, filepath.Join(realParent, native)) {
 		return fmt.Errorf("symlink %s -> %q escapes the destination", target, linkname)
 	}
@@ -189,8 +197,8 @@ func verifySymlinksWithin(root string) error {
 			if lerr != nil {
 				return lerr
 			}
-			if filepath.IsAbs(link) {
-				return fmt.Errorf("symlink %s -> %q is absolute", path, link)
+			if err := checkRelativeLink(link); err != nil {
+				return fmt.Errorf("symlink %s: %w", path, err)
 			}
 			realDir, derr := filepath.EvalSymlinks(filepath.Dir(path))
 			if derr != nil {
@@ -328,6 +336,61 @@ func extractZip(src, dest string) error {
 	return verifySymlinksWithin(dest)
 }
 
+// extractArchive extracts a .zip, .tar.gz or .tgz (chosen by archivePath's
+// suffix) into dest with the confinement rules above.
+func extractArchive(archivePath, dest string) error {
+	switch {
+	case strings.HasSuffix(archivePath, ".zip"):
+		return extractZip(archivePath, dest)
+	case strings.HasSuffix(archivePath, ".tar.gz"), strings.HasSuffix(archivePath, ".tgz"):
+		return extractTarGz(archivePath, dest)
+	}
+	return fmt.Errorf("unsupported archive type: %s", filepath.Base(archivePath))
+}
+
+// hoistDir moves everything in dest/sub up into dest and removes dest/sub
+// (with whatever else sub's top directory held, e.g. a JDK's Contents/).
+// sub is slash-separated. It refuses when a moved name already exists.
+func hoistDir(dest, sub string) error {
+	parts := strings.SplitN(sub, "/", 2)
+	top := filepath.Join(dest, parts[0])
+	src := filepath.Join(dest, filepath.FromSlash(sub))
+	if fi, err := os.Lstat(src); err != nil || !fi.IsDir() {
+		return fmt.Errorf("archive has no %s directory", sub)
+	}
+	// Park the top dir under a unique name so sub may contain its own name.
+	parked, err := os.MkdirTemp(dest, ".xpm-hoist-")
+	if err != nil {
+		return err
+	}
+	if err := os.Remove(parked); err != nil {
+		return err
+	}
+	if err := os.Rename(top, parked); err != nil {
+		return err
+	}
+	if len(parts) == 2 {
+		src = filepath.Join(parked, filepath.FromSlash(parts[1]))
+	} else {
+		src = parked
+	}
+	entries, err := os.ReadDir(src)
+	if err != nil {
+		return err
+	}
+	for _, e := range entries {
+		if _, err := os.Lstat(filepath.Join(dest, e.Name())); err == nil {
+			return fmt.Errorf("cannot hoist %s: %s already exists", sub, e.Name())
+		}
+	}
+	for _, e := range entries {
+		if err := os.Rename(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name())); err != nil {
+			return err
+		}
+	}
+	return os.RemoveAll(parked)
+}
+
 // copyDirectory copies a directory recursively, handling symlinks.
 func copyDirectory(src, dest string) error {
 	// Ensure destination exists
```

- [ ] **Step 5: Keep node.go and go.go compiling**

Mechanical: add `"context"` to the imports and pass `context.TODO()` (Task 6 rewrites both files).

```diff
--- a/internal/env/runtimes/node.go
+++ b/internal/env/runtimes/node.go
@@ -1,6 +1,7 @@
 package runtimes
 
 import (
+	"context"
 	"encoding/json"
 	"fmt"
 	"net/http"
@@ -211,7 +212,7 @@ func (n *NodeInstaller) InstallWithAlias(version string, dest string, alias stri
 
 	filename := fmt.Sprintf("node-v%s-%s-%s.%s", version, nodeos, arch, ext)
 	base := fmt.Sprintf("%s/v%s", nodeDistURL, version)
-	sums, err := fetchSmall(base + "/SHASUMS256.txt")
+	sums, err := fetchSmall(context.TODO(), base+"/SHASUMS256.txt")
 	if err != nil {
 		return fmt.Errorf("fetch Node.js checksums: %w", err)
 	}
@@ -220,7 +221,7 @@ func (n *NodeInstaller) InstallWithAlias(version string, dest string, alias stri
 		return err
 	}
 	fmt.Printf("Downloading %s/%s...\n", base, filename)
-	archive, err := downloadVerified(base+"/"+filename, want)
+	archive, err := downloadVerified(context.TODO(), base+"/"+filename, want)
 	if err != nil {
 		return err
 	}
```
```diff
--- a/internal/env/runtimes/go.go
+++ b/internal/env/runtimes/go.go
@@ -1,6 +1,7 @@
 package runtimes
 
 import (
+	"context"
 	"encoding/json"
 	"fmt"
 	"os"
@@ -25,7 +26,7 @@ func (g *GoInstaller) Name() string {
 
 // ListRemote fetches available stable Go versions (newest first).
 func (g *GoInstaller) ListRemote() ([]string, error) {
-	data, err := fetchSmall(goDLURL + "/?mode=json&include=all")
+	data, err := fetchSmall(context.TODO(), goDLURL+"/?mode=json&include=all")
 	if err != nil {
 		return nil, err
 	}
@@ -117,7 +118,7 @@ func (g *GoInstaller) Install(version string, dest string) error {
 		return err
 	}
 	fmt.Printf("Downloading %s/%s...\n", goDLURL, filename)
-	archive, err := downloadVerified(goDLURL+"/"+filename, want)
+	archive, err := downloadVerified(context.TODO(), goDLURL+"/"+filename, want)
 	if err != nil {
 		return err
 	}
@@ -159,7 +160,7 @@ func (g *GoInstaller) BinaryPaths(version, dest string) []string {
 func goReleaseChecksum(filename string) (string, error) {
 	var lastErr error
 	for _, u := range []string{goDLURL + "/?mode=json", goDLURL + "/?mode=json&include=all"} {
-		meta, err := fetchSmall(u)
+		meta, err := fetchSmall(context.TODO(), u)
 		if err != nil {
 			lastErr = fmt.Errorf("fetch Go release list: %w", err)
 			continue
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/env/runtimes/ -run '.' -count=1`
Expected: `ok`; the existing extraction-confinement tests (`TestExtract*`, `TestMakeSymlinkRejectsRootRelativeTargets`, `TestVerifySymlinksWithinCatchesEscapingLink`) still pass.

- [ ] **Step 7: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 8: Commit (only this task's files)**

```bash
git add internal/env/runtimes/download.go \
  internal/env/runtimes/utils.go \
  internal/env/runtimes/node.go \
  internal/env/runtimes/go.go \
  internal/env/runtimes/download_test.go \
  internal/env/runtimes/helpers_test.go
git commit -m "env/runtimes: ctx-aware download helpers, extractArchive, hoistDir, one symlink rule"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 3: One active-version resolver and atomic state files (R5, R6, R12)

**Files:**
- Create: `internal/env/state.go`, `internal/env/state_test.go`, `internal/env/fake_test.go`
- Modify (whole file): `internal/env/manager.go`, `internal/env/use.go`, `internal/env/list.go`, `internal/env/remove.go`, `internal/env/validate_test.go`
- Delete: `internal/env/detect.go` (`DetectLocalEnv`, `FindXpmEnv`, `ActivateVersions`, `ActivateFromLocalEnv`; nothing outside `internal/env` uses them)
- Modify (hunks): `internal/env/install.go` (drop the auto-`use` after install; alias metadata via `writeMeta`), `internal/env/shims.go` (old `CreateShims` calls `ActiveVersion`), `internal/cli/env_cmd.go` (`use`, `current`, `remove` callers)

**Interfaces:**
- Consumes: Task 1 `SortVersionsDesc`, `HighestMatch`.
- Produces (used by Tasks 4, 5, 13):
  - `var ErrNoVersion`, `var ErrNotInstalled` (match with `errors.Is`)
  - `type Active struct{ Version, Source string; Global bool }`
  - `func (m *Manager) ActiveVersion(runtime string) (Active, error)` — not installed: returns `Active{Version: <raw value>, Source}` plus an error wrapping `ErrNotInstalled`; invalid entry: error `invalid <rt> version "<v>" in <path>`; nothing configured: error wrapping `ErrNoVersion`
  - `func (m *Manager) InstalledVersions(runtime string) ([]string, error)` (newest first; skips hidden entries and non-version names)
  - `func (m *Manager) resolveInstalled(runtime, spec string) (string, bool)` (exact, `lts` via metadata alias, `latest`, partial: stable first, then prereleases)
  - `func (m *Manager) GlobalVersion(runtime string) (string, error)`, `func (m *Manager) SetGlobalVersion(runtime, version string) error`, `func (m *Manager) GetActivePath() string`
  - `func SetLocalVersion(dir, runtime, version string) (string, error)` (returns the `.xpm-env` path)
  - `func UseVersion(m *Manager, runtime, spec string, global bool) (Active, error)`
  - `func RemoveVersion(ctx context.Context, m *Manager, runtime, version string) error` (Task 4 adds the `Remover` call)
  - `type versionMeta struct{ Version, Alias string }`, `readMeta(dir) versionMeta`, `writeMeta(dir, versionMeta) error`, `writeFileAtomic(path string, data []byte, perm os.FileMode) error`, `parseEnvFile(string) map[string]string`, `updateEnvContent(content, key, value string) string`
  - tests: `chdir(t, dir)`, `isolate(t) *Manager` (sets HOME, chdirs to a temp dir), `fakeRT = "xpmfake"`, `writeFakeBinaries(dir) error`, `installFake(t, m, version, alias) string`
  - Removed: `GetActiveVersion`, `SetActiveVersion`, `resolveAliasToVersion`, `loadDefaults`/`defaults.json`, `readEnvFile`/`writeEnvFile`, `splitLines`/`trimSpace`/`splitKeyValue`, `getSystemPHPVersion`, `getVersionAlias`/`saveVersionAlias`, `isValidInstallation`.

- [ ] **Step 1: Write the failing tests**

`internal/env/fake_test.go`:

```go
package env

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeRT is the test-only runtime name; the real installers live in
// internal/env/runtimes, which this package cannot import.
const fakeRT = "xpmfake"

func writeFakeBinaries(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		return err
	}
	for _, b := range []string{"fakebin", "fakebin2"} {
		if err := os.WriteFile(filepath.Join(dir, "bin", b), []byte("#!/bin/sh\n"), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// installFake creates an installed xpmfake version without InstallRuntime.
func installFake(t *testing.T, m *Manager, version, alias string) string {
	t.Helper()
	dir := filepath.Join(m.GetRuntimesPath(), fakeRT, version)
	if err := writeFakeBinaries(dir); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(dir, versionMeta{Version: version, Alias: alias}); err != nil {
		t.Fatal(err)
	}
	return dir
}
```

`internal/env/state_test.go`:

```go
package env

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActiveVersionNothingConfigured(t *testing.T) {
	m := isolate(t)
	if _, err := m.ActiveVersion("node"); !errors.Is(err, ErrNoVersion) {
		t.Fatalf("err = %v, want ErrNoVersion", err)
	}
}

func TestActiveVersionPrefersNearestXpmEnvWithTheKey(t *testing.T) {
	m := isolate(t)
	for _, v := range []string{"18.19.0", "20.9.0", "20.11.1", "200.1.0"} {
		installFake(t, m, v, "")
	}
	if err := m.SetGlobalVersion(fakeRT, "18.19.0"); err != nil {
		t.Fatal(err)
	}
	root, _ := os.Getwd()
	if err := os.WriteFile(filepath.Join(root, ".xpm-env"), []byte("# pins\n"+fakeRT+"=20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// A nearer .xpm-env without the key must not stop the walk.
	if err := os.WriteFile(filepath.Join(root, "a", ".xpm-env"), []byte("othert=1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, sub)

	a, err := m.ActiveVersion(fakeRT)
	if err != nil {
		t.Fatal(err)
	}
	want := Active{Version: "20.11.1", Source: filepath.Join(root, ".xpm-env")}
	if a != want {
		t.Fatalf("got %+v, want %+v", a, want)
	}

	chdir(t, t.TempDir())
	a, err = m.ActiveVersion(fakeRT)
	if err != nil || a.Version != "18.19.0" || !a.Global || a.Source != m.GetActivePath() {
		t.Fatalf("global: got %+v, %v", a, err)
	}
}

func TestActiveVersionAliasesAndMissing(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "20.11.1", "lts")
	installFake(t, m, "22.1.0", "")
	cases := map[string]string{"lts": "20.11.1", "latest": "22.1.0", "22": "22.1.0", "20.11.1": "20.11.1"}
	for spec, want := range cases {
		if err := os.WriteFile(".xpm-env", []byte(fakeRT+"="+spec+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		a, err := m.ActiveVersion(fakeRT)
		if err != nil || a.Version != want {
			t.Errorf("%s: got %+v, %v; want %s", spec, a, err, want)
		}
	}
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=19\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := m.ActiveVersion(fakeRT)
	if !errors.Is(err, ErrNotInstalled) || a.Version != "19" || !strings.HasSuffix(a.Source, ".xpm-env") {
		t.Fatalf("got %+v, %v; want ErrNotInstalled with the raw value and source", a, err)
	}
}

func TestActiveVersionIgnoresStagingDirs(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "1.0.0", "")
	if err := os.MkdirAll(filepath.Join(m.GetRuntimesPath(), fakeRT, ".tmp-2.0.0-123", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := m.InstalledVersions(fakeRT)
	if err != nil || len(got) != 1 || got[0] != "1.0.0" {
		t.Fatalf("InstalledVersions = %v, %v", got, err)
	}
}

func TestInstalledVersionsIgnoresLegacyAliasDirs(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "20.11.1", "")
	for _, legacy := range []string{"latest", "lts"} { // pre-P5 layouts
		if err := writeFakeBinaries(filepath.Join(m.GetRuntimesPath(), fakeRT, legacy)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=latest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := m.ActiveVersion(fakeRT)
	if err != nil || a.Version != "20.11.1" {
		t.Fatalf("got %+v, %v; a legacy dir named latest must not be used", a, err)
	}
}

func TestSetGlobalVersionIsSortedJSONWithNewline(t *testing.T) {
	m := isolate(t)
	if err := m.SetGlobalVersion("node", "20.11.0"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetGlobalVersion("go", "1.22.3"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.GetActivePath())
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"go\": \"1.22.3\",\n  \"node\": \"20.11.0\"\n}\n"
	if string(data) != want {
		t.Fatalf("active.json = %q, want %q", data, want)
	}
	entries, _ := os.ReadDir(m.GetEnvPath())
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestUpdateEnvContent(t *testing.T) {
	cases := []struct{ in, key, val, want string }{
		{"", "node", "20.11.0", "node=20.11.0\n"},
		{"# my pins\n\ngo=1.22.0\n", "node", "20", "# my pins\n\ngo=1.22.0\nnode=20\n"},
		{"go=1.22.0", "node", "20", "go=1.22.0\nnode=20\n"},
		{"# c\n  node = 18\ngo=1.22.0\nnode=16\n", "node", "20", "# c\n  node =20\ngo=1.22.0\nnode=16\n"},
		{"#node=1\nnode=2\r\n", "node", "3", "#node=1\nnode=3\r\n"},
	}
	for _, c := range cases {
		if got := updateEnvContent(c.in, c.key, c.val); got != c.want {
			t.Errorf("updateEnvContent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseEnvFileFirstOccurrenceWins(t *testing.T) {
	got := parseEnvFile("# x=1\n node = 18 \nnode=20\nbad line\n=3\ngo=\n")
	if len(got) != 1 || got["node"] != "18" {
		t.Fatalf("got %v", got)
	}
}

func TestUseVersionWritesExactInstalledVersion(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "20.9.0", "")
	installFake(t, m, "20.11.1", "")
	if err := os.WriteFile(".xpm-env", []byte("# keep me\ngo=1.22.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	a, err := UseVersion(m, fakeRT, "20", false)
	if err != nil || a.Version != "20.11.1" || a.Global {
		t.Fatalf("UseVersion = %+v, %v", a, err)
	}
	data, _ := os.ReadFile(".xpm-env")
	if string(data) != "# keep me\ngo=1.22.0\n"+fakeRT+"=20.11.1\n" {
		t.Fatalf(".xpm-env = %q", data)
	}
	if fi, _ := os.Stat(".xpm-env"); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600 kept", fi.Mode().Perm())
	}

	a, err = UseVersion(m, fakeRT, "20.9.0", true)
	if err != nil || !a.Global || a.Source != m.GetActivePath() {
		t.Fatalf("global UseVersion = %+v, %v", a, err)
	}
	if g, _ := m.GlobalVersion(fakeRT); g != "20.9.0" {
		t.Fatalf("active.json has %q", g)
	}

	if _, err := UseVersion(m, fakeRT, "21", false); err == nil || !strings.Contains(err.Error(), "xpm env install "+fakeRT+"@21") {
		t.Fatalf("not-installed error = %v", err)
	}
}

func TestRemoveVersionRefusesActiveAndGlobal(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "1.0.0", "")
	installFake(t, m, "2.0.0", "")
	installFake(t, m, "3.0.0", "")
	if err := m.SetGlobalVersion(fakeRT, "1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=2.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := RemoveVersion(ctx, m, fakeRT, "2.0.0"); err == nil || !strings.Contains(err.Error(), "active here") {
		t.Fatalf("removing the local pin: %v", err)
	}
	if err := RemoveVersion(ctx, m, fakeRT, "1.0.0"); err == nil || !strings.Contains(err.Error(), "global default") {
		t.Fatalf("removing the global default: %v", err)
	}
	if err := RemoveVersion(ctx, m, fakeRT, "3.0.0"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(m.GetRuntimesPath(), fakeRT))
	if len(entries) != 2 {
		t.Fatalf("left %d entries, want 2 (no .tmp-old-*)", len(entries))
	}
}

func TestListInstalledSemverOrder(t *testing.T) {
	m := isolate(t)
	for _, v := range []string{"1.9.0", "1.10.0", "1.2.0"} {
		installFake(t, m, v, "")
	}
	got, err := ListInstalled(m)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, v := range got[fakeRT] {
		names = append(names, v.Version)
	}
	if strings.Join(names, " ") != "1.10.0 1.9.0 1.2.0" {
		t.Fatalf("order = %v", names)
	}
}
```

`internal/env/validate_test.go`: `isolate` also sets HOME and gains a `chdir` helper; `RemoveVersion` takes a context; `TestLocalEnvIgnoresPathLikeVersion` becomes `TestLocalEnvRejectsPathLikeVersion` (fail closed instead of silently ignoring). Full diff:

```diff
--- a/internal/env/validate_test.go
+++ b/internal/env/validate_test.go
@@ -1,6 +1,8 @@
 package env
 
 import (
+	"context"
+	"errors"
 	"go/parser"
 	"go/token"
 	"os"
@@ -11,19 +13,25 @@ import (
 	"github.com/crenspire/xpm/internal/config"
 )
 
-// isolate chdirs into a fresh temp dir (the repo root has its own .xpm-env)
-// and returns a Manager rooted in another temp dir.
-func isolate(t *testing.T) *Manager {
+// chdir is t.Chdir for Go < 1.24.
+func chdir(t *testing.T, dir string) {
 	t.Helper()
 	old, err := os.Getwd()
 	if err != nil {
 		t.Fatal(err)
 	}
-	if err := os.Chdir(t.TempDir()); err != nil {
+	if err := os.Chdir(dir); err != nil {
 		t.Fatal(err)
 	}
 	t.Cleanup(func() { _ = os.Chdir(old) })
+}
 
+// isolate points HOME at a temp dir, chdirs into another (the repo root has
+// its own .xpm-env) and returns a Manager rooted in a third.
+func isolate(t *testing.T) *Manager {
+	t.Helper()
+	t.Setenv("HOME", t.TempDir())
+	chdir(t, t.TempDir())
 	cfg := config.Config{Env: config.EnvConfig{Enabled: true, Path: t.TempDir()}}
 	m, err := NewManager(cfg)
 	if err != nil {
@@ -69,7 +77,7 @@ func TestRemoveVersionRejectsDangerousInput(t *testing.T) {
 	}
 	cases := [][2]string{{"node", ""}, {"node", ".."}, {"node", "."}, {"../x", "1"}, {"", "1"}}
 	for _, c := range cases {
-		if err := RemoveVersion(m, c[0], c[1]); err == nil {
+		if err := RemoveVersion(context.Background(), m, c[0], c[1]); err == nil {
 			t.Errorf("RemoveVersion(%q, %q) = nil, want error", c[0], c[1])
 		}
 	}
@@ -78,18 +86,25 @@ func TestRemoveVersionRejectsDangerousInput(t *testing.T) {
 			t.Fatalf("node %s was deleted by a rejected call", v)
 		}
 	}
-	if err := RemoveVersion(m, "node", "18.0.0"); err != nil {
+	if err := RemoveVersion(context.Background(), m, "node", "18.0.0"); err != nil {
 		t.Fatalf("legit removal failed: %v", err)
 	}
 }
 
-func TestLocalEnvIgnoresPathLikeVersion(t *testing.T) {
+func TestLocalEnvRejectsPathLikeVersion(t *testing.T) {
 	m := isolate(t)
 	if err := os.WriteFile(".xpm-env", []byte("node=../../../../tmp/evil\n"), 0o644); err != nil {
 		t.Fatal(err)
 	}
-	if v, _ := m.GetActiveVersion("node"); strings.Contains(v, "..") {
-		t.Fatalf("GetActiveVersion returned path-like version %q from .xpm-env", v)
+	a, err := m.ActiveVersion("node")
+	if err == nil || !strings.Contains(err.Error(), `invalid node version "../../../../tmp/evil"`) {
+		t.Fatalf("ActiveVersion = %+v, %v; want an invalid-version error", a, err)
+	}
+	if errors.Is(err, ErrNoVersion) || errors.Is(err, ErrNotInstalled) {
+		t.Fatal("an invalid entry must fail closed, not fall back")
+	}
+	if strings.Contains(a.Version, "..") {
+		t.Fatalf("returned path-like version %q", a.Version)
 	}
 }
 
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/ -run '.' -count=1`
Expected: build failure: `undefined: writeMeta`, `m.ActiveVersion undefined`, `undefined: SetLocalVersion` ...

- [ ] **Step 3: Implement `internal/env/state.go`**

```go
package env

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoVersion means no .xpm-env and no active.json entry names the runtime.
var ErrNoVersion = errors.New("no version configured")

// ErrNotInstalled means a version is configured but none installed matches it.
var ErrNotInstalled = errors.New("not installed")

// Active is the version of a runtime in effect for the current directory.
type Active struct {
	Version string // exact installed version (or the raw value when not installed)
	Source  string // the .xpm-env or active.json that set it
	Global  bool   // true when Source is active.json
}

const envFileName = ".xpm-env"

// ActiveVersion resolves the runtime's version for the current directory:
// the nearest .xpm-env (walking up from cwd) that has a key for it, else
// active.json. The value may be exact, partial ("20"), "latest" or "lts";
// it is matched against installed versions. Shims and the CLI share this.
func (m *Manager) ActiveVersion(runtime string) (Active, error) {
	raw, source, global, err := m.configuredVersion(runtime)
	if err != nil {
		return Active{}, err
	}
	a := Active{Version: raw, Source: source, Global: global}
	if ValidateVersionSpec(raw) != nil {
		return Active{}, fmt.Errorf("invalid %s version %q in %s", runtime, raw, source)
	}
	exact, ok := m.resolveInstalled(runtime, raw)
	if !ok {
		return a, fmt.Errorf("%s@%s is %w (set in %s)", runtime, raw, ErrNotInstalled, source)
	}
	a.Version = exact
	return a, nil
}

// configuredVersion finds the raw configured value and where it came from.
func (m *Manager) configuredVersion(runtime string) (raw, source string, global bool, err error) {
	if cwd, werr := os.Getwd(); werr == nil {
		for dir := cwd; ; {
			path := filepath.Join(dir, envFileName)
			if data, rerr := os.ReadFile(path); rerr == nil {
				if v, ok := parseEnvFile(string(data))[runtime]; ok {
					return v, path, false, nil
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	active, err := m.loadActiveVersions()
	if err != nil {
		return "", "", false, err
	}
	if v, ok := active[runtime]; ok {
		return v, m.activePath, true, nil
	}
	return "", "", false, fmt.Errorf("%s: %w", runtime, ErrNoVersion)
}

// InstalledVersions lists the installed versions of runtime, newest first.
// Hidden entries (".tmp-*", ".lock") and names that are not versions
// (pre-P5 "latest"/"lts" alias dirs) are ignored.
func (m *Manager) InstalledVersions(runtime string) ([]string, error) {
	if err := ValidateRuntimeName(runtime); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(m.runtimesPath, runtime))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") || ValidateVersionSpec(name) != nil {
			continue
		}
		if _, ok := ParseVersion(name); !ok {
			continue
		}
		versions = append(versions, name)
	}
	SortVersionsDesc(versions)
	return versions, nil
}

// resolveInstalled matches a configured value against installed versions:
// exact name, "lts" (highest whose metadata alias is lts), "latest"
// (highest), or a component-wise prefix (highest match, stable first).
func (m *Manager) resolveInstalled(runtime, spec string) (string, bool) {
	installed, err := m.InstalledVersions(runtime)
	if err != nil || len(installed) == 0 {
		return "", false
	}
	for _, v := range installed {
		if v == spec {
			return v, true
		}
	}
	if spec == "lts" {
		for _, v := range installed { // newest first
			if readMeta(filepath.Join(m.runtimesPath, runtime, v)).Alias == "lts" {
				return v, true
			}
		}
		return "", false
	}
	if v, ok := HighestMatch(spec, installed, false); ok {
		return v, true
	}
	return HighestMatch(spec, installed, true)
}

// versionMeta is <versionDir>/.xpm-meta.json.
type versionMeta struct {
	Version string `json:"version"`
	Alias   string `json:"alias"`
}

const metaFileName = ".xpm-meta.json"

func readMeta(dir string) versionMeta {
	var meta versionMeta
	data, err := os.ReadFile(filepath.Join(dir, metaFileName))
	if err == nil {
		_ = json.Unmarshal(data, &meta) // a damaged file only loses the alias
	}
	return meta
}

func writeMeta(dir string, meta versionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, metaFileName), append(data, '\n'), 0o644)
}

// loadActiveVersions reads active.json; a missing file is an empty map.
func (m *Manager) loadActiveVersions() (map[string]string, error) {
	active := map[string]string{}
	data, err := os.ReadFile(m.activePath)
	if errors.Is(err, os.ErrNotExist) {
		return active, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &active); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", m.activePath, err)
	}
	return active, nil
}

// GlobalVersion returns the raw active.json entry for runtime ("" if none).
func (m *Manager) GlobalVersion(runtime string) (string, error) {
	active, err := m.loadActiveVersions()
	if err != nil {
		return "", err
	}
	return active[runtime], nil
}

// SetGlobalVersion writes runtime=version into active.json atomically
// (sorted keys, trailing newline).
func (m *Manager) SetGlobalVersion(runtime, version string) error {
	active, err := m.loadActiveVersions()
	if err != nil {
		return err
	}
	active[runtime] = version
	data, err := json.MarshalIndent(active, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(m.activePath, append(data, '\n'), 0o644)
}

// SetLocalVersion writes runtime=version into dir/.xpm-env atomically,
// keeping comments, blank lines and key order.
func SetLocalVersion(dir, runtime, version string) (string, error) {
	path := filepath.Join(dir, envFileName)
	perm := os.FileMode(0o644)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if fi, serr := os.Stat(path); serr == nil {
		perm = fi.Mode().Perm()
	}
	return path, writeFileAtomic(path, []byte(updateEnvContent(string(data), runtime, version)), perm)
}

// parseEnvFile parses `key=value` lines; '#' starts a comment line. The
// first occurrence of a key wins.
func parseEnvFile(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			continue
		}
		if _, seen := result[key]; !seen {
			result[key] = value
		}
	}
	return result
}

// updateEnvContent replaces the value of the first `key=` line (keeping its
// key text and indentation) or appends `key=value`.
func updateEnvContent(content, key, value string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		k, _, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		eq := strings.Index(line, "=")
		cr := ""
		if strings.HasSuffix(line, "\r") {
			cr = "\r"
		}
		lines[i] = line[:eq+1] + value + cr
		return strings.Join(lines, "\n")
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + key + "=" + value + "\n"
}

// writeFileAtomic writes data to a temp file in path's directory and renames
// it over path, so readers see the old or the new file, never a mix.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, perm)
	}
	if werr == nil {
		werr = os.Rename(name, path)
	}
	if werr != nil {
		_ = os.Remove(name)
		return werr
	}
	return nil
}
```

- [ ] **Step 4: Slim `internal/env/manager.go`** (replace the whole file; the state code moved to `state.go`)

```go
// Package env provides runtime version management functionality.
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crenspire/xpm/internal/config"
)

// Manager manages runtime versions and environment.
type Manager struct {
	config       config.Config
	envPath      string
	runtimesPath string
	shimsPath    string
	activePath   string
}

// NewManager creates a new environment manager.
func NewManager(cfg config.Config) (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	envPath := cfg.Env.Path
	if envPath == "" {
		envPath = "~/.xpm/env"
	}
	if envPath == "~" {
		envPath = homeDir
	} else if strings.HasPrefix(envPath, "~/") {
		envPath = filepath.Join(homeDir, envPath[2:])
	}

	m := &Manager{
		config:       cfg,
		envPath:      envPath,
		runtimesPath: filepath.Join(envPath, "runtimes"),
		shimsPath:    filepath.Join(envPath, "shims"),
		activePath:   filepath.Join(envPath, "active.json"),
	}

	if err := m.EnsureDirs(); err != nil {
		return nil, err
	}

	return m, nil
}

// GetEnvPath returns the environment root path.
func (m *Manager) GetEnvPath() string {
	return m.envPath
}

// GetRuntimesPath returns the runtimes directory path.
func (m *Manager) GetRuntimesPath() string {
	return m.runtimesPath
}

// GetShimsPath returns the shims directory path.
func (m *Manager) GetShimsPath() string {
	return m.shimsPath
}

// GetActivePath returns the path of the global active.json.
func (m *Manager) GetActivePath() string {
	return m.activePath
}

// EnsureDirs creates necessary directories.
func (m *Manager) EnsureDirs() error {
	for _, dir := range []string{m.envPath, m.runtimesPath, m.shimsPath} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}
```

- [ ] **Step 5: Replace `internal/env/use.go`** (no shim or PATH work here any more; the CLI does it)

```go
package env

import (
	"fmt"
	"os"
)

// UseVersion resolves spec against INSTALLED versions (same matching as
// ActiveVersion) and records the exact version: in active.json when global,
// else in ./.xpm-env. It returns what was written and where.
func UseVersion(m *Manager, runtime, spec string, global bool) (Active, error) {
	if err := ValidateRuntimeName(runtime); err != nil {
		return Active{}, err
	}
	if err := ValidateVersionSpec(spec); err != nil {
		return Active{}, err
	}
	exact, ok := m.resolveInstalled(runtime, spec)
	if !ok {
		return Active{}, fmt.Errorf("%s@%s is not installed\nInstall it: xpm env install %s@%s", runtime, spec, runtime, spec)
	}
	if global {
		if err := m.SetGlobalVersion(runtime, exact); err != nil {
			return Active{}, err
		}
		return Active{Version: exact, Source: m.activePath, Global: true}, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Active{}, err
	}
	path, err := SetLocalVersion(cwd, runtime, exact)
	if err != nil {
		return Active{}, err
	}
	return Active{Version: exact, Source: path}, nil
}
```

- [ ] **Step 6: Replace `internal/env/remove.go`**

```go
package env

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// RemoveVersion removes an installed version. It refuses the version that is
// active here or set as the global default.
func RemoveVersion(_ context.Context, m *Manager, runtime, version string) error {
	dir, err := m.versionDir(runtime, version)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s@%s is not installed", runtime, version)
	}
	if a, err := m.ActiveVersion(runtime); err == nil && a.Version == version {
		return fmt.Errorf("cannot remove %s@%s: it is active here (set in %s)\nSwitch first: xpm env use %s@<other-version>", runtime, version, a.Source, runtime)
	}
	if g, err := m.GlobalVersion(runtime); err == nil && g != "" {
		if exact, ok := m.resolveInstalled(runtime, g); ok && exact == version {
			return fmt.Errorf("cannot remove %s@%s: it is the global default (set in %s)\nSwitch first: xpm env use --global %s@<other-version>", runtime, version, m.activePath, runtime)
		}
	}
	// Rename first so the version disappears atomically, then delete.
	trash := filepath.Join(filepath.Dir(dir), ".tmp-old-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.Rename(dir, trash); err != nil {
		return fmt.Errorf("remove %s@%s: %w", runtime, version, err)
	}
	return os.RemoveAll(trash)
}
```

- [ ] **Step 7: Replace `internal/env/list.go`**

```go
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ANSI color codes for terminal output
const (
	colorReset = "\033[0m"
	colorBold  = "\033[1m"
	colorCyan  = "\033[36m"
	colorGreen = "\033[32m"
)

// VersionInfo contains information about an installed version.
type VersionInfo struct {
	Version string
	Path    string
	Active  bool
	Alias   string // "lts" or "latest" when installed through that alias
}

// ListInstalled returns installed versions per runtime, newest first.
func ListInstalled(m *Manager) (map[string][]VersionInfo, error) {
	result := make(map[string][]VersionInfo)
	entries, err := os.ReadDir(m.GetRuntimesPath())
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read runtimes directory: %w", err)
	}
	for _, entry := range entries {
		runtime := entry.Name()
		if !entry.IsDir() || ValidateRuntimeName(runtime) != nil {
			continue
		}
		versions, err := m.InstalledVersions(runtime)
		if err != nil || len(versions) == 0 {
			continue
		}
		active, _ := m.ActiveVersion(runtime)
		for _, v := range versions {
			dir := filepath.Join(m.GetRuntimesPath(), runtime, v)
			result[runtime] = append(result[runtime], VersionInfo{
				Version: v,
				Path:    dir,
				Active:  v == active.Version,
				Alias:   readMeta(dir).Alias,
			})
		}
	}
	return result, nil
}

// FormatInstalled formats the installed versions for display.
func FormatInstalled(installed map[string][]VersionInfo) string {
	if len(installed) == 0 {
		return "No runtimes installed.\n\nInstall one:\n  xpm env install node@20\n  xpm env install python@3.12\n  xpm env install go@latest\n\nRuntimes: node, go, python, java, rust, bun, deno, php\nSee available versions: xpm env ls-remote <runtime>\n"
	}

	runtimes := make([]string, 0, len(installed))
	for runtime := range installed {
		runtimes = append(runtimes, runtime)
	}
	sort.Strings(runtimes)

	var b strings.Builder
	for _, runtime := range runtimes {
		fmt.Fprintf(&b, "%s%s%s%s:\n", colorBold, colorCyan, runtime, colorReset)
		for _, v := range installed[runtime] {
			alias := ""
			if v.Alias != "" {
				alias = " (" + v.Alias + ")"
			}
			if v.Active {
				fmt.Fprintf(&b, "%s%s  → %s%s%s %s%s(active)%s\n", colorBold, colorGreen, v.Version, colorReset, alias, colorBold, colorGreen, colorReset)
			} else {
				fmt.Fprintf(&b, "  - %s%s\n", v.Version, alias)
			}
		}
	}
	return b.String()
}
```

- [ ] **Step 8: Delete `internal/env/detect.go`**

```bash
rm internal/env/detect.go
```

- [ ] **Step 9: Adapt the remaining callers**

`internal/env/install.go` (install no longer pins anything; Task 4 rewrites this file):

```diff
--- a/internal/env/install.go
+++ b/internal/env/install.go
@@ -96,7 +96,7 @@ func InstallRuntimeWithAlias(manager *Manager, runtime, version, alias string) e
 			fmt.Printf("%s@%s is already installed at %s\n", runtime, resolvedVersion, dest)
 			// Still save alias if provided
 			if detectedAlias != "" {
-				saveVersionAlias(dest, detectedAlias)
+				_ = writeMeta(dest, versionMeta{Version: resolvedVersion, Alias: detectedAlias})
 			}
 			return nil
 		}
@@ -130,7 +130,7 @@ func InstallRuntimeWithAlias(manager *Manager, runtime, version, alias string) e
 
 	// Save alias metadata if detected (after successful installation)
 	if detectedAlias != "" {
-		if err := saveVersionAlias(dest, detectedAlias); err != nil {
+		if err := writeMeta(dest, versionMeta{Version: resolvedVersion, Alias: detectedAlias}); err != nil {
 			logx.Info("failed to save alias metadata: %v", err)
 		}
 	}
@@ -152,13 +152,6 @@ func InstallRuntimeWithAlias(manager *Manager, runtime, version, alias string) e
 
 	fmt.Printf("✓ Installed %s@%s at %s\n", runtime, version, dest)
 
-	// Automatically activate the installed version (local)
-	// Use the resolved version, not the original (which might be an alias)
-	if err := UseVersion(manager, runtime, resolvedVersion, false); err != nil {
-		logx.Info("failed to auto-activate %s@%s: %v", runtime, resolvedVersion, err)
-		// Don't fail installation if activation fails
-	}
-
 	// Update shims
 	if err := CreateShims(manager); err != nil {
 		logx.Info("failed to update shims: %v", err)
```

`internal/env/shims.go` (old `CreateShims`; Task 5 rewrites this file):

```diff
--- a/internal/env/shims.go
+++ b/internal/env/shims.go
@@ -35,10 +35,11 @@ func CreateShims(manager *Manager) error {
 		}
 
 		// Get active version to determine binary paths
-		version, err := manager.GetActiveVersion(runtime)
+		active, err := manager.ActiveVersion(runtime)
 		if err != nil {
 			continue
 		}
+		version := active.Version
 
 		versionPath := filepath.Join(runtimesPath, runtime, version)
 		binaryPaths := installer.BinaryPaths(version, versionPath)
```

`internal/cli/env_cmd.go` (Task 13 rewrites this file):

```diff
--- a/internal/cli/env_cmd.go
+++ b/internal/cli/env_cmd.go
@@ -1,6 +1,7 @@
 package cli
 
 import (
+	"context"
 	"fmt"
 	"os"
 	"strings"
@@ -119,10 +120,15 @@ func cmdEnvUse(manager *env.Manager, args []string) int {
 		return 1
 	}
 
-	if err := env.UseVersion(manager, runtime, version, global); err != nil {
+	a, err := env.UseVersion(manager, runtime, version, global)
+	if err != nil {
 		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
 		return 1
 	}
+	fmt.Printf("Using %s@%s (%s)\n", runtime, a.Version, a.Source)
+	if err := env.CreateShims(manager); err != nil {
+		fmt.Fprintf(os.Stderr, "warning: could not update shims: %v\n", err)
+	}
 
 	return 0
 }
@@ -163,9 +169,9 @@ func cmdEnvCurrent(manager *env.Manager) int {
 	found := false
 
 	for _, runtime := range runtimes {
-		version, err := manager.GetActiveVersion(runtime)
+		a, err := manager.ActiveVersion(runtime)
 		if err == nil {
-			fmt.Printf("%s: %s\n", runtime, version)
+			fmt.Printf("%s %s (%s)\n", runtime, a.Version, a.Source)
 			found = true
 		}
 	}
@@ -191,10 +197,11 @@ func cmdEnvRemove(manager *env.Manager, args []string) int {
 		return 1
 	}
 
-	if err := env.RemoveVersion(manager, runtime, version); err != nil {
+	if err := env.RemoveVersion(context.Background(), manager, runtime, version); err != nil {
 		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
 		return 1
 	}
+	fmt.Printf("Removed %s@%s\n", runtime, version)
 
 	return 0
 }
```

- [ ] **Step 10: Run the tests to verify they pass**

Run: `go test ./internal/env/ -run '.' -count=1`
Expected: `ok`, including `TestRemoveVersionRejectsDangerousInput` and `TestLocalEnvRejectsPathLikeVersion`.

- [ ] **Step 11: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 12: Commit (only this task's files)**

```bash
git add internal/env/state.go \
  internal/env/state_test.go \
  internal/env/fake_test.go \
  internal/env/manager.go \
  internal/env/use.go \
  internal/env/list.go \
  internal/env/remove.go \
  internal/env/validate_test.go \
  internal/env/detect.go \
  internal/env/install.go \
  internal/env/shims.go \
  internal/cli/env_cmd.go
git commit -m "env: one active-version resolver and atomic state files"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 4: Installer interface v2 and the atomic install engine (R4, R8)

**Files:**
- Modify (whole file): `internal/env/runtime.go`, `internal/env/install.go`, `internal/env/remote.go`, `internal/env/fake_test.go`
- Create: `internal/env/lock_unix.go`, `internal/env/lock_other.go`, `internal/env/install_test.go`
- Modify (hunks): `internal/env/manager.go` (output writer), `internal/env/remove.go` (`Remover`), `internal/env/shims.go` (old `CreateShims`), `internal/env/validate_test.go` (`isolate` discards output), `internal/cli/env_cmd.go` (install with Ctrl-C, ls-remote with ctx)
- Modify (signatures only): every `internal/env/runtimes/*.go` installer, `internal/env/runtimes/rust_test.go`

**Interfaces:**
- Consumes: Task 1 `ParseVersion`, `HighestMatch`; Task 3 `versionDir`, `writeMeta`, `readMeta`, `GlobalVersion`, `SetGlobalVersion`, `InstalledVersions`.
- Produces:
  - `type RuntimeInstaller interface { Name() string; ListRemote(ctx context.Context) ([]string, error); Install(ctx context.Context, req InstallRequest) error; BinaryPaths() []string }`
  - `type InstallRequest struct{ Version, Dest, Root string }`
  - optional: `type Resolver interface{ Resolve(ctx context.Context, spec string) (string, error) }`, `type LTSResolver interface{ LatestLTS(ctx context.Context) (string, error) }`, `type Remover interface{ Remove(ctx context.Context, version, dir, root string) error }`
  - `func ResolveSpec(ctx context.Context, inst RuntimeInstaller, spec string) (string, error)`
  - `func InstallRuntime(ctx context.Context, m *Manager, rt, spec string) (string, error)` (returns the exact version)
  - `func ListRemote(ctx context.Context, runtime string) ([]string, error)` (no Manager argument any more)
  - `func ListRuntimes() []string` (sorted), `func binaryOwner(binary string) (runtime, rel string, ok bool)`, `func unregisterInstaller(name string)` (tests)
  - `func lockFile(ctx context.Context, path string, onWait func()) (func(), error)` (flock; polls every 100 ms so Ctrl-C works while waiting)
  - `func (m *Manager) SetOutput(w io.Writer)`, `func (m *Manager) printf(format string, args ...any)`
  - `func verifyInstallation(dir string, binaryPaths []string) error`
  - tests: `fakeInstaller` (+ `fakeLTS`, `fakeResolver`), `useFake(t, inst)`
  - Messages: `Resolved <rt>@<spec> to <v>`, `<rt>@<v> is already installed`, `Waiting for another xpm process installing <rt>...`, `Installing <rt>@<v>...`, `Installed <rt>@<v>`, `Set <rt>@<v> as the global default`, `Use it here: xpm env use <rt>@<v>`, `<rt> has no lts alias`, `no <rt> version matches <spec> (see: xpm env ls-remote <rt>)`.

- [ ] **Step 1: Write the failing tests**

`internal/env/fake_test.go` (whole file):

```go
package env

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fakeRT is the test-only runtime name; the real installers live in
// internal/env/runtimes, which this package cannot import.
const fakeRT = "xpmfake"

// fakeInstaller records calls and installs tiny executable files.
type fakeInstaller struct {
	mu        sync.Mutex
	remote    []string
	remoteErr error
	lts       string // non-empty: also implements LTSResolver via fakeLTS
	installed []string
	install   func(ctx context.Context, req InstallRequest) error // nil: write the binaries
	listCalls int
}

func (f *fakeInstaller) Name() string { return fakeRT }

func (f *fakeInstaller) ListRemote(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return f.remote, f.remoteErr
}

func (f *fakeInstaller) Install(ctx context.Context, req InstallRequest) error {
	f.mu.Lock()
	f.installed = append(f.installed, req.Version)
	fn := f.install
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return writeFakeBinaries(req.Dest)
}

func (f *fakeInstaller) BinaryPaths() []string { return []string{"bin/fakebin", "bin/fakebin2"} }

// fakeLTS adds LatestLTS to fakeInstaller.
type fakeLTS struct{ *fakeInstaller }

func (f fakeLTS) LatestLTS(context.Context) (string, error) {
	if f.lts == "" {
		return "", errors.New("no lts")
	}
	return f.lts, nil
}

// fakeResolver adds Resolve to fakeInstaller.
type fakeResolver struct {
	*fakeInstaller
	resolve func(spec string) (string, error)
}

func (f fakeResolver) Resolve(_ context.Context, spec string) (string, error) {
	return f.resolve(spec)
}

func writeFakeBinaries(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		return err
	}
	for _, b := range []string{"fakebin", "fakebin2"} {
		if err := os.WriteFile(filepath.Join(dir, "bin", b), []byte("#!/bin/sh\n"), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// useFake registers inst as the xpmfake runtime for one test.
func useFake(t *testing.T, inst RuntimeInstaller) {
	t.Helper()
	RegisterInstaller(fakeRT, inst)
	t.Cleanup(func() { unregisterInstaller(fakeRT) })
}

// installFake creates an installed xpmfake version without InstallRuntime.
func installFake(t *testing.T, m *Manager, version, alias string) string {
	t.Helper()
	dir := filepath.Join(m.GetRuntimesPath(), fakeRT, version)
	if err := writeFakeBinaries(dir); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(dir, versionMeta{Version: version, Alias: alias}); err != nil {
		t.Fatal(err)
	}
	return dir
}
```

`internal/env/install_test.go`:

```go
package env

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolveSpec(t *testing.T) {
	ctx := context.Background()
	base := &fakeInstaller{remote: []string{"20.9.0", "20.11.1", "200.1.0", "21.0.0-rc.1", "18.19.0", "2.5rc1"}}
	cases := []struct {
		spec, want, err string
	}{
		{spec: "20", want: "20.11.1"},
		{spec: "latest", want: "200.1.0"},
		{spec: "21", err: "no xpmfake version matches 21 (see: xpm env ls-remote xpmfake)"},
		{spec: "2.5rc1", want: "2.5rc1"},
		{spec: "2.5", err: "no xpmfake version matches 2.5"},
		{spec: "lts", err: "xpmfake has no lts alias"},
		{spec: "../x", err: "invalid version"},
	}
	for _, c := range cases {
		got, err := ResolveSpec(ctx, base, c.spec)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: err = %v, want %q", c.spec, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.spec, got, err, c.want)
		}
	}

	offline := &fakeInstaller{remoteErr: errors.New("network must not be used")}
	if got, err := ResolveSpec(ctx, offline, "1.2.3"); err != nil || got != "1.2.3" {
		t.Errorf("exact spec: got %q, %v", got, err)
	}
	if offline.listCalls != 0 {
		t.Error("an exact spec must not list remote versions")
	}

	lts := fakeLTS{&fakeInstaller{lts: "20.11.1"}}
	if got, err := ResolveSpec(ctx, lts, "lts"); err != nil || got != "20.11.1" {
		t.Errorf("lts: got %q, %v", got, err)
	}

	res := fakeResolver{&fakeInstaller{}, func(spec string) (string, error) { return "9.9-" + spec, nil }}
	if got, err := ResolveSpec(ctx, res, "1"); err != nil || got != "9.9-1" {
		t.Errorf("Resolver: got %q, %v", got, err)
	}
	evil := fakeResolver{&fakeInstaller{}, func(string) (string, error) { return "../../etc", nil }}
	if _, err := ResolveSpec(ctx, evil, "1"); err == nil {
		t.Error("a path-like resolved version must be rejected")
	}
}

func noTmpLeft(t *testing.T, m *Manager) {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(m.GetRuntimesPath(), fakeRT))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("staging dir left behind: %s", e.Name())
		}
	}
}

func TestInstallRuntimeInstallsAtomicallyAndSetsGlobalDefault(t *testing.T) {
	m := isolate(t)
	var out bytes.Buffer
	m.SetOutput(&out)
	f := &fakeInstaller{remote: []string{"1.2.0", "1.10.0"}}
	var stagedIn string
	f.install = func(_ context.Context, req InstallRequest) error {
		stagedIn = req.Dest
		if req.Root != m.GetEnvPath() {
			t.Errorf("Root = %q", req.Root)
		}
		if entries, _ := os.ReadDir(req.Dest); len(entries) != 0 {
			t.Error("Dest must be an empty dir")
		}
		return writeFakeBinaries(req.Dest)
	}
	useFake(t, f)

	exact, err := InstallRuntime(context.Background(), m, fakeRT, "latest")
	if err != nil || exact != "1.10.0" {
		t.Fatalf("InstallRuntime = %q, %v", exact, err)
	}
	if !strings.HasPrefix(filepath.Base(stagedIn), ".tmp-1.10.0-") || filepath.Dir(stagedIn) != filepath.Join(m.GetRuntimesPath(), fakeRT) {
		t.Fatalf("staged in %s", stagedIn)
	}
	dir := filepath.Join(m.GetRuntimesPath(), fakeRT, "1.10.0")
	if meta := readMeta(dir); meta.Version != "1.10.0" || meta.Alias != "latest" {
		t.Fatalf("meta = %+v", meta)
	}
	noTmpLeft(t, m)
	if g, _ := m.GlobalVersion(fakeRT); g != "1.10.0" {
		t.Fatalf("global = %q", g)
	}
	if _, err := os.Stat(".xpm-env"); err == nil {
		t.Fatal("install wrote .xpm-env")
	}
	for _, want := range []string{"Resolved xpmfake@latest to 1.10.0", "Set xpmfake@1.10.0 as the global default"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q lacks %q", out.String(), want)
		}
	}

	out.Reset()
	if _, err := InstallRuntime(context.Background(), m, fakeRT, "1.10.0"); err != nil {
		t.Fatal(err)
	}
	if len(f.installed) != 1 || !strings.Contains(out.String(), "already installed") {
		t.Fatalf("second install ran the installer again: %v / %q", f.installed, out.String())
	}

	out.Reset()
	if _, err := InstallRuntime(context.Background(), m, fakeRT, "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if g, _ := m.GlobalVersion(fakeRT); g != "1.10.0" {
		t.Fatalf("an existing global default was replaced: %q", g)
	}
	if !strings.Contains(out.String(), "Use it here: xpm env use xpmfake@1.2.0") {
		t.Fatalf("output %q", out.String())
	}
}

func TestInstallRuntimeFailuresLeaveNothing(t *testing.T) {
	cases := map[string]func(ctx context.Context, req InstallRequest) error{
		"installer error": func(_ context.Context, req InstallRequest) error {
			_ = os.WriteFile(filepath.Join(req.Dest, "partial"), []byte("x"), 0o644)
			return errors.New("boom")
		},
		"missing binary": func(_ context.Context, req InstallRequest) error {
			return os.WriteFile(filepath.Join(req.Dest, "README"), []byte("x"), 0o644)
		},
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			m := isolate(t)
			useFake(t, &fakeInstaller{install: fn})
			if _, err := InstallRuntime(context.Background(), m, fakeRT, "1.0.0"); err == nil {
				t.Fatal("want an error")
			}
			if _, err := os.Stat(filepath.Join(m.GetRuntimesPath(), fakeRT, "1.0.0")); err == nil {
				t.Fatal("a partial version dir exists")
			}
			noTmpLeft(t, m)
			if g, _ := m.GlobalVersion(fakeRT); g != "" {
				t.Fatalf("failed install set global %q", g)
			}
		})
	}
}

func TestInstallRuntimeCancelledLeavesNothing(t *testing.T) {
	m := isolate(t)
	ctx, cancel := context.WithCancel(context.Background())
	useFake(t, &fakeInstaller{install: func(ctx context.Context, req InstallRequest) error {
		if err := writeFakeBinaries(req.Dest); err != nil {
			return err
		}
		cancel() // Ctrl-C arrives while the installer runs
		return ctx.Err()
	}})
	_, err := InstallRuntime(ctx, m, fakeRT, "1.0.0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(m.GetRuntimesPath(), fakeRT, "1.0.0")); err == nil {
		t.Fatal("cancelled install left a version dir")
	}
	noTmpLeft(t, m)
}

func TestInstallRuntimeReplacesCorruptAndStaleDirs(t *testing.T) {
	m := isolate(t)
	rtDir := filepath.Join(m.GetRuntimesPath(), fakeRT)
	for _, d := range []string{"1.0.0/lib", ".tmp-1.0.0-old"} {
		if err := os.MkdirAll(filepath.Join(rtDir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeInstaller{}
	useFake(t, f)
	if _, err := InstallRuntime(context.Background(), m, fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if len(f.installed) != 1 {
		t.Fatal("a dir without its binaries must be reinstalled")
	}
	if _, err := os.Stat(filepath.Join(rtDir, "1.0.0", "lib")); err == nil {
		t.Fatal("the corrupt install was not replaced")
	}
	noTmpLeft(t, m)
}

func TestInstallRuntimeWaitsForConcurrentInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock is Unix-only")
	}
	m := isolate(t)
	var out bytes.Buffer
	m.SetOutput(&out)
	f := &fakeInstaller{}
	useFake(t, f)
	rtDir := filepath.Join(m.GetRuntimesPath(), fakeRT)
	if err := os.MkdirAll(rtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// "Another process" holds the lock and finishes the same version.
	unlock, err := lockFile(context.Background(), filepath.Join(rtDir, ".lock"), func() {})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := InstallRuntime(context.Background(), m, fakeRT, "1.0.0")
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	installFake(t, m, "1.0.0", "")
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("install never got the lock")
	}
	if len(f.installed) != 0 {
		t.Fatal("installed again after waiting; must re-check under the lock")
	}
	for _, want := range []string{"Waiting for another xpm process installing xpmfake...", "xpmfake@1.0.0 is already installed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q lacks %q", out.String(), want)
		}
	}
}

func TestLockFileWaitsAndHonoursCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("flock is Unix-only")
	}
	path := filepath.Join(t.TempDir(), ".lock")
	unlock, err := lockFile(context.Background(), path, func() { t.Error("first lock must not wait") })
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	waited := false
	if _, err := lockFile(ctx, path, func() { waited = true }); !errors.Is(err, context.DeadlineExceeded) || !waited {
		t.Fatalf("second lock: err=%v waited=%v", err, waited)
	}

	got := make(chan error, 1)
	go func() {
		u, err := lockFile(context.Background(), path, func() {})
		if err == nil {
			u()
		}
		got <- err
	}()
	time.Sleep(150 * time.Millisecond)
	unlock()
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiter never got the lock")
	}
}
```

In `internal/env/validate_test.go`, discard progress output in `isolate`:

```diff
--- a/internal/env/validate_test.go
+++ b/internal/env/validate_test.go
@@ -5,6 +5,7 @@ import (
 	"errors"
 	"go/parser"
 	"go/token"
+	"io"
 	"os"
 	"path/filepath"
 	"strings"
@@ -27,7 +28,8 @@ func chdir(t *testing.T, dir string) {
 }
 
 // isolate points HOME at a temp dir, chdirs into another (the repo root has
-// its own .xpm-env) and returns a Manager rooted in a third.
+// its own .xpm-env) and returns a Manager rooted in a third. Progress
+// output is discarded.
 func isolate(t *testing.T) *Manager {
 	t.Helper()
 	t.Setenv("HOME", t.TempDir())
@@ -37,6 +39,7 @@ func isolate(t *testing.T) *Manager {
 	if err != nil {
 		t.Fatal(err)
 	}
+	m.SetOutput(io.Discard)
 	return m
 }
 
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/ -run 'ResolveSpec|InstallRuntime|LockFile' -count=1`
Expected: build failure: `undefined: InstallRequest`, `undefined: ResolveSpec`, `undefined: lockFile`, `m.SetOutput undefined` ...

- [ ] **Step 3: Replace `internal/env/runtime.go`**

```go
package env

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
)

// RuntimeInstaller is implemented by every runtime (internal/env/runtimes).
type RuntimeInstaller interface {
	// Name returns the runtime name ("node").
	Name() string
	// ListRemote returns exact available versions, in any order.
	ListRemote(ctx context.Context) ([]string, error)
	// Install puts req.Version into req.Dest, an existing EMPTY staging dir.
	Install(ctx context.Context, req InstallRequest) error
	// BinaryPaths lists slash-separated paths, relative to the version dir,
	// that must exist after Install. Their base names become shims.
	BinaryPaths() []string
}

// InstallRequest is what InstallRuntime hands an installer.
type InstallRequest struct {
	Version string // exact version
	Dest    string // empty staging dir; renamed into place after verification
	Root    string // xpm env root, for shared tool homes (rustup)
}

// Resolver overrides the generic spec resolution of ResolveSpec.
type Resolver interface {
	Resolve(ctx context.Context, spec string) (string, error)
}

// LTSResolver gives the "lts" alias a meaning.
type LTSResolver interface {
	LatestLTS(ctx context.Context) (string, error)
}

// Remover cleans up state outside the version dir (rustup toolchains).
type Remover interface {
	Remove(ctx context.Context, version, dir, root string) error
}

var (
	installersMu sync.RWMutex
	installers   = make(map[string]RuntimeInstaller)
)

// RegisterInstaller registers a runtime installer.
func RegisterInstaller(name string, installer RuntimeInstaller) {
	installersMu.Lock()
	defer installersMu.Unlock()
	installers[name] = installer
}

// unregisterInstaller removes a test installer.
func unregisterInstaller(name string) {
	installersMu.Lock()
	defer installersMu.Unlock()
	delete(installers, name)
}

// GetInstaller returns the installer for the given runtime name.
func GetInstaller(name string) (RuntimeInstaller, error) {
	installersMu.RLock()
	installer, ok := installers[name]
	installersMu.RUnlock()
	if ok {
		return installer, nil
	}
	if owner, _, found := binaryOwner(name); found && owner != name {
		return nil, fmt.Errorf("%s is not a runtime; it comes with %s\nInstall it with: xpm env install %s@<version>", name, owner, owner)
	}
	return nil, fmt.Errorf("unknown runtime %q (available: %s)", name, strings.Join(ListRuntimes(), ", "))
}

// ListRuntimes returns all registered runtime names, sorted.
func ListRuntimes() []string {
	installersMu.RLock()
	defer installersMu.RUnlock()
	names := make([]string, 0, len(installers))
	for name := range installers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// binaryOwner finds the runtime whose BinaryPaths has base name binary.
func binaryOwner(binary string) (runtime, rel string, ok bool) {
	for _, name := range ListRuntimes() {
		installersMu.RLock()
		inst := installers[name]
		installersMu.RUnlock()
		for _, p := range inst.BinaryPaths() {
			if path.Base(p) == binary {
				return name, p, true
			}
		}
	}
	return "", "", false
}
```

- [ ] **Step 4: Per-runtime lock**

`internal/env/lock_unix.go`:

```go
//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package env

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

// lockFile takes an exclusive flock on path. If another process holds it,
// onWait is called once and the lock is retried until ctx is done.
func lockFile(ctx context.Context, path string, onWait func()) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	fd := int(f.Fd()) //nolint:gosec // G115: a file descriptor always fits in int
	waited := false
	for {
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(fd, syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = f.Close()
			return nil, err
		}
		if !waited {
			waited = true
			onWait()
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
```

`internal/env/lock_other.go`:

```go
//go:build !(darwin || linux || freebsd || netbsd || openbsd || dragonfly)

package env

import "context"

// lockFile is a no-op where flock is unavailable (xpm env is Unix-only).
func lockFile(ctx context.Context, _ string, _ func()) (func(), error) {
	return func() {}, ctx.Err()
}
```

- [ ] **Step 5: Replace `internal/env/install.go`**

```go
package env

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ResolveSpec turns a user spec into one exact version. An installer's
// Resolver wins; otherwise "latest" is the highest stable remote version,
// "lts" needs an LTSResolver, a spec with three or more numeric components
// is taken as-is (no network), and a partial spec picks the highest
// matching remote version (stable only unless the spec is a prerelease).
func ResolveSpec(ctx context.Context, inst RuntimeInstaller, spec string) (string, error) {
	if err := ValidateVersionSpec(spec); err != nil {
		return "", err
	}
	rt := inst.Name()
	exact, err := resolveSpec(ctx, inst, spec)
	if err != nil {
		return "", err
	}
	if err := ValidateVersionSpec(exact); err != nil {
		return "", fmt.Errorf("%s: resolving %s gave an unusable version: %w", rt, spec, err)
	}
	return exact, nil
}

func resolveSpec(ctx context.Context, inst RuntimeInstaller, spec string) (string, error) {
	rt := inst.Name()
	if r, ok := inst.(Resolver); ok {
		return r.Resolve(ctx, spec)
	}
	if spec == "lts" {
		l, ok := inst.(LTSResolver)
		if !ok {
			return "", fmt.Errorf("%s has no lts alias", rt)
		}
		return l.LatestLTS(ctx)
	}
	pv, parsed := ParseVersion(spec)
	if spec != "latest" && parsed && len(pv.Nums) >= 3 {
		return spec, nil
	}
	versions, err := inst.ListRemote(ctx)
	if err != nil {
		return "", fmt.Errorf("list %s versions: %w", rt, err)
	}
	if v, ok := HighestMatch(spec, versions, parsed && pv.IsPrerelease()); ok {
		return v, nil
	}
	return "", fmt.Errorf("no %s version matches %s (see: xpm env ls-remote %s)", rt, spec, rt)
}

// InstallRuntime resolves spec, then installs that exact version atomically:
// it stages into runtimes/<rt>/.tmp-*, verifies every BinaryPaths entry,
// writes .xpm-meta.json and renames the stage into place, all under a
// per-runtime lock. On any failure or cancellation nothing is left behind.
// It never writes .xpm-env; if the runtime has no global version yet, the
// installed one becomes the global default.
func InstallRuntime(ctx context.Context, m *Manager, rt, spec string) (string, error) {
	if err := ValidateRuntimeName(rt); err != nil {
		return "", err
	}
	if err := ValidateVersionSpec(spec); err != nil {
		return "", err
	}
	inst, err := GetInstaller(rt)
	if err != nil {
		return "", err
	}
	exact, err := ResolveSpec(ctx, inst, spec)
	if err != nil {
		return "", err
	}
	if exact != spec {
		m.printf("Resolved %s@%s to %s\n", rt, spec, exact)
	}
	dest, err := m.versionDir(rt, exact)
	if err != nil {
		return "", err
	}
	alias := ""
	if spec == "lts" || spec == "latest" {
		alias = spec
	}

	if verifyInstallation(dest, inst.BinaryPaths()) == nil {
		m.printf("%s@%s is already installed\n", rt, exact)
		return exact, m.afterInstall(rt, exact, dest, alias)
	}

	rtDir := filepath.Dir(dest)
	if err := os.MkdirAll(rtDir, 0o755); err != nil {
		return "", err
	}
	unlock, err := lockFile(ctx, filepath.Join(rtDir, ".lock"), func() {
		m.printf("Waiting for another xpm process installing %s...\n", rt)
	})
	if err != nil {
		return "", err
	}
	defer unlock()

	if verifyInstallation(dest, inst.BinaryPaths()) == nil { // another process won
		m.printf("%s@%s is already installed\n", rt, exact)
		return exact, m.afterInstall(rt, exact, dest, alias)
	}
	removeStale(rtDir)

	staging, err := os.MkdirTemp(rtDir, ".tmp-"+exact+"-")
	if err != nil {
		return "", err
	}
	done := false
	defer func() {
		if !done {
			_ = os.RemoveAll(staging)
		}
	}()

	m.printf("Installing %s@%s...\n", rt, exact)
	if err := inst.Install(ctx, InstallRequest{Version: exact, Dest: staging, Root: m.envPath}); err != nil {
		return "", fmt.Errorf("install %s@%s: %w", rt, exact, err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := verifyInstallation(staging, inst.BinaryPaths()); err != nil {
		return "", fmt.Errorf("install %s@%s: %w", rt, exact, err)
	}
	if err := writeMeta(staging, versionMeta{Version: exact, Alias: alias}); err != nil {
		return "", err
	}
	if _, err := os.Lstat(dest); err == nil { // corrupt leftover: move aside, delete
		old := filepath.Join(rtDir, ".tmp-old-"+strconv.FormatInt(time.Now().UnixNano(), 36))
		if err := os.Rename(dest, old); err != nil {
			return "", err
		}
		_ = os.RemoveAll(old)
	}
	if err := os.Rename(staging, dest); err != nil {
		return "", err
	}
	done = true
	m.printf("Installed %s@%s\n", rt, exact)
	return exact, m.afterInstall(rt, exact, "", "")
}

// afterInstall records an "lts" alias on an existing install and sets the
// global default when the runtime has none.
func (m *Manager) afterInstall(rt, exact, existingDir, alias string) error {
	if existingDir != "" && alias == "lts" && readMeta(existingDir).Alias != "lts" {
		if err := writeMeta(existingDir, versionMeta{Version: exact, Alias: alias}); err != nil {
			return err
		}
	}
	global, err := m.GlobalVersion(rt)
	if err != nil {
		return err
	}
	if global == "" {
		if err := m.SetGlobalVersion(rt, exact); err != nil {
			return err
		}
		m.printf("Set %s@%s as the global default\n", rt, exact)
		return nil
	}
	m.printf("Use it here: xpm env use %s@%s\n", rt, exact)
	return nil
}

// removeStale deletes leftovers of interrupted installs. Callers hold the
// runtime lock, so no live install owns them.
func removeStale(rtDir string) {
	entries, err := os.ReadDir(rtDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			_ = os.RemoveAll(filepath.Join(rtDir, e.Name()))
		}
	}
}

// verifyInstallation checks that every expected binary exists, is not a
// directory and (on Unix) is executable.
func verifyInstallation(dir string, binaryPaths []string) error {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not installed", dir)
	}
	for _, rel := range binaryPaths {
		fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("expected binary %s is missing", rel)
		}
		if fi.IsDir() {
			return fmt.Errorf("expected binary %s is a directory", rel)
		}
		if runtime.GOOS != "windows" && fi.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("expected binary %s is not executable", rel)
		}
	}
	return nil
}
```

- [ ] **Step 6: Output writer, ctx-aware `ListRemote`, `Remover`**

`internal/env/manager.go`:

```diff
--- a/internal/env/manager.go
+++ b/internal/env/manager.go
@@ -3,6 +3,7 @@ package env
 
 import (
 	"fmt"
+	"io"
 	"os"
 	"path/filepath"
 	"strings"
@@ -17,6 +18,7 @@ type Manager struct {
 	runtimesPath string
 	shimsPath    string
 	activePath   string
+	out          io.Writer // progress and status messages
 }
 
 // NewManager creates a new environment manager.
@@ -42,6 +44,7 @@ func NewManager(cfg config.Config) (*Manager, error) {
 		runtimesPath: filepath.Join(envPath, "runtimes"),
 		shimsPath:    filepath.Join(envPath, "shims"),
 		activePath:   filepath.Join(envPath, "active.json"),
+		out:          os.Stdout,
 	}
 
 	if err := m.EnsureDirs(); err != nil {
@@ -51,6 +54,9 @@ func NewManager(cfg config.Config) (*Manager, error) {
 	return m, nil
 }
 
+// SetOutput redirects progress and status messages.
+func (m *Manager) SetOutput(w io.Writer) { m.out = w }
+
 // GetEnvPath returns the environment root path.
 func (m *Manager) GetEnvPath() string {
 	return m.envPath
@@ -80,3 +86,7 @@ func (m *Manager) EnsureDirs() error {
 	}
 	return nil
 }
+
+func (m *Manager) printf(format string, args ...any) {
+	_, _ = fmt.Fprintf(m.out, format, args...)
+}
```

`internal/env/remote.go` (whole file):

```go
package env

import (
	"context"
	"fmt"
	"strings"
)

// ListRemote returns the runtime's available versions, newest first.
func ListRemote(ctx context.Context, runtime string) ([]string, error) {
	installer, err := GetInstaller(runtime)
	if err != nil {
		return nil, err
	}
	versions, err := installer.ListRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote versions: %w", err)
	}
	SortVersionsDesc(versions)
	return versions, nil
}

// FormatRemote formats remote versions (newest first) for display.
func FormatRemote(runtime string, versions []string) string {
	if len(versions) == 0 {
		return fmt.Sprintf("No versions available for %s.\n", runtime)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Available %s versions:\n", runtime)
	shown := versions
	if len(shown) > 20 {
		shown = shown[:20]
		fmt.Fprintf(&b, "(showing the newest 20 of %d versions)\n\n", len(versions))
	}
	for _, v := range shown {
		fmt.Fprintf(&b, "  %s\n", v)
	}
	return b.String()
}
```

`internal/env/remove.go`:

```diff
--- a/internal/env/remove.go
+++ b/internal/env/remove.go
@@ -11,7 +11,7 @@ import (
 
 // RemoveVersion removes an installed version. It refuses the version that is
 // active here or set as the global default.
-func RemoveVersion(_ context.Context, m *Manager, runtime, version string) error {
+func RemoveVersion(ctx context.Context, m *Manager, runtime, version string) error {
 	dir, err := m.versionDir(runtime, version)
 	if err != nil {
 		return err
@@ -27,6 +27,13 @@ func RemoveVersion(_ context.Context, m *Manager, runtime, version string) error
 			return fmt.Errorf("cannot remove %s@%s: it is the global default (set in %s)\nSwitch first: xpm env use --global %s@<other-version>", runtime, version, m.activePath, runtime)
 		}
 	}
+	if inst, err := GetInstaller(runtime); err == nil {
+		if r, ok := inst.(Remover); ok {
+			if err := r.Remove(ctx, version, dir, m.envPath); err != nil {
+				return fmt.Errorf("remove %s@%s: %w", runtime, version, err)
+			}
+		}
+	}
 	// Rename first so the version disappears atomically, then delete.
 	trash := filepath.Join(filepath.Dir(dir), ".tmp-old-"+strconv.FormatInt(time.Now().UnixNano(), 36))
 	if err := os.Rename(dir, trash); err != nil {
```

`internal/env/shims.go` (old `CreateShims`: binary names no longer depend on a version):

```diff
--- a/internal/env/shims.go
+++ b/internal/env/shims.go
@@ -34,15 +34,7 @@ func CreateShims(manager *Manager) error {
 			continue
 		}
 
-		// Get active version to determine binary paths
-		active, err := manager.ActiveVersion(runtime)
-		if err != nil {
-			continue
-		}
-		version := active.Version
-
-		versionPath := filepath.Join(runtimesPath, runtime, version)
-		binaryPaths := installer.BinaryPaths(version, versionPath)
+		binaryPaths := installer.BinaryPaths()
 
 		// Extract binary names
 		var binaries []string
```

- [ ] **Step 7: Adapt every installer to the v2 signatures (mechanical; behaviour fixes come in Tasks 6–11)**

The rule, applied to each of `bun.go`, `deno.go`, `go.go`, `java.go`, `node.go`, `php.go`, `python.go`, `rust.go` in `internal/env/runtimes/`:
1. `ListRemote() (...)` becomes `ListRemote(_ context.Context) (...)`; internal calls `x.ListRemote()` become `x.ListRemote(context.Background())`.
2. The old `Install(version string, dest string) error` is renamed `install`, and a v2 `Install(_ context.Context, req env.InstallRequest) error` calls `install(req.Version, req.Dest)` then the existing `PostInstall` (the engine no longer calls `PostInstall`).
3. `BinaryPaths(version, dest string)` becomes `BinaryPaths()`; node's `PostInstall` calls `n.BinaryPaths()`.
4. `node.go` also gets `LatestLTS(ctx)` (wrapping `GetLTSVersion`) so `node@lts` resolves.
5. Add `"context"` to the imports.

The exact diffs:

`internal/env/runtimes/bun.go`:

```diff
--- a/internal/env/runtimes/bun.go
+++ b/internal/env/runtimes/bun.go
@@ -1,6 +1,7 @@
 package runtimes
 
 import (
+	"context"
 	"encoding/json"
 	"fmt"
 	"io"
@@ -26,7 +27,7 @@ func (b *BunInstaller) Name() string {
 }
 
 // ListRemote fetches available Bun versions from GitHub releases.
-func (b *BunInstaller) ListRemote() ([]string, error) {
+func (b *BunInstaller) ListRemote(_ context.Context) ([]string, error) {
 	resp, err := http.Get("https://api.github.com/repos/oven-sh/bun/releases")
 	if err != nil {
 		return nil, err
@@ -58,8 +59,17 @@ func (b *BunInstaller) ValidateVersion(version string) error {
 	return nil
 }
 
+// Install adapts the v1 installer to the v2 interface; the installer
+// rewrite replaces it.
+func (b *BunInstaller) Install(_ context.Context, req env.InstallRequest) error {
+	if err := b.install(req.Version, req.Dest); err != nil {
+		return err
+	}
+	return b.PostInstall(req.Version, req.Dest)
+}
+
 // Install downloads and installs a Bun version.
-func (b *BunInstaller) Install(version string, dest string) error {
+func (b *BunInstaller) install(version string, dest string) error {
 	// Determine platform
 	goos := runtime.GOOS
 	goarch := runtime.GOARCH
@@ -161,7 +171,7 @@ func (b *BunInstaller) PostInstall(version, dest string) error {
 }
 
 // BinaryPaths returns the paths to Bun binaries.
-func (b *BunInstaller) BinaryPaths(version, dest string) []string {
+func (b *BunInstaller) BinaryPaths() []string {
 	if runtime.GOOS == "windows" {
 		return []string{"bin\\bun.exe"}
 	}
```

`internal/env/runtimes/deno.go`:

```diff
--- a/internal/env/runtimes/deno.go
+++ b/internal/env/runtimes/deno.go
@@ -1,6 +1,7 @@
 package runtimes
 
 import (
+	"context"
 	"encoding/json"
 	"fmt"
 	"io"
@@ -26,7 +27,7 @@ func (d *DenoInstaller) Name() string {
 }
 
 // ListRemote fetches available Deno versions from GitHub releases.
-func (d *DenoInstaller) ListRemote() ([]string, error) {
+func (d *DenoInstaller) ListRemote(_ context.Context) ([]string, error) {
 	resp, err := http.Get("https://api.github.com/repos/denoland/deno/releases")
 	if err != nil {
 		return nil, err
@@ -58,8 +59,17 @@ func (d *DenoInstaller) ValidateVersion(version string) error {
 	return nil
 }
 
+// Install adapts the v1 installer to the v2 interface; the installer
+// rewrite replaces it.
+func (d *DenoInstaller) Install(_ context.Context, req env.InstallRequest) error {
+	if err := d.install(req.Version, req.Dest); err != nil {
+		return err
+	}
+	return d.PostInstall(req.Version, req.Dest)
+}
+
 // Install downloads and installs a Deno version.
-func (d *DenoInstaller) Install(version string, dest string) error {
+func (d *DenoInstaller) install(version string, dest string) error {
 	// Determine platform
 	goos := runtime.GOOS
 	goarch := runtime.GOARCH
@@ -154,7 +164,7 @@ func (d *DenoInstaller) PostInstall(version, dest string) error {
 }
 
 // BinaryPaths returns the paths to Deno binaries.
-func (d *DenoInstaller) BinaryPaths(version, dest string) []string {
+func (d *DenoInstaller) BinaryPaths() []string {
 	if runtime.GOOS == "windows" {
 		return []string{"bin\\deno.exe"}
 	}
```

`internal/env/runtimes/go.go`:

```diff
--- a/internal/env/runtimes/go.go
+++ b/internal/env/runtimes/go.go
@@ -25,7 +25,7 @@ func (g *GoInstaller) Name() string {
 }
 
 // ListRemote fetches available stable Go versions (newest first).
-func (g *GoInstaller) ListRemote() ([]string, error) {
+func (g *GoInstaller) ListRemote(_ context.Context) ([]string, error) {
 	data, err := fetchSmall(context.TODO(), goDLURL+"/?mode=json&include=all")
 	if err != nil {
 		return nil, err
@@ -77,7 +77,7 @@ func (g *GoInstaller) ValidateVersion(version string) error {
 
 // GetLatestVersion returns the latest Go version.
 func (g *GoInstaller) GetLatestVersion() (string, error) {
-	versions, err := g.ListRemote()
+	versions, err := g.ListRemote(context.Background())
 	if err != nil {
 		return "", err
 	}
@@ -88,8 +88,17 @@ func (g *GoInstaller) GetLatestVersion() (string, error) {
 	return versions[0], nil
 }
 
+// Install adapts the v1 installer to the v2 interface; the installer
+// rewrite replaces it.
+func (g *GoInstaller) Install(_ context.Context, req env.InstallRequest) error {
+	if err := g.install(req.Version, req.Dest); err != nil {
+		return err
+	}
+	return g.PostInstall(req.Version, req.Dest)
+}
+
 // Install downloads and installs a Go version.
-func (g *GoInstaller) Install(version string, dest string) error {
+func (g *GoInstaller) install(version string, dest string) error {
 	// Handle special aliases
 	if version == "latest" {
 		latest, err := g.GetLatestVersion()
@@ -150,7 +159,7 @@ func (g *GoInstaller) PostInstall(version, dest string) error {
 }
 
 // BinaryPaths returns the paths to Go binaries.
-func (g *GoInstaller) BinaryPaths(version, dest string) []string {
+func (g *GoInstaller) BinaryPaths() []string {
 	return []string{"bin/go"}
 }
 
```

`internal/env/runtimes/java.go`:

```diff
--- a/internal/env/runtimes/java.go
+++ b/internal/env/runtimes/java.go
@@ -1,6 +1,7 @@
 package runtimes
 
 import (
+	"context"
 	"encoding/json"
 	"fmt"
 	"io"
@@ -26,7 +27,7 @@ func (j *JavaInstaller) Name() string {
 }
 
 // ListRemote fetches available Java versions from Adoptium API.
-func (j *JavaInstaller) ListRemote() ([]string, error) {
+func (j *JavaInstaller) ListRemote(_ context.Context) ([]string, error) {
 	// Use Adoptium API
 	url := "https://api.adoptium.net/v3/info/available_releases"
 	resp, err := http.Get(url)
@@ -66,8 +67,17 @@ func (j *JavaInstaller) ValidateVersion(version string) error {
 	return nil
 }
 
+// Install adapts the v1 installer to the v2 interface; the installer
+// rewrite replaces it.
+func (j *JavaInstaller) Install(_ context.Context, req env.InstallRequest) error {
+	if err := j.install(req.Version, req.Dest); err != nil {
+		return err
+	}
+	return j.PostInstall(req.Version, req.Dest)
+}
+
 // Install downloads and installs a Java version.
-func (j *JavaInstaller) Install(version string, dest string) error {
+func (j *JavaInstaller) install(version string, dest string) error {
 	// Determine platform
 	goos := runtime.GOOS
 	goarch := runtime.GOARCH
@@ -175,7 +185,7 @@ func (j *JavaInstaller) PostInstall(version, dest string) error {
 }
 
 // BinaryPaths returns the paths to Java binaries.
-func (j *JavaInstaller) BinaryPaths(version, dest string) []string {
+func (j *JavaInstaller) BinaryPaths() []string {
 	if runtime.GOOS == "windows" {
 		return []string{"bin\\java.exe", "bin\\javac.exe", "bin\\keytool.exe"}
 	}
```

`internal/env/runtimes/node.go`:

```diff
--- a/internal/env/runtimes/node.go
+++ b/internal/env/runtimes/node.go
@@ -32,7 +32,7 @@ type NodeRelease struct {
 }
 
 // ListRemote fetches available Node.js versions.
-func (n *NodeInstaller) ListRemote() ([]string, error) {
+func (n *NodeInstaller) ListRemote(_ context.Context) ([]string, error) {
 	resp, err := http.Get("https://nodejs.org/dist/index.json")
 	if err != nil {
 		return nil, err
@@ -56,7 +56,7 @@ func (n *NodeInstaller) ListRemote() ([]string, error) {
 
 // GetLatestVersion returns the latest current version.
 func (n *NodeInstaller) GetLatestVersion() (string, error) {
-	versions, err := n.ListRemote()
+	versions, err := n.ListRemote(context.Background())
 	if err != nil {
 		return "", err
 	}
@@ -67,6 +67,11 @@ func (n *NodeInstaller) GetLatestVersion() (string, error) {
 	return versions[0], nil
 }
 
+// LatestLTS implements env.LTSResolver.
+func (n *NodeInstaller) LatestLTS(_ context.Context) (string, error) {
+	return n.GetLTSVersion()
+}
+
 // GetLTSVersion returns the latest LTS version.
 func (n *NodeInstaller) GetLTSVersion() (string, error) {
 	resp, err := http.Get("https://nodejs.org/dist/index.json")
@@ -111,7 +116,7 @@ func (n *NodeInstaller) resolveVersion(version string) (string, error) {
 	}
 
 	// Fetch available versions
-	versions, err := n.ListRemote()
+	versions, err := n.ListRemote(context.Background())
 	if err != nil {
 		return "", err
 	}
@@ -148,9 +153,18 @@ func (n *NodeInstaller) ValidateVersion(version string) error {
 	return nil
 }
 
+// Install adapts the v1 installer to the v2 interface; the installer
+// rewrite replaces it.
+func (n *NodeInstaller) Install(_ context.Context, req env.InstallRequest) error {
+	if err := n.install(req.Version, req.Dest); err != nil {
+		return err
+	}
+	return n.PostInstall(req.Version, req.Dest)
+}
+
 // Install downloads and installs a Node.js version.
 // Returns the resolved version and any alias used.
-func (n *NodeInstaller) Install(version string, dest string) error {
+func (n *NodeInstaller) install(version string, dest string) error {
 	return n.InstallWithAlias(version, dest, "")
 }
 
@@ -375,7 +389,7 @@ func (n *NodeInstaller) InstallWithAlias(version string, dest string, alias stri
 // PostInstall performs post-installation setup.
 func (n *NodeInstaller) PostInstall(version, dest string) error {
 	// Ensure binaries are executable
-	binaryPaths := n.BinaryPaths(version, dest)
+	binaryPaths := n.BinaryPaths()
 	for _, relPath := range binaryPaths {
 		fullPath := filepath.Join(dest, relPath)
 		if info, err := os.Stat(fullPath); err == nil {
@@ -389,7 +403,7 @@ func (n *NodeInstaller) PostInstall(version, dest string) error {
 }
 
 // BinaryPaths returns the paths to Node.js binaries.
-func (n *NodeInstaller) BinaryPaths(version, dest string) []string {
+func (n *NodeInstaller) BinaryPaths() []string {
 	if runtime.GOOS == "windows" {
 		return []string{"node.exe", "npm.cmd", "npx.cmd"}
 	}
```

`internal/env/runtimes/php.go`:

```diff
--- a/internal/env/runtimes/php.go
+++ b/internal/env/runtimes/php.go
@@ -1,6 +1,7 @@
 package runtimes
 
 import (
+	"context"
 	"encoding/json"
 	"fmt"
 	"net/http"
@@ -26,7 +27,7 @@ func (p *PHPInstaller) Name() string {
 }
 
 // ListRemote fetches available PHP versions.
-func (p *PHPInstaller) ListRemote() ([]string, error) {
+func (p *PHPInstaller) ListRemote(_ context.Context) ([]string, error) {
 	// PHP.net releases API
 	resp, err := http.Get("https://www.php.net/releases/index.php?json&max=100")
 	if err != nil {
@@ -116,7 +117,7 @@ func (p *PHPInstaller) ValidateVersion(version string) error {
 
 // GetLatestVersion returns the latest PHP version.
 func (p *PHPInstaller) GetLatestVersion() (string, error) {
-	versions, err := p.ListRemote()
+	versions, err := p.ListRemote(context.Background())
 	if err != nil {
 		return "", err
 	}
@@ -127,8 +128,17 @@ func (p *PHPInstaller) GetLatestVersion() (string, error) {
 	return versions[0], nil
 }
 
+// Install adapts the v1 installer to the v2 interface; the installer
+// rewrite replaces it.
+func (p *PHPInstaller) Install(_ context.Context, req env.InstallRequest) error {
+	if err := p.install(req.Version, req.Dest); err != nil {
+		return err
+	}
+	return p.PostInstall(req.Version, req.Dest)
+}
+
 // Install downloads and installs a PHP version.
-func (p *PHPInstaller) Install(version string, dest string) error {
+func (p *PHPInstaller) install(version string, dest string) error {
 	// Handle special aliases
 	if version == "latest" {
 		latest, err := p.GetLatestVersion()
@@ -492,7 +502,7 @@ exec "%s" "$@"
 }
 
 // BinaryPaths returns the paths to PHP binaries.
-func (p *PHPInstaller) BinaryPaths(version, dest string) []string {
+func (p *PHPInstaller) BinaryPaths() []string {
 	if runtime.GOOS == "windows" {
 		return []string{"php.exe"}
 	}
```

`internal/env/runtimes/python.go`:

```diff
--- a/internal/env/runtimes/python.go
+++ b/internal/env/runtimes/python.go
@@ -1,6 +1,7 @@
 package runtimes
 
 import (
+	"context"
 	"fmt"
 	"io"
 	"net/http"
@@ -25,7 +26,7 @@ func (p *PythonInstaller) Name() string {
 }
 
 // ListRemote fetches available Python versions from pyenv mirror.
-func (p *PythonInstaller) ListRemote() ([]string, error) {
+func (p *PythonInstaller) ListRemote(_ context.Context) ([]string, error) {
 	// Use pyenv's version list API or python.org
 	// For simplicity, we'll fetch from a known source
 	// In production, this could use python.org/downloads API
@@ -107,7 +108,7 @@ func (p *PythonInstaller) ValidateVersion(version string) error {
 
 // GetLatestVersion returns the latest Python version.
 func (p *PythonInstaller) GetLatestVersion() (string, error) {
-	versions, err := p.ListRemote()
+	versions, err := p.ListRemote(context.Background())
 	if err != nil {
 		return "", err
 	}
@@ -118,8 +119,17 @@ func (p *PythonInstaller) GetLatestVersion() (string, error) {
 	return versions[0], nil
 }
 
+// Install adapts the v1 installer to the v2 interface; the installer
+// rewrite replaces it.
+func (p *PythonInstaller) Install(_ context.Context, req env.InstallRequest) error {
+	if err := p.install(req.Version, req.Dest); err != nil {
+		return err
+	}
+	return p.PostInstall(req.Version, req.Dest)
+}
+
 // Install downloads and installs a Python version.
-func (p *PythonInstaller) Install(version string, dest string) error {
+func (p *PythonInstaller) install(version string, dest string) error {
 	// Handle special aliases
 	if version == "latest" {
 		latest, err := p.GetLatestVersion()
@@ -229,7 +239,7 @@ func (p *PythonInstaller) PostInstall(version, dest string) error {
 }
 
 // BinaryPaths returns the paths to Python binaries.
-func (p *PythonInstaller) BinaryPaths(version, dest string) []string {
+func (p *PythonInstaller) BinaryPaths() []string {
 	if runtime.GOOS == "windows" {
 		return []string{"python.exe", "python3.exe", "Scripts\\pip.exe"}
 	}
```

`internal/env/runtimes/rust.go`:

```diff
--- a/internal/env/runtimes/rust.go
+++ b/internal/env/runtimes/rust.go
@@ -1,6 +1,7 @@
 package runtimes
 
 import (
+	"context"
 	"fmt"
 	"os"
 	"os/exec"
@@ -24,7 +25,7 @@ func (r *RustInstaller) Name() string {
 }
 
 // ListRemote fetches available Rust versions.
-func (r *RustInstaller) ListRemote() ([]string, error) {
+func (r *RustInstaller) ListRemote(_ context.Context) ([]string, error) {
 	// Use rustup to list available toolchains
 	if rustupExists() {
 		cmd := exec.Command("rustup", "toolchain", "list", "--available")
@@ -69,10 +70,19 @@ func (r *RustInstaller) ValidateVersion(version string) error {
 	return nil
 }
 
+// Install adapts the v1 installer to the v2 interface; the installer
+// rewrite replaces it.
+func (r *RustInstaller) Install(_ context.Context, req env.InstallRequest) error {
+	if err := r.install(req.Version, req.Dest); err != nil {
+		return err
+	}
+	return r.PostInstall(req.Version, req.Dest)
+}
+
 // Install installs a Rust toolchain via the user's rustup. xpm deliberately
 // does not bootstrap rustup itself: that meant running an unverified download
 // that also rewrote ~/.cargo and shell profiles.
-func (r *RustInstaller) Install(version string, dest string) error {
+func (r *RustInstaller) install(version string, dest string) error {
 	if !rustupExists() {
 		return fmt.Errorf("rust needs rustup: install it from https://rustup.rs, then re-run `xpm env install rust@%s`", version)
 	}
@@ -113,7 +123,7 @@ func (r *RustInstaller) PostInstall(version, dest string) error {
 }
 
 // BinaryPaths returns the paths to Rust binaries.
-func (r *RustInstaller) BinaryPaths(version, dest string) []string {
+func (r *RustInstaller) BinaryPaths() []string {
 	if runtime.GOOS == "windows" {
 		return []string{"bin\\rustc.exe", "bin\\cargo.exe"}
 	}
```

`internal/env/runtimes/rust_test.go`:

```diff
--- a/internal/env/runtimes/rust_test.go
+++ b/internal/env/runtimes/rust_test.go
@@ -1,17 +1,20 @@
 package runtimes
 
 import (
+	"context"
 	"os"
 	"path/filepath"
 	"strings"
 	"testing"
+
+	"github.com/crenspire/xpm/internal/env"
 )
 
 func TestRustInstallWithoutRustupFailsSafely(t *testing.T) {
 	t.Setenv("PATH", t.TempDir()) // no rustup anywhere
 	dest := filepath.Join(t.TempDir(), "rust", "1.75.0")
 
-	err := (&RustInstaller{}).Install("1.75.0", dest)
+	err := (&RustInstaller{}).Install(context.Background(), env.InstallRequest{Version: "1.75.0", Dest: dest})
 	if err == nil || !strings.Contains(err.Error(), "rustup.rs") {
 		t.Fatalf("want an error pointing at https://rustup.rs, got %v", err)
 	}
```

- [ ] **Step 8: CLI install with Ctrl-C handling; ls-remote with ctx**

`internal/cli/env_cmd.go` (Task 13 rewrites the file; this keeps it building and gives install its exit code 130):

```diff
--- a/internal/cli/env_cmd.go
+++ b/internal/cli/env_cmd.go
@@ -2,9 +2,12 @@ package cli
 
 import (
 	"context"
+	"errors"
 	"fmt"
 	"os"
+	"os/signal"
 	"strings"
+	"syscall"
 
 	"github.com/crenspire/xpm/internal/config"
 	"github.com/crenspire/xpm/internal/env"
@@ -87,16 +90,19 @@ func cmdEnvInstall(manager *env.Manager, args []string) int {
 		return 1
 	}
 
-	// Detect if an alias was used
-	var alias string
-	if version == "latest" || version == "lts" {
-		alias = version
-	}
-
-	if err := env.InstallRuntimeWithAlias(manager, runtime, version, alias); err != nil {
+	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
+	defer stop()
+	if _, err := env.InstallRuntime(ctx, manager, runtime, version); err != nil {
+		if errors.Is(err, context.Canceled) {
+			fmt.Fprintln(os.Stderr, "Cancelled; nothing was installed.")
+			return 130
+		}
 		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
 		return 1
 	}
+	if err := env.CreateShims(manager); err != nil {
+		fmt.Fprintf(os.Stderr, "warning: could not update shims: %v\n", err)
+	}
 
 	return 0
 }
@@ -153,7 +159,7 @@ func cmdEnvListRemote(manager *env.Manager, args []string) int {
 	}
 
 	runtime := args[0]
-	versions, err := env.ListRemote(manager, runtime)
+	versions, err := env.ListRemote(context.Background(), runtime)
 	if err != nil {
 		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
 		return 1
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/env/ -run '.' -count=1`
Expected: `ok`.

- [ ] **Step 10: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 11: Commit (only this task's files)**

```bash
git add internal/env/runtime.go \
  internal/env/install.go \
  internal/env/install_test.go \
  internal/env/lock_unix.go \
  internal/env/lock_other.go \
  internal/env/manager.go \
  internal/env/remote.go \
  internal/env/remove.go \
  internal/env/shims.go \
  internal/env/fake_test.go \
  internal/env/validate_test.go \
  internal/env/runtimes/bun.go \
  internal/env/runtimes/deno.go \
  internal/env/runtimes/go.go \
  internal/env/runtimes/java.go \
  internal/env/runtimes/node.go \
  internal/env/runtimes/php.go \
  internal/env/runtimes/python.go \
  internal/env/runtimes/rust.go \
  internal/env/runtimes/rust_test.go \
  internal/cli/env_cmd.go
git commit -m "env: installer interface v2 and atomic, locked installs"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 5: Busybox shims and dispatch from `main` (R2, R3)

**Files:**
- Modify (whole file): `internal/env/shims.go` (the Go-template shim compiler is deleted), `internal/env/manager.go`, `cmd/xpm/main.go`, `internal/env/validate_test.go`
- Create: `internal/env/exec_unix.go`, `internal/env/exec_windows.go`, `internal/env/shims_test.go`, `internal/env/path.go` (holds `UpdatePATH`/`CheckPATH` moved verbatim out of `shims.go`; Task 12 rewrites it)
- Modify (hunk): `internal/env/fake_test.go`

**Interfaces:**
- Consumes: Task 3 `ActiveVersion`, `ErrNoVersion`, `ErrNotInstalled`, `InstalledVersions`, `versionDir`; Task 4 `binaryOwner`, `ListRuntimes`, `GetInstaller`.
- Produces:
  - `func ShimName(argv0 string) (string, bool)` (false on Windows, for `xpm`, and for names no installer provides)
  - `func RunShim(name string, args []string) int`
  - `func CreateShims(m *Manager) error` (symlinks to the xpm executable; prunes everything else in the shims dir)
  - `func (m *Manager) SetExecutable(path string)`; package seams `osExecutable = os.Executable`, `execFn = execProcess`, `shimStderr io.Writer = os.Stderr`
  - `func findSystemBinary(name, pathEnv, shimsDir, self string) (string, bool)`, `func shimEnv(environ []string, binDir, name string) ([]string, error)`, `func realPath(p string) string`
  - `const shimDepthVar = "XPM_SHIM_DEPTH"`, `maxShimDepth = 4`
  - exit codes: 127 (not installed / no system binary / binary not provided), 1 (invalid entry, recursion), 126 (exec failed)

- [ ] **Step 1: Write the failing tests**

`internal/env/shims_test.go` (exec is never real: `execFn` records the call):

```go
package env

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("xpm env shims are Unix-only")
	}
}

func TestShimName(t *testing.T) {
	skipOnWindows(t)
	useFake(t, &fakeInstaller{})
	cases := map[string]bool{
		"/usr/local/xpm/shims/fakebin": true,
		"fakebin2":                     true,
		"fakebin.exe":                  true,
		"xpm":                          false,
		"/usr/local/bin/xpm":           false,
		"unknown":                      false,
		"":                             false,
	}
	for argv0, want := range cases {
		if _, ok := ShimName(argv0); ok != want {
			t.Errorf("ShimName(%q) ok = %v, want %v", argv0, ok, want)
		}
	}
}

func TestCreateShimsLinksEveryBinaryAndPrunes(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	useFake(t, &fakeInstaller{})
	installFake(t, m, "1.0.0", "")
	shims := m.GetShimsPath()
	for _, junk := range []string{"node", "node.go", "stale.tmp-abc"} {
		if err := os.WriteFile(filepath.Join(shims, junk), []byte("old compiled shim"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(t.TempDir(), "xpm")
	m.SetExecutable(exe)
	if err := CreateShims(m); err != nil {
		t.Fatal(err)
	}
	if err := CreateShims(m); err != nil { // idempotent
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(shims)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "fakebin,fakebin2" {
		t.Fatalf("shims = %v", names)
	}
	if target, _ := os.Readlink(filepath.Join(shims, "fakebin")); target != exe {
		t.Fatalf("fakebin -> %q, want %q", target, exe)
	}

	upgraded := filepath.Join(t.TempDir(), "xpm-new")
	m.SetExecutable(upgraded)
	if err := CreateShims(m); err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(filepath.Join(shims, "fakebin2")); target != upgraded {
		t.Fatalf("not re-pointed: %q", target)
	}
}

// shimRun sets up HOME-based config (default env root ~/.xpm/env), a fake
// xpm executable and a recording execFn.
type shimRun struct {
	m      *Manager
	stderr bytes.Buffer
	path   string
	argv   []string
	env    []string
	calls  int
}

func newShimRun(t *testing.T) *shimRun {
	t.Helper()
	skipOnWindows(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(shimDepthVar, "")
	chdir(t, t.TempDir())
	useFake(t, &fakeInstaller{})
	exe := filepath.Join(t.TempDir(), "xpm")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe, oldExec, oldErr := osExecutable, execFn, shimStderr
	t.Cleanup(func() { osExecutable, execFn, shimStderr = oldExe, oldExec, oldErr })
	osExecutable = func() (string, error) { return exe, nil }

	r := &shimRun{}
	shimStderr = &r.stderr
	execFn = func(path string, argv, env []string) error {
		r.calls++
		r.path, r.argv, r.env = path, argv, env
		return nil
	}
	m, err := NewManager(configWithDefaults())
	if err != nil {
		t.Fatal(err)
	}
	m.SetOutput(&bytes.Buffer{})
	r.m = m
	return r
}

func (r *shimRun) envVar(key string) string {
	for _, kv := range r.env {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			return v
		}
	}
	return ""
}

func TestRunShimExecsTheActiveVersion(t *testing.T) {
	r := newShimRun(t)
	dir := installFake(t, r.m, "1.2.3", "")
	if err := r.m.SetGlobalVersion(fakeRT, "1.2"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin")

	if code := RunShim("fakebin", []string{"-v", "x y"}); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr.String())
	}
	bin := filepath.Join(dir, "bin", "fakebin")
	if r.path != bin || strings.Join(r.argv, "|") != bin+"|-v|x y" {
		t.Fatalf("exec %q %q", r.path, r.argv)
	}
	if got := r.envVar("PATH"); got != filepath.Join(dir, "bin")+string(os.PathListSeparator)+"/usr/bin" {
		t.Fatalf("PATH = %q", got)
	}
	if got := r.envVar(shimDepthVar); got != "1" {
		t.Fatalf("depth = %q", got)
	}
}

func TestRunShimConfiguredButNotInstalled(t *testing.T) {
	r := newShimRun(t)
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sys := t.TempDir()
	if err := os.WriteFile(filepath.Join(sys, "fakebin"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", sys)
	if code := RunShim("fakebin", nil); code != 127 || r.calls != 0 {
		t.Fatalf("exit %d calls %d; an explicit pin must not fall back", code, r.calls)
	}
	cwd, _ := os.Getwd()
	want := "xpm: xpmfake@9.9.9 is not installed (set in " + filepath.Join(cwd, ".xpm-env") + ")\nRun: xpm env install xpmfake@9.9.9\n"
	if r.stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", r.stderr.String(), want)
	}
}

func TestRunShimInvalidEntryFailsClosed(t *testing.T) {
	r := newShimRun(t)
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=../../evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := RunShim("fakebin", nil); code != 1 || r.calls != 0 {
		t.Fatalf("exit %d calls %d", code, r.calls)
	}
	if !strings.HasPrefix(r.stderr.String(), `xpm: invalid xpmfake version "../../evil" in `) {
		t.Fatalf("stderr = %q", r.stderr.String())
	}
}

func TestRunShimFallsBackToSystemBinary(t *testing.T) {
	r := newShimRun(t)
	exe, _ := osExecutable()
	selfDir := t.TempDir() // a PATH dir whose fakebin is a link back to xpm
	if err := os.Symlink(exe, filepath.Join(selfDir, "fakebin")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(r.m.GetShimsPath(), "fakebin")); err != nil {
		t.Fatal(err)
	}
	sys := t.TempDir()
	sysBin := filepath.Join(sys, "fakebin")
	if err := os.WriteFile(sysBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sep := string(os.PathListSeparator)
	t.Setenv("PATH", r.m.GetShimsPath()+sep+selfDir+sep+sys)

	if code := RunShim("fakebin", []string{"a"}); code != 0 {
		t.Fatalf("exit %d: %s", code, r.stderr.String())
	}
	if r.path != sysBin || r.envVar("PATH") != os.Getenv("PATH") {
		t.Fatalf("exec %q with PATH %q", r.path, r.envVar("PATH"))
	}
}

func TestRunShimNoVersionNoSystemBinary(t *testing.T) {
	r := newShimRun(t)
	t.Setenv("PATH", t.TempDir())
	if code := RunShim("fakebin", nil); code != 127 {
		t.Fatalf("exit %d", code)
	}
	want := "xpm: no xpmfake version is configured and no system fakebin was found on PATH\nInstall one: xpm env install xpmfake@<version>\n"
	if r.stderr.String() != want {
		t.Fatalf("stderr = %q", r.stderr.String())
	}
}

func TestRunShimDisabledConfigUsesSystem(t *testing.T) {
	r := newShimRun(t)
	installFake(t, r.m, "1.0.0", "")
	if err := r.m.SetGlobalVersion(fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	cfgDir := filepath.Join(home, ".config", "xpm")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "xpmrc.json"), []byte(`{"env":{"enabled":false}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sys := t.TempDir()
	if err := os.WriteFile(filepath.Join(sys, "fakebin"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", sys)
	if code := RunShim("fakebin", nil); code != 0 || r.path != filepath.Join(sys, "fakebin") {
		t.Fatalf("exit %d exec %q", code, r.path)
	}
}

func TestRunShimRecursionGuard(t *testing.T) {
	r := newShimRun(t)
	installFake(t, r.m, "1.0.0", "")
	if err := r.m.SetGlobalVersion(fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(shimDepthVar, "4")
	if code := RunShim("fakebin", nil); code != 1 || r.calls != 0 {
		t.Fatalf("exit %d calls %d", code, r.calls)
	}
	if r.stderr.String() != "xpm: shim recursion detected for fakebin\n" {
		t.Fatalf("stderr = %q", r.stderr.String())
	}
}

func TestRunShimBinaryMissingFromVersion(t *testing.T) {
	r := newShimRun(t)
	dir := installFake(t, r.m, "1.0.0", "")
	if err := os.Remove(filepath.Join(dir, "bin", "fakebin2")); err != nil {
		t.Fatal(err)
	}
	if err := r.m.SetGlobalVersion(fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if code := RunShim("fakebin2", nil); code != 127 || r.stderr.String() != "xpm: fakebin2 is not provided by xpmfake@1.0.0\n" {
		t.Fatalf("exit %d stderr %q", code, r.stderr.String())
	}
}
```

Append to `internal/env/fake_test.go`:

```diff
--- a/internal/env/fake_test.go
+++ b/internal/env/fake_test.go
@@ -7,6 +7,8 @@ import (
 	"path/filepath"
 	"sync"
 	"testing"
+
+	"github.com/crenspire/xpm/internal/config"
 )
 
 // fakeRT is the test-only runtime name; the real installers live in
@@ -97,3 +99,7 @@ func installFake(t *testing.T, m *Manager, version, alias string) string {
 	}
 	return dir
 }
+
+// configWithDefaults is the default config (env enabled, ~/.xpm/env), as
+// RunShim sees it when no config file exists.
+func configWithDefaults() config.Config { return config.Load() }
```

`internal/env/validate_test.go`: delete `TestShimTemplateGuardsVersion` (its code generator is gone; the fail-closed rule is pinned by `TestRunShimInvalidEntryFailsClosed`) and make `isolate` point shims at a fake executable:

```diff
--- a/internal/env/validate_test.go
+++ b/internal/env/validate_test.go
@@ -3,8 +3,6 @@ package env
 import (
 	"context"
 	"errors"
-	"go/parser"
-	"go/token"
 	"io"
 	"os"
 	"path/filepath"
@@ -28,8 +26,8 @@ func chdir(t *testing.T, dir string) {
 }
 
 // isolate points HOME at a temp dir, chdirs into another (the repo root has
-// its own .xpm-env) and returns a Manager rooted in a third. Progress
-// output is discarded.
+// its own .xpm-env) and returns a Manager rooted in a third. Shims link to a
+// fake executable path and progress output is discarded.
 func isolate(t *testing.T) *Manager {
 	t.Helper()
 	t.Setenv("HOME", t.TempDir())
@@ -39,6 +37,7 @@ func isolate(t *testing.T) *Manager {
 	if err != nil {
 		t.Fatal(err)
 	}
+	m.SetExecutable(filepath.Join(t.TempDir(), "xpm"))
 	m.SetOutput(io.Discard)
 	return m
 }
@@ -110,16 +109,3 @@ func TestLocalEnvRejectsPathLikeVersion(t *testing.T) {
 		t.Fatalf("returned path-like version %q", a.Version)
 	}
 }
-
-func TestShimTemplateGuardsVersion(t *testing.T) {
-	code, err := generateShimCode("node", "node", "/opt/xpm env")
-	if err != nil {
-		t.Fatal(err)
-	}
-	if _, err := parser.ParseFile(token.NewFileSet(), "shim.go", code, 0); err != nil {
-		t.Fatalf("generated shim is not valid Go: %v", err)
-	}
-	if !strings.Contains(code, "!safeVersion(version)") {
-		t.Fatal("shim does not validate the resolved version before using it as a path")
-	}
-}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/ -run 'Shim' -count=1`
Expected: build failure: `undefined: ShimName`, `undefined: RunShim`, `undefined: osExecutable`, `m.SetExecutable undefined` ...

- [ ] **Step 3: Move `UpdatePATH` and `CheckPATH` into `internal/env/path.go`**

Cut both functions unchanged from the old `shims.go` into a new `internal/env/path.go` with this header (Task 12 replaces them):

```go
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)
```

- [ ] **Step 4: Replace `internal/env/shims.go`**

```go
package env

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/crenspire/xpm/internal/config"
)

// Seams for tests: never exec a real runtime from a test.
var (
	execFn               = execProcess
	shimStderr io.Writer = os.Stderr
)

const (
	shimDepthVar = "XPM_SHIM_DEPTH"
	maxShimDepth = 4
)

// ShimName reports whether argv0 (as the OS gave it to xpm) names a binary
// that some registered runtime provides, i.e. xpm was started via a shim.
func ShimName(argv0 string) (string, bool) {
	if goruntime.GOOS == "windows" {
		return "", false
	}
	name := strings.TrimSuffix(filepath.Base(argv0), ".exe")
	if name == "" || name == "xpm" || name == "." || name == string(filepath.Separator) {
		return "", false
	}
	if _, _, ok := binaryOwner(name); !ok {
		return "", false
	}
	return name, true
}

// RunShim runs the binary `name` from the version active for the current
// directory, or the system one when no version is configured. It only
// returns on failure (or after a test execFn); the code is the exit status.
func RunShim(name string, args []string) int {
	cfg := config.Load()
	rt, rel, ok := binaryOwner(name)
	if !ok {
		return shimFail(127, "xpm: %s is not provided by any runtime xpm manages", name)
	}
	m, err := NewManager(cfg)
	if err != nil {
		return shimFail(1, "xpm: %v", err)
	}
	if !cfg.Env.Enabled {
		return m.runSystem(rt, name, args)
	}
	a, err := m.ActiveVersion(rt)
	switch {
	case errors.Is(err, ErrNoVersion):
		return m.runSystem(rt, name, args)
	case errors.Is(err, ErrNotInstalled):
		return shimFail(127, "xpm: %s@%s is not installed (set in %s)\nRun: xpm env install %s@%s", rt, a.Version, a.Source, rt, a.Version)
	case err != nil:
		return shimFail(1, "xpm: %v", err)
	}
	dir, err := m.versionDir(rt, a.Version)
	if err != nil {
		return shimFail(1, "xpm: %v", err)
	}
	bin := filepath.Join(dir, filepath.FromSlash(rel))
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		return shimFail(127, "xpm: %s is not provided by %s@%s", name, rt, a.Version)
	}
	env, err := shimEnv(os.Environ(), filepath.Dir(bin), name)
	if err != nil {
		return shimFail(1, "xpm: %v", err)
	}
	return runExec(bin, args, env)
}

// runSystem execs the first `name` on PATH that is neither in the shims dir
// nor a link back to this xpm executable.
func (m *Manager) runSystem(rt, name string, args []string) int {
	bin, ok := findSystemBinary(name, os.Getenv("PATH"), m.shimsPath, m.executable)
	if !ok {
		return shimFail(127, "xpm: no %s version is configured and no system %s was found on PATH\nInstall one: xpm env install %s@<version>", rt, name, rt)
	}
	env, err := shimEnv(os.Environ(), "", name)
	if err != nil {
		return shimFail(1, "xpm: %v", err)
	}
	return runExec(bin, args, env)
}

func runExec(bin string, args, env []string) int {
	argv := append([]string{bin}, args...)
	if err := execFn(bin, argv, env); err != nil {
		return shimFail(126, "xpm: exec %s: %v", bin, err)
	}
	return 0 // only reached with a test execFn
}

func shimFail(code int, format string, args ...any) int {
	_, _ = fmt.Fprintf(shimStderr, format+"\n", args...)
	return code
}

// shimEnv returns environ with XPM_SHIM_DEPTH incremented and, when binDir
// is set, binDir prepended to PATH so nested `#!/usr/bin/env node` calls hit
// the same version directly.
func shimEnv(environ []string, binDir, name string) ([]string, error) {
	depth := 0
	pathVal, hasPath := "", false
	out := make([]string, 0, len(environ)+2)
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case shimDepthVar:
			depth, _ = strconv.Atoi(v)
		case "PATH":
			pathVal, hasPath = v, true
		default:
			out = append(out, kv)
		}
	}
	depth++
	if depth > maxShimDepth {
		return nil, fmt.Errorf("shim recursion detected for %s", name)
	}
	if binDir != "" {
		if hasPath && pathVal != "" {
			pathVal = binDir + string(os.PathListSeparator) + pathVal
		} else {
			pathVal, hasPath = binDir, true
		}
	}
	if hasPath {
		out = append(out, "PATH="+pathVal)
	}
	return append(out, shimDepthVar+"="+strconv.Itoa(depth)), nil
}

// findSystemBinary searches pathEnv for an executable `name`, skipping the
// shims dir and anything that resolves to the xpm executable itself.
func findSystemBinary(name, pathEnv, shimsDir, self string) (string, bool) {
	realShims := realPath(shimsDir)
	realSelf := realPath(self)
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		if filepath.Clean(dir) == filepath.Clean(shimsDir) || realPath(dir) == realShims {
			continue
		}
		cand := filepath.Join(dir, name)
		fi, err := os.Stat(cand)
		if err != nil || fi.IsDir() || fi.Mode().Perm()&0o111 == 0 {
			continue
		}
		if realSelf != "" && realPath(cand) == realSelf {
			continue
		}
		if abs, err := filepath.Abs(cand); err == nil {
			cand = abs
		}
		return cand, true
	}
	return "", false
}

func realPath(p string) string {
	if p == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// CreateShims makes <shims>/<name> a symlink to the xpm executable for every
// binary of every installed runtime, re-pointing existing links, and deletes
// everything else in the shims dir (old compiled shims, stale names).
func CreateShims(m *Manager) error {
	if m.executable == "" {
		return errors.New("cannot determine the path of the xpm executable")
	}
	want := map[string]bool{}
	for _, rt := range ListRuntimes() {
		versions, err := m.InstalledVersions(rt)
		if err != nil || len(versions) == 0 {
			continue
		}
		inst, err := GetInstaller(rt)
		if err != nil {
			continue
		}
		for _, p := range inst.BinaryPaths() {
			want[path.Base(p)] = true
		}
	}
	if err := os.MkdirAll(m.shimsPath, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := m.linkShim(n); err != nil {
			return fmt.Errorf("shim %s: %w", n, err)
		}
	}
	entries, err := os.ReadDir(m.shimsPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !want[e.Name()] {
			if err := os.RemoveAll(filepath.Join(m.shimsPath, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// linkShim atomically points <shims>/<name> at the xpm executable.
func (m *Manager) linkShim(name string) error {
	link := filepath.Join(m.shimsPath, name)
	if cur, err := os.Readlink(link); err == nil && cur == m.executable {
		return nil
	}
	var rnd [6]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return err
	}
	tmp := link + ".tmp-" + hex.EncodeToString(rnd[:])
	if err := os.Symlink(m.executable, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
```

`internal/env/exec_unix.go`:

```go
//go:build !windows

package env

import "syscall"

// execProcess replaces xpm with the runtime binary; argv[0] is its absolute path.
func execProcess(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
```

`internal/env/exec_windows.go`:

```go
//go:build windows

package env

import "errors"

// execProcess is unavailable: xpm env (and its shims) are Unix-only.
func execProcess(string, []string, []string) error {
	return errors.New("xpm shims are not supported on Windows")
}
```

- [ ] **Step 5: The executable shims link to (`internal/env/manager.go`)**

```diff
--- a/internal/env/manager.go
+++ b/internal/env/manager.go
@@ -11,6 +11,10 @@ import (
 	"github.com/crenspire/xpm/internal/config"
 )
 
+// osExecutable finds the running xpm binary; shims link to it. A seam so
+// tests never point shims at the test binary.
+var osExecutable = os.Executable
+
 // Manager manages runtime versions and environment.
 type Manager struct {
 	config       config.Config
@@ -18,6 +22,7 @@ type Manager struct {
 	runtimesPath string
 	shimsPath    string
 	activePath   string
+	executable   string    // what shims link to (os.Executable, not EvalSymlinks'd)
 	out          io.Writer // progress and status messages
 }
 
@@ -38,12 +43,18 @@ func NewManager(cfg config.Config) (*Manager, error) {
 		envPath = filepath.Join(homeDir, envPath[2:])
 	}
 
+	exe, err := osExecutable()
+	if err != nil {
+		exe = ""
+	}
+
 	m := &Manager{
 		config:       cfg,
 		envPath:      envPath,
 		runtimesPath: filepath.Join(envPath, "runtimes"),
 		shimsPath:    filepath.Join(envPath, "shims"),
 		activePath:   filepath.Join(envPath, "active.json"),
+		executable:   exe,
 		out:          os.Stdout,
 	}
 
@@ -54,6 +65,9 @@ func NewManager(cfg config.Config) (*Manager, error) {
 	return m, nil
 }
 
+// SetExecutable changes the path shims link to (tests use a fake binary).
+func (m *Manager) SetExecutable(path string) { m.executable = path }
+
 // SetOutput redirects progress and status messages.
 func (m *Manager) SetOutput(w io.Writer) { m.out = w }
 
```

- [ ] **Step 6: Dispatch in `cmd/xpm/main.go`** (replace the file)

```go
// Package main is the entry point for the xpm (Universal Package Manager) CLI.
//
// XPM provides a unified interface for managing packages across multiple ecosystems
// including npm, pip, composer, cargo, maven, gradle, and go modules.
//
// When started through a runtime shim (a symlink named node, python, go, ...
// pointing at xpm), it runs that runtime's active version instead.
//
// Usage:
//
//	xpm install <package>     Install a package (searches all ecosystems)
//	xpm install               Auto-detect and install project dependencies
//	xpm which <package>       Check which ecosystems have a package
//	xpm doctor                Check installed package managers
//	xpm version               Show version information
//	xpm help                  Show help
package main

import (
	"os"

	"github.com/crenspire/xpm/internal/cli"
	"github.com/crenspire/xpm/internal/env"
	_ "github.com/crenspire/xpm/internal/env/runtimes" // register runtime installers for shim dispatch
)

func main() {
	if name, ok := env.ShimName(os.Args[0]); ok {
		os.Exit(env.RunShim(name, os.Args[1:]))
	}
	os.Exit(cli.Run())
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/env/ -run '.' -count=1`
Expected: `ok` (shim tests skip on Windows).

- [ ] **Step 8: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 9: Commit (only this task's files)**

```bash
git add internal/env/shims.go \
  internal/env/shims_test.go \
  internal/env/exec_unix.go \
  internal/env/exec_windows.go \
  internal/env/path.go \
  internal/env/manager.go \
  internal/env/fake_test.go \
  internal/env/validate_test.go \
  cmd/xpm/main.go
git commit -m "env: busybox shims dispatched from main, with system fallback"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 6: Node.js and Go installers (R10)

**Files:**
- Modify (whole file): `internal/env/runtimes/node.go`, `internal/env/runtimes/go.go`, `internal/env/runtimes/go_test.go`
- Modify (hunks): `internal/env/runtimes/download.go` (`hostOS`, `hostArch`), `internal/env/runtimes/helpers_test.go`
- Create: `internal/env/runtimes/node_test.go`, `internal/env/runtimes/installer_test.go`

**Interfaces:**
- Consumes: Task 2 helpers; Task 4 `env.InstallRequest`, `env.ResolveSpec`, `env.LTSResolver`.
- Produces:
  - vars `hostOS = runtime.GOOS`, `hostArch = runtime.GOARCH` (every installer picks assets from these; tests override them)
  - `func nodeAsset(goos, goarch, version string) (file, dir string, err error)`; `NodeInstaller.LatestLTS(ctx)`; `NodeInstaller.BinaryPaths() = bin/node, bin/npm, bin/npx`
  - `func goAsset(goos, goarch, version string) string`; `func goReleaseChecksum(ctx context.Context, filename string) (string, error)`; `GoInstaller.BinaryPaths() = bin/go, bin/gofmt`
  - test helpers: `setHost(t, goos, goarch)`, `tarGzBytes(t, []tarEntry) []byte`, `shaBytes([]byte) string`, `dir(name) tarEntry`, `link(name, to) tarEntry`, `readFile(dir, rel) (string, error)`, `installInto(t, inst, version) string`, `skipWindows(t)`
  - Deleted: node's `NodeRelease`, `GetLatestVersion`, `GetLTSVersion`, `resolveVersion`, `ValidateVersion`, `InstallWithAlias`, `install`, `PostInstall`, `removeString`, the npm-symlink rebuild and every debug print; go's `ValidateVersion`, `GetLatestVersion`, `install`, `PostInstall`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/env/runtimes/helpers_test.go`:

```diff
--- a/internal/env/runtimes/helpers_test.go
+++ b/internal/env/runtimes/helpers_test.go
@@ -8,6 +8,7 @@ import (
 	"net/http"
 	"net/http/httptest"
 	"os"
+	"path/filepath"
 	"testing"
 )
 
@@ -54,6 +55,23 @@ func setVar[T any](t *testing.T, p *T, v T) {
 	t.Cleanup(func() { *p = old })
 }
 
+// setHost pretends to run on goos/goarch.
+func setHost(t *testing.T, goos, goarch string) {
+	t.Helper()
+	setVar(t, &hostOS, goos)
+	setVar(t, &hostArch, goarch)
+}
+
+// tarGzBytes builds a .tar.gz in memory (see makeTarGz).
+func tarGzBytes(t *testing.T, entries []tarEntry) []byte {
+	t.Helper()
+	data, err := os.ReadFile(makeTarGz(t, entries))
+	if err != nil {
+		t.Fatal(err)
+	}
+	return data
+}
+
 // zipBytes builds a zip in memory; names ending in "/" are directories.
 func zipBytes(t *testing.T, files map[string]string) []byte {
 	t.Helper()
@@ -76,4 +94,13 @@ func zipBytes(t *testing.T, files map[string]string) []byte {
 	return buf.Bytes()
 }
 
+func shaBytes(b []byte) string { return sha(string(b)) }
+
 func reg(name, body string) tarEntry { return tarEntry{name: name, body: body, typ: tar.TypeReg} }
+func dir(name string) tarEntry       { return tarEntry{name: name, typ: tar.TypeDir} }
+func link(name, to string) tarEntry  { return tarEntry{name: name, link: to, typ: tar.TypeSymlink} }
+
+func readFile(dir, rel string) (string, error) {
+	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
+	return string(b), err
+}
```

`internal/env/runtimes/installer_test.go`:

```go
package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

// installInto runs inst.Install into a fresh empty dir and checks that
// every BinaryPaths entry exists and is executable.
func installInto(t *testing.T, inst env.RuntimeInstaller, version string) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "stage")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := inst.Install(context.Background(), env.InstallRequest{Version: version, Dest: dest, Root: root}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	for _, rel := range inst.BinaryPaths() {
		fi, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s missing: %v", rel, err)
		}
		if fi.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s is not executable", rel)
		}
	}
	return dest
}

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("archives with symlinks and exec bits: Unix-only")
	}
}
```

`internal/env/runtimes/node_test.go`:

```go
package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

const nodeIndex = `[
 {"version":"v25.1.0","lts":false},
 {"version":"v24.11.0","lts":"Krypton"},
 {"version":"v22.21.1","lts":"Jod"},
 {"version":"v20.11.0","lts":"Iron"},
 {"version":"v20.9.0","lts":"Iron"}
]`

func TestNodeListRemoteAndLTS(t *testing.T) {
	url, _ := newServer(t, map[string]route{"/index.json": {body: []byte(nodeIndex)}})
	setVar(t, &nodeDistURL, url)
	n := &NodeInstaller{}
	got, err := n.ListRemote(context.Background())
	if err != nil || strings.Join(got, " ") != "25.1.0 24.11.0 22.21.1 20.11.0 20.9.0" {
		t.Fatalf("ListRemote = %v, %v", got, err)
	}
	if v, err := n.LatestLTS(context.Background()); err != nil || v != "24.11.0" {
		t.Fatalf("LatestLTS = %q, %v", v, err)
	}
	ctx := context.Background()
	for spec, want := range map[string]string{"20": "20.11.0", "lts": "24.11.0", "latest": "25.1.0"} {
		if v, err := env.ResolveSpec(ctx, n, spec); err != nil || v != want {
			t.Errorf("ResolveSpec(%s) = %q, %v; want %s", spec, v, err, want)
		}
	}
}

func TestNodeAsset(t *testing.T) {
	cases := []struct{ goos, goarch, file, dir string }{
		{"darwin", "arm64", "node-v20.11.0-darwin-arm64.tar.gz", "node-v20.11.0-darwin-arm64"},
		{"linux", "amd64", "node-v20.11.0-linux-x64.tar.gz", "node-v20.11.0-linux-x64"},
		{"windows", "amd64", "node-v20.11.0-win-x64.zip", "node-v20.11.0-win-x64"},
		{"windows", "arm64", "node-v20.11.0-win-arm64.zip", "node-v20.11.0-win-arm64"},
	}
	for _, c := range cases {
		file, dir, err := nodeAsset(c.goos, c.goarch, "20.11.0")
		if err != nil || file != c.file || dir != c.dir {
			t.Errorf("%s/%s: %s %s %v", c.goos, c.goarch, file, dir, err)
		}
	}
	if _, _, err := nodeAsset("plan9", "386", "20.11.0"); err == nil {
		t.Error("unsupported platform accepted")
	}
}

func TestNodeInstallKeepsNpmSymlinks(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	top := "node-v20.11.0-linux-x64"
	archive := tarGzBytes(t, []tarEntry{
		dir(top + "/"), dir(top + "/bin/"),
		reg(top+"/bin/node", "node"),
		reg(top+"/lib/node_modules/npm/bin/npm-cli.js", "npm"),
		reg(top+"/lib/node_modules/npm/bin/npx-cli.js", "npx"),
		link(top+"/bin/npm", "../lib/node_modules/npm/bin/npm-cli.js"),
		link(top+"/bin/npx", "../lib/node_modules/npm/bin/npx-cli.js"),
	})
	sums := shaBytes(archive) + "  " + top + ".tar.gz\n"
	url, _ := newServer(t, map[string]route{
		"/v20.11.0/SHASUMS256.txt":     {body: []byte(sums)},
		"/v20.11.0/" + top + ".tar.gz": {body: archive},
	})
	setVar(t, &nodeDistURL, url)
	dest := installInto(t, &NodeInstaller{}, "20.11.0")
	if got, _ := readFile(dest, "bin/npm"); got != "npm" {
		t.Fatalf("bin/npm resolves to %q", got)
	}
	if target, _ := os.Readlink(filepath.Join(dest, "bin", "npx")); target != "../lib/node_modules/npm/bin/npx-cli.js" {
		t.Fatalf("bin/npx -> %q", target)
	}
}

func TestNodeInstallNeedsPublishedChecksum(t *testing.T) {
	setHost(t, "linux", "amd64")
	url, _ := newServer(t, map[string]route{"/v20.11.0/SHASUMS256.txt": {body: []byte("abc  other.tar.gz\n")}})
	setVar(t, &nodeDistURL, url)
	err := (&NodeInstaller{}).Install(context.Background(), env.InstallRequest{Version: "20.11.0", Dest: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "no published checksum") {
		t.Fatalf("err = %v", err)
	}
}
```

`internal/env/runtimes/go_test.go` (whole file):

```go
package runtimes

import (
	"context"
	"reflect"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

const goFeed = `[
 {"version":"go1.27rc3","stable":false},
 {"version":"go1.26.2","stable":true},
 {"version":"go1.26.2","stable":true},
 {"version":"go1.26.1","stable":true}
]`

func TestStableGoVersionsSkipsPrereleases(t *testing.T) {
	got, err := stableGoVersions([]byte(goFeed))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.26.2", "1.26.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestGoLatestSkipsReleaseCandidate(t *testing.T) {
	url, _ := newServer(t, map[string]route{"/?mode=json&include=all": {body: []byte(goFeed)}})
	setVar(t, &goDLURL, url)
	v, err := env.ResolveSpec(context.Background(), &GoInstaller{}, "latest")
	if err != nil || v != "1.26.2" {
		t.Fatalf("latest = %q, %v; want 1.26.2", v, err)
	}
	if v, err := env.ResolveSpec(context.Background(), &GoInstaller{}, "1.26"); err != nil || v != "1.26.2" {
		t.Fatalf("1.26 = %q, %v", v, err)
	}
}

func TestGoAsset(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  "go1.26.2.darwin-arm64.tar.gz",
		{"linux", "amd64"}:   "go1.26.2.linux-amd64.tar.gz",
		{"linux", "arm"}:     "go1.26.2.linux-armv6l.tar.gz",
		{"windows", "amd64"}: "go1.26.2.windows-amd64.zip",
		{"windows", "arm64"}: "go1.26.2.windows-arm64.zip",
	}
	for p, want := range cases {
		if got := goAsset(p[0], p[1], "1.26.2"); got != want {
			t.Errorf("%v: %s, want %s", p, got, want)
		}
	}
}

func TestGoInstallVerifiesAndHoists(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	archive := tarGzBytes(t, []tarEntry{
		dir("go/"), dir("go/bin/"), reg("go/bin/go", "go"), reg("go/bin/gofmt", "gofmt"), reg("go/VERSION", "go1.26.2"),
	})
	feed := `[{"version":"go1.26.2","stable":true,"files":[{"filename":"go1.26.2.linux-amd64.tar.gz","sha256":"` + shaBytes(archive) + `"}]}]`
	url, _ := newServer(t, map[string]route{
		"/?mode=json":                  {body: []byte(feed)},
		"/go1.26.2.linux-amd64.tar.gz": {body: archive},
	})
	setVar(t, &goDLURL, url)
	dest := installInto(t, &GoInstaller{}, "1.26.2")
	if _, err := readFile(dest, "VERSION"); err != nil {
		t.Fatalf("go/ was not hoisted: %v", err)
	}
}

func TestGoInstallRejectsTamperedArchive(t *testing.T) {
	setHost(t, "linux", "amd64")
	feed := `[{"version":"go1.26.2","stable":true,"files":[{"filename":"go1.26.2.linux-amd64.tar.gz","sha256":"` + sha("original") + `"}]}]`
	url, _ := newServer(t, map[string]route{
		"/?mode=json":                  {body: []byte(feed)},
		"/go1.26.2.linux-amd64.tar.gz": {body: []byte("tampered")},
	})
	setVar(t, &goDLURL, url)
	err := (&GoInstaller{}).Install(context.Background(), env.InstallRequest{Version: "1.26.2", Dest: t.TempDir()})
	if err == nil {
		t.Fatal("tampered archive accepted")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run 'Node|Go' -count=1`
Expected: build failure: `undefined: hostOS`, `undefined: nodeAsset`, `undefined: goAsset` ...

- [ ] **Step 3: Platform variables in `internal/env/runtimes/download.go`**

```diff
--- a/internal/env/runtimes/download.go
+++ b/internal/env/runtimes/download.go
@@ -11,6 +11,7 @@ import (
 	"net/http"
 	"os"
 	"path"
+	"runtime"
 	"strings"
 	"time"
 )
@@ -22,6 +23,12 @@ var (
 	githubAPI   = "https://api.github.com"
 )
 
+// hostOS and hostArch pick release assets; tests override them.
+var (
+	hostOS   = runtime.GOOS
+	hostArch = runtime.GOARCH
+)
+
 // progress receives "Downloading ..." lines; tests silence it.
 var progress io.Writer = os.Stdout
 
```

- [ ] **Step 4: Replace `internal/env/runtimes/node.go`**

```go
package runtimes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// NodeInstaller installs Node.js from nodejs.org, verified against SHASUMS256.txt.
type NodeInstaller struct{}

func init() {
	env.RegisterInstaller("node", &NodeInstaller{})
}

// Name returns the runtime name.
func (n *NodeInstaller) Name() string { return "node" }

// nodeRelease is one entry of nodejs.org/dist/index.json.
type nodeRelease struct {
	Version string `json:"version"`
	LTS     any    `json:"lts"` // false, or the LTS codename
}

func (n *NodeInstaller) index(ctx context.Context) ([]nodeRelease, error) {
	var releases []nodeRelease
	if err := fetchJSON(ctx, nodeDistURL+"/index.json", &releases); err != nil {
		return nil, err
	}
	return releases, nil
}

// ListRemote returns every published Node.js version.
func (n *NodeInstaller) ListRemote(ctx context.Context) ([]string, error) {
	releases, err := n.index(ctx)
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(releases))
	for _, r := range releases {
		versions = append(versions, strings.TrimPrefix(r.Version, "v"))
	}
	return versions, nil
}

// LatestLTS returns the newest LTS release (index.json is newest first).
func (n *NodeInstaller) LatestLTS(ctx context.Context) (string, error) {
	releases, err := n.index(ctx)
	if err != nil {
		return "", err
	}
	for _, r := range releases {
		switch v := r.LTS.(type) {
		case bool:
			if v {
				return strings.TrimPrefix(r.Version, "v"), nil
			}
		case string:
			if v != "" {
				return strings.TrimPrefix(r.Version, "v"), nil
			}
		}
	}
	return "", errors.New("nodejs.org lists no LTS release")
}

// nodeAsset names the archive and its top directory for a platform.
func nodeAsset(goos, goarch, version string) (file, dir string, err error) {
	osName := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "win"}[goos]
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if osName == "" || arch == "" {
		return "", "", fmt.Errorf("nodejs.org publishes no Node.js binaries for %s/%s", goos, goarch)
	}
	dir = fmt.Sprintf("node-v%s-%s-%s", version, osName, arch)
	if goos == "windows" {
		return dir + ".zip", dir, nil
	}
	return dir + ".tar.gz", dir, nil
}

// Install downloads, verifies and unpacks Node.js into req.Dest.
func (n *NodeInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	file, dir, err := nodeAsset(hostOS, hostArch, req.Version)
	if err != nil {
		return err
	}
	base := nodeDistURL + "/v" + req.Version
	sums, err := fetchSmall(ctx, base+"/SHASUMS256.txt")
	if err != nil {
		return fmt.Errorf("fetch Node.js checksums: %w", err)
	}
	want, err := checksumFromSums(string(sums), file)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, base+"/"+file, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	return hoistDir(req.Dest, dir)
}

// BinaryPaths: corepack is not shipped from Node 25 on, so it is not listed.
func (n *NodeInstaller) BinaryPaths() []string {
	return []string{"bin/node", "bin/npm", "bin/npx"}
}
```

- [ ] **Step 5: Replace `internal/env/runtimes/go.go`**

```go
package runtimes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// GoInstaller installs Go from go.dev, verified against the release feed.
type GoInstaller struct{}

func init() {
	env.RegisterInstaller("go", &GoInstaller{})
}

// Name returns the runtime name.
func (g *GoInstaller) Name() string { return "go" }

// ListRemote returns every stable Go version (the include=all feed).
func (g *GoInstaller) ListRemote(ctx context.Context) ([]string, error) {
	data, err := fetchSmall(ctx, goDLURL+"/?mode=json&include=all")
	if err != nil {
		return nil, err
	}
	return stableGoVersions(data)
}

// stableGoVersions parses the go.dev release feed and returns the stable
// versions (without the "go" prefix) in feed order, de-duplicated. Betas and
// release candidates are skipped.
func stableGoVersions(releasesJSON []byte) ([]string, error) {
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.Unmarshal(releasesJSON, &releases); err != nil {
		return nil, err
	}
	var versions []string
	seen := make(map[string]bool)
	for _, release := range releases {
		if !release.Stable {
			continue
		}
		version := strings.TrimPrefix(release.Version, "go")
		if !seen[version] {
			versions = append(versions, version)
			seen[version] = true
		}
	}
	return versions, nil
}

// goAsset names the Go archive for a platform (Windows ships .zip).
func goAsset(goos, goarch, version string) string {
	if goarch == "arm" {
		goarch = "armv6l"
	}
	if goos == "windows" {
		return fmt.Sprintf("go%s.%s-%s.zip", version, goos, goarch)
	}
	return fmt.Sprintf("go%s.%s-%s.tar.gz", version, goos, goarch)
}

// Install downloads, verifies and unpacks Go into req.Dest.
func (g *GoInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	file := goAsset(hostOS, hostArch, req.Version)
	want, err := goReleaseChecksum(ctx, file)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, goDLURL+"/"+file, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	return hoistDir(req.Dest, "go")
}

// BinaryPaths returns the Go binaries.
func (g *GoInstaller) BinaryPaths() []string { return []string{"bin/go", "bin/gofmt"} }

// goReleaseChecksum looks the archive up in the small current-releases feed
// first and only falls back to the full (include=all) feed when absent.
// It fails closed: no checksum means an error.
func goReleaseChecksum(ctx context.Context, filename string) (string, error) {
	var lastErr error
	for _, u := range []string{goDLURL + "/?mode=json", goDLURL + "/?mode=json&include=all"} {
		meta, err := fetchSmall(ctx, u)
		if err != nil {
			lastErr = fmt.Errorf("fetch Go release list: %w", err)
			continue
		}
		want, err := goChecksum(meta, filename)
		if err != nil {
			lastErr = err
			continue
		}
		return want, nil
	}
	return "", lastErr
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/env/runtimes/ -run '.' -count=1`
Expected: `ok`.

- [ ] **Step 7: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 8: Commit (only this task's files)**

```bash
git add internal/env/runtimes/node.go \
  internal/env/runtimes/go.go \
  internal/env/runtimes/download.go \
  internal/env/runtimes/helpers_test.go \
  internal/env/runtimes/installer_test.go \
  internal/env/runtimes/node_test.go \
  internal/env/runtimes/go_test.go
git commit -m "env/runtimes: node and go installers on the shared helpers"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 7: Bun and Deno with checksums (R10)

**Files:**
- Create: `internal/env/runtimes/github.go`, `internal/env/runtimes/bun_test.go`, `internal/env/runtimes/deno_test.go`
- Modify (whole file): `internal/env/runtimes/bun.go`, `internal/env/runtimes/deno.go`
- Modify (hunk): `internal/env/runtimes/installer_test.go`

**Interfaces:**
- Consumes: Task 2 `fetchGitHubReleases`, `fetchSmall`, `isNotFound`, `downloadVerified`, `extractArchive`, `checksumFromSums`; Task 6 `hostOS`, `hostArch`, test helpers.
- Produces:
  - var `githubDownloadURL = "https://github.com"`
  - `func versionsFromTags(rels []githubRelease, prefix string) []string`, `func moveToBin(dest, from, name string) error`
  - `func bunAsset(goos, goarch string) (zip, dir string, err error)`; `BunInstaller.BinaryPaths() = bin/bun, bin/bunx` (`bin/bunx` is a symlink to `bun`)
  - `func denoAsset(goos, goarch string) (string, error)`, `func parseSumFile(body, name string) (string, error)`; `DenoInstaller.BinaryPaths() = bin/deno`
  - test helper `installReq(t, version) env.InstallRequest`

- [ ] **Step 1: Write the failing tests**

Append to `internal/env/runtimes/installer_test.go`:

```diff
--- a/internal/env/runtimes/installer_test.go
+++ b/internal/env/runtimes/installer_test.go
@@ -40,3 +40,9 @@ func skipWindows(t *testing.T) {
 		t.Skip("archives with symlinks and exec bits: Unix-only")
 	}
 }
+
+// installReq is an InstallRequest into a fresh empty dir.
+func installReq(t *testing.T, version string) env.InstallRequest {
+	t.Helper()
+	return env.InstallRequest{Version: version, Dest: t.TempDir(), Root: t.TempDir()}
+}
```

`internal/env/runtimes/bun_test.go`:

```go
package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionsFromTags(t *testing.T) {
	rels := []githubRelease{
		{TagName: "bun-v1.4.2"}, {TagName: "canary"}, {TagName: "bun-v1.4.3-canary.1"},
		{TagName: "bun-v1.4.1", Draft: true}, {TagName: "v1.0.0"}, {TagName: "bun-v1.3.0"},
	}
	if got := strings.Join(versionsFromTags(rels, "bun-v"), " "); got != "1.4.2 1.3.0" {
		t.Fatalf("bun versions = %q", got)
	}
	if got := strings.Join(versionsFromTags([]githubRelease{{TagName: "v2.9.7"}, {TagName: "std/0.1"}}, "v"), " "); got != "2.9.7" {
		t.Fatalf("deno versions = %q", got)
	}
}

func TestBunAsset(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  "bun-darwin-aarch64.zip",
		{"darwin", "amd64"}:  "bun-darwin-x64.zip",
		{"linux", "arm64"}:   "bun-linux-aarch64.zip",
		{"windows", "amd64"}: "bun-windows-x64.zip",
	}
	for p, want := range cases {
		if zip, _, err := bunAsset(p[0], p[1]); err != nil || zip != want {
			t.Errorf("%v: %s %v, want %s", p, zip, err, want)
		}
	}
}

func TestBunInstallVerifiesAndAddsBunx(t *testing.T) {
	skipWindows(t)
	setHost(t, "darwin", "arm64")
	archive := zipBytes(t, map[string]string{"bun-darwin-aarch64/bun": "bun-binary"})
	sums := sha("other") + "  bun-darwin-aarch64-profile.zip\n" + shaBytes(archive) + "  bun-darwin-aarch64.zip\n"
	base := "/oven-sh/bun/releases/download/bun-v1.4.2/"
	url, _ := newServer(t, map[string]route{
		base + "SHASUMS256.txt":         {body: []byte(sums)},
		base + "bun-darwin-aarch64.zip": {body: archive},
	})
	setVar(t, &githubDownloadURL, url)
	dest := installInto(t, &BunInstaller{}, "1.4.2")
	if target, _ := os.Readlink(filepath.Join(dest, "bin", "bunx")); target != "bun" {
		t.Fatalf("bunx -> %q", target)
	}
	if _, err := os.Stat(filepath.Join(dest, "bun-darwin-aarch64")); err == nil {
		t.Fatal("archive dir left behind")
	}
}

func TestBunInstallRejectsChecksumMismatch(t *testing.T) {
	setHost(t, "linux", "amd64")
	base := "/oven-sh/bun/releases/download/bun-v1.4.2/"
	url, _ := newServer(t, map[string]route{
		base + "SHASUMS256.txt":    {body: []byte(sha("good") + "  bun-linux-x64.zip\n")},
		base + "bun-linux-x64.zip": {body: []byte("evil")},
	})
	setVar(t, &githubDownloadURL, url)
	if err := (&BunInstaller{}).Install(context.Background(), installReq(t, "1.4.2")); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
}
```

`internal/env/runtimes/deno_test.go`:

```go
package runtimes

import (
	"context"
	"testing"
)

func TestDenoAsset(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  "deno-aarch64-apple-darwin.zip",
		{"darwin", "amd64"}:  "deno-x86_64-apple-darwin.zip",
		{"linux", "amd64"}:   "deno-x86_64-unknown-linux-gnu.zip",
		{"windows", "amd64"}: "deno-x86_64-pc-windows-msvc.zip",
	}
	for p, want := range cases {
		if got, err := denoAsset(p[0], p[1]); err != nil || got != want {
			t.Errorf("%v: %s %v, want %s", p, got, err, want)
		}
	}
}

func TestDenoInstallRootLevelBinary(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	archive := zipBytes(t, map[string]string{"deno": "deno-binary"})
	base := "/denoland/deno/releases/download/v2.9.7/deno-x86_64-unknown-linux-gnu.zip"
	url, _ := newServer(t, map[string]route{
		base + ".sha256sum": {body: []byte(shaBytes(archive) + "  deno-x86_64-unknown-linux-gnu.zip\n")},
		base:                {body: archive},
	})
	setVar(t, &githubDownloadURL, url)
	dest := installInto(t, &DenoInstaller{}, "2.9.7")
	if got, _ := readFile(dest, "bin/deno"); got != "deno-binary" {
		t.Fatalf("bin/deno = %q", got)
	}
}

func TestDenoWithoutChecksumIsRefused(t *testing.T) {
	setHost(t, "linux", "amd64")
	url, seen := newServer(t, map[string]route{})
	setVar(t, &githubDownloadURL, url)
	err := (&DenoInstaller{}).Install(context.Background(), installReq(t, "1.46.3"))
	want := "deno 1.46.3 publishes no SHA-256 checksum for deno-x86_64-unknown-linux-gnu.zip; xpm only installs verified downloads (Deno 2.0.6+ publish them)"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("downloaded without a checksum: %d requests", len(*seen))
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run 'Bun|Deno|VersionsFromTags' -count=1`
Expected: build failure: `undefined: versionsFromTags`, `undefined: bunAsset`, `undefined: githubDownloadURL` ...

- [ ] **Step 3: `internal/env/runtimes/github.go`**

```go
package runtimes

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// githubDownloadURL is where GitHub release assets live.
var githubDownloadURL = "https://github.com"

// versionsFromTags turns release tags into versions: tags must start with
// prefix ("bun-v", "v"); drafts and canary builds are skipped.
func versionsFromTags(rels []githubRelease, prefix string) []string {
	var versions []string
	for _, r := range rels {
		v, ok := strings.CutPrefix(r.TagName, prefix)
		if !ok || r.Draft || strings.Contains(v, "canary") || env.ValidateVersionSpec(v) != nil {
			continue
		}
		versions = append(versions, v)
	}
	return versions
}

// moveToBin moves dest/<from> to dest/bin/<name>.
func moveToBin(dest, from, name string) error {
	if err := os.MkdirAll(filepath.Join(dest, "bin"), 0o755); err != nil {
		return err
	}
	return os.Rename(filepath.Join(dest, filepath.FromSlash(from)), filepath.Join(dest, "bin", name))
}
```

- [ ] **Step 4: Replace `internal/env/runtimes/bun.go`**

```go
package runtimes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/crenspire/xpm/internal/env"
)

// BunInstaller installs Bun from GitHub releases, verified against SHASUMS256.txt.
type BunInstaller struct{}

func init() {
	env.RegisterInstaller("bun", &BunInstaller{})
}

// Name returns the runtime name.
func (b *BunInstaller) Name() string { return "bun" }

// ListRemote returns versions from release tags like "bun-v1.4.2".
func (b *BunInstaller) ListRemote(ctx context.Context) ([]string, error) {
	rels, err := fetchGitHubReleases(ctx, "oven-sh/bun")
	if err != nil {
		return nil, err
	}
	return versionsFromTags(rels, "bun-v"), nil
}

// bunAsset names the release zip and its top directory for a platform.
func bunAsset(goos, goarch string) (zip, dir string, err error) {
	osName := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "windows"}[goos]
	arch := map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	if osName == "" || arch == "" {
		return "", "", fmt.Errorf("bun publishes no binaries for %s/%s", goos, goarch)
	}
	dir = fmt.Sprintf("bun-%s-%s", osName, arch)
	return dir + ".zip", dir, nil
}

// Install downloads, verifies and unpacks Bun into req.Dest/bin, and adds
// bin/bunx -> bun (bun acts as bunx when invoked under that name).
func (b *BunInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	zip, dir, err := bunAsset(hostOS, hostArch)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("%s/oven-sh/bun/releases/download/bun-v%s", githubDownloadURL, req.Version)
	sums, err := fetchSmall(ctx, base+"/SHASUMS256.txt")
	if err != nil {
		return fmt.Errorf("fetch bun checksums: %w", err)
	}
	want, err := checksumFromSums(string(sums), zip)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, base+"/"+zip, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	if err := moveToBin(req.Dest, dir+"/bun", "bun"); err != nil {
		return fmt.Errorf("bun archive layout changed: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(req.Dest, dir)); err != nil {
		return err
	}
	return os.Symlink("bun", filepath.Join(req.Dest, "bin", "bunx"))
}

// BinaryPaths returns the Bun binaries.
func (b *BunInstaller) BinaryPaths() []string { return []string{"bin/bun", "bin/bunx"} }
```

- [ ] **Step 5: Replace `internal/env/runtimes/deno.go`**

```go
package runtimes

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// DenoInstaller installs Deno from GitHub releases, verified against the
// per-asset .sha256sum file.
type DenoInstaller struct{}

func init() {
	env.RegisterInstaller("deno", &DenoInstaller{})
}

// Name returns the runtime name.
func (d *DenoInstaller) Name() string { return "deno" }

// ListRemote returns versions from release tags like "v2.9.7".
func (d *DenoInstaller) ListRemote(ctx context.Context) ([]string, error) {
	rels, err := fetchGitHubReleases(ctx, "denoland/deno")
	if err != nil {
		return nil, err
	}
	return versionsFromTags(rels, "v"), nil
}

// denoAsset names the release zip for a platform.
func denoAsset(goos, goarch string) (string, error) {
	triple := map[string]string{"darwin": "apple-darwin", "linux": "unknown-linux-gnu", "windows": "pc-windows-msvc"}[goos]
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	if triple == "" || arch == "" {
		return "", fmt.Errorf("deno publishes no binaries for %s/%s", goos, goarch)
	}
	return fmt.Sprintf("deno-%s-%s.zip", arch, triple), nil
}

// parseSumFile reads "<hex>  <name>" (or a bare "<hex>").
func parseSumFile(body, name string) (string, error) {
	fields := strings.Fields(body)
	if len(fields) == 1 {
		return fields[0], nil
	}
	return checksumFromSums(body, name)
}

// Install downloads, verifies and unpacks Deno; its zip holds a root-level
// deno binary, which moves to bin/deno.
func (d *DenoInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	zip, err := denoAsset(hostOS, hostArch)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/denoland/deno/releases/download/v%s/%s", githubDownloadURL, req.Version, zip)
	body, err := fetchSmall(ctx, url+".sha256sum")
	if isNotFound(err) {
		return fmt.Errorf("deno %s publishes no SHA-256 checksum for %s; xpm only installs verified downloads (Deno 2.0.6+ publish them)", req.Version, zip)
	}
	if err != nil {
		return fmt.Errorf("fetch deno checksum: %w", err)
	}
	want, err := parseSumFile(string(body), zip)
	if err != nil {
		return err
	}
	archive, err := downloadVerified(ctx, url, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	if err := moveToBin(req.Dest, "deno", "deno"); err != nil {
		return fmt.Errorf("deno archive has no deno binary at its root: %w", err)
	}
	return nil
}

// BinaryPaths returns the Deno binary.
func (d *DenoInstaller) BinaryPaths() []string { return []string{"bin/deno"} }
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/env/runtimes/ -run '.' -count=1`
Expected: `ok`.

- [ ] **Step 7: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 8: Commit (only this task's files)**

```bash
git add internal/env/runtimes/github.go \
  internal/env/runtimes/bun.go \
  internal/env/runtimes/deno.go \
  internal/env/runtimes/bun_test.go \
  internal/env/runtimes/deno_test.go \
  internal/env/runtimes/installer_test.go
git commit -m "env/runtimes: bun and deno with checksum verification"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 8: Java via the Adoptium API, with checksums and macOS `Contents/Home` (R10)

**Files:**
- Modify (whole file): `internal/env/runtimes/java.go`
- Modify (hunk): `internal/env/runtimes/utils.go` (add `singleTopDir`)
- Create: `internal/env/runtimes/java_test.go`

**Interfaces:**
- Consumes: Task 2 `fetchJSON`, `downloadVerified`, `extractArchive`, `hoistDir`, `isNotFound`; Task 6 `hostOS`, `hostArch`.
- Produces:
  - var `adoptiumAPI = "https://api.adoptium.net"`
  - `JavaInstaller` implements `env.Resolver`: `latest` → `most_recent_feature_release`, `lts` → `most_recent_lts` (both from `/v3/info/available_releases`), a major → `/v3/assets/latest/<major>/hotspot?architecture=&image_type=jdk&os=&vendor=eclipse` → `release_name` without `jdk-`; other digit-leading specs are exact
  - `func adoptiumPlatform(goos, goarch string) (osName, arch string, err error)`, `func javaVersionFromRelease(name string) string`, `func javaReleaseName(version string) string`, `func singleTopDir(dest string) (string, error)`
  - `JavaInstaller.BinaryPaths() = bin/java, bin/javac, bin/jar, bin/keytool`

Note (ruling R10 says `binary.package`): the `/v3/assets/release_name/...` endpoint answers with a release object whose packages are under `binaries[0].package` (checked live on 2026-10-07: `{"binaries":[{"package":{"checksum":...,"link":...}}], ...}`); only `/v3/assets/latest/...` uses `[{binary:{...}}]`. The code reads `binaries[0].package`.

- [ ] **Step 1: Write the failing tests**

`internal/env/runtimes/java_test.go`:

```go
package runtimes

import (
	"context"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

const adoptiumInfo = `{"available_releases":[8,11,17,21,25],"available_lts_releases":[8,11,17,21,25],"most_recent_feature_release":25,"most_recent_lts":25}`

func TestJavaReleaseNames(t *testing.T) {
	cases := map[string]string{"jdk-21.0.12.1+1": "21.0.12.1+1", "jdk8u422-b05": "8u422-b05", "jdk-17.0.9+9": "17.0.9+9"}
	for name, v := range cases {
		if got := javaVersionFromRelease(name); got != v {
			t.Errorf("%s -> %s, want %s", name, got, v)
		}
		if got := javaReleaseName(v); got != name {
			t.Errorf("%s -> %s, want %s", v, got, name)
		}
	}
}

func TestAdoptiumPlatform(t *testing.T) {
	cases := map[[2]string][2]string{
		{"darwin", "arm64"}:  {"mac", "aarch64"},
		{"linux", "amd64"}:   {"linux", "x64"},
		{"windows", "amd64"}: {"windows", "x64"},
	}
	for p, want := range cases {
		o, a, err := adoptiumPlatform(p[0], p[1])
		if err != nil || o != want[0] || a != want[1] {
			t.Errorf("%v: %s %s %v", p, o, a, err)
		}
	}
}

func TestJavaResolve(t *testing.T) {
	setHost(t, "darwin", "arm64")
	q := "architecture=aarch64&image_type=jdk&os=mac&vendor=eclipse"
	url, _ := newServer(t, map[string]route{
		"/v3/info/available_releases":       {body: []byte(adoptiumInfo)},
		"/v3/assets/latest/21/hotspot?" + q: {body: []byte(`[{"release_name":"jdk-21.0.12.1+1"}]`)},
		"/v3/assets/latest/25/hotspot?" + q: {body: []byte(`[{"release_name":"jdk-25.0.1+8"}]`)},
		"/v3/assets/latest/8/hotspot?" + q:  {body: []byte(`[{"release_name":"jdk8u422-b05"}]`)},
	})
	setVar(t, &adoptiumAPI, url)
	j := &JavaInstaller{}
	ctx := context.Background()
	for spec, want := range map[string]string{"21": "21.0.12.1+1", "8": "8u422-b05", "latest": "25.0.1+8", "lts": "25.0.1+8", "21.0.4+7": "21.0.4+7"} {
		if got, err := env.ResolveSpec(ctx, j, spec); err != nil || got != want {
			t.Errorf("%s: %q, %v; want %s", spec, got, err, want)
		}
	}
	if _, err := j.Resolve(ctx, "stable"); err == nil {
		t.Error("stable accepted")
	}
	if got, err := j.ListRemote(ctx); err != nil || strings.Join(got, ",") != "8,11,17,21,25" {
		t.Errorf("ListRemote = %v, %v", got, err)
	}
}

func TestJavaInstallMacHoistsContentsHome(t *testing.T) {
	skipWindows(t)
	setHost(t, "darwin", "arm64")
	top := "jdk-21.0.12.1+1/"
	var entries []tarEntry
	entries = append(entries, dir(top), reg(top+"Contents/Info.plist", "plist"))
	for _, b := range []string{"java", "javac", "jar", "keytool"} {
		entries = append(entries, reg(top+"Contents/Home/bin/"+b, b))
	}
	archive := tarGzBytes(t, entries)
	release := `{"binaries":[{"package":{"checksum":"` + shaBytes(archive) + `","link":"LINK","name":"OpenJDK21U-jdk_aarch64_mac_hotspot_21.0.12.1_1.tar.gz"}}],"release_name":"jdk-21.0.12.1+1"}`
	q := "architecture=aarch64&heap_size=normal&image_type=jdk&jvm_impl=hotspot&os=mac"
	// The handler reads routes per request, so the release JSON can name
	// the server's own URL once it is known.
	routes := map[string]route{"/dl/OpenJDK21U-jdk_aarch64_mac_hotspot_21.0.12.1_1.tar.gz": {body: archive}}
	url, seen := newServer(t, routes)
	routes["/v3/assets/release_name/eclipse/jdk-21.0.12.1+1?"+q] = route{body: []byte(strings.Replace(release, "LINK", url+"/dl/OpenJDK21U-jdk_aarch64_mac_hotspot_21.0.12.1_1.tar.gz", 1))}
	setVar(t, &adoptiumAPI, url)
	dest := installInto(t, &JavaInstaller{}, "21.0.12.1+1")
	if _, err := readFile(dest, "Info.plist"); err == nil {
		t.Fatal("Contents/ leaked into the JDK root")
	}
	if got := (*seen)[0].URL.EscapedPath(); got != "/v3/assets/release_name/eclipse/jdk-21.0.12.1%2B1" {
		t.Fatalf("release path %s: '+' must be sent as %%2B", got)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run 'Java|Adoptium' -count=1`
Expected: build failure: `undefined: javaVersionFromRelease`, `undefined: adoptiumPlatform`, `undefined: adoptiumAPI` ...

- [ ] **Step 3: Add `singleTopDir` to `internal/env/runtimes/utils.go`**

```diff
--- a/internal/env/runtimes/utils.go
+++ b/internal/env/runtimes/utils.go
@@ -391,6 +391,29 @@ func hoistDir(dest, sub string) error {
 	return os.RemoveAll(parked)
 }
 
+// singleTopDir returns the only directory at the root of dest (ignoring
+// hidden entries), as found in JDK archives ("jdk-21.0.4+7/").
+func singleTopDir(dest string) (string, error) {
+	entries, err := os.ReadDir(dest)
+	if err != nil {
+		return "", err
+	}
+	var dirs []string
+	for _, e := range entries {
+		if strings.HasPrefix(e.Name(), ".") {
+			continue
+		}
+		if !e.IsDir() {
+			return "", fmt.Errorf("archive has %s at its root; expected a single directory", e.Name())
+		}
+		dirs = append(dirs, e.Name())
+	}
+	if len(dirs) != 1 {
+		return "", fmt.Errorf("archive has %d top-level directories; expected 1", len(dirs))
+	}
+	return dirs[0], nil
+}
+
 // copyDirectory copies a directory recursively, handling symlinks.
 func copyDirectory(src, dest string) error {
 	// Ensure destination exists
```

- [ ] **Step 4: Replace `internal/env/runtimes/java.go`**

```go
package runtimes

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// adoptiumAPI is the Eclipse Temurin (Adoptium) API.
var adoptiumAPI = "https://api.adoptium.net"

var javaMajorRe = regexp.MustCompile(`^[0-9]+$`)

// JavaInstaller installs Eclipse Temurin JDKs via the Adoptium API,
// verified against the checksum the API publishes for each package.
type JavaInstaller struct{}

func init() {
	env.RegisterInstaller("java", &JavaInstaller{})
}

// Name returns the runtime name.
func (j *JavaInstaller) Name() string { return "java" }

type adoptiumReleases struct {
	AvailableReleases []int `json:"available_releases"`
	MostRecentFeature int   `json:"most_recent_feature_release"`
	MostRecentLTS     int   `json:"most_recent_lts"`
}

type adoptiumPackage struct {
	Checksum string `json:"checksum"`
	Link     string `json:"link"`
	Name     string `json:"name"`
}

func (j *JavaInstaller) available(ctx context.Context) (adoptiumReleases, error) {
	var r adoptiumReleases
	err := fetchJSON(ctx, adoptiumAPI+"/v3/info/available_releases", &r)
	return r, err
}

// ListRemote returns the available feature releases (majors: "21", "17", ...).
func (j *JavaInstaller) ListRemote(ctx context.Context) ([]string, error) {
	r, err := j.available(ctx)
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(r.AvailableReleases))
	for _, v := range r.AvailableReleases {
		versions = append(versions, strconv.Itoa(v))
	}
	return versions, nil
}

// adoptiumPlatform maps Go's GOOS/GOARCH to Adoptium's os/architecture.
func adoptiumPlatform(goos, goarch string) (osName, arch string, err error) {
	osName = map[string]string{"darwin": "mac", "linux": "linux", "windows": "windows"}[goos]
	arch = map[string]string{"amd64": "x64", "arm64": "aarch64"}[goarch]
	if osName == "" || arch == "" {
		return "", "", fmt.Errorf("adoptium publishes no JDK for %s/%s", goos, goarch)
	}
	return osName, arch, nil
}

// javaVersionFromRelease turns "jdk-21.0.12.1+1" into "21.0.12.1+1" and
// "jdk8u422-b05" into "8u422-b05".
func javaVersionFromRelease(name string) string {
	if v, ok := strings.CutPrefix(name, "jdk-"); ok {
		return v
	}
	return strings.TrimPrefix(name, "jdk")
}

// javaReleaseName is the inverse of javaVersionFromRelease.
func javaReleaseName(version string) string {
	if strings.HasPrefix(version, "8u") {
		return "jdk" + version
	}
	return "jdk-" + version
}

// Resolve maps "latest" (newest feature release), "lts" (newest LTS) and a
// major ("21") to that major's newest exact release; anything else that
// starts with a digit is taken as an exact release ("21.0.4+7").
func (j *JavaInstaller) Resolve(ctx context.Context, spec string) (string, error) {
	major := spec
	switch {
	case spec == "latest" || spec == "lts":
		r, err := j.available(ctx)
		if err != nil {
			return "", err
		}
		n := r.MostRecentFeature
		if spec == "lts" {
			n = r.MostRecentLTS
		}
		if n == 0 {
			return "", fmt.Errorf("adoptium did not report a %s release", spec)
		}
		major = strconv.Itoa(n)
	case javaMajorRe.MatchString(spec):
	case spec[0] >= '0' && spec[0] <= '9':
		return spec, nil
	default:
		return "", fmt.Errorf("unknown java version %q: use a major (21), latest, lts or an exact release (21.0.4+7)", spec)
	}
	osName, arch, err := adoptiumPlatform(hostOS, hostArch)
	if err != nil {
		return "", err
	}
	q := url.Values{"architecture": {arch}, "image_type": {"jdk"}, "os": {osName}, "vendor": {"eclipse"}}
	var assets []struct {
		ReleaseName string `json:"release_name"`
	}
	if err := fetchJSON(ctx, fmt.Sprintf("%s/v3/assets/latest/%s/hotspot?%s", adoptiumAPI, major, q.Encode()), &assets); err != nil {
		return "", err
	}
	if len(assets) == 0 || assets[0].ReleaseName == "" {
		return "", fmt.Errorf("adoptium has no JDK %s for %s/%s", major, osName, arch)
	}
	return javaVersionFromRelease(assets[0].ReleaseName), nil
}

// Install downloads, verifies and unpacks the JDK. The archive's single top
// directory is hoisted; on macOS the JDK home (Contents/Home) becomes the root.
func (j *JavaInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	osName, arch, err := adoptiumPlatform(hostOS, hostArch)
	if err != nil {
		return err
	}
	q := url.Values{"architecture": {arch}, "heap_size": {"normal"}, "image_type": {"jdk"}, "jvm_impl": {"hotspot"}, "os": {osName}}
	name := strings.ReplaceAll(url.PathEscape(javaReleaseName(req.Version)), "+", "%2B")
	var release struct {
		Binaries []struct {
			Package adoptiumPackage `json:"package"`
		} `json:"binaries"`
	}
	if err := fetchJSON(ctx, fmt.Sprintf("%s/v3/assets/release_name/eclipse/%s?%s", adoptiumAPI, name, q.Encode()), &release); err != nil {
		if isNotFound(err) {
			return fmt.Errorf("adoptium has no release %s (see: xpm env ls-remote java)", javaReleaseName(req.Version))
		}
		return err
	}
	if len(release.Binaries) == 0 || release.Binaries[0].Package.Link == "" {
		return fmt.Errorf("adoptium has no %s JDK package for %s/%s", javaReleaseName(req.Version), osName, arch)
	}
	pkg := release.Binaries[0].Package
	archive, err := downloadVerified(ctx, pkg.Link, pkg.Checksum)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	top, err := singleTopDir(req.Dest)
	if err != nil {
		return err
	}
	if err := hoistDir(req.Dest, top); err != nil {
		return err
	}
	if fi, err := os.Stat(filepath.Join(req.Dest, "Contents", "Home")); err == nil && fi.IsDir() {
		return hoistDir(req.Dest, "Contents/Home")
	}
	return nil
}

// BinaryPaths returns the JDK binaries.
func (j *JavaInstaller) BinaryPaths() []string {
	return []string{"bin/java", "bin/javac", "bin/jar", "bin/keytool"}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/env/runtimes/ -run '.' -count=1`
Expected: `ok`.

- [ ] **Step 6: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 7: Commit (only this task's files)**

```bash
git add internal/env/runtimes/java.go \
  internal/env/runtimes/java_test.go \
  internal/env/runtimes/utils.go
git commit -m "env/runtimes: java via the Adoptium API with checksums"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 9: Python from python-build-standalone (R10)

**Files:**
- Modify (whole file): `internal/env/runtimes/python.go` (the python.org scraper, the macOS/Linux "install it yourself" stubs and `installWindows` are deleted)
- Create: `internal/env/runtimes/python_test.go`

**Interfaces:**
- Consumes: Task 1 `env.SortVersionsDesc`, `env.HighestMatch`, `env.ParseVersion`; Task 2 helpers; Task 6 `hostOS`, `hostArch`.
- Produces:
  - var `pbsLatestURL = "https://raw.githubusercontent.com/astral-sh/python-build-standalone/latest-release/latest-release.json"`
  - `type pbsRelease struct{ Tag, Prefix, Sums, Triple string }` with `func (r pbsRelease) versions() []string`
  - `func pbsTriple(goos, goarch string) (string, error)`, `func pbsAsset(version, tag, triple string) string`
  - `PythonInstaller` implements `env.Resolver`; error for an exact version the release lacks: `python <v> is not available: python-build-standalone <tag> provides 3.10.22, 3.11.17, ...` (ascending)
  - `PythonInstaller.BinaryPaths() = bin/python, bin/python3, bin/pip, bin/pip3`

- [ ] **Step 1: Write the failing tests**

`internal/env/runtimes/python_test.go`:

```go
package runtimes

import (
	"context"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

// pbsServer fakes latest-release.json, SHA256SUMS and one archive.
func pbsServer(t *testing.T, archive []byte) {
	t.Helper()
	tag := "20261003"
	sums := strings.Join([]string{
		shaBytes(archive) + "  cpython-3.12.15+20261003-x86_64-unknown-linux-gnu-install_only.tar.gz",
		sha("a") + "  cpython-3.12.15+20261003-x86_64-unknown-linux-gnu-install_only_stripped.tar.gz",
		sha("b") + "  cpython-3.13.9+20261003-x86_64-unknown-linux-gnu-freethreaded-install_only.tar.gz",
		sha("c") + "  cpython-3.11.17+20261003-x86_64-unknown-linux-gnu-install_only.tar.gz",
		sha("d") + "  cpython-3.14.0rc3+20261003-x86_64-unknown-linux-gnu-install_only.tar.gz",
		sha("e") + "  cpython-3.13.9+20261003-aarch64-apple-darwin-install_only.tar.gz",
	}, "\n") + "\n"
	routes := map[string]route{
		"/rel/" + tag + "/SHA256SUMS": {body: []byte(sums)},
		"/rel/" + tag + "/cpython-3.12.15+20261003-x86_64-unknown-linux-gnu-install_only.tar.gz": {body: archive},
	}
	url, _ := newServer(t, routes)
	routes["/latest-release.json"] = route{body: []byte(`{"tag":"` + tag + `","asset_url_prefix":"` + url + `/rel/` + tag + `"}`)}
	setVar(t, &pbsLatestURL, url+"/latest-release.json")
}

func TestPythonVersionsAndResolve(t *testing.T) {
	setHost(t, "linux", "amd64")
	pbsServer(t, []byte("x"))
	p := &PythonInstaller{}
	ctx := context.Background()
	got, err := p.ListRemote(ctx)
	env.SortVersionsDesc(got)
	if err != nil || strings.Join(got, " ") != "3.14.0rc3 3.12.15 3.11.17" {
		t.Fatalf("ListRemote = %v, %v", got, err)
	}
	for spec, want := range map[string]string{"latest": "3.12.15", "3": "3.12.15", "3.11": "3.11.17", "3.12.15": "3.12.15"} {
		if v, err := env.ResolveSpec(ctx, p, spec); err != nil || v != want {
			t.Errorf("%s: %q, %v; want %s", spec, v, err, want)
		}
	}
	_, err = env.ResolveSpec(ctx, p, "3.12.4")
	if err == nil || err.Error() != "python 3.12.4 is not available: python-build-standalone 20261003 provides 3.11.17, 3.12.15, 3.14.0rc3" {
		t.Fatalf("missing exact: %v", err)
	}
}

func TestPbsTriple(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}: "aarch64-apple-darwin",
		{"darwin", "amd64"}: "x86_64-apple-darwin",
		{"linux", "amd64"}:  "x86_64-unknown-linux-gnu",
		{"linux", "arm64"}:  "aarch64-unknown-linux-gnu",
	}
	for p, want := range cases {
		if got, err := pbsTriple(p[0], p[1]); err != nil || got != want {
			t.Errorf("%v: %s %v", p, got, err)
		}
	}
	if _, err := pbsTriple("windows", "amd64"); err == nil {
		t.Error("windows accepted")
	}
}

func TestPythonInstallHoistsPythonDir(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	archive := tarGzBytes(t, []tarEntry{
		dir("python/"), dir("python/bin/"),
		reg("python/bin/python3.12", "py"),
		link("python/bin/python", "python3.12"),
		link("python/bin/python3", "python3.12"),
		reg("python/bin/pip", "pip"),
		reg("python/bin/pip3", "pip"),
		reg("python/lib/python3.12/os.py", "os"),
	})
	pbsServer(t, archive)
	dest := installInto(t, &PythonInstaller{}, "3.12.15")
	if got, _ := readFile(dest, "lib/python3.12/os.py"); got != "os" {
		t.Fatal("python/ was not hoisted")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run 'Python|Pbs' -count=1`
Expected: build failure: `undefined: pbsLatestURL`, `undefined: pbsTriple`.

- [ ] **Step 3: Replace `internal/env/runtimes/python.go`**

```go
package runtimes

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// pbsLatestURL describes the newest python-build-standalone release.
var pbsLatestURL = "https://raw.githubusercontent.com/astral-sh/python-build-standalone/latest-release/latest-release.json"

// PythonInstaller installs CPython builds from python-build-standalone
// (the builds uv and rye use), verified against the release's SHA256SUMS.
type PythonInstaller struct{}

func init() {
	env.RegisterInstaller("python", &PythonInstaller{})
}

// Name returns the runtime name.
func (p *PythonInstaller) Name() string { return "python" }

// pbsRelease is one python-build-standalone release and its checksums.
type pbsRelease struct {
	Tag    string
	Prefix string // asset URL prefix
	Sums   string // SHA256SUMS content
	Triple string
}

// pbsTriple maps GOOS/GOARCH to the supported build triples.
func pbsTriple(goos, goarch string) (string, error) {
	t := map[string]string{
		"darwin/arm64": "aarch64-apple-darwin",
		"darwin/amd64": "x86_64-apple-darwin",
		"linux/amd64":  "x86_64-unknown-linux-gnu",
		"linux/arm64":  "aarch64-unknown-linux-gnu",
	}[goos+"/"+goarch]
	if t == "" {
		return "", fmt.Errorf("xpm installs Python only on macOS and Linux (x86_64, arm64), not %s/%s", goos, goarch)
	}
	return t, nil
}

func (p *PythonInstaller) latest(ctx context.Context) (pbsRelease, error) {
	triple, err := pbsTriple(hostOS, hostArch)
	if err != nil {
		return pbsRelease{}, err
	}
	var meta struct {
		Tag            string `json:"tag"`
		AssetURLPrefix string `json:"asset_url_prefix"`
	}
	if err := fetchJSON(ctx, pbsLatestURL, &meta); err != nil {
		return pbsRelease{}, err
	}
	if meta.Tag == "" || meta.AssetURLPrefix == "" {
		return pbsRelease{}, fmt.Errorf("unexpected python-build-standalone release info from %s", pbsLatestURL)
	}
	sums, err := fetchSmall(ctx, strings.TrimSuffix(meta.AssetURLPrefix, "/")+"/SHA256SUMS")
	if err != nil {
		return pbsRelease{}, fmt.Errorf("fetch python-build-standalone checksums: %w", err)
	}
	return pbsRelease{Tag: meta.Tag, Prefix: strings.TrimSuffix(meta.AssetURLPrefix, "/"), Sums: string(sums), Triple: triple}, nil
}

// pbsAsset names the install_only archive of version in a release.
func pbsAsset(version, tag, triple string) string {
	return fmt.Sprintf("cpython-%s+%s-%s-install_only.tar.gz", version, tag, triple)
}

// versions lists the CPython versions this release offers for its triple
// (only the plain install_only flavour).
func (r pbsRelease) versions() []string {
	suffix := "+" + r.Tag + "-" + r.Triple + "-install_only.tar.gz"
	var out []string
	for _, line := range strings.Split(r.Sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if !strings.HasPrefix(name, "cpython-") || !strings.HasSuffix(name, suffix) {
			continue
		}
		v := strings.TrimSuffix(strings.TrimPrefix(name, "cpython-"), suffix)
		if env.ValidateVersionSpec(v) == nil {
			out = append(out, v)
		}
	}
	return out
}

// ListRemote returns the versions of the newest release for this platform.
func (p *PythonInstaller) ListRemote(ctx context.Context) ([]string, error) {
	r, err := p.latest(ctx)
	if err != nil {
		return nil, err
	}
	return r.versions(), nil
}

// Resolve picks from the newest release only: "latest" or a partial
// version selects the highest stable match; an exact version must be offered.
func (p *PythonInstaller) Resolve(ctx context.Context, spec string) (string, error) {
	r, err := p.latest(ctx)
	if err != nil {
		return "", err
	}
	versions := r.versions()
	offered := append([]string(nil), versions...)
	env.SortVersionsDesc(offered)
	for i, j := 0, len(offered)-1; i < j; i, j = i+1, j-1 {
		offered[i], offered[j] = offered[j], offered[i]
	}
	list := strings.Join(offered, ", ")
	pv, ok := env.ParseVersion(spec)
	if spec != "latest" && ok && len(pv.Nums) >= 3 {
		for _, v := range versions {
			if v == spec {
				return v, nil
			}
		}
		return "", fmt.Errorf("python %s is not available: python-build-standalone %s provides %s", spec, r.Tag, list)
	}
	if v, found := env.HighestMatch(spec, versions, false); found {
		return v, nil
	}
	return "", fmt.Errorf("no python version matches %s: python-build-standalone %s provides %s", spec, r.Tag, list)
}

// Install downloads, verifies and unpacks CPython into req.Dest.
func (p *PythonInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	r, err := p.latest(ctx)
	if err != nil {
		return err
	}
	asset := pbsAsset(req.Version, r.Tag, r.Triple)
	want, err := checksumFromSums(r.Sums, asset)
	if err != nil {
		return fmt.Errorf("python %s is not in python-build-standalone %s: %w", req.Version, r.Tag, err)
	}
	archive, err := downloadVerified(ctx, r.Prefix+"/"+asset, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	if err := extractArchive(archive, req.Dest); err != nil {
		return err
	}
	return hoistDir(req.Dest, "python")
}

// BinaryPaths returns the Python binaries.
func (p *PythonInstaller) BinaryPaths() []string {
	return []string{"bin/python", "bin/python3", "bin/pip", "bin/pip3"}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/env/runtimes/ -run '.' -count=1`
Expected: `ok`.

- [ ] **Step 5: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 6: Commit (only this task's files)**

```bash
git add internal/env/runtimes/python.go \
  internal/env/runtimes/python_test.go
git commit -m "env/runtimes: python from python-build-standalone"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 10: Rust through a private rustup (R10)

**Files:**
- Create: `internal/env/runtimes/cmd.go`
- Modify (whole file): `internal/env/runtimes/rust.go`, `internal/env/runtimes/rust_test.go`

**Interfaces:**
- Consumes: Task 2 `fetchSmall`, `isNotFound`, `downloadVerifiedTo`; Task 4 `env.Resolver`, `env.Remover`; Task 6 `hostOS`, `hostArch`; Task 7 `installReq`.
- Produces:
  - seam `var runCmd func(ctx context.Context, name string, args, extraEnv []string) error` (Task 11 reuses it for brew)
  - vars `rustDistURL = "https://static.rust-lang.org/dist"`, `rustupDistURL = "https://static.rust-lang.org/rustup/dist"`
  - `func rustHostTriple(goos, goarch string) (string, error)`, `func rustEnv(root string) []string`, `func parseRustChannelVersion(manifest string) (string, error)`, `func bootstrapRustup(ctx context.Context, root, host string) error`
  - `RustInstaller` implements `env.Resolver` and `env.Remover`; `BinaryPaths() = bin/rustc, bin/cargo`; `<version dir>/bin` is a symlink to `<root>/rustup/toolchains/<v>-<host>/bin`
  - Deleted: `rustupExists`, `installViaRustup` (copied `~/.rustup` toolchains), the hard-coded version fallback, `ValidateVersion`, `PostInstall`.

- [ ] **Step 1: Write the failing tests**

`internal/env/runtimes/rust_test.go` (replace the file; the old test expected a "needs rustup" error):

```go
package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

const rustStable = "manifest-version = \"2\"\n[pkg.cargo]\nversion = \"0.100.0 (abc 2026-09-20)\"\n[pkg.rust]\nversion = \"1.99.0 (b940084d7 2026-09-28)\"\n"

func TestParseRustChannelVersion(t *testing.T) {
	if v, err := parseRustChannelVersion(rustStable); err != nil || v != "1.99.0" {
		t.Fatalf("got %q, %v", v, err)
	}
	if _, err := parseRustChannelVersion("[pkg.cargo]\nversion = \"1.0.0 (x)\"\n"); err == nil {
		t.Fatal("manifest without [pkg.rust] accepted")
	}
}

func TestRustResolve(t *testing.T) {
	url, _ := newServer(t, map[string]route{
		"/channel-rust-stable.toml": {body: []byte(rustStable)},
		"/channel-rust-1.80.toml":   {body: []byte("[pkg.rust]\nversion = \"1.80.1 (3f5fd8dd4 2024-08-06)\"\n")},
	})
	setVar(t, &rustDistURL, url)
	r := &RustInstaller{}
	ctx := context.Background()
	for spec, want := range map[string]string{"stable": "1.99.0", "latest": "1.99.0", "1.80": "1.80.1", "1.75.0": "1.75.0"} {
		if v, err := env.ResolveSpec(ctx, r, spec); err != nil || v != want {
			t.Errorf("%s: %q, %v; want %s", spec, v, err, want)
		}
	}
	for _, spec := range []string{"nightly", "beta"} {
		_, err := r.Resolve(ctx, spec)
		if err == nil || err.Error() != "rust channels other than stable are not supported; use an exact version or stable" {
			t.Errorf("%s: %v", spec, err)
		}
	}
	if _, err := r.Resolve(ctx, "1.81"); err == nil {
		t.Error("missing channel accepted")
	}
}

type cmdCall struct {
	name string
	args []string
	env  []string
}

func TestRustInstallBootstrapsPrivateRustup(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	host := "x86_64-unknown-linux-gnu"
	initBin := []byte("rustup-init-binary")
	url, _ := newServer(t, map[string]route{
		"/" + host + "/rustup-init":        {body: initBin},
		"/" + host + "/rustup-init.sha256": {body: []byte(shaBytes(initBin) + " *./rustup-init\n")},
	})
	setVar(t, &rustupDistURL, url)

	var calls []cmdCall
	setVar(t, &runCmd, func(_ context.Context, name string, args, extra []string) error {
		calls = append(calls, cmdCall{name, args, extra})
		root := filepath.Dir(strings.TrimPrefix(extra[1], "CARGO_HOME="))
		if filepath.Base(name) != "rustup" { // rustup-init: lay down cargo/bin/rustup
			if err := os.MkdirAll(filepath.Join(root, "cargo", "bin"), 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(root, "cargo", "bin", "rustup"), []byte("rustup"), 0o755)
		}
		if args[1] != "install" {
			return nil
		}
		bin := filepath.Join(root, "rustup", "toolchains", args[2]+"-"+host, "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			return err
		}
		for _, b := range []string{"rustc", "cargo"} {
			if err := os.WriteFile(filepath.Join(bin, b), []byte(b), 0o755); err != nil {
				return err
			}
		}
		return nil
	})

	req := installReq(t, "1.99.0")
	if err := (&RustInstaller{}).Install(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
	if strings.Join(calls[0].args, " ") != "-y --no-modify-path --default-toolchain none --profile minimal" {
		t.Fatalf("rustup-init args = %v", calls[0].args)
	}
	wantEnv := []string{"RUSTUP_HOME=" + filepath.Join(req.Root, "rustup"), "CARGO_HOME=" + filepath.Join(req.Root, "cargo"), "RUSTUP_INIT_SKIP_PATH_CHECK=yes"}
	if strings.Join(calls[0].env, "|") != strings.Join(wantEnv, "|") {
		t.Fatalf("env = %v", calls[0].env)
	}
	if calls[1].name != filepath.Join(req.Root, "cargo", "bin", "rustup") ||
		strings.Join(calls[1].args, " ") != "toolchain install 1.99.0 --profile minimal --no-self-update" {
		t.Fatalf("toolchain call = %+v", calls[1])
	}
	if got, _ := readFile(req.Dest, "bin/rustc"); got != "rustc" {
		t.Fatalf("bin/rustc = %q", got)
	}

	// Second toolchain: rustup is already there, no second bootstrap.
	calls = nil
	req2 := env.InstallRequest{Version: "1.80.1", Dest: t.TempDir(), Root: req.Root}
	if err := (&RustInstaller{}).Install(context.Background(), req2); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].args[0] != "toolchain" {
		t.Fatalf("calls = %+v", calls)
	}

	calls = nil
	if err := (&RustInstaller{}).Remove(context.Background(), "1.80.1", req2.Dest, req.Root); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || strings.Join(calls[0].args, " ") != "toolchain uninstall 1.80.1" {
		t.Fatalf("remove = %+v", calls)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run 'Rust' -count=1`
Expected: build failure: `undefined: parseRustChannelVersion`, `undefined: rustDistURL`, `undefined: runCmd` ...

- [ ] **Step 3: `internal/env/runtimes/cmd.go`**

```go
package runtimes

import (
	"context"
	"os"
	"os/exec"
)

// runCmd runs a helper tool (rustup, brew) with extra environment, its
// output going to the user. A seam: tests never run real tools.
var runCmd = func(ctx context.Context, name string, args, extraEnv []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

- [ ] **Step 4: Replace `internal/env/runtimes/rust.go`**

```go
package runtimes

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

var (
	rustDistURL   = "https://static.rust-lang.org/dist"
	rustupDistURL = "https://static.rust-lang.org/rustup/dist"

	rustMinorRe = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	rustExactRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

// RustInstaller installs Rust toolchains with a private rustup:
// RUSTUP_HOME=<root>/rustup, CARGO_HOME=<root>/cargo. It never touches
// ~/.rustup, ~/.cargo or shell profiles.
type RustInstaller struct{}

func init() {
	env.RegisterInstaller("rust", &RustInstaller{})
}

// Name returns the runtime name.
func (r *RustInstaller) Name() string { return "rust" }

// rustHostTriple maps GOOS/GOARCH to rustup's host triple.
func rustHostTriple(goos, goarch string) (string, error) {
	t := map[string]string{
		"darwin/arm64": "aarch64-apple-darwin",
		"darwin/amd64": "x86_64-apple-darwin",
		"linux/amd64":  "x86_64-unknown-linux-gnu",
		"linux/arm64":  "aarch64-unknown-linux-gnu",
	}[goos+"/"+goarch]
	if t == "" {
		return "", fmt.Errorf("xpm installs Rust only on macOS and Linux (x86_64, arm64), not %s/%s", goos, goarch)
	}
	return t, nil
}

// rustEnv points rustup at xpm's private homes.
func rustEnv(root string) []string {
	return []string{
		"RUSTUP_HOME=" + filepath.Join(root, "rustup"),
		"CARGO_HOME=" + filepath.Join(root, "cargo"),
		"RUSTUP_INIT_SKIP_PATH_CHECK=yes",
	}
}

func rustupPath(root string) string { return filepath.Join(root, "cargo", "bin", "rustup") }

// parseRustChannelVersion reads `version = "1.99.0 (b940084d7 2026-09-28)"`
// from the [pkg.rust] table of a channel-rust-*.toml manifest.
func parseRustChannelVersion(manifest string) (string, error) {
	inRust := false
	sc := bufio.NewScanner(strings.NewReader(manifest))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inRust = line == "[pkg.rust]"
			continue
		}
		if !inRust || !strings.HasPrefix(line, "version") {
			continue
		}
		_, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		s, err := strconv.Unquote(strings.TrimSpace(val))
		if err != nil {
			return "", fmt.Errorf("parse rust channel version %q: %w", val, err)
		}
		if f := strings.Fields(s); len(f) > 0 && rustExactRe.MatchString(f[0]) {
			return f[0], nil
		}
		return "", fmt.Errorf("unexpected rust channel version %q", s)
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", errors.New("rust channel manifest has no [pkg.rust] version")
}

// Resolve: "stable"/"latest" -> current stable; "1.80" -> newest 1.80.x;
// "1.80.1" as-is. Beta and nightly are not supported.
func (r *RustInstaller) Resolve(ctx context.Context, spec string) (string, error) {
	var channel string
	switch {
	case spec == "stable" || spec == "latest":
		channel = "stable"
	case spec == "beta" || spec == "nightly" || strings.HasPrefix(spec, "beta-") || strings.HasPrefix(spec, "nightly-"):
		return "", errors.New("rust channels other than stable are not supported; use an exact version or stable")
	case rustExactRe.MatchString(spec):
		return spec, nil
	case rustMinorRe.MatchString(spec):
		channel = spec
	default:
		return "", fmt.Errorf("unknown rust version %q: use stable, 1.80 or 1.80.1", spec)
	}
	body, err := fetchSmall(ctx, rustDistURL+"/channel-rust-"+channel+".toml")
	if isNotFound(err) {
		return "", fmt.Errorf("no rust release matches %s", spec)
	}
	if err != nil {
		return "", err
	}
	return parseRustChannelVersion(string(body))
}

// ListRemote returns only the current stable version; any 1.x.y released
// since 1.0 can be installed by exact version.
func (r *RustInstaller) ListRemote(ctx context.Context) ([]string, error) {
	v, err := r.Resolve(ctx, "stable")
	if err != nil {
		return nil, err
	}
	return []string{v}, nil
}

// bootstrapRustup installs rustup into <root>/cargo/bin once, from a
// checksum-verified rustup-init, without touching shell profiles.
func bootstrapRustup(ctx context.Context, root, host string) error {
	if _, err := os.Stat(rustupPath(root)); err == nil {
		return nil
	}
	url := rustupDistURL + "/" + host + "/rustup-init"
	sum, err := fetchSmall(ctx, url+".sha256")
	if err != nil {
		return fmt.Errorf("fetch rustup-init checksum: %w", err)
	}
	fields := strings.Fields(string(sum))
	if len(fields) == 0 {
		return errors.New("empty rustup-init checksum")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	initPath, err := downloadVerifiedTo(ctx, url, fields[0], root)
	if err != nil {
		return err
	}
	defer os.Remove(initPath)
	if err := os.Chmod(initPath, 0o755); err != nil {
		return err
	}
	args := []string{"-y", "--no-modify-path", "--default-toolchain", "none", "--profile", "minimal"}
	if err := runCmd(ctx, initPath, args, rustEnv(root)); err != nil {
		return fmt.Errorf("rustup-init: %w", err)
	}
	if _, err := os.Stat(rustupPath(root)); err != nil {
		return fmt.Errorf("rustup-init did not install %s", rustupPath(root))
	}
	return nil
}

// Install installs toolchain req.Version with the private rustup and links
// req.Dest/bin to the toolchain's bin directory.
func (r *RustInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	host, err := rustHostTriple(hostOS, hostArch)
	if err != nil {
		return err
	}
	if err := bootstrapRustup(ctx, req.Root, host); err != nil {
		return err
	}
	args := []string{"toolchain", "install", req.Version, "--profile", "minimal", "--no-self-update"}
	if err := runCmd(ctx, rustupPath(req.Root), args, rustEnv(req.Root)); err != nil {
		return fmt.Errorf("rustup toolchain install %s: %w", req.Version, err)
	}
	bin := filepath.Join(req.Root, "rustup", "toolchains", req.Version+"-"+host, "bin")
	if fi, err := os.Stat(bin); err != nil || !fi.IsDir() {
		return fmt.Errorf("rustup installed no toolchain at %s", bin)
	}
	return os.Symlink(bin, filepath.Join(req.Dest, "bin"))
}

// Remove uninstalls the toolchain from the private rustup.
func (r *RustInstaller) Remove(ctx context.Context, version, _, root string) error {
	if _, err := os.Stat(rustupPath(root)); err != nil {
		return nil // nothing to uninstall
	}
	return runCmd(ctx, rustupPath(root), []string{"toolchain", "uninstall", version}, rustEnv(root))
}

// BinaryPaths returns the Rust binaries.
func (r *RustInstaller) BinaryPaths() []string { return []string{"bin/rustc", "bin/cargo"} }
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/env/runtimes/ -run '.' -count=1`
Expected: `ok`.

- [ ] **Step 6: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 7: Commit (only this task's files)**

```bash
git add internal/env/runtimes/cmd.go \
  internal/env/runtimes/rust.go \
  internal/env/runtimes/rust_test.go
git commit -m "env/runtimes: rust through a private rustup"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 11: PHP through Homebrew links; the `latest` crash (R10)

**Files:**
- Modify (whole file): `internal/env/runtimes/php.go`
- Modify (hunks): `internal/env/runtimes/cmd.go` (`cmdOutput`, `lookPath`), `internal/env/runtimes/utils.go` (delete `copyDirectory`, now unused)
- Create: `internal/env/runtimes/php_test.go`

**Interfaces:**
- Consumes: Task 2 `fetchJSON`; Task 6 `hostOS`; Task 10 `runCmd`.
- Produces:
  - var `phpReleasesURL = "https://www.php.net/releases/index.php"`; seams `cmdOutput func(ctx context.Context, name string, args ...string) ([]byte, error)`, `lookPath = exec.LookPath`
  - `var errPHPNeedsBrew` = `PHP needs Homebrew on macOS (https://brew.sh); on Linux install PHP with your system package manager`
  - `func latestPHPMinor(ctx context.Context, major string) (string, error)`; `PHPInstaller` implements `env.Resolver`; `BinaryPaths() = bin/php` (absolute symlink to `<brew --prefix shivammathur/php/php@X.Y>/bin/php`)
  - Deleted: `sortVersions`, `compareVersions`, `ValidateVersion`, `GetLatestVersion`, `phpbrewExists`, `installViaPhpbrew`, `homebrewExists`, `installViaHomebrew`, `installPrebuiltMacOS`, `PostInstall`, `createMacOSWrappers`, every curl|bash hint; `copyDirectory` in `utils.go`.

- [ ] **Step 1: Write the failing tests**

`internal/env/runtimes/php_test.go`:

```go
package runtimes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

func phpServer(t *testing.T) {
	t.Helper()
	url, _ := newServer(t, map[string]route{
		"/releases/index.php?json&version=8": {body: []byte(`{"version":"8.5.11","date":"24 Sep 2026"}`)},
		"/releases/index.php?json&version=7": {body: []byte(`{"version":"7.4.33"}`)},
	})
	setVar(t, &phpReleasesURL, url+"/releases/index.php")
}

func TestPHPResolve(t *testing.T) {
	phpServer(t)
	p := &PHPInstaller{}
	ctx := context.Background()
	for spec, want := range map[string]string{"latest": "8.5", "8": "8.5", "7": "7.4", "8.3": "8.3"} {
		if v, err := env.ResolveSpec(ctx, p, spec); err != nil || v != want {
			t.Errorf("%s: %q, %v; want %s", spec, v, err, want)
		}
	}
	_, err := p.Resolve(ctx, "8.3.12")
	if err == nil || err.Error() != "PHP is installed per minor version through Homebrew; use php@8.3" {
		t.Fatalf("patch: %v", err)
	}
	got, err := p.ListRemote(ctx)
	if err != nil || strings.Join(got, " ") != "8.5 8.4 8.3 8.2 8.1 8.0 7.4" {
		t.Fatalf("ListRemote = %v, %v", got, err)
	}
}

func TestPHPNeedsBrew(t *testing.T) {
	setHost(t, "linux", "amd64")
	if err := (&PHPInstaller{}).Install(context.Background(), installReq(t, "8.3")); !errors.Is(err, errPHPNeedsBrew) {
		t.Fatalf("linux: %v", err)
	}
	setHost(t, "darwin", "arm64")
	setVar(t, &lookPath, func(string) (string, error) { return "", errors.New("not found") })
	err := (&PHPInstaller{}).Install(context.Background(), installReq(t, "8.3"))
	if err == nil || err.Error() != "PHP needs Homebrew on macOS (https://brew.sh); on Linux install PHP with your system package manager" {
		t.Fatalf("no brew: %v", err)
	}
}

func TestPHPInstallLinksHomebrewBinary(t *testing.T) {
	skipWindows(t)
	setHost(t, "darwin", "arm64")
	prefix := t.TempDir()
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "bin", "php"), []byte("php"), 0o755); err != nil {
		t.Fatal(err)
	}
	setVar(t, &lookPath, func(string) (string, error) { return "/opt/homebrew/bin/brew", nil })
	var ran []string
	setVar(t, &runCmd, func(_ context.Context, name string, args, _ []string) error {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return nil
	})
	setVar(t, &cmdOutput, func(_ context.Context, name string, args ...string) ([]byte, error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return []byte(prefix + "\n"), nil
	})
	dest := installInto(t, &PHPInstaller{}, "8.3")
	want := "/opt/homebrew/bin/brew install shivammathur/php/php@8.3|/opt/homebrew/bin/brew --prefix shivammathur/php/php@8.3"
	if strings.Join(ran, "|") != want {
		t.Fatalf("ran %v", ran)
	}
	if target, _ := os.Readlink(filepath.Join(dest, "bin", "php")); target != filepath.Join(prefix, "bin", "php") {
		t.Fatalf("bin/php -> %q", target)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run 'PHP' -count=1`
Expected: build failure: `undefined: phpReleasesURL`, `undefined: errPHPNeedsBrew`, `undefined: lookPath`, `undefined: cmdOutput`.

- [ ] **Step 3: Seams in `internal/env/runtimes/cmd.go`**

```diff
--- a/internal/env/runtimes/cmd.go
+++ b/internal/env/runtimes/cmd.go
@@ -15,3 +15,11 @@ var runCmd = func(ctx context.Context, name string, args, extraEnv []string) err
 	cmd.Stderr = os.Stderr
 	return cmd.Run()
 }
+
+// cmdOutput runs a helper tool and returns its stdout (brew --prefix).
+var cmdOutput = func(ctx context.Context, name string, args ...string) ([]byte, error) {
+	return exec.CommandContext(ctx, name, args...).Output()
+}
+
+// lookPath finds helper tools on PATH.
+var lookPath = exec.LookPath
```

- [ ] **Step 4: Replace `internal/env/runtimes/php.go`**

```go
package runtimes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/crenspire/xpm/internal/env"
)

// phpReleasesURL is php.net's release API.
var phpReleasesURL = "https://www.php.net/releases/index.php"

var (
	phpMajorRe = regexp.MustCompile(`^[0-9]+$`)
	phpMinorRe = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	phpPatchRe = regexp.MustCompile(`^([0-9]+\.[0-9]+)\.[0-9]+`)
)

// errPHPNeedsBrew is returned wherever xpm cannot install PHP itself.
var errPHPNeedsBrew = errors.New("PHP needs Homebrew on macOS (https://brew.sh); on Linux install PHP with your system package manager")

// PHPInstaller links Homebrew's shivammathur/php builds, one per minor
// version ("8.3"). macOS only.
type PHPInstaller struct{}

func init() {
	env.RegisterInstaller("php", &PHPInstaller{})
}

// Name returns the runtime name.
func (p *PHPInstaller) Name() string { return "php" }

// latestMinor asks php.net for the newest release of a major ("8" -> "8.5").
func latestPHPMinor(ctx context.Context, major string) (string, error) {
	var r struct {
		Version string `json:"version"`
	}
	if err := fetchJSON(ctx, phpReleasesURL+"?json&version="+major, &r); err != nil {
		return "", err
	}
	m := phpPatchRe.FindStringSubmatch(r.Version)
	if m == nil {
		return "", fmt.Errorf("php.net reported no release for PHP %s", major)
	}
	return m[1], nil
}

// Resolve: "latest" or a major -> its newest minor; "8.3" as-is; a patch
// version is refused (Homebrew installs one build per minor).
func (p *PHPInstaller) Resolve(ctx context.Context, spec string) (string, error) {
	switch {
	case spec == "latest":
		return latestPHPMinor(ctx, "8")
	case phpMajorRe.MatchString(spec):
		return latestPHPMinor(ctx, spec)
	case phpMinorRe.MatchString(spec):
		return spec, nil
	}
	if m := phpPatchRe.FindStringSubmatch(spec); m != nil {
		return "", fmt.Errorf("PHP is installed per minor version through Homebrew; use php@%s", m[1])
	}
	return "", fmt.Errorf("unknown PHP version %q: use latest, 8 or 8.3", spec)
}

// ListRemote returns 8.<latest> down to 8.0, then 7.4.
func (p *PHPInstaller) ListRemote(ctx context.Context) ([]string, error) {
	latest, err := latestPHPMinor(ctx, "8")
	if err != nil {
		return nil, err
	}
	_, minorStr, _ := strings.Cut(latest, ".")
	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		return nil, fmt.Errorf("php.net reported %q", latest)
	}
	var versions []string
	for m := minor; m >= 0; m-- {
		versions = append(versions, "8."+strconv.Itoa(m))
	}
	return append(versions, "7.4"), nil
}

// Install runs `brew install shivammathur/php/php@X.Y` and links
// req.Dest/bin/php to that formula's php (no copy, no wrapper scripts).
func (p *PHPInstaller) Install(ctx context.Context, req env.InstallRequest) error {
	if hostOS != "darwin" {
		return errPHPNeedsBrew
	}
	brew, err := lookPath("brew")
	if err != nil {
		return errPHPNeedsBrew
	}
	formula := "shivammathur/php/php@" + req.Version
	if err := runCmd(ctx, brew, []string{"install", formula}, nil); err != nil {
		return fmt.Errorf("brew install %s: %w", formula, err)
	}
	out, err := cmdOutput(ctx, brew, "--prefix", formula)
	if err != nil {
		return fmt.Errorf("brew --prefix %s: %w", formula, err)
	}
	php := filepath.Join(strings.TrimSpace(string(out)), "bin", "php")
	if fi, err := os.Stat(php); err != nil || fi.IsDir() {
		return fmt.Errorf("brew installed %s but %s is missing", formula, php)
	}
	if err := os.MkdirAll(filepath.Join(req.Dest, "bin"), 0o755); err != nil {
		return err
	}
	return os.Symlink(php, filepath.Join(req.Dest, "bin", "php"))
}

// BinaryPaths returns the PHP binary.
func (p *PHPInstaller) BinaryPaths() []string { return []string{"bin/php"} }
```

- [ ] **Step 5: Delete `copyDirectory` from `internal/env/runtimes/utils.go`**

Delete the whole function `copyDirectory` (from its doc comment `// copyDirectory copies a directory recursively, handling symlinks.` to the end of the file). Nothing references it any more: `grep -rn copyDirectory internal/` must print nothing.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/env/runtimes/ -run '.' -count=1`
Expected: `ok`.

- [ ] **Step 7: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 8: Commit (only this task's files)**

```bash
git add internal/env/runtimes/php.go \
  internal/env/runtimes/php_test.go \
  internal/env/runtimes/cmd.go \
  internal/env/runtimes/utils.go
git commit -m "env/runtimes: php through Homebrew links; fix the latest crash"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 12: `setup-path` (R11)

**Files:**
- Modify (whole file): `internal/env/path.go`
- Create: `internal/env/path_test.go`
- Modify (hunk): `internal/cli/env_cmd.go` (`cmdEnvSetupPath`)

**Interfaces:**
- Consumes: `Manager.GetShimsPath`; Task 3 `chdir`, `isolate`; Task 5 `skipOnWindows`.
- Produces:
  - `type ProfileEdit struct{ File, Line string; Changed bool }`
  - `func SetupPATH(m *Manager) (ProfileEdit, error)` (replaces `UpdatePATH`)
  - `func CheckPATH(m *Manager) bool` (exact entry match), `func onPATH(pathEnv, dir string) bool`
  - `func shellProfile(shell, home, goos string, getenv func(string) string) (file string, fish bool)`, `func pathLine(shims string, fish bool) string`, `func hasLine(file, line string) bool`

- [ ] **Step 1: Write the failing tests**

`internal/env/path_test.go`:

```go
package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellProfile(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	cases := []struct {
		shell, goos string
		envs        map[string]string
		want        string
		fish        bool
	}{
		{"zsh", "darwin", nil, "/h/.zshrc", false},
		{"zsh", "linux", map[string]string{"ZDOTDIR": "/z"}, "/z/.zshrc", false},
		{"bash", "linux", nil, "/h/.bashrc", false},
		{"bash", "darwin", nil, "/h/.bash_profile", false},
		{"fish", "linux", nil, "/h/.config/fish/conf.d/xpm.fish", true},
		{"fish", "linux", map[string]string{"XDG_CONFIG_HOME": "/x"}, "/x/fish/conf.d/xpm.fish", true},
		{"sh", "linux", nil, "/h/.profile", false},
		{"tcsh", "darwin", nil, "/h/.profile", false},
		{"", "linux", nil, "/h/.profile", false},
	}
	for _, c := range cases {
		env = c.envs
		file, fish := shellProfile(c.shell, "/h", c.goos, getenv)
		if file != filepath.FromSlash(c.want) || fish != c.fish {
			t.Errorf("%s/%s: got %s %v, want %s %v", c.shell, c.goos, file, fish, c.want, c.fish)
		}
	}
}

func TestPathLineEscapes(t *testing.T) {
	if got := pathLine(`/a b/"q"/$x/`+"`c`"+`/\`, false); got != `export PATH="/a b/\"q\"/\$x/\`+"`c\\`"+`/\\:$PATH"` {
		t.Errorf("sh line = %s", got)
	}
	if got := pathLine(`/it's/\`, true); got != `fish_add_path --prepend '/it\'s/\\'` {
		t.Errorf("fish line = %s", got)
	}
}

func TestSetupPATHAppendsOnceToOneFile(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	home, _ := os.UserHomeDir()
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")
	rc := filepath.Join(home, ".zshrc")
	// Mentioning the shims dir in a comment is not "configured".
	orig := "# old: " + m.GetShimsPath() + "\nalias ll='ls -l'"
	if err := os.WriteFile(rc, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	edit, err := SetupPATH(m)
	if err != nil || !edit.Changed || edit.File != rc {
		t.Fatalf("SetupPATH = %+v, %v", edit, err)
	}
	data, _ := os.ReadFile(rc)
	want := orig + "\n# Added by xpm\nexport PATH=\"" + m.GetShimsPath() + ":$PATH\"\n"
	if string(data) != want {
		t.Fatalf(".zshrc = %q, want %q", data, want)
	}
	if fi, _ := os.Stat(rc); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode changed to %v", fi.Mode().Perm())
	}
	edit, err = SetupPATH(m)
	if err != nil || edit.Changed {
		t.Fatalf("second SetupPATH = %+v, %v", edit, err)
	}
	if data2, _ := os.ReadFile(rc); string(data2) != want {
		t.Fatal("second run changed the file")
	}
	for _, other := range []string{".zprofile", ".bashrc", ".profile"} {
		if _, err := os.Stat(filepath.Join(home, other)); err == nil {
			t.Fatalf("%s was created", other)
		}
	}
}

func TestSetupPATHFishCreatesConfDir(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	home, _ := os.UserHomeDir()
	t.Setenv("SHELL", "/usr/local/bin/fish")
	t.Setenv("XDG_CONFIG_HOME", "")
	edit, err := SetupPATH(m)
	if err != nil {
		t.Fatal(err)
	}
	if edit.File != filepath.Join(home, ".config", "fish", "conf.d", "xpm.fish") {
		t.Fatalf("file = %s", edit.File)
	}
	data, _ := os.ReadFile(edit.File)
	if !strings.Contains(string(data), "fish_add_path --prepend '"+m.GetShimsPath()+"'") {
		t.Fatalf("content = %q", data)
	}
}

func TestOnPATHExactEntry(t *testing.T) {
	sep := string(os.PathListSeparator)
	shims := filepath.FromSlash("/h/.xpm/env/shims")
	cases := map[string]bool{
		shims + sep + "/usr/bin":                 true,
		"/usr/bin" + sep + shims + "/":           true,
		shims + "-old" + sep + "/usr/bin":        false,
		filepath.FromSlash("/h/.xpm/env/shimsx"): false,
		"":                                       false,
	}
	for p, want := range cases {
		if got := onPATH(p, shims); got != want {
			t.Errorf("onPATH(%q) = %v, want %v", p, got, want)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/ -run 'ShellProfile|PathLine|SetupPATH|OnPATH' -count=1`
Expected: build failure: `undefined: shellProfile`, `undefined: pathLine`, `undefined: SetupPATH`, `undefined: onPATH`.

- [ ] **Step 3: Replace `internal/env/path.go`**

```go
package env

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

// ProfileEdit reports what SetupPATH did.
type ProfileEdit struct {
	File    string // the one shell startup file xpm uses
	Line    string // the PATH line
	Changed bool   // false when File already had Line
}

// shellProfile picks the startup file for a shell (base name of $SHELL).
func shellProfile(shell, home, goos string, getenv func(string) string) (file string, fish bool) {
	switch shell {
	case "zsh":
		dir := getenv("ZDOTDIR")
		if dir == "" {
			dir = home
		}
		return filepath.Join(dir, ".zshrc"), false
	case "bash":
		if goos == "darwin" {
			return filepath.Join(home, ".bash_profile"), false
		}
		return filepath.Join(home, ".bashrc"), false
	case "fish":
		dir := getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".config")
		}
		return filepath.Join(dir, "fish", "conf.d", "xpm.fish"), true
	}
	return filepath.Join(home, ".profile"), false
}

// pathLine is the line that puts shims first on PATH.
func pathLine(shims string, fish bool) string {
	if fish {
		r := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
		return "fish_add_path --prepend '" + r.Replace(shims) + "'"
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")
	return `export PATH="` + r.Replace(shims) + `:$PATH"`
}

// hasLine reports whether file contains line (whitespace-trimmed, whole line).
func hasLine(file, line string) bool {
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}

// SetupPATH appends the shims PATH line to exactly one startup file for the
// user's shell ($SHELL), unless that file already has it.
func SetupPATH(m *Manager) (ProfileEdit, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ProfileEdit{}, err
	}
	file, fish := shellProfile(filepath.Base(os.Getenv("SHELL")), home, goruntime.GOOS, os.Getenv)
	edit := ProfileEdit{File: file, Line: pathLine(m.GetShimsPath(), fish)}
	if hasLine(file, edit.Line) {
		return edit, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return edit, err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return edit, err
	}
	if _, err := f.WriteString("\n# Added by xpm\n" + edit.Line + "\n"); err != nil {
		_ = f.Close()
		return edit, err
	}
	if err := f.Close(); err != nil {
		return edit, err
	}
	edit.Changed = true
	return edit, nil
}

// CheckPATH reports whether the shims dir is an exact PATH entry.
func CheckPATH(m *Manager) bool {
	return onPATH(os.Getenv("PATH"), m.GetShimsPath())
}

func onPATH(pathEnv, dir string) bool {
	want := filepath.Clean(dir)
	for _, p := range filepath.SplitList(pathEnv) {
		if p != "" && filepath.Clean(p) == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Point the CLI at `SetupPATH`** (`internal/cli/env_cmd.go`)

```diff
--- a/internal/cli/env_cmd.go
+++ b/internal/cli/env_cmd.go
@@ -214,10 +214,16 @@ func cmdEnvRemove(manager *env.Manager, args []string) int {
 
 // cmdEnvSetupPath handles `xpm env setup-path`.
 func cmdEnvSetupPath(manager *env.Manager) int {
-	if err := env.UpdatePATH(manager); err != nil {
+	edit, err := env.SetupPATH(manager)
+	if err != nil {
 		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
 		return 1
 	}
+	if !edit.Changed {
+		fmt.Printf("PATH already configured in %s\n", edit.File)
+		return 0
+	}
+	fmt.Printf("Added %s to PATH in %s\nRestart your shell or run: source %s\n", manager.GetShimsPath(), edit.File, edit.File)
 	return 0
 }
 
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/env/ -run '.' -count=1`
Expected: `ok`.

- [ ] **Step 6: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 7: Commit (only this task's files)**

```bash
git add internal/env/path.go \
  internal/env/path_test.go \
  internal/cli/env_cmd.go
git commit -m "env: setup-path edits one profile per shell; exact PATH match"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

### Task 13: `xpm env` CLI and README (R1, R13, R15)

**Files:**
- Modify (whole file): `internal/cli/env_cmd.go`
- Create: `internal/cli/env_cmd_test.go` (see "Ownership note" in the Design rulings section)
- Modify: `README.md`, only the `### Runtime versions (experimental)` section (from that heading up to, not including, `### Global flags`)

**Interfaces:**
- Consumes: everything above: `env.InstallRuntime`, `env.UseVersion`, `env.ListInstalled`/`FormatInstalled`, `env.ListRemote`/`FormatRemote`, `Manager.ActiveVersion`, `env.ErrNoVersion`/`ErrNotInstalled`, `env.RemoveVersion`, `env.CreateShims`, `env.SetupPATH`, `env.CheckPATH`, `env.ListRuntimes`, `Manager.SetExecutable`.
- Produces: seams `envGOOS = runtime.GOOS`, `newEnvManager = env.NewManager`; `func parseUseArgs(args []string) (spec string, global bool, err error)`; `func writeCurrent(m *env.Manager, out, errOut io.Writer)`. Exit codes: 0 ok, 1 error/usage/disabled/Windows, 130 install cancelled.

- [ ] **Step 1: Write the failing tests**

`internal/cli/env_cmd_test.go` (uses the package's existing `isolatedHome`, `chdir`, `captureStdout`, `captureStderr`, `writeConfig` helpers; registers a fake runtime `xpmclifake`, so no network):

```go
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/env"
)

// cliFake is a test runtime; the registry is global, so register it once.
type cliFake struct{}

var (
	cliFakeOnce    sync.Once
	cliFakeInstall func(ctx context.Context, req env.InstallRequest) error
)

func (cliFake) Name() string                                 { return "xpmclifake" }
func (cliFake) ListRemote(context.Context) ([]string, error) { return []string{"1.0.0", "1.1.0"}, nil }
func (cliFake) BinaryPaths() []string                        { return []string{"bin/clifakebin"} }
func (cliFake) Install(ctx context.Context, req env.InstallRequest) error {
	if cliFakeInstall != nil {
		return cliFakeInstall(ctx, req)
	}
	if err := os.MkdirAll(filepath.Join(req.Dest, "bin"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(req.Dest, "bin", "clifakebin"), []byte("#!/bin/sh\n"), 0o755)
}

// envTest isolates HOME and cwd, registers the fake runtime and makes
// shims point at a fake xpm path. It returns the project dir and env root.
func envTest(t *testing.T) (proj, root string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("xpm env is Unix-only")
	}
	isolatedHome(t)
	chdir(t, t.TempDir())
	proj, _ = os.Getwd() // resolved (/private/var vs /var on macOS)
	cliFakeOnce.Do(func() { env.RegisterInstaller("xpmclifake", cliFake{}) })
	old := newEnvManager
	exe := filepath.Join(t.TempDir(), "xpm")
	newEnvManager = func(c config.Config) (*env.Manager, error) {
		m, err := env.NewManager(c)
		if err == nil {
			m.SetExecutable(exe)
		}
		return m, err
	}
	t.Cleanup(func() { newEnvManager = old; cliFakeInstall = nil })
	home, _ := os.UserHomeDir()
	return proj, filepath.Join(home, ".xpm", "env")
}

func TestEnvDisabledOnWindows(t *testing.T) {
	old := envGOOS
	envGOOS = "windows"
	t.Cleanup(func() { envGOOS = old })
	var code int
	errOut := captureStderr(t, func() { code = cmdEnv([]string{"list"}) })
	if code != 1 || errOut != "xpm env is not supported on Windows yet: runtime downloads, shims and PATH setup are Unix-only (macOS, Linux).\n" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestParseUseArgs(t *testing.T) {
	cases := []struct {
		args   []string
		spec   string
		global bool
		ok     bool
	}{
		{[]string{"node@20"}, "node@20", false, true},
		{[]string{"--global", "node@20"}, "node@20", true, true},
		{[]string{"node@20", "-g"}, "node@20", true, true},
		{[]string{"node@20", "go@1.22"}, "", false, false},
		{[]string{"--force", "node@20"}, "", false, false},
		{nil, "", false, false},
	}
	for _, c := range cases {
		spec, global, err := parseUseArgs(c.args)
		if (err == nil) != c.ok || spec != c.spec || global != c.global {
			t.Errorf("%v: %q %v %v", c.args, spec, global, err)
		}
	}
}

func TestEnvInstallSetsGlobalDefaultAndShims(t *testing.T) {
	proj, root := envTest(t)
	var code int
	out := captureStdout(t, func() { code = cmdEnv([]string{"install", "xpmclifake@1"}) })
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, want := range []string{"Resolved xpmclifake@1 to 1.1.0", "Set xpmclifake@1.1.0 as the global default", "xpm env setup-path"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(proj, ".xpm-env")); err == nil {
		t.Fatal("install wrote .xpm-env")
	}
	if _, err := os.Readlink(filepath.Join(root, "shims", "clifakebin")); err != nil {
		t.Fatalf("shim not created: %v", err)
	}

	out = captureStdout(t, func() { code = cmdEnv([]string{"use", "xpmclifake@1.1.0"}) })
	if code != 0 || !strings.Contains(out, "Using xpmclifake@1.1.0 ("+filepath.Join(proj, ".xpm-env")+")") {
		t.Fatalf("use: exit %d\n%s", code, out)
	}
}

func TestEnvInstallCancelledExits130(t *testing.T) {
	_, root := envTest(t)
	cliFakeInstall = func(context.Context, env.InstallRequest) error { return context.Canceled }
	var code int
	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() { code = cmdEnv([]string{"install", "xpmclifake@1.0.0"}) })
	})
	if code != 130 || !strings.Contains(errOut, "Cancelled; nothing was installed.") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "runtimes", "xpmclifake"))
	for _, e := range entries {
		if e.Name() != ".lock" {
			t.Fatalf("left behind: %s", e.Name())
		}
	}
}

func TestEnvCurrentListsSortedWithSourceAndWarns(t *testing.T) {
	proj, _ := envTest(t)
	_ = captureStdout(t, func() { cmdEnv([]string{"install", "xpmclifake@1.0.0"}) })
	m, err := newEnvManager(config.Load())
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	writeCurrent(m, &out, &errOut)
	if !strings.Contains(out.String(), "xpmclifake 1.0.0 ("+m.GetActivePath()+")\n") {
		t.Fatalf("stdout %q", out.String())
	}

	if err := os.WriteFile(filepath.Join(proj, ".xpm-env"), []byte("xpmclifake=9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	writeCurrent(m, &out, &errOut)
	if !strings.Contains(out.String(), "xpmclifake 9 (") || !strings.Contains(errOut.String(), "xpm env install xpmclifake@9") {
		t.Fatalf("stdout %q stderr %q", out.String(), errOut.String())
	}
}

func TestEnvDisabledByConfig(t *testing.T) {
	envTest(t)
	writeConfig(t, `{"env":{"enabled":false}}`)
	var code int
	errOut := captureStderr(t, func() { code = cmdEnv([]string{"list"}) })
	if code != 1 || !strings.Contains(errOut, "disabled") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ -run 'Env|ParseUseArgs' -count=1`
Expected: build failure: `undefined: envGOOS`, `undefined: newEnvManager`, `undefined: parseUseArgs`, `undefined: writeCurrent`.

- [ ] **Step 3: Replace `internal/cli/env_cmd.go`**

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	goruntime "runtime"
	"strings"
	"syscall"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/env"
	_ "github.com/crenspire/xpm/internal/env/runtimes" // register runtime installers
)

// Seams for tests.
var (
	envGOOS       = goruntime.GOOS
	newEnvManager = env.NewManager
)

const envUsage = `Usage: xpm env <command>

Commands:
  install <runtime>@<version>          Install a version (does not touch .xpm-env)
  use <runtime>@<version> [--global]   Pin a version here (.xpm-env) or globally
  list                                 List installed versions
  ls-remote <runtime>                  List available versions (newest 20)
  current                              Show the version in effect for each runtime
  remove <runtime>@<version>           Remove an installed version
  reshim                               Recreate the shims (links to xpm)
  setup-path                           Put the shims directory on your shell's PATH

Runtimes: node, go, python, java, rust, bun, deno, php
Versions: exact (20.11.0), partial (20), latest, lts (node, java)
`

// cmdEnv handles the `xpm env` command group.
func cmdEnv(args []string) int {
	if envGOOS == "windows" {
		fmt.Fprintln(os.Stderr, "xpm env is not supported on Windows yet: runtime downloads, shims and PATH setup are Unix-only (macOS, Linux).")
		return 1
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, envUsage)
		return 1
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(envUsage)
		return 0
	}

	cfg := config.Load()
	if !cfg.Env.Enabled {
		fmt.Fprintln(os.Stderr, "Runtime version management is disabled (env.enabled is false in your xpm config).")
		return 1
	}
	m, err := newEnvManager(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "install":
		return cmdEnvInstall(m, rest)
	case "use":
		return cmdEnvUse(m, rest)
	case "list", "ls":
		return cmdEnvList(m, rest)
	case "ls-remote":
		return cmdEnvListRemote(rest)
	case "current":
		return cmdEnvCurrent(m, rest)
	case "remove", "rm", "uninstall":
		return cmdEnvRemove(m, rest)
	case "reshim":
		return cmdEnvReshim(m, rest)
	case "setup-path":
		return cmdEnvSetupPath(m, rest)
	}
	fmt.Fprintf(os.Stderr, "Unknown env command: %s\n\n%s", sub, envUsage)
	return 1
}

// envUsageError prints a one-line usage for a subcommand.
func envUsageError(usage string) int {
	fmt.Fprintf(os.Stderr, "Usage: xpm env %s\n", usage)
	return 1
}

// interruptible returns a context cancelled by Ctrl-C or SIGTERM.
func interruptible() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// reshim recreates the shim set; a failure is a warning, not a failed command.
func reshim(m *env.Manager) {
	if err := env.CreateShims(m); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update shims: %v\n", err)
	}
}

// pathHint tells the user to run setup-path while shims are not on PATH.
func pathHint(m *env.Manager) {
	if !env.CheckPATH(m) {
		fmt.Printf("\nShims are not on your PATH yet. Run: xpm env setup-path\n")
	}
}

func cmdEnvInstall(m *env.Manager, args []string) int {
	if len(args) != 1 {
		return envUsageError("install <runtime>@<version>   (e.g. node@20, go@latest, java@lts)")
	}
	rt, spec, err := parseRuntimeVersion(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	ctx, stop := interruptible()
	defer stop()
	if _, err := env.InstallRuntime(ctx, m, rt, spec); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "Cancelled; nothing was installed.")
			return 130
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	reshim(m)
	pathHint(m)
	return 0
}

// parseUseArgs accepts --global/-g anywhere and exactly one runtime@version.
func parseUseArgs(args []string) (spec string, global bool, err error) {
	var specs []string
	for _, a := range args {
		switch {
		case a == "--global" || a == "-g":
			global = true
		case strings.HasPrefix(a, "-"):
			return "", false, fmt.Errorf("unknown flag %s", a)
		default:
			specs = append(specs, a)
		}
	}
	if len(specs) != 1 {
		return "", false, errors.New("expected exactly one <runtime>@<version>")
	}
	return specs[0], global, nil
}

func cmdEnvUse(m *env.Manager, args []string) int {
	arg, global, err := parseUseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return envUsageError("use <runtime>@<version> [--global]")
	}
	rt, spec, err := parseRuntimeVersion(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	a, err := env.UseVersion(m, rt, spec, global)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Using %s@%s (%s)\n", rt, a.Version, a.Source)
	if global {
		if here, err := m.ActiveVersion(rt); err == nil && !here.Global {
			fmt.Printf("Note: %s pins %s@%s in this directory\n", here.Source, rt, here.Version)
		}
	}
	reshim(m)
	pathHint(m)
	return 0
}

func cmdEnvList(m *env.Manager, args []string) int {
	if len(args) != 0 {
		return envUsageError("list")
	}
	installed, err := env.ListInstalled(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Print(env.FormatInstalled(installed))
	return 0
}

func cmdEnvListRemote(args []string) int {
	if len(args) != 1 {
		return envUsageError("ls-remote <runtime>")
	}
	if err := env.ValidateRuntimeName(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	ctx, stop := interruptible()
	defer stop()
	versions, err := env.ListRemote(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Print(env.FormatRemote(args[0], versions))
	return 0
}

// writeCurrent prints "<rt> <version> (<source>)" per configured runtime,
// sorted by runtime; problems go to errOut as warnings.
func writeCurrent(m *env.Manager, out, errOut io.Writer) {
	found := false
	for _, rt := range env.ListRuntimes() {
		a, err := m.ActiveVersion(rt)
		switch {
		case errors.Is(err, env.ErrNoVersion):
			continue
		case errors.Is(err, env.ErrNotInstalled):
			_, _ = fmt.Fprintf(out, "%s %s (%s)\n", rt, a.Version, a.Source)
			_, _ = fmt.Fprintf(errOut, "warning: %v; run: xpm env install %s@%s\n", err, rt, a.Version)
		case err != nil:
			_, _ = fmt.Fprintf(errOut, "warning: %v\n", err)
			continue
		default:
			_, _ = fmt.Fprintf(out, "%s %s (%s)\n", rt, a.Version, a.Source)
		}
		found = true
	}
	if !found {
		_, _ = fmt.Fprintln(out, "No runtime versions configured. Install one: xpm env install <runtime>@<version>")
	}
}

func cmdEnvCurrent(m *env.Manager, args []string) int {
	if len(args) != 0 {
		return envUsageError("current")
	}
	writeCurrent(m, os.Stdout, os.Stderr)
	return 0
}

func cmdEnvRemove(m *env.Manager, args []string) int {
	if len(args) != 1 {
		return envUsageError("remove <runtime>@<version>")
	}
	rt, version, err := parseRuntimeVersion(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	ctx, stop := interruptible()
	defer stop()
	if err := env.RemoveVersion(ctx, m, rt, version); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Removed %s@%s\n", rt, version)
	reshim(m)
	return 0
}

func cmdEnvReshim(m *env.Manager, args []string) int {
	if len(args) != 0 {
		return envUsageError("reshim")
	}
	if err := env.CreateShims(m); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Printf("Shims updated in %s\n", m.GetShimsPath())
	return 0
}

func cmdEnvSetupPath(m *env.Manager, args []string) int {
	if len(args) != 0 {
		return envUsageError("setup-path")
	}
	edit, err := env.SetupPATH(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if !edit.Changed {
		fmt.Printf("PATH already configured in %s\n", edit.File)
		return 0
	}
	fmt.Printf("Added %s to PATH in %s\nRestart your shell or run: source %s\n", m.GetShimsPath(), edit.File, edit.File)
	return 0
}

// parseRuntimeVersion parses and validates a runtime@version specification.
func parseRuntimeVersion(spec string) (runtime, version string, err error) {
	runtime, version, ok := strings.Cut(spec, "@")
	if !ok {
		return "", "", fmt.Errorf("invalid format %q: expected <runtime>@<version>", spec)
	}
	if err := env.ValidateRuntimeName(runtime); err != nil {
		return "", "", err
	}
	if err := env.ValidateVersionSpec(version); err != nil {
		return "", "", err
	}
	return runtime, version, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run '.' -count=1`
Expected: `ok`, including the untouched `TestRunHelpWritesNoFiles` in `startup_test.go`.

- [ ] **Step 5: Rewrite the README section**

Replace everything from `### Runtime versions (experimental)` up to (not including) `### Global flags` with the block below. Leave the supported-platforms table row near README line 56 alone (P4 owns it).

````markdown
### Runtime versions (experimental)

macOS and Linux only; on Windows `xpm env` exits with an error.

```bash
xpm env install node@20        # newest 20.x; also node@20.11.0, node@lts, node@latest
xpm env use node@20            # pin in ./.xpm-env (exact installed version written)
xpm env use --global go@1.26   # default everywhere else (~/.xpm/env/active.json)
xpm env setup-path             # put ~/.xpm/env/shims first on PATH (one shell file)
xpm env current                # version in effect here, and which file set it
xpm env list                   # installed versions, newest first
xpm env ls-remote python       # newest 20 available versions
xpm env remove node@20.11.0
xpm env reshim                 # recreate the shims (e.g. after moving the xpm binary)
```

| Runtime | Source | Verified with |
|---|---|---|
| node | nodejs.org | `SHASUMS256.txt` |
| go | go.dev | SHA-256 from the go.dev release feed |
| python | [python-build-standalone](https://github.com/astral-sh/python-build-standalone) (newest release only) | `SHA256SUMS` |
| java | Eclipse Temurin (Adoptium API); `java@21`, `java@lts` | Adoptium package checksum |
| bun | GitHub releases | `SHASUMS256.txt` |
| deno | GitHub releases (2.0.6+) | per-file `.sha256sum` |
| rust | a private rustup in `~/.xpm/env` (never touches `~/.cargo` or your shell profile); `rust@stable`, `rust@1.80` | rustup-init `.sha256`, then rustup |
| php | Homebrew `shivammathur/php` on macOS, per minor version (`php@8.3`) | Homebrew |

How it works:
- **Install never pins.** `install` downloads, verifies and unpacks into a staging directory, then renames it into place, so an interrupted install (Ctrl-C exits 130) leaves nothing behind. The first version of a runtime becomes the global default; `install` never writes `.xpm-env`.
- **`use` vs `use --global`.** `use` writes `.xpm-env` in the current directory (comments and order are kept); `--global` writes `active.json`. Lookup order: the nearest `.xpm-env` up from the current directory that names the runtime, then `active.json`. Values may be exact (`20.11.0`), partial (`20`), `latest` or `lts`, matched against installed versions.
- **Shims are links to xpm itself** (`~/.xpm/env/shims/node -> xpm`): no Go toolchain needed. A shim runs the version in effect, with that version's `bin` first on `PATH`. If nothing is configured it runs the system binary from `PATH`; if `.xpm-env` names a version that is not installed it fails (exit 127) and tells you what to install.
- **`setup-path`** appends one line to one file for your `$SHELL`: `~/.zshrc` (or `$ZDOTDIR/.zshrc`), `~/.bashrc` (Linux) / `~/.bash_profile` (macOS), `~/.config/fish/conf.d/xpm.fish`, or `~/.profile`. Running it again changes nothing.
- GitHub API calls (bun, deno version lists) use `GITHUB_TOKEN` when set; partial versions match the 100 most recent releases.
````

Check every claim against the code before committing: sources and checksums (Tasks 6–11), `install` never writes `.xpm-env` and sets the first global default (Task 4), lookup order (Task 3), exit 127 and system fallback (Task 5), exit 130 (Task 4/13), one profile file (Task 12), `GITHUB_TOKEN` and the 100-release window (Task 2).

- [ ] **Step 6: Manual smoke test (network; optional, never in CI)**

In a throwaway HOME, with no Go toolchain on PATH, the roadmap exit criterion:

```bash
go build -o /tmp/xpm-p5 ./cmd/xpm
export H=$(mktemp -d) P=$(mktemp -d)
cd "$P" && HOME=$H /tmp/xpm-p5 env install node@20 && HOME=$H /tmp/xpm-p5 env use node@20
HOME=$H PATH=$H/.xpm/env/shims:/usr/bin:/bin node -v      # v20.x
HOME=$H PATH=$H/.xpm/env/shims:/usr/bin:/bin npm -v       # npm's `#!/usr/bin/env node` hits the same node
HOME=$H /tmp/xpm-p5 env current
```
(Done during planning on 2026-10-07 on darwin/arm64 for node@20, python@3.12, java@21, bun@latest, deno@2 and go@1.26: all ran through the shims.)

- [ ] **Step 7: Run the full gate**

From the worktree root:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l . && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0 run ./...
```
Expected: every package `ok` (or `[no test files]`), `gofmt -l .` prints nothing, golangci-lint prints `0 issues.`

- [ ] **Step 8: Commit (only this task's files)**

```bash
git add internal/cli/env_cmd.go \
  internal/cli/env_cmd_test.go \
  README.md
git commit -m "env: CLI rework (Windows gate, reshim, Ctrl-C exit 130) and README"
```
(No Co-Authored-By, "Generated with" or Claude-Session lines: the commit-msg hook rejects them.)

---

## Self-review notes

**Spec coverage: rulings.**

| Ruling | Where |
|---|---|
| R1 Windows gate, compiles, Windows asset names | Task 13 (`TestEnvDisabledOnWindows`); Task 5 (`ShimName` false, `exec_windows.go`); Tasks 6–8 asset tables with Windows rows; every Unix-only test skips |
| R2 symlink shims, union of BinaryPaths, prune, atomic replace, reshim | Task 5 (`CreateShims`, `linkShim`); Task 13 (`reshim` subcommand, reshim after install/use/remove) |
| R3 dispatch, fallback, messages, depth, exec | Task 5 (all seven steps have a test) |
| R4 atomic install, lock, staging, meta, Ctrl-C 130 | Task 4; Task 13 (`TestEnvInstallCancelledExits130`) |
| R5 resolver, sentinels, fail closed | Task 3; Task 5 uses it in shims; Task 13 `current` |
| R6 atomic active.json / .xpm-env, install never pins, global default, `use` | Tasks 3, 4 |
| R7 semver | Task 1 |
| R8 interface v2, ResolveSpec rules, messages | Task 4; installers in Tasks 6–11 |
| R9 helpers, 8 MiB error, GitHub token, suffix dispatch, hoistDir, checkRelativeLink, copyDirectory gone | Task 2; Task 11 deletes `copyDirectory` |
| R10 installers | Tasks 6 (node, go), 7 (bun, deno), 8 (java), 9 (python), 10 (rust), 11 (php) |
| R11 setup-path | Task 12 |
| R12 dead code | Tasks 3 (detect.go, defaults, string helpers, getSystemPHPVersion), 5 (template compiler), 6, 9, 10, 11 |
| R13 CLI | Task 13 |
| R14 script runner | out of scope (recorded) |
| R15 README | Task 13 Step 5 |

**Spec coverage: roadmap P5 items.**
1. busybox shims, `syscall.Exec`, no Go toolchain: Task 5.
2. system fallback: Task 5.
3. atomic installs, lock, atomic state writes with comments kept: Tasks 3, 4.
4. resolve before `dest`, prefix matching, semver sort: Tasks 1, 4.
5. installer fixes:
   - Go `include=all` + Windows zip: Task 6;
   - Bun tag prefix + Windows name: Task 7;
   - Deno root binary: Task 7;
   - Java `Contents/Home`: Task 8;
   - PHP `latest` crash: Task 11;
   - Rust private rustup with `--no-modify-path`: Task 10;
   - Python via python-build-standalone: Task 9.
6. checksums for Bun/Deno/Java: Tasks 7, 8.
7. setup-path: Task 12.
8. script runner: R14, out of scope.
9. Windows decision: R1, Task 13.

Exit criteria:
- `node -v` through a shim with no Go toolchain: Task 13 Step 6, run during planning.
- shim < 5 ms: no network, no subprocess, a handful of stats; measured 37 ms for `node -e 0` total, including node's own startup.
- Ctrl-C leaves no partial version: `TestInstallRuntimeCancelledLeavesNothing`, `TestEnvInstallCancelledExits130`.

**Contradictions found in the brief and how this plan resolves them.**
- R10 java says the release_name endpoint gives `binary.package.*`. The live API returns `binaries[0].package` there. Task 8 reads `binaries[0].package` and its test fixture uses the live shape.
- R4 "then block" vs Ctrl-C: a blocking flock cannot be interrupted once `signal.NotifyContext` owns SIGINT. Task 4 polls with `LOCK_NB` against the context instead.
- R9 "fetchSmall caps at 8 MiB" vs GitHub release lists of 4–6 MB: fetchSmall keeps 8 MiB; the releases list uses its own 32 MiB cap.
- R9 "extractArchive(archivePath, dest) by suffix" vs `downloadVerified` writing `xpm-dl-*` with no suffix: the temp file now keeps the URL's archive suffix.

**Placeholder scan.** No TBD/TODO. Every code step shows the code (whole file, or a diff generated from the verified commits). "Mechanical" adaptations in Tasks 2 and 4 come with their exact diffs.

**Type consistency.** These names were checked across tasks against the compiled code:
- `Active`, `ErrNoVersion`, `ErrNotInstalled`, `InstallRequest`, `Resolver`, `LTSResolver`, `Remover`;
- `ResolveSpec`, `InstallRuntime(ctx, m, rt, spec) (string, error)`, `UseVersion(m, rt, spec, global) (Active, error)`, `RemoveVersion(ctx, m, rt, v) error`, `ListRemote(ctx, rt)`;
- `CreateShims(m) error`, `SetupPATH(m) (ProfileEdit, error)`, `CheckPATH(m) bool`;
- `fetchSmall`/`fetchJSON`/`fetchGitHubReleases`/`downloadVerified(ctx, …)`, `hoistDir(dest, sub)`, `hostOS`/`hostArch`, `runCmd`/`cmdOutput`/`lookPath`.

**Review Focus check.** Each of the five lines names a test in the task that owns the code. Two of those tests were added for this review: `TestInstallRuntimeWaitsForConcurrentInstall` (Task 4) and `TestInstalledVersionsIgnoresLegacyAliasDirs` (Task 3, with the `ParseVersion` filter in `InstalledVersions`).
