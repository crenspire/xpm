<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
    <img alt="xpm — one CLI for every ecosystem" src="docs/assets/logo-light.svg" width="420">
  </picture>
</p>

<p align="center">
  <b>One package manager CLI for npm, pip, Composer, Cargo, Maven and Go.</b><br>
  Search every registry at once, install with one syntax, run any project's scripts.
</p>

<p align="center">
  <a href="https://github.com/crenspire/xpm/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/crenspire/xpm/actions/workflows/ci.yml/badge.svg?branch=develop"></a>
  <a href="https://crenspire.github.io/xpm/"><img alt="Website" src="https://img.shields.io/badge/site-crenspire.github.io%2Fxpm-7c3aed"></a>
  <img alt="Go 1.22+" src="https://img.shields.io/badge/go-1.22%2B-00ADD8">
  <a href="LICENSE"><img alt="MIT" src="https://img.shields.io/badge/license-MIT-22d3ee"></a>
</p>

---

```console
$ xpm which axios
Searching for "axios"...

Found in:
- cargo: axios @0.1.0 - A simple HTTP client for Rust
- composer: swlib/saber - Swoole coroutine HTTP client
- maven: org.mvnpm.at.nestjs:axios @12.0.1 - Maven artifact
- npm: axios @1.20.0 - Promise based HTTP client for the browser and node.js
  → Also available via: yarn, pnpm, bun
- pip: axios @0.4.0 - Command line utility to access https://family.axioscloud.it

$ xpm install axios          # picks the right tool for this project (npm/yarn/pnpm/bun …)
$ xpm install requests@2.31  # in a Python project → pip install requests==2.31
$ xpm run test               # runs package.json / composer.json / pyproject scripts
```

## Why xpm

