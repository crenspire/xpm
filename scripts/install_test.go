package scripts_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fakeTag = "v1.2.3"

// fakeArchiveName is the archive install.sh picks on this machine.
func fakeArchiveName() string {
	return "xpm_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
}

// skipUnlessRunnable skips when install.sh cannot run here.
func skipUnlessRunnable(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("install.sh supports linux and darwin, not %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("install.sh supports amd64 and arm64, not %s", runtime.GOARCH)
	}
	for _, tool := range []string{"sh", "tar", "awk", "mktemp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found", tool)
		}
	}
	if !anyOnPath("sha256sum", "shasum") {
		t.Skip("no sha256 tool (sha256sum or shasum)")
	}
	if !anyOnPath("curl", "wget") {
		t.Skip("no downloader (curl or wget)")
	}
}

func anyOnPath(names ...string) bool {
	for _, n := range names {
		if _, err := exec.LookPath(n); err == nil {
			return true
		}
	}
	return false
}

// fakeArchive returns a .tar.gz holding an xpm shell script that prints "fake xpm".
func fakeArchive(t *testing.T) []byte {
	t.Helper()
	script := []byte("#!/bin/sh\necho fake xpm\n")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		data []byte
		mode int64
	}{
		{"README.md", []byte("readme\n"), 0o644},
		{"xpm", script, 0o755},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// serveRelease writes archive and checksums under <root>/v1.2.3/ and serves root.
func serveRelease(t *testing.T, archive []byte, checksums string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, fakeTag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fakeArchiveName()), archive, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(root)))
	t.Cleanup(srv.Close)
	return srv.URL
}

// runInstall runs install.sh against baseURL and returns the combined output,
// the install directory and the run error.
func runInstall(t *testing.T, baseURL string) (out, bin string, err error) {
	t.Helper()
	script, err := filepath.Abs("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	bin = filepath.Join(home, "bin")
	cmd := exec.Command("sh", script)
	cmd.Env = append(os.Environ(),
		"XPM_VERSION="+fakeTag,
		"XPM_DOWNLOAD_URL="+baseURL,
		"XPM_INSTALL_DIR="+bin,
		"HOME="+home,
		"NO_PROXY=127.0.0.1,localhost",
		"no_proxy=127.0.0.1,localhost",
	)
	b, err := cmd.CombinedOutput()
	return string(b), bin, err
}

func decoyLine() string {
	return strings.Repeat("ab", 32) + "  xpm_1.2.3_plan9_mips.tar.gz\n"
}

func TestInstallScriptInstallsVerifiedArchive(t *testing.T) {
	skipUnlessRunnable(t)
	archive := fakeArchive(t)
	url := serveRelease(t, archive, decoyLine()+sum(archive)+"  "+fakeArchiveName()+"\n")

	out, bin, err := runInstall(t, url)
	if err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, out)
	}
	want := "xpm " + fakeTag + " installed to " + filepath.Join(bin, "xpm")
	if !strings.Contains(out, want) {
		t.Errorf("output missing %q:\n%s", want, out)
	}
	got, err := exec.Command(filepath.Join(bin, "xpm")).Output()
	if err != nil {
		t.Fatalf("installed xpm does not run: %v", err)
	}
	if strings.TrimSpace(string(got)) != "fake xpm" {
		t.Errorf("installed xpm printed %q, want %q", got, "fake xpm")
	}
}

func TestInstallScriptRejectsChecksumMismatch(t *testing.T) {
	skipUnlessRunnable(t)
	archive := fakeArchive(t)
	url := serveRelease(t, archive, decoyLine()+sum([]byte("tampered"))+"  "+fakeArchiveName()+"\n")

	out, bin, err := runInstall(t, url)
	if err == nil {
		t.Fatalf("install.sh succeeded on a checksum mismatch:\n%s", out)
	}
	if !strings.Contains(out, "checksum mismatch") {
		t.Errorf("output missing %q:\n%s", "checksum mismatch", out)
	}
	if _, statErr := os.Stat(filepath.Join(bin, "xpm")); !os.IsNotExist(statErr) {
		t.Errorf("xpm was installed despite the mismatch (stat err %v)", statErr)
	}
}

func TestInstallScriptRejectsMissingChecksum(t *testing.T) {
	skipUnlessRunnable(t)
	archive := fakeArchive(t)
	url := serveRelease(t, archive, decoyLine())

	out, bin, err := runInstall(t, url)
	if err == nil {
		t.Fatalf("install.sh succeeded without a checksum entry:\n%s", out)
	}
	if !strings.Contains(out, "no entry") {
		t.Errorf("output missing %q:\n%s", "no entry", out)
	}
	if _, statErr := os.Stat(filepath.Join(bin, "xpm")); !os.IsNotExist(statErr) {
		t.Errorf("xpm was installed without a checksum entry (stat err %v)", statErr)
	}
}
