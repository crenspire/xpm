# Plan B — P2 Security Hardening

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the exploitable holes found in the 2026-10-06 review: archive path traversal, unverified runtime downloads, path injection via versions and `.xpm-env`, data loss from `env remove node@`, option injection via package names, and an unverified rustup bootstrap.

**Architecture:**
- Small, independently testable guard functions sit at the trust boundaries:
  - `safeJoin` / `ensureRealParentWithin` for archive entries;
  - `ValidateRuntimeName` / `ValidateVersionSpec` / `Manager.versionDir` for anything that becomes a path;
  - a leading-dash check in `pm` validation;
  - `downloadVerified` + checksum parsers for downloads.
- Existing code calls these guards instead of `filepath.Join` and `http.Get` directly.
- Nothing is redesigned here; the structural rework is P5.

**Tech Stack:** Go 1.22 stdlib (`archive/tar`, `archive/zip`, `crypto/sha256`, `net/http/httptest`, `go/parser`).

**Spec:** `docs/superpowers/plans/2026-10-06-xpm-roadmap.md`, section "P2 — Security hardening".

## Global Constraints

- Prerequisite: Plan A Task 6 is merged (P0 gate green). Plan A Task 9 already removed startup auto-activation; don't re-add it.
- `go 1.22` floor and no new module dependencies.
- **Lane ownership:** Tasks 1, 2, 4 and 5 belong to lane B (`internal/env/**`, `internal/cli/env_cmd.go`). Task 3 touches `internal/pm` and `internal/cli/search_cmd.go`, which lane A owns, so **lane A executes Task 3**. It has no dependency on Tasks 1, 2, 4 or 5.
- Every guard fails closed: on doubt it returns an error, never silently continues.
- Commit messages must not contain AI co-author trailers or "Generated with" lines.

## Review Focus

1. **Re-extracting into a directory that already contains a symlink pointing outside it.** The archive's entries must not be written through that symlink. Test in Task 1 (`TestExtractRefusesWriteThroughExistingSymlink`).
2. **Node's real archive layout** (`bin/npm` is a relative symlink into `lib/`) must still extract and resolve after hardening. Otherwise we've "secured" Node into uselessness. Test in Task 1 (`TestExtractTarGzKeepsInternalSymlinks`).
3. **The repo's own `.xpm-env` (`node=20.11.0`) leaking into env tests run from the repo root.** Every env test must `chdir` into a temp dir. Enforced by the helper in Task 2.
4. **A checksum mismatch must leave no partial archive behind** in the temp dir, and must never reach extraction. Test in Task 4.
5. **Version aliases that are legitimate but look odd**: `lts`, `latest`, `stable`, `1.22rc1`, `8.3`, `21.0.2+13` must all still validate. Test in Task 2.

---

## File map

| File | Change | Task |
|---|---|---|
| `internal/env/runtimes/utils.go` | hardened `extractTarGz`, `extractZip`; new `safeJoin`, `ensureRealParentWithin`, `writeEntry`, `makeSymlink` | 1 |
| `internal/env/runtimes/extract_test.go` | new | 1 |
| `internal/env/validate.go` | new: `ValidateRuntimeName`, `ValidateVersionSpec`, `(*Manager).versionDir` | 2 |
| `internal/env/validate_test.go` | new | 2 |
| `internal/cli/env_cmd.go` | `parseRuntimeVersion` validates | 2 |
| `internal/env/remove.go`, `internal/env/install.go`, `internal/env/manager.go`, `internal/env/detect.go`, `internal/env/shims.go` | use the validators | 2 |
| `internal/pm/validation.go`, `internal/pm/validation_test.go` (new), `internal/cli/search_cmd.go` | leading-dash rejection; validate TUI names | 3 |
| `internal/env/runtimes/download.go`, `internal/env/runtimes/download_test.go` | new: `downloadVerified`, `fetchSmall`, `checksumFromSums`, `goChecksum` | 4 |
| `internal/env/runtimes/node.go`, `internal/env/runtimes/go.go` | verified downloads | 4 |
| `internal/env/runtimes/rust.go`, `internal/env/runtimes/rust_test.go` (new) | delete the unverified rustup bootstrap | 5 |

---

### Task 1: Archive extraction cannot escape its destination

**Files:**
- Modify: `internal/env/runtimes/utils.go` (replace `extractTarGz` and `extractZip`; add helpers)
- Create: `internal/env/runtimes/extract_test.go`

**Interfaces:**
- Produces (package `runtimes`):
  - `func safeJoin(root, name string) (string, error)`
  - `func ensureRealParentWithin(root, target string) error`
  - `func extractTarGz(src, dest string) error`: same signature; now also extracts symlinks and hardlinks.
  - `func extractZip(src, dest string) error`: same signature.

