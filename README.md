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
- **Respects your project.** On install, lockfiles decide which tool runs (`yarn.lock` → yarn, `poetry.lock` → poetry, …). `update`/`remove` get the same treatment in P3.
- **Safe by default.** Package names that look like flags are rejected, runtime downloads are checked against published SHA-256 checksums, and archive extraction cannot write outside its folder.

## Status

xpm is young. The core commands are solid; the bigger subsystems are being rebuilt and are marked **experimental**. See the [roadmap](docs/superpowers/plans/2026-10-06-xpm-roadmap.md).

| Area | Commands | Status |
|---|---|---|
| Cross-registry search & lookup | `which`, `search`, `info` | ✅ Stable |
| Install / update / remove | `install`, `update`, `remove`, `list` | ✅ Stable (Go/Gradle via `install` coming in P3) |
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
make build          # stripped binary at ./xpm (~10 MB)
sudo mv xpm /usr/local/bin/
```

## Supported package managers

| Ecosystem | Tools | Lockfile that selects it | Registry searched |
|---|---|---|---|
| Node.js | npm, yarn, pnpm, bun | `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lockb` | npm |
| Python | pip, poetry, pipenv | `requirements.txt`, `poetry.lock`, `Pipfile.lock` | PyPI |
| PHP | Composer | `composer.lock` | Packagist |
| Rust | Cargo | `Cargo.lock` | crates.io |
| Java | Maven, Gradle | — (prints the dependency snippet) | Maven Central |
| Go | Go modules | `go.sum` | — (install by module path) |

## Usage

### Find a package everywhere

```bash
xpm which lodash        # which registries have it, latest versions, descriptions
xpm search              # interactive TUI search across registries
xpm info serde          # details for one package
```

If a registry doesn't answer within 2.5 s, xpm returns what the others found instead of waiting. If every registry fails (for example, you're offline), the command exits with an error rather than claiming "no matches".

### Install

```bash
xpm install axios               # search, then install with the right tool
xpm install axios@1.7.0         # pin a version (universal @ syntax)
xpm install -g typescript       # global install where the tool supports it
xpm install                     # no args: install this project's dependencies
```

When a package exists in several ecosystems, xpm asks which one you mean. Lockfiles in the current directory narrow the choice of tool.

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
| `interactive` | `true` | Prompt for choices; `false` picks the preferred option (CI) |
| `autoInstallPM` | `true` | Offer to install a missing package manager (always asks first) |
| `searchUI.enabled` | `true` | Use the TUI for `xpm search` |
| `scripts.prefer` | `[]` | Which script source wins when names collide |
| `env.enabled` / `env.path` | `true` / `~/.xpm/env` | Runtime manager on/off and install root |
| `graph.depth` | `5` | Max depth for `xpm graph` |
| `workspace.parallel` | `true` | Run workspace operations in parallel |

### Environment variables

| Variable | Effect |
|---|---|
| `XPM_NO_CACHE=1` | Skip the on-disk lookup cache (results are cached for 1 h, "not found" for 15 min) |

## Performance

Measured on macOS arm64 (2026-10-06):

| | Before | Now |
|---|---|---|
| `xpm which axios`, first lookup | 3–6 s | ~1.0–1.4 s |
| `xpm which axios`, repeat | 3–6 s | < 10 ms |
| Worst case, one registry hangs | up to 20 s | ≤ 2.5 s |
| npm data per lookup (`typescript`) | 15.7 MB | 3.5 KB |

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
