# XPM - Universal Package Manager

A unified command-line interface for managing packages across multiple ecosystems. Search, install, and manage packages from npm, pip, composer, cargo, maven, gradle, and go modules through a single tool.

## Features

- **Cross-ecosystem search**: Find packages across npm, PyPI, Packagist, crates.io, and Maven Central simultaneously
- **Unified installation**: Install packages using consistent syntax regardless of the underlying package manager
- **Universal version syntax**: Use `package@version` syntax across all package managers
- **Runtime version manager**: Install and switch between versions of Node.js, Python, PHP, Go, Java, Rust, Bun, and Deno (similar to asdf/nvm/pyenv)
- **Script runner**: Run project scripts from `package.json`, `composer.json`, or `pyproject.toml`
- **Lock file detection**: Automatically detects lock files and uses the corresponding package manager
- **Unified lockfile**: Generate `xpm-lock.yaml` that tracks all ecosystem lockfiles in one place
- **Dependency cache**: Global cross-language dependency cache to accelerate installs
- **Dependency graph**: Visualize dependencies across all ecosystems in a single graph
- **Workspace support**: Detect and manage monorepos across multiple ecosystems
- **Auto-detection**: Automatically detects project type from manifest files (`package.json`, `requirements.txt`, `Cargo.toml`, etc.)
- **Multiple Node.js package managers**: Supports npm, yarn, pnpm, and bun interchangeably
- **Multiple Python package managers**: Supports pip, poetry, and pipenv
- **Interactive mode**: Choose which ecosystem to install from when a package exists in multiple registries
- **Package manager installation**: Optionally install missing package managers automatically
- **Environment diagnostics**: Comprehensive diagnostics for runtimes, package managers, and project health

## Supported Package Managers

| Ecosystem | Package Manager | Binary | Lock File | Global Install |
|-----------|-----------------|--------|-----------|----------------|
| Node.js | npm | `npm` | `package-lock.json` | ✅ |
| Node.js | yarn | `yarn` | `yarn.lock` | ✅ |
| Node.js | pnpm | `pnpm` | `pnpm-lock.yaml` | ✅ |
| Node.js | bun | `bun` | `bun.lockb` | ✅ |
| Python | pip | `pip` | `requirements.txt` | ✅ |
| Python | poetry | `poetry` | `poetry.lock` | ❌ |
| Python | pipenv | `pipenv` | `Pipfile.lock` | ❌ |
| PHP | Composer | `composer` | `composer.lock` | ✅ |
| Rust | Cargo | `cargo` | `Cargo.lock` | ❌ (uses `cargo install`) |
| Go | Go Modules | `go` | `go.sum` | ❌ (uses `go install`) |
| Java | Maven | `mvn` | - | ❌ (prints dependency XML) |
| Java | Gradle | `gradle` | - | ❌ (prints dependency config) |

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/crenspire/xpm.git
cd xpm

# Build the binary
go build -o xpm ./cmd/xpm

# Move to PATH (optional)
sudo mv xpm /usr/local/bin/
```

### Using Go Install

```bash
go install github.com/crenspire/xpm/cmd/xpm@latest
```

## Usage

### Install a Package

Search across all ecosystems and install interactively:

```bash
xpm install axios
```

This will:
1. Search for "axios" in npm, PyPI, Packagist, crates.io, and Maven Central
2. Display results from each ecosystem where the package exists
3. Let you choose which one to install (in interactive mode)

### Install with Global Flag

```bash
xpm install --global axios
# or
xpm install -g axios
```

### Install a Specific Version

Use the universal `@version` syntax which works across all package managers:

```bash
# Node.js packages
xpm install axios@1.0.0           # Translates to: npm install axios@1.0.0

# Python packages
xpm install requests@2.31.0       # Translates to: pip install requests==2.31.0

# Composer packages
xpm install laravel/framework@10.0  # Translates to: composer require laravel/framework:10.0

# Cargo packages
xpm install serde@1.0             # Translates to: cargo add serde@1.0

