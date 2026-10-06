package env

import (
	"context"
	"fmt"
	"os"
)

// UseVersion resolves spec against INSTALLED versions (same matching as
// ActiveVersion) and records the exact version: in active.json when global,
// else in ./.xpm-env. It returns what was written and where.
func UseVersion(m *Manager, runtime, spec string, global bool) (Active, error) {
	if err := ValidateRuntimeName(runtime); err != nil {
		return Active{}, err
	}
	if err := ValidateVersionSpec(spec); err != nil {
		return Active{}, err
	}
	exact, ok := m.resolveInstalled(runtime, spec)
	if !ok {
		return Active{}, fmt.Errorf("%s@%s is not installed\nInstall it: xpm env install %s@%s", runtime, spec, runtime, spec)
	}
	if global {
		if err := m.withStateLock(context.Background(), func() error { return m.SetGlobalVersion(runtime, exact) }); err != nil {
			return Active{}, err
		}
		return Active{Version: exact, Source: m.activePath, Global: true}, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Active{}, err
	}
	var path string
	err = m.withStateLock(context.Background(), func() error {
		var err error
		path, err = SetLocalVersion(cwd, runtime, exact)
		return err
	})
	if err != nil {
		return Active{}, err
	}
	return Active{Version: exact, Source: path}, nil
}