- [ ] **Step 1: Write the failing tests**

Create `internal/env/runtimes/extract_test.go`:

```go
package runtimes

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type tarEntry struct {
	name, body, link string
	typ              byte
}

func makeTarGz(t *testing.T, entries []tarEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typ, Mode: 0o755, Linkname: e.link}
		if e.typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, c := range []interface{ Close() error }{tw, gz, f} {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// newDest returns <tmp>/parent/dest so tests can check nothing lands in <tmp>/parent.
func newDest(t *testing.T) (dest, parent string) {
	t.Helper()
	parent = filepath.Join(t.TempDir(), "parent")
	dest = filepath.Join(parent, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	return dest, parent
}

func TestExtractTarGzRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../evil", "a/../../evil", "/abs-evil"} {
		t.Run(name, func(t *testing.T) {
			dest, parent := newDest(t)
			src := makeTarGz(t, []tarEntry{{name: name, body: "x", typ: tar.TypeReg}})
			if err := extractTarGz(src, dest); err == nil {
				t.Fatalf("extracting %q succeeded; want error", name)
			}
			if _, err := os.Stat(filepath.Join(parent, "evil")); err == nil {
				t.Fatal("file was written outside dest")
			}
		})
	}
}

func TestExtractTarGzRejectsEscapingSymlink(t *testing.T) {
	for _, link := range []string{"../../etc", "/etc"} {
		dest, _ := newDest(t)
		src := makeTarGz(t, []tarEntry{{name: "lnk", link: link, typ: tar.TypeSymlink}})
		if err := extractTarGz(src, dest); err == nil {
			t.Fatalf("symlink -> %q accepted; want error", link)
		}
	}
}

func TestExtractTarGzKeepsInternalSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dest, _ := newDest(t)
	src := makeTarGz(t, []tarEntry{
		{name: "node-v20/", typ: tar.TypeDir},
		{name: "node-v20/lib/node_modules/npm/bin/npm-cli.js", body: "cli", typ: tar.TypeReg},
		{name: "node-v20/bin/npm", link: "../lib/node_modules/npm/bin/npm-cli.js", typ: tar.TypeSymlink},
		{name: "node-v20/bin/node", body: "bin", typ: tar.TypeReg},
	})
	if err := extractTarGz(src, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "node-v20", "bin", "npm"))
	if err != nil || string(got) != "cli" {
		t.Fatalf("bin/npm should resolve to npm-cli.js, got %q err=%v", got, err)
	}
	info, err := os.Stat(filepath.Join(dest, "node-v20", "bin", "node"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("bin/node should be executable, mode=%v err=%v", info.Mode(), err)
	}
}

func TestExtractRefusesWriteThroughExistingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dest, parent := newDest(t)
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "pre")); err != nil {
		t.Fatal(err)
	}
	src := makeTarGz(t, []tarEntry{{name: "pre/evil", body: "x", typ: tar.TypeReg}})
	if err := extractTarGz(src, dest); err == nil {
		t.Fatal("wrote through a symlink that points outside dest")
	}
	if _, err := os.Stat(filepath.Join(outside, "evil")); err == nil {
		t.Fatal("file landed outside dest")
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	dest, parent := newDest(t)
	path := filepath.Join(t.TempDir(), "a.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../evil")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("x"))
	zw.Close()
	f.Close()

	if err := extractZip(path, dest); err == nil {
		t.Fatal("zip traversal accepted")
	}
	if _, err := os.Stat(filepath.Join(parent, "evil")); err == nil {
		t.Fatal("file was written outside dest")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run 'Extract' -v`
Expected: `TestExtractTarGzRejectsTraversal`, `TestExtractTarGzRejectsEscapingSymlink`, `TestExtractRefusesWriteThroughExistingSymlink` and `TestExtractZipRejectsTraversal` FAIL. `TestExtractTarGzKeepsInternalSymlinks` FAILS too, because symlinks are dropped today.

- [ ] **Step 3: Implement**

In `internal/env/runtimes/utils.go`, add `"errors"` and `"strings"` to the imports and replace `extractTarGz` and `extractZip` with:

```go
// safeJoin joins an archive entry name onto root, rejecting names that would
// land outside root: absolute paths, volume names, or ".." escapes.
func safeJoin(root, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("archive entry has an empty name")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	sep := string(filepath.Separator)
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || strings.HasPrefix(clean, sep) ||
		clean == ".." || strings.HasPrefix(clean, ".."+sep) {
		return "", fmt.Errorf("archive entry %q escapes the destination", name)
	}
	rootClean := filepath.Clean(root)
	target := filepath.Join(rootClean, clean)
	if target != rootClean && !strings.HasPrefix(target, rootClean+sep) {
		return "", fmt.Errorf("archive entry %q escapes the destination", name)
	}
	return target, nil
}

// ensureRealParentWithin resolves symlinks in target's parent directory and
// fails if the real location is outside root. It stops archives from writing
// through a symlink (planted earlier or pre-existing) that points elsewhere.
func ensureRealParentWithin(root, target string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return err
	}
	sep := string(filepath.Separator)
	if realParent != realRoot && !strings.HasPrefix(realParent, realRoot+sep) {
		return fmt.Errorf("refusing to write %s: its directory resolves outside the destination", target)
	}
	return nil
}

// writeEntry creates target (inside root) with r's contents. Permissions are
// limited to 0755 and the owner always gets rw.
func writeEntry(root, target string, mode os.FileMode, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := ensureRealParentWithin(root, target); err != nil {
		return err
	}
	_ = os.Remove(target) // never write through a pre-existing symlink at target
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, (mode.Perm()&0o755)|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// makeSymlink creates target -> linkname only if linkname is relative and
// resolves (lexically) inside root.
func makeSymlink(root, target, linkname string) error {
	if linkname == "" || filepath.IsAbs(linkname) || filepath.VolumeName(linkname) != "" {
		return fmt.Errorf("symlink %s -> %q: only relative targets are allowed", target, linkname)
	}
	rootClean := filepath.Clean(root)
	resolved := filepath.Join(filepath.Dir(target), filepath.FromSlash(linkname))
	sep := string(filepath.Separator)
	if resolved != rootClean && !strings.HasPrefix(resolved, rootClean+sep) {
		return fmt.Errorf("symlink %s -> %q escapes the destination", target, linkname)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := ensureRealParentWithin(root, target); err != nil {
		return err
	}
	_ = os.Remove(target)
	return os.Symlink(linkname, target)
}

// extractTarGz extracts a .tar.gz into dest. Every entry is confined to dest;
// relative symlinks and hardlinks inside dest are preserved (Node's bin/npm,
// bin/npx and bin/corepack are symlinks).
func extractTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(dest, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeEntry(dest, target, os.FileMode(header.Mode), tr); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := makeSymlink(dest, target, header.Linkname); err != nil {
				return err
			}
		case tar.TypeLink:
			linkSrc, err := safeJoin(dest, header.Linkname)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := ensureRealParentWithin(dest, target); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Link(linkSrc, target); err != nil {
				return err
			}
		}
		// Other types (devices, FIFOs, PAX metadata) are intentionally skipped.
	}
}

// extractZip extracts a .zip into dest with the same confinement rules.
func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target, err := safeJoin(dest, f.Name)
		if err != nil {
			return err
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			rc, err := f.Open()
			if err != nil {
				return err
			}
			link, err := io.ReadAll(io.LimitReader(rc, 4096))
			rc.Close()
			if err != nil {
				return err
			}
			if err := makeSymlink(dest, target, string(link)); err != nil {
				return err
			}
		default:
			rc, err := f.Open()
			if err != nil {
				return err
			}
			err = writeEntry(dest, target, mode, rc)
			rc.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/env/runtimes/ -v -run Extract`
Expected: PASS (on Windows, the two symlink tests SKIP)

Because extraction now keeps symlinks, Node's `bin/npm`/`bin/npx`/`bin/corepack` arrive intact. The workaround in `node.go` that rebuilds two of them (around lines 341–368) may now fail with "file exists". Run: `go test ./internal/env/...`. If you can, also run `go build -o /tmp/xpm-t ./cmd/xpm && /tmp/xpm-t env install node@20.11.0`. If the workaround errors, change its `os.Symlink` calls to skip when `os.Lstat(target)` already succeeds. Delete nothing else in `node.go`; P5 rewrites it.

- [ ] **Step 5: Commit**

```bash
git add internal/env/runtimes/utils.go internal/env/runtimes/extract_test.go internal/env/runtimes/node.go
git commit -m "security(env): confine archive extraction to dest; preserve internal symlinks"
```

---

### Task 2: Strict runtime/version validation wherever they become paths

**Files:**
- Create: `internal/env/validate.go`, `internal/env/validate_test.go`
- Modify: `internal/cli/env_cmd.go` (`parseRuntimeVersion`, around lines 211–217)
- Modify: `internal/env/remove.go` (`RemoveVersion`)
- Modify: `internal/env/install.go` (the `dest := filepath.Join(manager.GetRuntimesPath(), runtime, resolvedVersion)` line, around line 82; plus validation at function entry)
- Modify: `internal/env/manager.go` (`getLocalVersion`)
- Modify: `internal/env/detect.go` (`ActivateVersions`)
- Modify: `internal/env/shims.go` (shim template, lines 102–282)