# Go modules
xpm install github.com/gin-gonic/gin@v1.9.0  # Translates to: go get ...@v1.9.0
```

### Lock File Detection

XPM automatically detects lock files in your project directory and uses the corresponding package manager:

**Node.js ecosystem:**
- `yarn.lock` → uses yarn
- `pnpm-lock.yaml` → uses pnpm
- `bun.lockb` → uses bun
- `package-lock.json` → uses npm

**Python ecosystem:**
- `poetry.lock` → uses poetry
- `Pipfile.lock` → uses pipenv
- `requirements.txt` → uses pip

**Behavior:**
- **Single lock file**: Automatically uses that package manager (no prompt)
- **Multiple lock files**: Shows only the matching package managers for selection
- **No lock file**: Prompts you to choose which package manager to use

Example:
```bash
# In a project with yarn.lock
$ xpm install lodash
Detected yarn.lock - using yarn
Installing lodash via yarn...

# In a project with both package-lock.json and yarn.lock
$ xpm install lodash
Multiple Node.js lock files detected.
? Select package manager:
  > npm
    yarn
```

### Auto-detect and Install Dependencies

Run without a package name to detect project files and install dependencies:

```bash
xpm install
```

XPM will detect:
- `package.json` → runs `npm install` (or yarn/pnpm/bun based on preference)
- `composer.json` → runs `composer install`
- `Cargo.toml` → runs `cargo build`
- `go.mod` → runs `go mod tidy`
- `pom.xml` → runs `mvn install`
- `build.gradle` → runs `gradle build`
- `requirements.txt` → runs `pip install -r requirements.txt`
- `pyproject.toml` → runs `pip install .`

### Run Project Scripts

Run scripts defined in your project configuration files:

```bash
# List available scripts
xpm run

# Run a specific script
xpm run dev
xpm run build
xpm run test

# Pass arguments to the script
xpm run build -- --watch
```

XPM automatically detects and uses the appropriate package manager based on lock files:
- `bun.lockb` → uses Bun
- `pnpm-lock.yaml` → uses pnpm
- `yarn.lock` → uses Yarn
- Default → uses npm

Supported configuration files:
- `package.json` (npm/yarn/pnpm/bun scripts)
- `composer.json` (Composer scripts)
- `pyproject.toml` (Python entry points)

Example output:
```
$ xpm run
Scripts from package.json:

  build  webpack --mode production
  dev    webpack serve --mode development
  lint   eslint src/
  test   jest

Run a script with: xpm run <script-name>
Pass arguments with: xpm run <script-name> -- <args>
```

### Check Package Availability

See which ecosystems have a specific package:

```bash
xpm which lodash
```

Output:
```
Searching for "lodash"...

Found in:
- composer: lodash-php/lodash-php - A port of Lodash to PHP
  → Add to composer.json or run `composer require ...`.
- maven: org.mvnpm.at.types:lodash @4.17.16 - Maven artifact
  → Add to pom.xml as dependency.
- npm: lodash @4.17.21 - Lodash modular utilities.
  → Also available via: yarn, pnpm, bun
- pip: lodash @0.0.1 - python implementation for lodash
  → Also available via: poetry, pipenv

Not found in:
- cargo (Rust)
- go modules (Go)
- gradle (Java)
```

Note: If a package is found in `pip`, it's also available via `poetry` and `pipenv` since they all use PyPI. Similarly, npm packages are available via `yarn`, `pnpm`, and `bun`.

### Runtime Version Management

XPM includes a unified runtime version manager that lets you install and switch between versions of programming language runtimes, similar to asdf, nvm, pyenv, or goenv.

#### Install a Runtime Version

```bash
# Install Node.js
xpm env install node@20.11.0

# Install Python
xpm env install python@3.12.1

# Install Go
xpm env install go@1.22.0

# Install Java
xpm env install java@17

# Install Rust
xpm env install rust@1.75.0

# Install Bun
xpm env install bun@1.1.0

# Install Deno
xpm env install deno@1.40.0

# Install PHP
xpm env install php@8.4
```

**Version Aliases:**
- Use `latest` to install the latest available version:
  ```bash
  xpm env install node@latest
  xpm env install python@latest
  xpm env install php@latest
  xpm env install go@latest
  ```

- Use `lts` for Node.js to install the latest LTS version:
  ```bash
  xpm env install node@lts
  ```

**Note:** After installation, the runtime is automatically activated (local scope), so you can use it immediately without running `xpm env use`.

#### Switch Runtime Versions

```bash
# Switch to a specific version (local to current directory)
xpm env use node@20.11.0