- **Fast.** All registries are queried in parallel with a 2.5 s deadline by default, and answers are cached on disk. A first lookup takes about a second; a repeat lookup takes under 10 ms.
- **One syntax.** `name@version` works everywhere: xpm translates it to `npm i name@v`, `pip install name==v`, `composer require name:v`, `cargo add name@v` or `go get name@v`.
- **Respects your project.** Inside a project, `xpm install <name>` uses the project's own ecosystem (`xpm install phpunit` next to `composer.json` runs `composer require phpunit/phpunit`, not npm's squatter), and lockfiles pick the tool inside it (`yarn.lock` → yarn, `poetry.lock` → poetry, `build.gradle` → Gradle) for `install`, `list`, `update`, `remove` and `ci`. Outside a project, a name that exists in several ecosystems is your choice (or your `prefer` list's).
- **Safe by default.** Package names that look like flags are rejected, runtime downloads are checked against published SHA-256 checksums, and archive extraction cannot write outside its folder.

## Status

xpm is young. The core commands are solid; the bigger subsystems are being rebuilt and are marked **experimental**. See the [roadmap](docs/superpowers/plans/2026-10-06-xpm-roadmap.md).

| Area | Commands | Status |
|---|---|---|
| Cross-registry search & lookup | `which`, `search`, `info` | ✅ Stable |
| Install / update / remove | `install`, `ci`, `update`, `remove`, `list` | ✅ Stable (Go modules and Gradle included) |
| Project scripts | `run` | ✅ Stable |
| Diagnostics | `doctor` | ✅ Stable |
| Runtime versions (node, go, …) | `env` | 🧪 Experimental: Node and Go are checksum-verified; Rust needs rustup; others in progress |
| Dependency graph | `graph` | 🧪 Experimental: parses npm, pnpm, yarn, Cargo, Go, Poetry, pyproject.toml, requirements.txt, Composer, Maven and Gradle files; runs build tools only with `--exec` |
| Unified lockfile | `lock` | 🧪 Experimental: records lockfile hashes in `xpm-lock.yaml`; `--verify` detects changed, added and removed lockfiles and fails on entries it cannot check |
| Monorepos | `workspaces`, `run --workspace`, `graph --workspace` | 🧪 Experimental: `xpm install --workspace` is not wired yet |

## Install

```bash
go install github.com/crenspire/xpm/cmd/xpm@latest
```

Or build from source:

```bash
git clone https://github.com/crenspire/xpm.git && cd xpm
make build          # stripped binary at ./xpm (~10–11 MB)
sudo mv xpm /usr/local/bin/
```

## Supported package managers

| Ecosystem | Tools | Lockfile that selects it | Registry searched |
|---|---|---|---|
| Node.js | npm, yarn, pnpm, bun | `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lock` / `bun.lockb` | npm |
| Python | pip, poetry, pipenv | `requirements.txt`, `poetry.lock`, `Pipfile` / `Pipfile.lock` | PyPI |
| PHP | Composer | `composer.json` | Packagist |
| Rust | Cargo | `Cargo.toml` | crates.io |
| Java | Maven, Gradle | `pom.xml` / `build.gradle(.kts)` pick the tool (xpm prints the dependency snippet) | Maven Central |
| Go | Go modules | `go.mod` | — (`xpm install github.com/x/y` runs `go get` directly) |

## Supported platforms

| OS | Core commands (`which`, `search`, `info`, `install`, `ci`, `list`, `update`, `remove`, `config`) | `run` | `search` TUI | `env` (experimental) |
|---|---|---|---|---|
| Linux | Yes, tested in CI | Yes (`sh -c` for pyproject/Cargo scripts) | Yes | Experimental: Node and Go verified; shims need `go` on PATH |
| macOS | Yes, tested in CI | Yes (`sh -c` for pyproject/Cargo scripts) | Yes | Experimental: Node and Go verified; shims need `go` on PATH |
| Windows | Yes, tested in CI | Needs `sh` on PATH (Git for Windows or WSL) for pyproject/Cargo scripts; package.json and composer.json scripts do not | Yes, in Windows Terminal or another modern console | Untested (see roadmap P5) |

Windows support for the core commands is covered by CI but is used less in practice than Linux and macOS. The TUI needs a terminal; without one `xpm search` prints plain output.

## Usage

`xpm help` lists every command and marks the experimental ones; `xpm man <command>` has the details for one command. The everyday commands are `which`, `search`, `info`, `install`, `ci`, `list`, `update`, `remove`, `run` and `config`.

### Find a package everywhere

```bash
xpm which lodash        # which registries have it, latest versions, descriptions
xpm search http client  # several words are one query; opens the interactive TUI on a terminal, plain output otherwise
xpm info serde          # details for one package
```

If a registry doesn't answer within 2.5 s, xpm shows what the others found. `xpm which`, `xpm info` and plain `xpm search` list that registry under **Unavailable**, separately from **Not found in**. If every registry fails (for example, you're offline), the command exits with an error rather than claiming "no matches". When nothing matches, `xpm which`, `xpm info`, `xpm search` and `xpm install` exit 1.

### Install

```bash
# in a Node project (package.json): the project's ecosystem and lockfile pick the tool
xpm install axios               # search, then install with the right tool
xpm install axios@1.7.0         # pin a version (universal @ syntax); without @ the tool picks its latest
xpm install axios lodash        # several packages, in order (stops at the first failure)

# global installs ignore the current project; prefer settles names found in several ecosystems
xpm config set prefer npm
xpm install -g typescript       # → npm install -g typescript (-g may also come last)
xpm install -g golang.org/x/tools/gopls   # Go: go install golang.org/x/tools/gopls@latest
xpm install github.com/gin-gonic/gin   # Go module path: runs go get, no registry search
xpm install                     # no args: install this project's dependencies with its own tool
xpm ci                          # frozen/locked install where the tool supports it (see below)
```

xpm installs the registry's own name (`xpm install monolog` → `composer require monolog/monolog`) and tells you when it differs from what you typed.

#### How xpm picks a tool

Live registries are full of namesakes: PyPI has an `axios`, npm has `requests` and a `phpunit` placeholder, crates.io has `express`. So xpm decides in this order, the same way with or without a terminal:

