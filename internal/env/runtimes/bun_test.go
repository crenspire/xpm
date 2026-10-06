package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionsFromTags(t *testing.T) {
	rels := []githubRelease{
		{TagName: "bun-v1.4.2"}, {TagName: "canary"}, {TagName: "bun-v1.4.3-canary.1"},
		{TagName: "bun-v1.4.1", Draft: true}, {TagName: "v1.0.0"}, {TagName: "bun-v1.3.0"},
	}
	if got := strings.Join(versionsFromTags(rels, "bun-v"), " "); got != "1.4.2 1.3.0" {
		t.Fatalf("bun versions = %q", got)
	}
	if got := strings.Join(versionsFromTags([]githubRelease{{TagName: "v2.9.7"}, {TagName: "std/0.1"}}, "v"), " "); got != "2.9.7" {
		t.Fatalf("deno versions = %q", got)
	}
}

func TestBunAsset(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  "bun-darwin-aarch64.zip",
		{"darwin", "amd64"}:  "bun-darwin-x64.zip",
		{"linux", "arm64"}:   "bun-linux-aarch64.zip",
		{"windows", "amd64"}: "bun-windows-x64.zip",
	}
	for p, want := range cases {
		if zip, _, err := bunAsset(p[0], p[1]); err != nil || zip != want {
			t.Errorf("%v: %s %v, want %s", p, zip, err, want)
		}
	}
}

func TestBunInstallVerifiesAndAddsBunx(t *testing.T) {
	skipWindows(t)
	setHost(t, "darwin", "arm64")
	archive := zipBytes(t, map[string]string{"bun-darwin-aarch64/bun": "bun-binary"})
	sums := sha("other") + "  bun-darwin-aarch64-profile.zip\n" + shaBytes(archive) + "  bun-darwin-aarch64.zip\n"
	base := "/oven-sh/bun/releases/download/bun-v1.4.2/"
	url, _ := newServer(t, map[string]route{
		base + "SHASUMS256.txt":         {body: []byte(sums)},
		base + "bun-darwin-aarch64.zip": {body: archive},
	})
	setVar(t, &githubDownloadURL, url)
	dest := installInto(t, &BunInstaller{}, "1.4.2")
	if target, _ := os.Readlink(filepath.Join(dest, "bin", "bunx")); target != "bun" {
		t.Fatalf("bunx -> %q", target)
	}
	if _, err := os.Stat(filepath.Join(dest, "bun-darwin-aarch64")); err == nil {
		t.Fatal("archive dir left behind")
	}
}

func TestBunInstallRejectsChecksumMismatch(t *testing.T) {
	setHost(t, "linux", "amd64")
	base := "/oven-sh/bun/releases/download/bun-v1.4.2/"
	url, _ := newServer(t, map[string]route{
		base + "SHASUMS256.txt":    {body: []byte(sha("good") + "  bun-linux-x64.zip\n")},
		base + "bun-linux-x64.zip": {body: []byte("evil")},
	})
	setVar(t, &githubDownloadURL, url)
	if err := (&BunInstaller{}).Install(context.Background(), installReq(t, "1.4.2")); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
}
