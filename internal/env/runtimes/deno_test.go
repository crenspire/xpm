package runtimes

import (
	"context"
	"testing"
)

func TestDenoAsset(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  "deno-aarch64-apple-darwin.zip",
		{"darwin", "amd64"}:  "deno-x86_64-apple-darwin.zip",
		{"linux", "amd64"}:   "deno-x86_64-unknown-linux-gnu.zip",
		{"windows", "amd64"}: "deno-x86_64-pc-windows-msvc.zip",
	}
	for p, want := range cases {
		if got, err := denoAsset(p[0], p[1]); err != nil || got != want {
			t.Errorf("%v: %s %v, want %s", p, got, err, want)
		}
	}
}

func TestDenoInstallRootLevelBinary(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	archive := zipBytes(t, map[string]string{"deno": "deno-binary"})
	base := "/denoland/deno/releases/download/v2.9.7/deno-x86_64-unknown-linux-gnu.zip"
	url, _ := newServer(t, map[string]route{
		base + ".sha256sum": {body: []byte(shaBytes(archive) + "  deno-x86_64-unknown-linux-gnu.zip\n")},
		base:                {body: archive},
	})
	setVar(t, &githubDownloadURL, url)
	dest := installInto(t, &DenoInstaller{}, "2.9.7")
	if got, _ := readFile(dest, "bin/deno"); got != "deno-binary" {
		t.Fatalf("bin/deno = %q", got)
	}
}

func TestDenoWithoutChecksumIsRefused(t *testing.T) {
	setHost(t, "linux", "amd64")
	url, seen := newServer(t, map[string]route{})
	setVar(t, &githubDownloadURL, url)
	err := (&DenoInstaller{}).Install(context.Background(), installReq(t, "1.46.3"))
	want := "deno 1.46.3 publishes no SHA-256 checksum for deno-x86_64-unknown-linux-gnu.zip; xpm only installs verified downloads (Deno 2.0.6+ publish them)"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("downloaded without a checksum: %d requests", len(*seen))
	}
}
