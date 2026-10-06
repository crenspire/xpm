package runtimes

import (
	"context"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

// pbsServer fakes latest-release.json, SHA256SUMS and one archive.
func pbsServer(t *testing.T, archive []byte) {
	t.Helper()
	tag := "20261003"
	sums := strings.Join([]string{
		shaBytes(archive) + "  cpython-3.12.15+20261003-x86_64-unknown-linux-gnu-install_only.tar.gz",
		sha("a") + "  cpython-3.12.15+20261003-x86_64-unknown-linux-gnu-install_only_stripped.tar.gz",
		sha("b") + "  cpython-3.13.9+20261003-x86_64-unknown-linux-gnu-freethreaded-install_only.tar.gz",
		sha("c") + "  cpython-3.11.17+20261003-x86_64-unknown-linux-gnu-install_only.tar.gz",
		sha("d") + "  cpython-3.14.0rc3+20261003-x86_64-unknown-linux-gnu-install_only.tar.gz",
		sha("e") + "  cpython-3.13.9+20261003-aarch64-apple-darwin-install_only.tar.gz",
	}, "\n") + "\n"
	routes := map[string]route{
		"/rel/" + tag + "/SHA256SUMS": {body: []byte(sums)},
		"/rel/" + tag + "/cpython-3.12.15+20261003-x86_64-unknown-linux-gnu-install_only.tar.gz": {body: archive},
	}
	url, _ := newServer(t, routes)
	routes["/latest-release.json"] = route{body: []byte(`{"tag":"` + tag + `","asset_url_prefix":"` + url + `/rel/` + tag + `"}`)}
	setVar(t, &pbsLatestURL, url+"/latest-release.json")
}

func TestPythonVersionsAndResolve(t *testing.T) {
	setHost(t, "linux", "amd64")
	pbsServer(t, []byte("x"))
	p := &PythonInstaller{}
	ctx := context.Background()
	got, err := p.ListRemote(ctx)
	env.SortVersionsDesc(got)
	if err != nil || strings.Join(got, " ") != "3.14.0rc3 3.12.15 3.11.17" {
		t.Fatalf("ListRemote = %v, %v", got, err)
	}
	for spec, want := range map[string]string{"latest": "3.12.15", "3": "3.12.15", "3.11": "3.11.17", "3.12.15": "3.12.15"} {
		if v, err := env.ResolveSpec(ctx, p, spec); err != nil || v != want {
			t.Errorf("%s: %q, %v; want %s", spec, v, err, want)
		}
	}
	_, err = env.ResolveSpec(ctx, p, "3.12.4")
	if err == nil || err.Error() != "python 3.12.4 is not available: python-build-standalone 20261003 provides 3.11.17, 3.12.15, 3.14.0rc3" {
		t.Fatalf("missing exact: %v", err)
	}
}

func TestPbsTriple(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}: "aarch64-apple-darwin",
		{"darwin", "amd64"}: "x86_64-apple-darwin",
		{"linux", "amd64"}:  "x86_64-unknown-linux-gnu",
		{"linux", "arm64"}:  "aarch64-unknown-linux-gnu",
	}
	for p, want := range cases {
		if got, err := pbsTriple(p[0], p[1]); err != nil || got != want {
			t.Errorf("%v: %s %v", p, got, err)
		}
	}
	if _, err := pbsTriple("windows", "amd64"); err == nil {
		t.Error("windows accepted")
	}
}

func TestPythonInstallHoistsPythonDir(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	archive := tarGzBytes(t, []tarEntry{
		dir("python/"), dir("python/bin/"),
		reg("python/bin/python3.12", "py"),
		link("python/bin/python", "python3.12"),
		link("python/bin/python3", "python3.12"),
		reg("python/bin/pip", "pip"),
		reg("python/bin/pip3", "pip"),
		reg("python/lib/python3.12/os.py", "os"),
	})
	pbsServer(t, archive)
	dest := installInto(t, &PythonInstaller{}, "3.12.15")
	if got, _ := readFile(dest, "lib/python3.12/os.py"); got != "os" {
		t.Fatal("python/ was not hoisted")
	}
}
