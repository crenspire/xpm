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

- **Fast.** All registries are queried in parallel with a hard 2.5 s deadline, and answers are cached on disk. A first lookup takes about a second; a repeat lookup takes under 10 ms.
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
| Dependency graph | `graph` | 🧪 Experimental |
| Unified lockfile | `lock` | 🧪 Experimental |
| Monorepos | `workspaces` | 🧪 Experimental |
| Global dependency cache | `cache` | 🧪 Experimental (not yet used by installs) |

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
| Linux | Yes, tested in CI | Yes (`sh -c` for pyproject/Cargo scripts) | Yes | Yes |
| macOS | Yes, tested in CI | Yes (`sh -c` for pyproject/Cargo scripts) | Yes | Yes |
| Windows | Yes, tested in CI | Needs `sh` on PATH (Git for Windows or WSL) for pyproject/Cargo scripts; package.json and composer.json scripts do not | Yes, in Windows Terminal or another modern console | Not supported yet (see roadmap P5) |

Windows support for the core commands is covered by CI but is used less in practice than Linux and macOS. The TUI needs a terminal; without one `xpm search` prints plain output.

## Usage

`xpm help` lists every command and marks the experimental ones; `xpm man <command>` has the details for one command. The everyday commands are `which`, `search`, `info`, `install`, `ci`, `list`, `update`, `remove`, `run` and `config`; `cc` and `cg` are short for `cache clean` and `cache gc` (experimental).

### Find a package everywhere

```bash
xpm which lodash        # which registries have it, latest versions, descriptions
xpm search http client  # several words are one query; opens the interactive TUI on a terminal, plain output otherwise
xpm info serde          # details for one package
```

If a registry doesn't answer within 2.5 s, xpm shows what the others found and lists that registry under **Unavailable**, separately from **Not found in**. If every registry fails (for example, you're offline), the command exits with an error rather than claiming "no matches". When nothing matches, `xpm which`, `xpm search` and `xpm install` exit 1.

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

### Runtime versions (experimental)

```bash
xpm env install node@20.11.0   # verified against nodejs.org SHASUMS256
xpm env install go@latest      # newest *stable* Go, verified against go.dev
xpm env use node@20.11.0       # pin for this directory (.xpm-env)
xpm env setup-path             # add xpm's shims to your shell PATH
xpm env list
```

### Global flags

Global flags go **before** the command: `xpm -v install axios` turns on verbose logs. Anything after the command belongs to that command, so `xpm run test -- --version` passes `--version` to your test script.

### Scripts and CI

Without a terminal (stdin and stdout both must be terminals; pipes and CI are not), xpm never prompts and the search TUI does not open. `xpm install <name>` follows [How xpm picks a tool](#how-xpm-picks-a-tool); where a terminal would show a menu, it refuses, lists the candidates and exits 1. To make CI installs deterministic: run xpm inside the project (where the project's ecosystem wins), or set `prefer` (`xpm config set prefer npm`) for runs outside a project and for global installs, or name the package exactly: `vendor/package` for Composer, `group:artifact` for Maven and Gradle, a module path for Go. If a registry did not answer, retry or turn it off (`xpm config set search.maven false`). `update` and `remove` (with or without a package name) refuse when several project types are present; `list` uses the first.

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | Error, cancelled, invalid arguments, **no matches**, or a refused non-interactive guess |
| other | `install` (no package argument), `ci`, `list` / `update` / `remove` / `run` pass through the underlying tool's exit code |

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
| `XPM_NO_CACHE=1` | Skip the on-disk lookup cache (results are cached for 1 h, "not found" for 15 min) |
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

Project layout: `cmd/xpm` (entrypoint) and `internal/` (`cli`, `search`, `pm`, `config`, `env`, `scripts`, `graph`, `lock`, `workspace`, `cache`, `doctor`, `tui`).

Contributions are welcome. Pick an item from the [roadmap](docs/superpowers/plans/2026-10-06-xpm-roadmap.md) and open a PR against `develop`.

## License

[MIT](LICENSE) © Crenspire Technologies