**Interfaces:**
- Produces (package `env`):
  - `func ValidateRuntimeName(name string) error`: matches `^[a-z][a-z0-9]{0,31}$`.
  - `func ValidateVersionSpec(v string) error`: matches `^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$` and contains no `..`.
  - `func (m *Manager) versionDir(runtime, version string) (string, error)`: validated path strictly inside `GetRuntimesPath()`.

- [ ] **Step 1: Write the failing tests**

Create `internal/env/validate_test.go`:

```go
package env

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
)

// isolate chdirs into a fresh temp dir (the repo root has its own .xpm-env)
// and returns a Manager rooted in another temp dir.
func isolate(t *testing.T) *Manager {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	cfg := config.Config{Env: config.EnvConfig{Enabled: true, Path: t.TempDir()}}
	m, err := NewManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestValidateVersionSpec(t *testing.T) {
	ok := []string{"20.11.0", "lts", "latest", "stable", "1.22rc1", "8.3", "21.0.2+13", "1.75.0-beta.1"}
	bad := []string{"", "..", "../x", "a/b", `a\b`, "1..2", " 1", "-1", ".hidden", strings.Repeat("9", 65)}
	for _, v := range ok {
		if err := ValidateVersionSpec(v); err != nil {
			t.Errorf("ValidateVersionSpec(%q) = %v, want nil", v, err)
		}
	}
	for _, v := range bad {
		if err := ValidateVersionSpec(v); err == nil {
			t.Errorf("ValidateVersionSpec(%q) = nil, want error", v)
		}
	}
}

func TestValidateRuntimeName(t *testing.T) {
	for _, n := range []string{"node", "python", "go", "php"} {
		if err := ValidateRuntimeName(n); err != nil {
			t.Errorf("%q: %v", n, err)
		}
	}
	for _, n := range []string{"", "..", "../x", "Node", "no de", "node/x"} {
		if err := ValidateRuntimeName(n); err == nil {
			t.Errorf("ValidateRuntimeName(%q) = nil, want error", n)
		}
	}
}

func TestRemoveVersionRejectsDangerousInput(t *testing.T) {
	m := isolate(t)
	for _, v := range []string{"18.0.0", "20.0.0"} {
		if err := os.MkdirAll(filepath.Join(m.GetRuntimesPath(), "node", v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := [][2]string{{"node", ""}, {"node", ".."}, {"node", "."}, {"../x", "1"}, {"", "1"}}
	for _, c := range cases {
		if err := RemoveVersion(m, c[0], c[1]); err == nil {
			t.Errorf("RemoveVersion(%q, %q) = nil, want error", c[0], c[1])
		}
	}
	for _, v := range []string{"18.0.0", "20.0.0"} {
		if _, err := os.Stat(filepath.Join(m.GetRuntimesPath(), "node", v)); err != nil {
			t.Fatalf("node %s was deleted by a rejected call", v)
		}
	}
	if err := RemoveVersion(m, "node", "18.0.0"); err != nil {
		t.Fatalf("legit removal failed: %v", err)
	}
}

func TestLocalEnvIgnoresPathLikeVersion(t *testing.T) {
	m := isolate(t)
	if err := os.WriteFile(".xpm-env", []byte("node=../../../../tmp/evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.GetActiveVersion("node"); strings.Contains(v, "..") {
		t.Fatalf("GetActiveVersion returned path-like version %q from .xpm-env", v)
	}
}

func TestShimTemplateGuardsVersion(t *testing.T) {
	code, err := generateShimCode("node", "node", "/opt/xpm env")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "shim.go", code, 0); err != nil {
		t.Fatalf("generated shim is not valid Go: %v", err)
	}
	if !strings.Contains(code, "!safeVersion(version)") {
		t.Fatal("shim does not validate the resolved version before using it as a path")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/ -v -run 'Validate|RemoveVersion|LocalEnv|ShimTemplate'`
Expected: build failure `undefined: ValidateVersionSpec` / `ValidateRuntimeName`

- [ ] **Step 3: Implement the validators**

Create `internal/env/validate.go`:

