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
	a, err := m.ActiveVersion(runtime)
	if err != nil && !errors.Is(err, ErrNoVersion) && !errors.Is(err, ErrNotInstalled) {
		return fmt.Errorf("cannot check whether %s@%s is in use: %w", runtime, version, err)
	}
	if err == nil && a.Version == version {
		return fmt.Errorf("cannot remove %s@%s: it is active here (set in %s)\nSwitch first: xpm env use %s@<other-version>", runtime, version, a.Source, runtime)
	}
	g, err := m.GlobalVersion(runtime)
	if err != nil {
		return fmt.Errorf("cannot check whether %s@%s is the global default: %w", runtime, version, err)
	}
	if g != "" {
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