# Switch globally
xpm env use node@20.11.0 --global
```

This creates a `.xpm-env` file in your project directory:
```
node=20.11.0
python=3.12.1
```

#### List Installed Versions

```bash
xpm env list
```

Output:
```
node:
  - 18.19.0
  → 20.11.0 (active)
  - 24.11.1 (lts)
  - 25.2.1 (latest)
python:
  - 3.10.9
  → 3.12.1 (active)
php:
  → 7.4 (active)
  - 8.4
```

The active version is highlighted in green, and aliases (like `lts` or `latest`) are shown in parentheses next to the version number.

#### List Available Remote Versions

```bash
xpm env ls-remote node
xpm env ls-remote python
```

#### Show Active Versions

```bash
xpm env current
```

Output:
```
node: 20.11.0
python: 3.12.1
go: 1.22.0
```

#### Remove a Version

```bash
xpm env remove node@18.19.0
```

#### Setup PATH (Configure Shell)

Automatically configure your shell profile to add XPM shims to PATH:

```bash
xpm env setup-path
```

This command detects your shell and adds the necessary PATH configuration to your shell profile file.

#### Auto-Activation

XPM automatically activates runtime versions from `.xpm-env` files when you run commands. The system walks up the directory tree to find the nearest `.xpm-env` file.

**Version Selection Precedence:**
1. Command flag (e.g., `--node=20.11.0`)
2. Local `.xpm-env` in current directory
3. Parent directory `.xpm-env` (walking up)
4. Global default from `~/.xpm/env/defaults.json`

**Auto-Activation:**
When you install a runtime version, it is automatically activated (local scope) so you can use it immediately. You don't need to run `xpm env use` after installation.

#### PATH Setup

After installing your first runtime, you need to add the shims directory to your PATH. XPM can automatically configure this for you:

```bash
# Automatically configure your shell profile
xpm env setup-path
```

This will add the shims directory to your shell profile:
- `~/.bashrc` (Bash)
- `~/.zshrc` (Zsh)
- `~/.config/fish/config.fish` (Fish)

After running `xpm env setup-path`, restart your shell or run:
```bash
source ~/.zshrc  # or ~/.bashrc for bash
```

**Manual Setup:**
If you prefer to set it up manually, add this line to your shell profile:
```bash
export PATH="$HOME/.xpm/env/shims:$PATH"
```

### Unified Lockfile

Generate a unified lockfile that tracks all ecosystem lockfiles:

```bash
# Generate xpm-lock.yaml
xpm lock

# Verify lockfiles haven't changed
xpm lock --verify
```

The `xpm-lock.yaml` file contains metadata from all detected lockfiles:
- Node.js: `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lockb`
- Python: `poetry.lock`, `requirements.lock`
- PHP: `composer.lock`
- Rust: `Cargo.lock`
- Go: `go.sum`
- Java: Maven/Gradle lockfiles

### Dependency Cache

Manage the global dependency cache:

```bash
# Show cache tree
xpm cache tree

# Show cache size
xpm cache size

# Clean entire cache
xpm cache clean

# Garbage collection (remove old artifacts)
xpm cache gc
```

### Dependency Graph

Visualize dependencies across all ecosystems:

```bash
# Show dependency tree
xpm graph

# Show graph for specific package
xpm graph react

# Export as JSON
xpm graph --json > graph.json

# Generate SVG visualization (requires GraphViz)
xpm graph --svg > graph.svg
```

### Workspace/Monorepo Support

Detect and manage workspaces across multiple ecosystems:

```bash
# List detected workspaces
xpm workspaces

# Install dependencies in all workspaces
xpm install --workspace

# Run task across all workspaces
xpm run --workspace build

# Generate combined dependency graph
xpm graph --workspace
```

Supported workspace types:
- Node.js: npm workspaces, Yarn workspaces, pnpm workspaces, Bun workspaces
- Python: Multi-project layouts with `pyproject.toml`
- Rust: Cargo workspaces
- Go: Go workspaces (`go.work`)
- Java: Maven multi-module, Gradle multi-project
- PHP: Composer multi-package layouts

### Interactive Package Search

Use the interactive TUI to search and install packages:

```bash
# Launch interactive search
xpm search

