package runtimes

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

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

// within reports whether path is root or lies under it (both already resolved).
func within(root, path string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// mkdirWithin creates dir (and missing parents) one component at a time from
// root, resolving every existing component. Any component that resolves outside
// root, or is not a directory, is an error; nothing is created through it.
func mkdirWithin(root, dir string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(dir))
	if err != nil {
		return err
	}
	sep := string(filepath.Separator)
	if rel == ".." || strings.HasPrefix(rel, ".."+sep) {
		return fmt.Errorf("directory %s is outside the destination", dir)
	}
	if rel == "." {
		return nil
	}
	cur := filepath.Clean(root)
	for _, part := range strings.Split(rel, sep) {
		cur = filepath.Join(cur, part)
		if _, err := os.Lstat(cur); errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(cur, 0o755); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(cur)
		if err != nil {
			return err
		}
		if !within(realRoot, resolved) {
			return fmt.Errorf("refusing to use %s: it resolves outside the destination", cur)
		}
		if fi, err := os.Stat(cur); err != nil || !fi.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", cur)
		}
	}
	return nil
}

// writeEntry creates target (inside root) with r's contents. Permissions are
// limited to 0755 and the owner always gets rw.
func writeEntry(root, target string, mode os.FileMode, r io.Reader) error {
	if err := mkdirWithin(root, filepath.Dir(target)); err != nil {
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
		_ = f.Close() // the copy error wins
		return err
	}
	return f.Close()
}

// checkRelativeLink accepts only relative link targets whose ".." parts
// form a leading run. A ".." after a real component would be followed by the
// kernel through whatever that component is (possibly another symlink), so
// lexical containment checks would lie.
func checkRelativeLink(link string) error {
	if link == "" || filepath.IsAbs(link) || filepath.VolumeName(link) != "" ||
		strings.HasPrefix(link, "/") || strings.HasPrefix(link, `\`) {
		return fmt.Errorf("link target %q: only relative targets are allowed", link)
	}
	seenName := false
	for _, part := range strings.Split(filepath.FromSlash(link), string(filepath.Separator)) {
		switch part {
		case "", ".":
		case "..":
			if seenName {
				return fmt.Errorf("link target %q: \"..\" after a path component is not allowed", link)
			}
		default:
			seenName = true
		}
	}
	return nil
}

// makeSymlink creates target -> linkname only if linkname is relative and
// resolves (lexically) inside root.
func makeSymlink(root, target, linkname string) error {
	if err := checkRelativeLink(linkname); err != nil {
		return fmt.Errorf("symlink %s: %w", target, err)
	}
	if err := mkdirWithin(root, filepath.Dir(target)); err != nil {
		return err
	}
	if err := ensureRealParentWithin(root, target); err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return err
	}
	native := filepath.Clean(filepath.FromSlash(linkname))
	if !within(realRoot, filepath.Join(realParent, native)) {
		return fmt.Errorf("symlink %s -> %q escapes the destination", target, linkname)
	}
	_ = os.Remove(target)
	return os.Symlink(native, target)
}

// verifySymlinksWithin walks root and fails if any symlink resolves outside it.
// Dangling links are resolved lexically from their real directory. It is the
// final guard against link chains assembled across entries.
func verifySymlinksWithin(root string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(realRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			link, lerr := os.Readlink(path)
			if lerr != nil {
				return lerr
			}
			if err := checkRelativeLink(link); err != nil {
				return fmt.Errorf("symlink %s: %w", path, err)
			}
			realDir, derr := filepath.EvalSymlinks(filepath.Dir(path))
			if derr != nil {
				return derr
			}
			resolved = filepath.Join(realDir, link)
		}
		if !within(realRoot, resolved) {
			return fmt.Errorf("symlink %s resolves outside the destination", path)
		}
		return nil
	})
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
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return verifySymlinksWithin(dest)
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
			if err := mkdirWithin(dest, target); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeEntry(dest, target, header.FileInfo().Mode(), tr); err != nil {
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
			if err := mkdirWithin(dest, filepath.Dir(target)); err != nil {
				return err
			}
			if err := ensureRealParentWithin(dest, target); err != nil {
				return err
			}
			realRoot, err := filepath.EvalSymlinks(dest)
			if err != nil {
				return err
			}
			realSrc, err := filepath.EvalSymlinks(linkSrc)
			if err != nil {
				return err
			}
			if !within(realRoot, realSrc) {
				return fmt.Errorf("hardlink %s -> %q resolves outside the destination", target, header.Linkname)
			}
			if fi, err := os.Lstat(linkSrc); err != nil || !fi.Mode().IsRegular() {
				return fmt.Errorf("hardlink %s -> %q: source is not a regular file", target, header.Linkname)
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
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		target, err := safeJoin(dest, f.Name)
		if err != nil {
			return err
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := mkdirWithin(dest, target); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			rc, err := f.Open()
			if err != nil {
				return err
			}
			link, err := io.ReadAll(io.LimitReader(rc, 4096))
			_ = rc.Close() // read-only; the read error is what matters
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
			_ = rc.Close() // read-only; the write error is what matters
			if err != nil {
				return err
			}
		}
	}
	return verifySymlinksWithin(dest)
}

// extractArchive extracts a .zip, .tar.gz or .tgz (chosen by archivePath's
// suffix) into dest with the confinement rules above.
func extractArchive(archivePath, dest string) error {
	switch {
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZip(archivePath, dest)
	case strings.HasSuffix(archivePath, ".tar.gz"), strings.HasSuffix(archivePath, ".tgz"):
		return extractTarGz(archivePath, dest)
	}
	return fmt.Errorf("unsupported archive type: %s", filepath.Base(archivePath))
}

// hoistDir moves everything in dest/sub up into dest and removes dest/sub
// (with whatever else sub's top directory held, e.g. a JDK's Contents/).
// sub is slash-separated. It refuses when a moved name already exists.
func hoistDir(dest, sub string) error {
	parts := strings.SplitN(sub, "/", 2)
	top := filepath.Join(dest, parts[0])
	src := filepath.Join(dest, filepath.FromSlash(sub))
	if fi, err := os.Lstat(src); err != nil || !fi.IsDir() {
		return fmt.Errorf("archive has no %s directory", sub)
	}
	// Park the top dir under a unique name so sub may contain its own name.
	parked, err := os.MkdirTemp(dest, ".xpm-hoist-")
	if err != nil {
		return err
	}
	if err := os.Remove(parked); err != nil {
		return err
	}
	if err := os.Rename(top, parked); err != nil {
		return err
	}
	if len(parts) == 2 {
		src = filepath.Join(parked, filepath.FromSlash(parts[1]))
	} else {
		src = parked
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := os.Lstat(filepath.Join(dest, e.Name())); err == nil {
			return fmt.Errorf("cannot hoist %s: %s already exists", sub, e.Name())
		}
	}
	for _, e := range entries {
		if err := os.Rename(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return err
		}
	}
	return os.RemoveAll(parked)
}

// singleTopDir returns the only directory at the root of dest (ignoring
// hidden entries), as found in JDK archives ("jdk-21.0.4+7/").
func singleTopDir(dest string) (string, error) {
	entries, err := os.ReadDir(dest)
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !e.IsDir() {
			return "", fmt.Errorf("archive has %s at its root; expected a single directory", e.Name())
		}
		dirs = append(dirs, e.Name())
	}
	if len(dirs) != 1 {
		return "", fmt.Errorf("archive has %d top-level directories; expected 1", len(dirs))
	}
	return dirs[0], nil
}
