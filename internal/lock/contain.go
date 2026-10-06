package lock

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// errOutsideRoot marks a path that leaves the project root.
var errOutsideRoot = errors.New("path is outside the project")

// checkLocal validates a slash-separated path read from xpm-lock.yaml. It
// must be relative, must not contain "..", backslashes, NUL bytes or a drive
// letter, and must name something below the root (not the root itself). The
// rules are the same on every OS, so a lock file cannot behave differently
// on Windows than it does on Linux.
func checkLocal(rel string) error {
	switch {
	case rel == "":
		return fmt.Errorf("empty path: %w", errOutsideRoot)
	case strings.ContainsRune(rel, '\\'):
		return fmt.Errorf("%q contains a backslash (use / separators): %w", rel, errOutsideRoot)
	case strings.ContainsRune(rel, 0):
		return fmt.Errorf("%q contains a NUL byte: %w", rel, errOutsideRoot)
	case strings.HasPrefix(rel, "/") || path.IsAbs(rel) || filepath.IsAbs(rel):
		return fmt.Errorf("%q is absolute: %w", rel, errOutsideRoot)
	case len(rel) >= 2 && rel[1] == ':':
		return fmt.Errorf("%q has a drive letter: %w", rel, errOutsideRoot)
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return fmt.Errorf("%q contains \"..\": %w", rel, errOutsideRoot)
		}
	}
	clean := path.Clean(rel)
	if clean == "." {
		return fmt.Errorf("%q names the project root, not a file: %w", rel, errOutsideRoot)
	}
	if !filepath.IsLocal(filepath.FromSlash(clean)) {
		return fmt.Errorf("%q is not a local path: %w", rel, errOutsideRoot)
	}
	return nil
}

// containedPath joins root and the slash-separated rel after checkLocal and,
// when the target exists, resolves symlinks and verifies that the result is
// inside root. It returns the resolved path, so the file checked is the file
// later opened. A target that does not exist (or a dangling symlink) is
// returned unresolved and without error (callers report it as missing); any
// other failure to inspect or resolve it is an error. Nothing outside root
// is ever returned.
func containedPath(root, rel string) (string, error) {
	if err := checkLocal(rel); err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(path.Clean(rel)))
	if _, err := os.Lstat(full); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return full, nil // missing: the caller reports it
		}
		return "", fmt.Errorf("inspect %q: %w", rel, err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	realFull, err := filepath.EvalSymlinks(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Dangling symlink: treat it as missing; it is never opened.
			return full, nil
		}
		return "", fmt.Errorf("resolve %q: %w", rel, err)
	}
	inside, err := filepath.Rel(realRoot, realFull)
	if err != nil || !filepath.IsLocal(inside) {
		return "", fmt.Errorf("%q resolves to %s: %w", rel, realFull, errOutsideRoot)
	}
	return realFull, nil
}

// fileExists reports whether path exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