# Pre-fill query
xpm search axios
```

The TUI supports:
- Real-time search across all ecosystems
- Arrow key navigation
- Enter to install selected package
- Automatic package manager selection

### Environment Diagnostics

Run comprehensive diagnostics on your environment:

```bash
xpm doctor
```

The Doctor+ command checks:
- **Runtimes**: Node.js, Python, PHP, Go, Rust, Java versions
- **Package Managers**: npm, yarn, pnpm, bun, composer, pip, cargo, go, mvn, gradle
- **Project Files**: Detects dependency files and lockfiles
- **Conflicts**: Identifies conflicting lockfiles (e.g., `yarn.lock` + `package-lock.json`)
- **Drift**: Detects when dependency files are newer than lockfiles
- **Security**: Runs security audits (npm audit, pip-audit, cargo audit, etc.)

Example output:
```
XPM Doctor+ Report
──────────────────────────────────────────────
[ Environment ]
✔ Node.js v20.4.0
✔ Python v3.11.2
✘ PHP missing
✔ Go v1.22
✔ Rust v1.76
✔ Java (JDK 17)

[ Package Managers ]
✔ npm
✔ yarn
✔ pnpm
✘ bun
✔ composer
✔ pip
✘ cargo-audit (optional tool missing)

[ Project Files ]
✔ package.json
✔ package-lock.json
✘ yarn.lock (conflicts with package-lock.json)
✔ go.mod + go.sum
✘ composer.lock missing

[ Dependency Drift ]
✘ package.json newer than package-lock.json
✔ Cargo.toml and Cargo.lock aligned

[ Security Scan ]
- npm: 2 vulnerabilities (1 high)
- pip: OK
- cargo: audit tool missing

[ Recommendations ]
- Remove one of yarn.lock or package-lock.json
- Run `npm install` to sync lockfile
- Install PHP if project requires it
- Install cargo-audit for Rust security scanning
```

### Verbose Mode

Add `-v` or `--verbose` for detailed logging:

```bash
xpm -v install axios
xpm --verbose which lodash
```

### Help

```bash
xpm help
xpm --help
xpm -h
```

### Version

```bash
xpm version
xpm --version
xpm -V
```

## Configuration

XPM uses a JSON configuration file located at:
- **Linux/macOS**: `~/.config/xpm/xpmrc.json`
- **Windows**: `%APPDATA%\xpm\xpmrc.json`

### Configuration Options

```json
{
  "prefer": ["npm", "pip"],
  "search": {
    "npm": true,
    "pip": true,
    "composer": true,
    "cargo": true,
    "maven": true
  },
  "autoInstallPM": true,
  "interactive": true,
  "scripts": {
    "prefer": ["npm", "composer"]
  },
  "lock": {
    "autoGenerate": true,
    "autoVerify": false
  },
  "cache": {
    "enabled": true,
    "path": "~/.xpm/cache",
    "maxAgeDays": 60,
    "maxVersions": 5
  },
  "graph": {
    "showVersions": true,
    "showEcosystem": true,
    "depth": 5
  },
  "workspace": {
    "enabled": true,
    "include": [],
    "exclude": ["**/test/**", "**/node_modules/**"],
    "parallel": true
  },
  "env": {
    "enabled": true,
    "path": "~/.xpm/env",
    "default": {
      "node": "20.11.0",
      "python": "3.12.1"
    }
  }
}
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `prefer` | `string[]` | `[]` | Preferred package managers, sorted first in results |
| `search` | `object` | all `true` | Enable/disable specific ecosystem searches |
| `autoInstallPM` | `bool` | `true` | Offer to install missing package managers |
| `interactive` | `bool` | `true` | Enable interactive prompts (disable for CI/scripts) |
| `scripts.prefer` | `string[]` | `[]` | Preferred script sources when same task exists in multiple files |
| `lock.autoGenerate` | `bool` | `true` | Automatically regenerate `xpm-lock.yaml` after installs |
| `lock.autoVerify` | `bool` | `false` | Automatically verify lockfiles before installs |
| `cache.enabled` | `bool` | `true` | Enable global dependency cache |
| `cache.path` | `string` | `~/.xpm/cache` | Cache directory path |
| `cache.maxAgeDays` | `int` | `60` | Maximum age for cached artifacts |
| `cache.maxVersions` | `int` | `5` | Maximum versions to keep per package |
| `graph.showVersions` | `bool` | `true` | Show versions in dependency graph |
| `graph.showEcosystem` | `bool` | `true` | Show ecosystem labels in graph |
| `graph.depth` | `int` | `5` | Maximum depth for dependency tree |
| `workspace.enabled` | `bool` | `true` | Enable workspace/monorepo detection |
| `workspace.include` | `string[]` | `[]` | Glob patterns for workspace directories to include |
| `workspace.exclude` | `string[]` | `["**/test/**"]` | Glob patterns for workspace directories to exclude |
| `workspace.parallel` | `bool` | `true` | Run workspace operations in parallel |
| `env.enabled` | `bool` | `true` | Enable runtime version management |
| `env.path` | `string` | `~/.xpm/env` | Root directory for runtime installations |
| `env.default` | `object` | `{}` | Default versions for each runtime |

