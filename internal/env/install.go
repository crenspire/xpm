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
// installed one becomes the global default. When only that last step fails
// it returns exact and a *PostInstallError.
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
		return exact, m.afterInstall(ctx, rt, exact, dest, alias)
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
		return exact, m.afterInstall(ctx, rt, exact, dest, alias)
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
		if cerr := ctx.Err(); cerr != nil { // a killed subprocess reports its own error
			return "", fmt.Errorf("install %s@%s: %w", rt, exact, cerr)
		}
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
	return exact, m.afterInstall(ctx, rt, exact, "", "")
}

// PostInstallError means the version is installed but recording it (the lts
// alias or the global default) failed; the CLI reports it as a warning.
type PostInstallError struct{ Err error }

func (e *PostInstallError) Error() string { return e.Err.Error() }
func (e *PostInstallError) Unwrap() error { return e.Err }

// afterInstall records an "lts" alias on an existing install and sets the
// global default when the runtime has none, under the state lock. A failure
// is a *PostInstallError.
func (m *Manager) afterInstall(ctx context.Context, rt, exact, existingDir, alias string) error {
	if existingDir != "" && alias == "lts" && readMeta(existingDir).Alias != "lts" {
		if err := writeMeta(existingDir, versionMeta{Version: exact, Alias: alias}); err != nil {
			return &PostInstallError{Err: err}
		}
	}
	err := m.withStateLock(ctx, func() error {
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
	})
	if err != nil {
		return &PostInstallError{Err: err}
	}
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