1. **Exact names only.** A hit counts as the package you typed when its name is the same, ignoring case (PyPI also ignores `-`/`_`/`.`). On Packagist, `vendor/<name>` with the vendor equal to the name also counts (`phpunit` → `phpunit/phpunit`). On Maven Central the artifactId must match **with the same case** (`Express` is not `express`), unless you typed the full `group:artifact`; npm and web-asset repackages (groups starting with `org.mvnpm` or `org.webjars`) never count. Anything else is a *closest match*: Packagist and Maven Central answer every name with some search hit (`axios` → `swlib/saber`).
2. **Inside a project, its ecosystem wins.** The current directory is a project for every ecosystem that has a project file there: `package.json` or a Node lockfile, `requirements.txt` / `pyproject.toml` / `Pipfile` / `poetry.lock`, `composer.json`, `Cargo.toml`, `go.mod`, `pom.xml`, `build.gradle(.kts)`. An exact hit in that ecosystem is installed right away, without a menu, and xpm says so: `Using composer for this PHP project (also found: Node).` Lockfiles pick the tool (`yarn.lock` → yarn); with several lockfiles in one ecosystem a terminal asks unless `prefer` decides, and without a terminal the first by `prefer` (then lockfile) order is used. Registries of other ecosystems that did not answer do not matter. You get the menu (or, without a terminal, a refusal that lists the candidates) when the project's own registry did not answer, when the name is exact in two of the project's ecosystems (say `package.json` and `requirements.txt` side by side), or when the project's ecosystem has no exact hit; that menu lists the project's ecosystem first, then exact hits elsewhere, then closest matches. `prefer` does not override the project.
3. **Outside a project, and for every global install.** A global install (`-g`) does not go into the current project, so the project does not choose it, even when you run it inside one. If the exact hits all come from one ecosystem, that one is installed. If they come from several, your `prefer` list settles it when it ranks exactly one of them first (`Using npm ("prefer" in config; also found: Python).`); otherwise a terminal shows a menu (exact hits first, closest matches last, marked "(closest match)") and a script gets an error such as `axios exists in several ecosystems: npm (Node), pip (Python). Set "prefer" in config (e.g. xpm config set prefer npm) or run inside a project`. If no hit is exact, a single registry's closest match is offered for confirmation (refused without a terminal; a Maven or Gradle snippet is only printed, so it is not confirmed), and closest matches from several registries are a menu or a refusal. A registry that did not answer stops every automatic choice here: a terminal shows the menu, a script gets an error.

Picking a closest match from a menu installs it without asking again. `xpm ci` runs the tool's strict form where one exists: `npm ci`, `pnpm`/`yarn`/`bun install --frozen-lockfile`, `yarn install --immutable` (yarn 2+), `pipenv install --deploy`, `cargo build --locked`, and `go mod download`. Other tools have no frozen flag, so `ci` runs their normal install: `composer install` (which installs from `composer.lock` when present), `pip install -r requirements.txt`, `poetry install`, `mvn install`, `gradle build`. `xpm ci` checks every detected project's tool before running any install, never deletes lockfiles, and deletes `node_modules/` (yarn, pnpm, bun) or Composer's `vendor/` only if you confirm. In a Go project `xpm install` runs `go mod tidy`, while `xpm ci` runs `go mod download`.

For a Maven or Gradle project (`pom.xml`, `build.gradle`), `xpm install <name>` prints the dependency snippet to paste (`Add com.google.guava:guava:33.3.1-jre to build.gradle(.kts):`) instead of editing your build file; no Maven or Gradle binary is needed for that. In the search TUI, a Maven Central hit can be installed as a Maven or a Gradle snippet.

xpm never pipes a remote install script into a shell. If a tool such as bun, rustup, Composer, Go, Maven, Gradle or npm (Node) is missing, it prints the official install instructions instead, without offering to install it.

### Run project scripts

```bash
xpm run                 # list scripts from package.json, composer.json, pyproject.toml, Cargo.toml
xpm run build
xpm run test -- --watch # everything after -- goes to the script untouched
```

