package runtimes

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func serve(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// isolateTemp makes os.CreateTemp("", ...) use a fresh dir we can inspect.
func isolateTemp(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	return dir
}

func TestDownloadVerifiedAcceptsMatchingHash(t *testing.T) {
	isolateTemp(t)
	url := serve(t, "archive-bytes")
	path, err := downloadVerified(url, sha("archive-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	got, _ := os.ReadFile(path)
	if string(got) != "archive-bytes" {
		t.Fatalf("content = %q", got)
	}
}

func TestDownloadVerifiedRejectsMismatchAndCleansUp(t *testing.T) {
	tmp := isolateTemp(t)
	url := serve(t, "tampered")
	if _, err := downloadVerified(url, sha("original")); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 0 {
		t.Fatalf("partial download left behind: %v", entries)
	}
}

func TestDownloadVerifiedRefusesWithoutChecksum(t *testing.T) {
	isolateTemp(t)
	if _, err := downloadVerified(serve(t, "x"), ""); err == nil {
		t.Fatal("download without checksum accepted")
	}
}

func TestChecksumFromSums(t *testing.T) {
	sums := "f76a47616ceb47b9766cb7182ec6b53100192349de6a8aebb11f3abce045748f  node-v20.11.0-aix-ppc64.tar.gz\n" +
		"94e443d007e2882f8e5aecc85d978f7591520dc3b642adc7583b3cb0b3fc37d7  node-v20.11.0-darwin-arm64.tar.gz\n"
	got, err := checksumFromSums(sums, "node-v20.11.0-darwin-arm64.tar.gz")
	if err != nil || got != "94e443d007e2882f8e5aecc85d978f7591520dc3b642adc7583b3cb0b3fc37d7" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := checksumFromSums(sums, "node-v20.11.0-linux-x64.tar.gz"); err == nil {
		t.Fatal("missing file must be an error")
	}
}

func TestGoChecksum(t *testing.T) {
	js := []byte(`[{"version":"go1.22.0","files":[
	  {"filename":"go1.22.0.darwin-arm64.tar.gz","sha256":"aaaa"},
	  {"filename":"go1.22.0.linux-amd64.tar.gz","sha256":"bbbb"}]}]`)
	got, err := goChecksum(js, "go1.22.0.linux-amd64.tar.gz")
	if err != nil || got != "bbbb" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := goChecksum(js, "go1.22.0.windows-amd64.zip"); err == nil {
		t.Fatal("missing file must be an error")
	}
}
