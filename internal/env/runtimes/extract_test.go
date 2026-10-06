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
