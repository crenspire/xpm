package env

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// RemoveVersion removes an installed version under the runtime lock. It
// refuses a version pinned by the .xpm-env in effect here; if the version is
// the global default, that active.json entry is cleared once the removal has
// succeeded (a failed removal leaves it untouched).
func RemoveVersion(ctx context.Context, m *Manager, runtime, version string) error {
	dir, err := m.versionDir(runtime, version)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s@%s is not installed", runtime, version)
	}
	unlock, err := lockFile(ctx, filepath.Join(filepath.Dir(dir), ".lock"), func() {
		m.printf("Waiting for another xpm process changing %s...\n", runtime)
	})
	if err != nil {
		return err
	}
	defer unlock()
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() { // another process removed it
		return fmt.Errorf("%s@%s is not installed", runtime, version)
	}
	var global string // the global default when it resolves to version
	if err := m.withStateLock(ctx, func() (err error) {
		global, err = m.checkRemovable(runtime, version)
		return err
	}); err != nil {
		return err
	}
	if inst, err := GetInstaller(runtime); err == nil {
		if r, ok := inst.(Remover); ok {
			if err := r.Remove(ctx, version, dir, m.envPath); err != nil {
				return fmt.Errorf("remove %s@%s: %w", runtime, version, err)
			}
		}
	}
	// Rename first so the version disappears atomically, then delete.
	trash := filepath.Join(filepath.Dir(dir), ".tmp-old-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.Rename(dir, trash); err != nil {
		return fmt.Errorf("remove %s@%s: %w", runtime, version, err)
	}
	rmErr := os.RemoveAll(trash)
	if global != "" {
		if err := m.withStateLock(ctx, func() error { return m.ClearGlobalVersion(runtime) }); err != nil {
			return err
		}
		m.printf("Cleared the global %s default (was %s)\n", runtime, global)
	}
	return rmErr
}

// checkRemovable refuses a version pinned by an .xpm-env in effect here. It
// returns the global default's value when that default resolves to version
// (the caller clears it after the removal), else "". The caller holds the
// state lock.
func (m *Manager) checkRemovable(runtime, version string) (string, error) {
	a, err := m.ActiveVersion(runtime)
	if err != nil && !errors.Is(err, ErrNoVersion) && !errors.Is(err, ErrNotInstalled) {
		return "", fmt.Errorf("cannot check whether %s@%s is in use: %w", runtime, version, err)
	}
	if err == nil && !a.Global && a.Version == version {
		return "", fmt.Errorf("cannot remove %s@%s: it is pinned in %s\nSwitch first: xpm env use %s@<other-version>", runtime, version, a.Source, runtime)
	}
	g, err := m.GlobalVersion(runtime)
	if err != nil {
		return "", fmt.Errorf("cannot check whether %s@%s is the global default: %w", runtime, version, err)
	}
	if g == "" {
		return "", nil
	}
	if exact, ok := m.resolveInstalled(runtime, g); !ok || exact != version {
		return "", nil
	}
	return g, nil
}