```go
package env

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	runtimeNameRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,31}$`)
	versionSpecRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,63}$`)
)

// ValidateRuntimeName accepts lowercase identifiers such as "node" or "python".
func ValidateRuntimeName(name string) error {
	if !runtimeNameRe.MatchString(name) {
		return fmt.Errorf("invalid runtime name %q", name)
	}
	return nil
}

// ValidateVersionSpec accepts versions and aliases ("20.11.0", "lts",
// "1.22rc1", "21.0.2+13") and rejects anything that could act as a path:
// separators, a leading dot or dash, "..", whitespace, or empty strings.
func ValidateVersionSpec(v string) error {
	if !versionSpecRe.MatchString(v) || strings.Contains(v, "..") {
		return fmt.Errorf("invalid version %q", v)
	}
	return nil
}

// versionDir returns <runtimes>/<runtime>/<version>, guaranteeing that it is
// strictly inside the runtimes root.
func (m *Manager) versionDir(runtime, version string) (string, error) {
	if err := ValidateRuntimeName(runtime); err != nil {
		return "", err
	}
	if err := ValidateVersionSpec(version); err != nil {
		return "", err
	}
	root := filepath.Clean(m.GetRuntimesPath())
	dir := filepath.Join(root, runtime, version)
	if !strings.HasPrefix(dir, root+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing path outside %s: %s", root, dir)
	}
	return dir, nil
}
```

- [ ] **Step 4: Use them at every boundary**

`internal/env/remove.go`, first line of `RemoveVersion`. Replace

```go
	versionPath := filepath.Join(manager.GetRuntimesPath(), runtime, version)
```

with

```go
	versionPath, err := manager.versionDir(runtime, version)
	if err != nil {
		return err
	}
```

Then delete the now-unused `"path/filepath"` import from `remove.go`.

`internal/env/install.go`: replace

```go
	dest := filepath.Join(manager.GetRuntimesPath(), runtime, resolvedVersion)
```

with

```go
	dest, err := manager.versionDir(runtime, resolvedVersion)
	if err != nil {
		return err
	}
```

(If the compiler reports `no new variables` or a shadowing complaint, rename to `dest, derr :=` and return `derr`.) This one change covers `latest`/`lts` resolution, because the *resolved* version is what becomes the path.

`internal/env/manager.go`, `getLocalVersion`: replace the final `return versions[runtime]` with

```go
	v := versions[runtime]
	if ValidateVersionSpec(v) != nil {
		return "" // ignore malformed or path-like entries from untrusted .xpm-env files
	}
	return v
```

`internal/env/detect.go`, `ActivateVersions`: as the first statement inside the `for runtime, version := range versions {` loop, add

```go
		if ValidateRuntimeName(runtime) != nil || ValidateVersionSpec(version) != nil {
			continue
		}
```

`internal/cli/env_cmd.go`: replace `parseRuntimeVersion` with

```go
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

`internal/env/shims.go`, inside the template string (between line 102 `tmpl := \`package main` and the closing backtick at line 282):

(a) Right after the `version := getActiveVersion(runtime, envPath)` / `if version == "" {...}` block in the template's `main`, add:

```go
	if !safeVersion(version) {
		fmt.Fprintf(os.Stderr, "xpm: refusing unsafe version %q for %s (check .xpm-env)\n", version, runtime)
		os.Exit(1)
	}
```

(b) At the end of the template (before the closing backtick), add:

```go
// safeVersion mirrors env.ValidateVersionSpec; shims are standalone binaries.
func safeVersion(v string) bool {
	if v == "" || len(v) > 64 || strings.Contains(v, "..") || v[0] == '.' || v[0] == '-' {
		return false
	}
	for _, r := range v {
		ok := r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r == '.' || r == '_' || r == '+' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}
```

The template already imports `strings`. Because `{{.EnvPath}}` is still pasted unescaped (fixed structurally in P5), change the template line `envPath := "{{.EnvPath}}"` to `envPath := {{printf "%q" .EnvPath}}` so paths containing `\` or `"` produce valid Go. The test passes `"/opt/xpm env"` to exercise this.

- [ ] **Step 5: Run the tests**

Run: `go test -race ./internal/env/... ./internal/cli/ -v -run 'Validate|RemoveVersion|LocalEnv|ShimTemplate|Extract'`
Expected: PASS
Run: `go build -o /tmp/xpm-t ./cmd/xpm && /tmp/xpm-t env remove node@; echo "exit=$?"`
Expected: `invalid version ""` and `exit=1`, with nothing deleted.

- [ ] **Step 6: Commit**

```bash
git add internal/env/ internal/cli/env_cmd.go
git commit -m "security(env): validate runtime/version before using them as paths"
```

---

### Task 3: Package names can't become options (executed by lane A)

**Files:**
- Modify: `internal/pm/validation.go` (`ValidatePackageName`, `ValidateGenericPackageName`, `ValidateVersion`)
- Create: `internal/pm/validation_test.go`
- Modify: `internal/cli/search_cmd.go` (`installFromSearchResult`, before `adapter.InstallPackage(result.Name, ...)`, around line 159)

**Interfaces:** No signature changes. The three validators now reject a leading `-`.

- [ ] **Step 1: Write the failing test**

