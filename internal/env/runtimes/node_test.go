package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

const nodeIndex = `[
 {"version":"v25.1.0","lts":false},
 {"version":"v24.11.0","lts":"Krypton"},
 {"version":"v22.21.1","lts":"Jod"},
 {"version":"v20.11.0","lts":"Iron"},
 {"version":"v20.9.0","lts":"Iron"}
]`

func TestNodeListRemoteAndLTS(t *testing.T) {
	url, _ := newServer(t, map[string]route{"/index.json": {body: []byte(nodeIndex)}})
	setVar(t, &nodeDistURL, url)
	n := &NodeInstaller{}
	got, err := n.ListRemote(context.Background())
	if err != nil || strings.Join(got, " ") != "25.1.0 24.11.0 22.21.1 20.11.0 20.9.0" {
		t.Fatalf("ListRemote = %v, %v", got, err)
	}
	if v, err := n.LatestLTS(context.Background()); err != nil || v != "24.11.0" {
		t.Fatalf("LatestLTS = %q, %v", v, err)
	}
	ctx := context.Background()
	for spec, want := range map[string]string{"20": "20.11.0", "lts": "24.11.0", "latest": "25.1.0"} {
		if v, err := env.ResolveSpec(ctx, n, spec); err != nil || v != want {
			t.Errorf("ResolveSpec(%s) = %q, %v; want %s", spec, v, err, want)
		}
	}
}

func TestNodeAsset(t *testing.T) {
	cases := []struct{ goos, goarch, file, dir string }{
		{"darwin", "arm64", "node-v20.11.0-darwin-arm64.tar.gz", "node-v20.11.0-darwin-arm64"},
		{"linux", "amd64", "node-v20.11.0-linux-x64.tar.gz", "node-v20.11.0-linux-x64"},
		{"windows", "amd64", "node-v20.11.0-win-x64.zip", "node-v20.11.0-win-x64"},
		{"windows", "arm64", "node-v20.11.0-win-arm64.zip", "node-v20.11.0-win-arm64"},
	}
	for _, c := range cases {
		file, dir, err := nodeAsset(c.goos, c.goarch, "20.11.0")
		if err != nil || file != c.file || dir != c.dir {
			t.Errorf("%s/%s: %s %s %v", c.goos, c.goarch, file, dir, err)
		}
	}
	if _, _, err := nodeAsset("plan9", "386", "20.11.0"); err == nil {
		t.Error("unsupported platform accepted")
	}
}

func TestNodeInstallKeepsNpmSymlinks(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	top := "node-v20.11.0-linux-x64"
	archive := tarGzBytes(t, []tarEntry{
		dir(top + "/"), dir(top + "/bin/"),
		reg(top+"/bin/node", "node"),
		reg(top+"/lib/node_modules/npm/bin/npm-cli.js", "npm"),
		reg(top+"/lib/node_modules/npm/bin/npx-cli.js", "npx"),
		link(top+"/bin/npm", "../lib/node_modules/npm/bin/npm-cli.js"),
		link(top+"/bin/npx", "../lib/node_modules/npm/bin/npx-cli.js"),
	})
	sums := shaBytes(archive) + "  " + top + ".tar.gz\n"
	url, _ := newServer(t, map[string]route{
		"/v20.11.0/SHASUMS256.txt":     {body: []byte(sums)},
		"/v20.11.0/" + top + ".tar.gz": {body: archive},
	})
	setVar(t, &nodeDistURL, url)
	dest := installInto(t, &NodeInstaller{}, "20.11.0")
	if got, _ := readFile(dest, "bin/npm"); got != "npm" {
		t.Fatalf("bin/npm resolves to %q", got)
	}
	if target, _ := os.Readlink(filepath.Join(dest, "bin", "npx")); target != "../lib/node_modules/npm/bin/npx-cli.js" {
		t.Fatalf("bin/npx -> %q", target)
	}
}

func TestNodeInstallNeedsPublishedChecksum(t *testing.T) {
	setHost(t, "linux", "amd64")
	url, _ := newServer(t, map[string]route{"/v20.11.0/SHASUMS256.txt": {body: []byte("abc  other.tar.gz\n")}})
	setVar(t, &nodeDistURL, url)
	err := (&NodeInstaller{}).Install(context.Background(), env.InstallRequest{Version: "20.11.0", Dest: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "no published checksum") {
		t.Fatalf("err = %v", err)
	}
}