### Example Configurations

#### Prefer Node.js ecosystem:
```json
{
  "prefer": ["npm", "yarn", "pnpm", "bun"]
}
```

#### Non-interactive mode (for CI/CD):
```json
{
  "interactive": false,
  "autoInstallPM": false
}
```

#### Disable specific ecosystems:
```json
{
  "search": {
    "maven": false,
    "gradle": false
  }
}
```

## Examples

### Node.js Project Setup

```bash
# Create a new project
mkdir my-app && cd my-app
npm init -y

# Install packages via xpm
xpm install express
xpm install -g typescript
```

### Python Project

```bash
# Create requirements.txt
echo "requests" > requirements.txt

# Install all dependencies
xpm install

# Or install a specific package
xpm install flask
```

### Rust Project

```bash
# In a Cargo project
xpm install serde

# For global tools
xpm install -g ripgrep
```

### Multi-ecosystem Monorepo

```bash
# In a directory with both package.json and requirements.txt
xpm install

# XPM will prompt:
# ? Detected multiple project types
#   Node (package.json)
#   Python (requirements.txt)
#   Run All
```

## Environment Variables

XPM respects standard environment variables for each package manager:
- `NPM_CONFIG_*` for npm
- `PIP_*` for pip
- `COMPOSER_*` for composer
- etc.

## Troubleshooting

### Package manager not found

Run `xpm doctor` to check which package managers are installed. XPM can attempt to install some missing package managers automatically if `autoInstallPM` is enabled.

### Runtime not working after installation

If a runtime (especially PHP on macOS) doesn't work after installation:

1. **Check if shims are in PATH:**
   ```bash
   xpm env setup-path
   source ~/.zshrc  # or ~/.bashrc
   ```

2. **Verify the runtime is active:**
   ```bash
   xpm env list
   xpm env current
   ```

3. **For PHP on macOS:** PHP installations use wrapper scripts to handle Homebrew library dependencies. If you encounter library loading errors, try reinstalling:
   ```bash
   xpm env remove php@8.4
   xpm env install php@8.4
   ```

### Permission errors with pip

Global pip installs may require sudo. XPM will warn you and suggest running the command manually:

```
Global pip installs often require sudo and can affect system Python.
Show recommended sudo command instead of running pip? [Yes/No]
```

### Slow searches

Searches query multiple registries in sequence. If certain ecosystems are slow or timing out, disable them in config:

```json
{
  "search": {
    "maven": false
  }
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Setup

```bash
# Clone the repository
git clone https://github.com/crenspire/xpm.git
cd xpm

# Install dependencies
go mod download

# Run tests
go test ./...

# Build
go build -o xpm ./cmd/xpm

# Run locally
./xpm doctor
```

### Project Structure

```
xpm/
├── cmd/xpm/          # Main entry point
│   └── main.go
├── internal/
│   ├── cli/          # CLI command handling
│   ├── config/       # Configuration loading
│   ├── logx/         # Logging utilities
│   ├── pm/           # Package manager adapters
│   ├── scripts/      # Script parser for package.json, etc.
│   └── search/       # Registry search functionality
├── go.mod
└── README.md
```

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- [promptui](https://github.com/manifoldco/promptui) for interactive prompts
- All the package registries that provide public APIs