Create `internal/pm/validation_test.go`:

```go
package pm

import "testing"

func TestValidatorsRejectLeadingDash(t *testing.T) {
	for _, name := range []string{"-g", "--global", "--working-dir=/etc", "-rf"} {
		if ValidateGenericPackageName(name) == nil {
			t.Errorf("ValidateGenericPackageName(%q) = nil, want error", name)
		}
		for _, id := range []ID{Npm, Yarn, Pip, Composer, Cargo, GoMod, Maven, Gradle} {
			if ValidatePackageName(name, id) == nil {
				t.Errorf("ValidatePackageName(%q, %s) = nil, want error", name, id)
			}
		}
	}
	if ValidateVersion("--registry=http://evil") == nil {
		t.Error("ValidateVersion accepted an option-looking version")
	}
}

func TestValidatorsStillAcceptRealNames(t *testing.T) {
	ok := map[ID]string{Npm: "@types/node", Pip: "requests", Composer: "monolog/monolog", Cargo: "serde", GoMod: "github.com/gin-gonic/gin", Maven: "com.google.guava:guava"}
	for id, name := range ok {
		if err := ValidatePackageName(name, id); err != nil {
			t.Errorf("ValidatePackageName(%q, %s) = %v", name, id, err)
		}
	}
	if err := ValidateVersion("1.2.3-beta.1"); err != nil {
		t.Error(err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/pm/ -run 'Validators' -v`
Expected: FAIL (`-g`, `--global`, etc. are accepted for several managers)

- [ ] **Step 3: Implement**

In `internal/pm/validation.go`, insert this right after the empty-string check in **each** of `ValidatePackageName`, `ValidateGenericPackageName` and `ValidateVersion`. Use field name `"package name"` in the first two and `"version"` in `ValidateVersion`:

```go
	// A leading dash would be parsed as an option by npm/composer/pip/cargo.
	if strings.HasPrefix(pkg, "-") {
		return NewValidationError("package name", pkg, "cannot start with '-'")
	}
```

In `ValidateVersion`, the variable is `version`:

```go
	if strings.HasPrefix(version, "-") {
		return NewValidationError("version", version, "cannot start with '-'")
	}
```

In `internal/cli/search_cmd.go`, immediately before `if err := adapter.InstallPackage(result.Name, false, nil, result.Extra); err != nil {`, add:

```go
	// Names here come from registry responses, not the user: validate them too.
	if err := pm.ValidatePackageName(result.Name, pmID); err != nil {
		fmt.Fprintf(os.Stderr, "Refusing to install %q: %v\n", result.Name, err)
		return 1
	}
```

- [ ] **Step 4: Run the tests**

Run: `go test -race ./internal/pm/ ./internal/cli/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pm/validation.go internal/pm/validation_test.go internal/cli/search_cmd.go
git commit -m "security(pm): reject option-like package names and versions; validate TUI picks"
```

---

### Task 4: SHA-256-verified downloads for Node and Go

**Files:**
- Create: `internal/env/runtimes/download.go`, `internal/env/runtimes/download_test.go`
- Modify: `internal/env/runtimes/node.go` (download block, around lines 223–240)
- Modify: `internal/env/runtimes/go.go` (download block around lines 108–136; `ListRemote` URL at line 30)

**Interfaces:**
- Produces (package `runtimes`):
  - `var downloadClient *http.Client`: overall 15 min timeout; 10 s TLS handshake; 30 s response-header timeout.
  - `func fetchSmall(url string) ([]byte, error)`: requires 200; body capped at 8 MiB.
  - `func downloadVerified(url, wantHex string) (string, error)`: returns a temp file path; the caller removes it.
  - `func checksumFromSums(sums, filename string) (string, error)`: parses the `SHASUMS256.txt` format.
  - `func goChecksum(releasesJSON []byte, filename string) (string, error)`.
  - `var nodeDistURL = "https://nodejs.org/dist"` and `var goDLURL = "https://go.dev/dl"`.

- [ ] **Step 1: Write the failing tests**

Create `internal/env/runtimes/download_test.go`:

