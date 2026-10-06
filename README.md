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
$ xpm install requests@2.31  # → pip install requests==2.31
$ xpm run test               # runs package.json / composer.json / pyproject scripts
```

## Why xpm

- **Fast.** All registries are queried in parallel with a hard 2.5 s deadline, and answers are cached on disk. A first lookup takes about a second; a repeat lookup takes under 10 ms.
- **One syntax.** `name@version` works everywhere: xpm translates it to `npm i name@v`, `pip install name==v`, `composer require name:v`, `cargo add name@v` or `go get name@v`.
- **Respects your project.** Lockfiles pick the tool inside their ecosystem (`yarn.lock` → yarn, `poetry.lock` → poetry, `build.gradle` → Gradle) for `install`, `list`, `update`, `remove` and `ci`. If a name exists in several ecosystems, xpm still asks which one you mean.
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

## Usage

### Find a package everywhere

```bash
xpm which lodash        # which registries have it, latest versions, descriptions
xpm search http client  # several words are one query; opens the interactive TUI on a terminal, plain output otherwise
xpm info serde          # details for one package
```

If a registry doesn't answer within 2.5 s, xpm shows what the others found and lists that registry under **Unavailable**, separately from **Not found in**. If every registry fails (for example, you're offline), the command exits with an error rather than claiming "no matches". When nothing matches, `xpm which`, `xpm search` and `xpm install` exit 1.

### Install

```bash
xpm install axios               # search, then install with the right tool
xpm install axios@1.7.0         # pin a version (universal @ syntax); without @ the tool picks its latest
xpm install axios lodash        # several packages, in order (stops at the first failure)
xpm install -g typescript       # global install where the tool supports it (-g may also come last)
xpm install github.com/gin-gonic/gin   # Go module path: runs go get, no registry search
xpm install                     # no args: install this project's dependencies with its own tool
xpm ci                          # frozen/locked install where the tool supports it (see below)
```

xpm installs the registry's own name (`xpm install monolog` → `composer require monolog/monolog`) and tells you when it differs from what you typed. A name that is only a close match (not the same package) is confirmed with you first, and refused when there is no terminal; PyPI names that differ only in case or `-`/`_`/`.` count as the same package. Lockfiles narrow the tool within an ecosystem; across ecosystems you choose. `xpm ci` runs the tool's strict form where one exists: `npm ci`, `pnpm`/`yarn`/`bun install --frozen-lockfile`, `yarn install --immutable` (yarn 2+), `pipenv install --deploy`, `cargo build --locked`, and `go mod download`. Other tools have no frozen flag, so `ci` runs their normal install: `composer install` (which installs from `composer.lock` when present), `pip install -r requirements.txt`, `poetry install`, `mvn install`, `gradle build`. `xpm ci` never deletes lockfiles, and deletes `node_modules/` (yarn, pnpm, bun) or Composer's `vendor/` only if you confirm. In a Go project `xpm install` runs `go mod tidy`, while `xpm ci` runs `go mod download`.

For a Maven or Gradle project (`pom.xml`, `build.gradle`), `xpm install <name>` prints the dependency snippet to paste instead of editing your build file. In the search TUI, a Maven Central hit can be installed as a Maven or a Gradle snippet.

xpm never pipes a remote install script into a shell. If a tool such as bun, rustup or Composer is missing, it prints the official install instructions instead.

### Run project scripts

```bash
xpm run                 # list scripts from package.json, composer.json, pyproject.toml, Cargo.toml
xpm run build
xpm run test -- --watch # everything after -- goes to the script untouched
```

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

Without a terminal (stdin and stdout both must be terminals; pipes and CI are not), xpm never prompts and the search TUI does not open. Installs pick the first match only within one ecosystem, or across ecosystems when your `prefer` list ranks exactly one of them first; otherwise they refuse and list the candidates. If a registry was unavailable they refuse to guess and exit 1, so retry or turn that registry off (`xpm config set search.maven false`). `update` and `remove` with a package name refuse when several project types are present.

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | Error, cancelled, invalid arguments, **no matches**, or a refused non-interactive guess |
| other | `install` (no package argument), `ci`, `list` / `update` / `remove` / `run` pass through the underlying tool's exit code |

### Changes in this release

Behaviour changes to check if you script xpm:

- **Exit codes.** "No matches" now exits 1 (it was 0) for `which`, `search` and `install`. A search where every registry fails exits 1 as well.
- **No terminal means no prompts.** When stdin or stdout is not a terminal, xpm behaves as if `interactive` were `false`: it never prompts, never opens the TUI, and refuses ambiguous or fuzzy installs instead of guessing.
- **Extra arguments are errors.** `xpm install a b c` installs all three; commands that take no package (`ci`, `list`) or exactly one (`which`, `info`) now reject extra arguments instead of ignoring them.
- **Unknown flags are errors.** `xpm install` accepts only `-g` / `--global` (before or after the packages) and `--` to end flags. `--global=false` and any other flag are rejected.
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
| `prefer` | `[]` | Package managers to list first when choosing |
| `search.<id>` | `true` | Turn a registry off (`npm`, `pip`, `composer`, `cargo`, `maven`) |
| `interactive` | `true` | Prompt for choices (only when a terminal is attached); `false` picks the preferred option |
| `autoInstallPM` | `true` | Offer to install a missing package manager (always asks first) |
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
