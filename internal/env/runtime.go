package env

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
)

// RuntimeInstaller is implemented by every runtime (internal/env/runtimes).
type RuntimeInstaller interface {
	// Name returns the runtime name ("node").
	Name() string
	// ListRemote returns exact available versions, in any order.
	ListRemote(ctx context.Context) ([]string, error)
	// Install puts req.Version into req.Dest, an existing EMPTY staging dir.
	Install(ctx context.Context, req InstallRequest) error
	// BinaryPaths lists slash-separated paths, relative to the version dir,
	// that must exist after Install. Their base names become shims.
	BinaryPaths() []string
}

// InstallRequest is what InstallRuntime hands an installer.
type InstallRequest struct {
	Version string // exact version
	Dest    string // empty staging dir; renamed into place after verification
	Root    string // xpm env root, for shared tool homes (rustup)
}

// Resolver overrides the generic spec resolution of ResolveSpec.
type Resolver interface {
	Resolve(ctx context.Context, spec string) (string, error)
}

// LTSResolver gives the "lts" alias a meaning.
type LTSResolver interface {
	LatestLTS(ctx context.Context) (string, error)
}

// Remover cleans up state outside the version dir (rustup toolchains).
type Remover interface {
	Remove(ctx context.Context, version, dir, root string) error
}

var (
	installersMu sync.RWMutex
	installers   = make(map[string]RuntimeInstaller)
)

// RegisterInstaller registers a runtime installer.
func RegisterInstaller(name string, installer RuntimeInstaller) {
	installersMu.Lock()
	defer installersMu.Unlock()
	installers[name] = installer
}

// unregisterInstaller removes a test installer.
func unregisterInstaller(name string) {
	installersMu.Lock()
	defer installersMu.Unlock()
	delete(installers, name)
}

// GetInstaller returns the installer for the given runtime name.
func GetInstaller(name string) (RuntimeInstaller, error) {
	installersMu.RLock()
	installer, ok := installers[name]
	installersMu.RUnlock()
	if ok {
		return installer, nil
	}
	if owner, _, found := binaryOwner(name); found && owner != name {
		return nil, fmt.Errorf("%s is not a runtime; it comes with %s\nInstall it with: xpm env install %s@<version>", name, owner, owner)
	}
	return nil, fmt.Errorf("unknown runtime %q (available: %s)", name, strings.Join(ListRuntimes(), ", "))
}

// ListRuntimes returns all registered runtime names, sorted.
func ListRuntimes() []string {
	installersMu.RLock()
	defer installersMu.RUnlock()
	names := make([]string, 0, len(installers))
	for name := range installers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// binaryOwner finds the runtime whose BinaryPaths has base name binary.
func binaryOwner(binary string) (runtime, rel string, ok bool) {
	for _, name := range ListRuntimes() {
		installersMu.RLock()
		inst := installers[name]
		installersMu.RUnlock()
		for _, p := range inst.BinaryPaths() {
			if path.Base(p) == binary {
				return name, p, true
			}
		}
	}
	return "", "", false
}