```go
package runtimes

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func serve(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// isolateTemp makes os.CreateTemp("", ...) use a fresh dir we can inspect.
func isolateTemp(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	return dir
}

func TestDownloadVerifiedAcceptsMatchingHash(t *testing.T) {
	isolateTemp(t)
	url := serve(t, "archive-bytes")
	path, err := downloadVerified(url, sha("archive-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	got, _ := os.ReadFile(path)
	if string(got) != "archive-bytes" {
		t.Fatalf("content = %q", got)
	}
}

func TestDownloadVerifiedRejectsMismatchAndCleansUp(t *testing.T) {
	tmp := isolateTemp(t)
	url := serve(t, "tampered")
	if _, err := downloadVerified(url, sha("original")); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 0 {
		t.Fatalf("partial download left behind: %v", entries)
	}
}

func TestDownloadVerifiedRefusesWithoutChecksum(t *testing.T) {
	isolateTemp(t)
	if _, err := downloadVerified(serve(t, "x"), ""); err == nil {
		t.Fatal("download without checksum accepted")
	}
}

func TestChecksumFromSums(t *testing.T) {
	sums := "f76a47616ceb47b9766cb7182ec6b53100192349de6a8aebb11f3abce045748f  node-v20.11.0-aix-ppc64.tar.gz\n" +
		"94e443d007e2882f8e5aecc85d978f7591520dc3b642adc7583b3cb0b3fc37d7  node-v20.11.0-darwin-arm64.tar.gz\n"
	got, err := checksumFromSums(sums, "node-v20.11.0-darwin-arm64.tar.gz")
	if err != nil || got != "94e443d007e2882f8e5aecc85d978f7591520dc3b642adc7583b3cb0b3fc37d7" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := checksumFromSums(sums, "node-v20.11.0-linux-x64.tar.gz"); err == nil {
		t.Fatal("missing file must be an error")
	}
}

func TestGoChecksum(t *testing.T) {
	js := []byte(`[{"version":"go1.22.0","files":[
	  {"filename":"go1.22.0.darwin-arm64.tar.gz","sha256":"aaaa"},
	  {"filename":"go1.22.0.linux-amd64.tar.gz","sha256":"bbbb"}]}]`)
	got, err := goChecksum(js, "go1.22.0.linux-amd64.tar.gz")
	if err != nil || got != "bbbb" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := goChecksum(js, "go1.22.0.windows-amd64.zip"); err == nil {
		t.Fatal("missing file must be an error")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/env/runtimes/ -run 'Download|Checksum' -v`
Expected: build failure `undefined: downloadVerified`

- [ ] **Step 3: Implement the helpers**

Create `internal/env/runtimes/download.go`:

```go
package runtimes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	nodeDistURL = "https://nodejs.org/dist"
	goDLURL     = "https://go.dev/dl"
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

// fetchSmall GETs a metadata document (checksum list, release JSON), capped at 8 MiB.
func fetchSmall(url string) ([]byte, error) {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// downloadVerified streams url to a temp file while hashing it, and returns
// the temp path only if its SHA-256 equals wantHex. On any failure the temp
// file is removed. The caller must os.Remove the returned path when done.
func downloadVerified(url, wantHex string) (string, error) {
	if len(wantHex) != sha256.Size*2 {
		return "", fmt.Errorf("refusing to download %s: no valid SHA-256 to verify against", url)
	}
	resp, err := downloadClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "xpm-dl-*")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmp.Name())
		if copyErr != nil {
			return "", fmt.Errorf("download %s: %w", url, copyErr)
		}
		return "", closeErr
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, wantHex) {
		os.Remove(tmp.Name())
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

- [ ] **Step 4: Run the helper tests**

Run: `go test -race ./internal/env/runtimes/ -run 'Download|Checksum' -v`
Expected: PASS

- [ ] **Step 5: Wire up Node**

In `internal/env/runtimes/node.go` (`InstallWithAlias`), replace everything from

```go
	url := fmt.Sprintf("https://nodejs.org/dist/v%s/%s", version, filename)
