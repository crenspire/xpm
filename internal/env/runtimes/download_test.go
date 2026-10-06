package runtimes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	path, err := downloadVerified(context.Background(), url, sha("archive-bytes"))
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
	if _, err := downloadVerified(context.Background(), url, sha("original")); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 0 {
		t.Fatalf("partial download left behind: %v", entries)
	}
}

func TestDownloadVerifiedRefusesWithoutChecksum(t *testing.T) {
	isolateTemp(t)
	if _, err := downloadVerified(context.Background(), serve(t, "x"), ""); err == nil {
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

func TestFetchSmallFailsInsteadOfTruncating(t *testing.T) {
	setVar(t, &maxSmallResponse, 16)
	url, _ := newServer(t, map[string]route{"/big": {body: []byte("0123456789abcdefX")}, "/ok": {body: []byte("0123456789abcdef")}})
	if _, err := fetchSmall(context.Background(), url+"/big"); err == nil || !strings.Contains(err.Error(), "response from "+url+"/big exceeds") {
		t.Fatalf("err = %v", err)
	}
	if b, err := fetchSmall(context.Background(), url+"/ok"); err != nil || len(b) != 16 {
		t.Fatalf("exactly the cap: %q %v", b, err)
	}
	if _, err := fetchSmall(context.Background(), url+"/missing"); !isNotFound(err) {
		t.Fatalf("404 not recognised: %v", err)
	}
}

func TestFetchHonoursContext(t *testing.T) {
	url := serve(t, "x")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchSmall(ctx, url); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if _, err := downloadVerified(ctx, url, sha("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("download err = %v", err)
	}
}

func TestFetchGitHubReleasesSendsTokenAndAccept(t *testing.T) {
	url, seen := newServer(t, map[string]route{"/repos/oven-sh/bun/releases?per_page=100": {body: []byte(`[{"tag_name":"bun-v1.4.2"}]`)}})
	setVar(t, &githubAPI, url)
	t.Setenv("GITHUB_TOKEN", "tok123")
	rels, err := fetchGitHubReleases(context.Background(), "oven-sh/bun")
	if err != nil || len(rels) != 1 || rels[0].TagName != "bun-v1.4.2" {
		t.Fatalf("got %+v, %v", rels, err)
	}
	r := (*seen)[0]
	if r.Header.Get("Authorization") != "Bearer tok123" || r.Header.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("headers = %v", r.Header)
	}
	t.Setenv("GITHUB_TOKEN", "")
	if _, err := fetchGitHubReleases(context.Background(), "oven-sh/bun"); err != nil {
		t.Fatal(err)
	}
	if h := (*seen)[1].Header.Get("Authorization"); h != "" {
		t.Fatalf("sent Authorization %q without a token", h)
	}
}

func TestDownloadVerifiedKeepsArchiveSuffix(t *testing.T) {
	isolateTemp(t)
	url, _ := newServer(t, map[string]route{"/a/node.tar.gz": {body: []byte("z")}})
	p, err := downloadVerified(context.Background(), url+"/a/node.tar.gz", sha("z"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	if !strings.HasSuffix(p, ".tar.gz") {
		t.Fatalf("temp file %s lost the archive suffix", p)
	}
}

func TestExtractArchiveBySuffix(t *testing.T) {
	dest := t.TempDir()
	zipPath := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(zipPath, zipBytes(t, map[string]string{"d/f": "zip"}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractArchive(zipPath, dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "d", "f")); string(b) != "zip" {
		t.Fatalf("zip content %q", b)
	}
	tgz := makeTarGz(t, []tarEntry{reg("t", "tar")})
	if err := extractArchive(tgz, dest); err != nil {
		t.Fatal(err)
	}
	if err := extractArchive(filepath.Join(t.TempDir(), "x.rar"), dest); err == nil {
		t.Fatal("unknown archive type accepted")
	}
}

func TestHoistDir(t *testing.T) {
	dest := t.TempDir()
	for _, p := range []string{"go/bin/go", "go/go/inner"} { // sub may contain its own name
		if err := os.MkdirAll(filepath.Join(dest, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, p), []byte(p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := hoistDir(dest, "go"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "bin", "go")); string(b) != "go/bin/go" {
		t.Fatalf("bin/go = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "go", "inner")); string(b) != "go/go/inner" {
		t.Fatalf("go/inner = %q", b)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 2 {
		t.Fatalf("leftovers: %v", entries)
	}

	if err := hoistDir(t.TempDir(), "missing"); err == nil || !strings.Contains(err.Error(), "archive has no missing directory") {
		t.Fatalf("missing sub: %v", err)
	}

	clash := t.TempDir()
	for _, p := range []string{"jdk/Contents/Home/bin/java", "jdk/bin/java"} {
		if err := os.MkdirAll(filepath.Join(clash, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(clash, p), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(clash, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := hoistDir(clash, "jdk"); err == nil || !strings.Contains(err.Error(), "bin already exists") {
		t.Fatalf("collision: %v", err)
	}
}

func TestHoistNestedSubDir(t *testing.T) {
	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, "Contents", "Home", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "Contents", "Info.plist"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := hoistDir(dest, "Contents/Home"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 1 || entries[0].Name() != "bin" {
		t.Fatalf("entries = %v", entries)
	}
}

func TestCheckRelativeLink(t *testing.T) {
	for _, ok := range []string{"bun", "../lib/x.js", "../../a/b", "./a"} {
		if err := checkRelativeLink(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/etc", `\evil`, "a/../..", "s/..", "a/../b"} {
		if err := checkRelativeLink(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestVerifySymlinksWithinRejectsDanglingDotDotAfterName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dest, _ := newDest(t)
	// Dangling (y does not exist), lexically inside, but ".." follows a name.
	if err := os.Symlink("y/../z", filepath.Join(dest, "x")); err != nil {
		t.Fatal(err)
	}
	if err := verifySymlinksWithin(dest); err == nil {
		t.Fatal("dangling link with a non-leading .. accepted")
	}
}

func TestFetchJSON(t *testing.T) {
	url, _ := newServer(t, map[string]route{"/ok": {body: []byte(`{"tag":"20261003"}`)}, "/bad": {body: []byte(`{`)}})
	var v struct {
		Tag string `json:"tag"`
	}
	if err := fetchJSON(context.Background(), url+"/ok", &v); err != nil || v.Tag != "20261003" {
		t.Fatalf("got %+v, %v", v, err)
	}
	if err := fetchJSON(context.Background(), url+"/bad", &v); err == nil || !strings.Contains(err.Error(), "parse "+url+"/bad") {
		t.Fatalf("bad JSON: %v", err)
	}
}