Scripts come from `package.json` `scripts`, `composer.json` `scripts`, `pyproject.toml` `[tool.xpm.scripts]` (falls back to `[tool.upm.scripts]`, then `[tool.poetry.scripts]`, then `[project.scripts]`) and `Cargo.toml` `[package.metadata.xpm.scripts]`. `package.json` and `composer.json` scripts run through the project's package manager (`npm`/`yarn`/`pnpm`/`bun run`, `composer run-script`); `pyproject.toml` and `Cargo.toml` scripts run through `sh -c` (on Windows `sh` must be on PATH, e.g. Git for Windows).

### Diagnose

```bash
xpm doctor              # installed tools, runtimes, lockfiles and project health
```

A security audit whose tool fails, prints unreadable output or audits nothing is reported as unavailable, never as passing. Lockfile drift is checked by content for `package-lock.json` (v2/v3), `pnpm-lock.yaml`, `Cargo.lock` and `go.sum`; `composer.lock`, `poetry.lock`, `uv.lock`, `pdm.lock` and `Pipfile.lock` are only checked to parse, and `yarn.lock` and bun lockfiles are not compared. File modification times are never used.

### Dependency graph, lockfile and monorepos (experimental)

```bash
xpm graph                  # dependency tree from the lockfiles and manifests (pom.xml, go.mod, pyproject.toml, requirements.txt) in this directory; repeats are marked (*)
xpm graph react            # only the subtree under react
xpm graph --depth 2        # limit tree depth (default: graph.depth, 5; 0 = unlimited)
xpm graph --json > g.json  # machine-readable; stdout carries only the graph, warnings go to stderr
xpm graph --svg > g.svg    # needs GraphViz `dot`
xpm graph --exec           # also run mvn / gradle / go mod graph for full Java and Go trees
xpm graph --workspace      # combine the graphs of all workspace projects
xpm lock                   # write xpm-lock.yaml (hashes of the lockfiles in the project root)
xpm lock --verify          # exit 1 if a lockfile changed, appeared or disappeared since `xpm lock`, or cannot be checked (unreadable, or a recorded path outside the project)
xpm workspaces             # list monorepo projects (npm/yarn/pnpm, Cargo, go.work, Poetry and uv, Maven, Gradle, Composer)
xpm run --workspace test   # run `test` in every project that defines it (`-- args` are passed to the task in every project)
```