```

through the line `tmpFile.Close()` that follows the `io.Copy` (the old lines ~224–240: `http.Get`, status check, `os.CreateTemp`, `defer os.Remove`, `io.Copy`, `Close`) with:

```go
	base := fmt.Sprintf("%s/v%s", nodeDistURL, version)
	sums, err := fetchSmall(base + "/SHASUMS256.txt")
	if err != nil {
		return fmt.Errorf("fetch Node.js checksums: %w", err)
	}
	want, err := checksumFromSums(string(sums), filename)
	if err != nil {
		return err
	}
	fmt.Printf("Downloading %s/%s...\n", base, filename)
	archive, err := downloadVerified(base+"/"+filename, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
```

Then replace every later `tmpFile.Name()` in this function with `archive` (the two `extractZip`/`extractTarGz` calls). Remove any imports that `go build` reports as unused (`net/http`, `io`).

- [ ] **Step 6: Wire up Go**

In `internal/env/runtimes/go.go`:

In `ListRemote`, change `http.Get("https://go.dev/dl/?mode=json")` to `downloadClient.Get(goDLURL + "/?mode=json&include=all")`. Without `include=all` only the two newest releases are listed.

In `Install`, replace everything from

```go
	url := fmt.Sprintf("https://go.dev/dl/%s", filename)
```

through the `tmpFile.Close()` after `io.Copy` (old lines ~109–136) with:

```go
	meta, err := fetchSmall(goDLURL + "/?mode=json&include=all")
	if err != nil {
		return fmt.Errorf("fetch Go release list: %w", err)
	}
	want, err := goChecksum(meta, filename)
	if err != nil {
		return err
	}
	fmt.Printf("Downloading %s/%s...\n", goDLURL, filename)
	archive, err := downloadVerified(goDLURL+"/"+filename, want)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
```

Then replace `extractTarGz(tmpFile.Name(), dest)` with `extractTarGz(archive, dest)`. Remove now-unused imports.

- [ ] **Step 7: Verify end-to-end (needs network)**

Run: `go build ./... && go test -race ./internal/env/...` → Expected: PASS
Run: `go build -o /tmp/xpm-t ./cmd/xpm && HOME=$(mktemp -d) /tmp/xpm-t env install node@20.11.0`
Expected: downloads, verifies, installs; `bin/npm` exists as a symlink (thanks to Task 1).
Run: `HOME=$(mktemp -d) /tmp/xpm-t env install go@1.22.0` → Expected: installs.

- [ ] **Step 8: Commit**

```bash
git add internal/env/runtimes/
git commit -m "security(env): verify Node and Go downloads against published SHA-256"
```

---

### Task 5: Remove the unverified rustup bootstrap

**Files:**
- Modify: `internal/env/runtimes/rust.go` (`Install`; delete `installRustup`)
- Create: `internal/env/runtimes/rust_test.go`

**Interfaces:** Produces: `(*RustInstaller).Install` errors with an actionable message when `rustup` isn't on PATH.

- [ ] **Step 1: Write the failing test**

Create `internal/env/runtimes/rust_test.go`:

```go
package runtimes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRustInstallWithoutRustupFailsSafely(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no rustup anywhere
	dest := filepath.Join(t.TempDir(), "rust", "1.75.0")

	err := (&RustInstaller{}).Install("1.75.0", dest)
	if err == nil || !strings.Contains(err.Error(), "rustup.rs") {
		t.Fatalf("want an error pointing at https://rustup.rs, got %v", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatal("dest must not be created")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/env/runtimes/ -run TestRustInstallWithoutRustupFailsSafely -v`
Expected: FAIL. Today it tries to download a nonexistent URL and run the response.

- [ ] **Step 3: Implement**

In `internal/env/runtimes/rust.go`, replace `Install` with:

```go
// Install installs a Rust toolchain via the user's rustup. xpm deliberately
// does not bootstrap rustup itself: that meant running an unverified download
// that also rewrote ~/.cargo and shell profiles.
func (r *RustInstaller) Install(version string, dest string) error {
	if !rustupExists() {
		return fmt.Errorf("rust needs rustup: install it from https://rustup.rs, then re-run `xpm env install rust@%s`", version)
	}
	return r.installViaRustup(version, dest)
}
```

Delete the whole `installRustup` function. Remove the imports `go build` reports as unused (likely `io`, `net/http`, `runtime`).

- [ ] **Step 4: Run all P2 tests**

Run: `go vet ./... && go test -race ./...`
Expected: PASS

- [ ] **Step 5: Commit and open the phase PR**

```bash
git add internal/env/runtimes/rust.go internal/env/runtimes/rust_test.go
git commit -m "security(env): stop downloading and executing an unverified rustup installer"
git push -u origin HEAD
gh pr create --base develop --title "P2: security hardening" --body "Implements docs/superpowers/plans/2026-10-06-xpm-p2-security.md (lane B: Tasks 1,2,4,5; lane A: Task 3)."
```

---

## Self-review notes

- **Spec coverage:** the roadmap's P2 scope bullets map to these tasks:
  - extraction → Task 1;
  - validation for install/remove/`.xpm-env`/shims → Task 2;
  - leading-dash and TUI names → Task 3;
  - Node/Go checksums plus a timeout client → Task 4;
  - rustup → Task 5.

  Bun/Deno/Java checksums are deliberately in P5 (those installers are broken anyway).
- **Type consistency:** `safeJoin`, `ensureRealParentWithin`, `writeEntry`, `makeSymlink`, `ValidateRuntimeName`, `ValidateVersionSpec`, `versionDir`, `downloadVerified`, `fetchSmall`, `checksumFromSums`, `goChecksum`, `nodeDistURL` and `goDLURL` are each defined once and used with matching signatures.
- **Known residual risk, accepted for P2:** shims are still compiled with `go build` (P5 replaces them). `.xpm-env` *runtime* keys aren't used as paths by shims (the shim's runtime name is baked in), so only the version needed guarding.
