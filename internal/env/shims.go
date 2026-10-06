package env

import (
	"context"
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
	m, err := newManager(cfg, false) // no writes on every runtime invocation
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

// shimEnv builds the child environment. For a managed version (binDir set)
// binDir is prepended to PATH, so nested `#!/usr/bin/env node` calls hit the
// same version directly, and XPM_SHIM_DEPTH is dropped: that child is a real
// binary and cannot loop back. For the system fallback (binDir empty)
// XPM_SHIM_DEPTH is incremented and capped, since the "system" binary may be
// another xpm copy's shim.
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
	if binDir != "" {
		if hasPath && pathVal != "" {
			pathVal = binDir + string(os.PathListSeparator) + pathVal
		} else {
			pathVal = binDir
		}
		return append(out, "PATH="+pathVal), nil
	}
	depth++
	if depth > maxShimDepth {
		return nil, fmt.Errorf("shim recursion detected for %s", name)
	}
	if hasPath {
		out = append(out, "PATH="+pathVal)
	}
	return append(out, shimDepthVar+"="+strconv.Itoa(depth)), nil
}

// findSystemBinary searches pathEnv for an executable `name`, skipping
// relative entries (like exec.LookPath's ErrDot), the shims dir and anything
// that resolves to the xpm executable itself.
func findSystemBinary(name, pathEnv, shimsDir, self string) (string, bool) {
	realShims := realPath(shimsDir)
	realSelf := realPath(self)
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" || !filepath.IsAbs(dir) {
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
// everything else in the shims dir (old compiled shims, stale names) except
// dot-names and in-flight "*.tmp-*" links. It runs under the state lock, so
// concurrent installs of different runtimes never prune each other's shims.
func CreateShims(m *Manager) error {
	if m.executable == "" {
		return errors.New("cannot determine the path of the xpm executable")
	}
	return m.withStateLock(context.Background(), m.createShims)
}

func (m *Manager) createShims() error {
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
		name := e.Name()
		if want[name] || strings.HasPrefix(name, ".") || strings.Contains(name, ".tmp-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.shimsPath, name)); err != nil {
			return err
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