`xpm graph` reads files only; it never runs a build tool unless you pass `--exec`. `xpm-lock.yaml` has no timestamps, so running `xpm lock` again on an unchanged project leaves the file untouched. Workspace commands honour `workspace.include` / `workspace.exclude` (glob lists matched against each project's path relative to the workspace root, `**` allowed). `workspace.parallel` applies to `run --workspace` (and to `install --workspace` once it is wired), not to `workspaces` or `graph --workspace`; a parallel run prints each project's output when that project finishes, not live. `graph --workspace` reads npm/pnpm/yarn and Cargo workspaces from the root lockfile, which covers their members.

The dependency cache (`xpm cache`) was removed: npm, pip, Cargo, Go and the others already keep their own caches.

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
- **Install never pins.** `install` downloads, verifies and unpacks into a staging directory, then renames it into place, so an interrupted install (Ctrl-C exits 130) leaves no partial version directory behind. Two runtimes keep files outside the version directory: rust toolchains live in xpm's private rustup (`remove rust@<v>` runs `rustup toolchain uninstall`), and PHP is a Homebrew `php@X.Y` formula (`remove php@X.Y` leaves the formula installed). The first version of a runtime becomes the global default; `install` never writes `.xpm-env`.
- **`use` vs `use --global`.** `use` writes `.xpm-env` in the current directory (comments and order are kept); `--global` writes `active.json`. Lookup order: the nearest `.xpm-env` up from the current directory that names the runtime, then `active.json`. Values may be exact (`20.11.0`), partial (`20`), `latest` or `lts`, matched against installed versions; `lts` matches only versions installed via `@lts`.
- **`.xpm-env` format.** One `runtime=version` per line. `#` starts a comment only at the start of a line; an inline comment (`node=20 # app`) makes the value an invalid version.
- **`remove`** refuses a version pinned by the `.xpm-env` in effect in the current directory. Removing the global default clears it from `active.json` (`Cleared the global <runtime> default`).
- **Shims are links to xpm itself** (`~/.xpm/env/shims/node -> xpm`): no Go toolchain needed. A shim runs the version in effect, with that version's `bin` first on `PATH`. If nothing is configured it runs the system binary from `PATH`; if `.xpm-env` names a version that is not installed it fails (exit 127) and tells you what to install. After upgrading or moving the xpm binary, run `xpm env reshim`.
- **Globally installed tools are not shimmed.** Only the runtime's own binaries get shims. `npm -g` packages and pip console scripts land in the version's `bin`, which is on `PATH` only inside a shimmed process (e.g. `npm run`); `go install` and `cargo install` write to your own `GOBIN`/`~/go/bin` and `CARGO_HOME`/`~/.cargo/bin`, which xpm does not add to `PATH`.
- **`setup-path`** appends one line to one file for your `$SHELL`: `~/.zshrc` (or `$ZDOTDIR/.zshrc`), `~/.bashrc` (Linux) / `~/.bash_profile` (macOS), `~/.config/fish/conf.d/xpm.fish`, or `~/.profile`. Running it again changes nothing.
- GitHub API calls (bun, deno version lists) use `GITHUB_TOKEN` when set; partial versions match the 100 most recent releases.

### Global flags

Global flags go **before** the command: `xpm -v install axios` turns on verbose logs. Anything after the command belongs to that command, so `xpm run test -- --version` passes `--version` to your test script.

### Scripts and CI

Without a terminal (stdin and stdout both must be terminals; pipes and CI are not), xpm never prompts and the search TUI does not open. `xpm install <name>` follows [How xpm picks a tool](#how-xpm-picks-a-tool); where a terminal would show a menu, it refuses, lists the candidates and exits 1. To make CI installs deterministic: run xpm inside the project (where the project's ecosystem wins), or set `prefer` (`xpm config set prefer npm`) for runs outside a project and for global installs, or name the package exactly: `vendor/package` for Composer, `group:artifact` for Maven and Gradle, a module path for Go. If a registry did not answer, retry or turn it off (`xpm config set search.maven false`). `update` and `remove` (with or without a package name) refuse when several project types are present; `list` uses the first.

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | Error, cancelled install prompt, invalid arguments, **no matches**, or a refused non-interactive guess |
| `2` | `graph` usage error (bad flag or argument) |
| other | `install` (no package argument), `ci`, `list` / `update` / `remove` / `run` pass through the underlying tool's exit code (`run --workspace` exits 1 if any project fails) |

### Changes in this release

Behaviour changes to check if you script xpm:

- **Exit codes.** "No matches" now exits 1 (it was 0) for `which`, `search` and `install`. A search where every registry fails exits 1 as well.
- **Project first.** Inside a project, `xpm install <name>` installs the project ecosystem's exact hit without a menu, even on a terminal, and ignores namesakes in other ecosystems (see [How xpm picks a tool](#how-xpm-picks-a-tool)). Global installs (`-g`) ignore the project and follow `prefer`. A Maven artifactId must now match the name's case, and Packagist's `<name>/<name>` counts as the name itself.
- **No terminal means no prompts.** When stdin or stdout is not a terminal, xpm behaves as if `interactive` were `false`: it never prompts, never opens the TUI, and refuses ambiguous or fuzzy installs instead of guessing.
- **Extra arguments are errors.** `xpm install a b c` installs all three; commands that take no package (`ci`, `list`) or exactly one (`which`, `info`) now reject extra arguments instead of ignoring them.
- **Unknown flags are errors.** `xpm install` accepts only `-g` / `--global` (before or after the packages) and `--` to end flags. `--global=false` is rejected (omit the flag for a local install), as is any other flag.
- **`-v` goes before the command.** `xpm -v install axios` is verbose logging; after the command, `-v` belongs to the command (`xpm install axios -v` is an unknown-flag error).
- **`xpm ci` is a native frozen install** (see above) and deletes nothing unasked; it no longer removes lockfiles.
- **Missing package managers** get printed official install steps; xpm no longer runs remote install scripts.

## Configuration

`~/.config/xpm/xpmrc.json` (Windows: `%APPDATA%\xpm\xpmrc.json`). Every key is optional; keys you leave out keep their defaults.

```json
{
  "prefer": ["pnpm", "pip"],
  "search": { "maven": false },
  "interactive": true,
  "autoInstallPM": true
}
```

| Key | Default | What it does |
|---|---|---|
| `prefer` | `[]` | Package managers to list first when choosing; outside a project and for `-g` it settles a name found in several ecosystems |
| `search.<id>` | `true` | Turn a registry off (`npm`, `pip`, `composer`, `cargo`, `maven`) |
| `interactive` | `true` | Prompt for choices (only when a terminal is attached); `false` decides as without a terminal (see [Scripts and CI](#scripts-and-ci)) |
| `autoInstallPM` | `true` | Offer to install a missing pnpm, yarn, pip, poetry or pipenv (always asks first); other tools get official install steps |
| `searchUI.enabled` | `true` | Use the TUI for `xpm search` |
| `searchUI.debounceMs` / `searchUI.pageSize` | `200` / `20` | TUI typing pause before searching / max rows per page |
| `timeout.default` | `0` | Seconds each registry may take; `0` = built-in 2.5 s. Older configs written by `xpm config set` contain `4`, which is also treated as the built-in deadline |
| `timeout.perRegistry` | `{}` | Per-registry override in seconds, e.g. `{"crates": 4}` (`npm`, `pypi`, `packagist`, `crates`, `maven`) |
| `scripts.prefer` | `[]` | Which script source wins when names collide |
| `env.enabled` / `env.path` | `true` / `~/.xpm/env` | Runtime manager on/off and install root |
| `graph.depth` | `5` | Max depth for `xpm graph` |
| `workspace.parallel` | `true` | Run workspace operations in parallel |

`xpm config set` accepts `prefer`, `autoInstallPM`, `interactive` and `search.<id>`, and validates the value before writing anything.

### Environment variables

| Variable | Effect |
|---|---|
| `XPM_NO_CACHE` (any non-empty value, e.g. `1`) | Skip the on-disk lookup cache (results are cached for 1 h, "not found" for 15 min) |
| `XPM_CACHE_DIR=<dir>` | Keep xpm's lookup cache in `<dir>/lookups` instead of the OS cache folder (used by `make perf`) |

## Performance

Measured on macOS arm64 (2026-10-06):

| | Before | Now |
|---|---|---|
| `xpm which axios`, first lookup | 3–6 s | ~1.0–1.4 s |
| `xpm which axios`, repeat | 3–6 s | < 10 ms |
| `xpm search axios` (plain), first lookup | 1.08 s* | ~1.2 s (budget ≤ 1.5 s); < 10 ms cached |
| Worst case, one registry hangs | up to 20 s | ≤ 2.5 s |
| npm data per lookup (`typescript`) | 15.7 MB | 3.5 KB |

*The old 1.08 s was only fast because crates.io rejected every request (403, no User-Agent), so Cargo never answered. Now all five registries answer, in about 1.2 s cold.

`make perf` checks these budgets (it needs `hyperfine` and `jq`).

## Development

```bash
make test        # go test ./...
make lint        # golangci-lint v2 (new code must be clean)
make perf        # latency/size budgets
make build
```

CI runs the tests on Linux, macOS and Windows with Go 1.22 and the latest stable Go, plus lint and a build check.

Project layout: `cmd/xpm` (entrypoint) and `internal/` (`cli`, `search`, `pm`, `config`, `env`, `scripts`, `graph`, `lock`, `workspace`, `doctor`, `tui`).

Contributions are welcome. Pick an item from the [roadmap](docs/superpowers/plans/2026-10-06-xpm-roadmap.md) and open a PR against `develop`.

## License

[MIT](LICENSE) © Crenspire Technologies
