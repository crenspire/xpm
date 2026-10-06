package runtimes

import (
	"context"
	"reflect"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

const goFeed = `[
 {"version":"go1.27rc3","stable":false},
 {"version":"go1.26.2","stable":true},
 {"version":"go1.26.2","stable":true},
 {"version":"go1.26.1","stable":true}
]`

func TestStableGoVersionsSkipsPrereleases(t *testing.T) {
	got, err := stableGoVersions([]byte(goFeed))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.26.2", "1.26.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestGoLatestSkipsReleaseCandidate(t *testing.T) {
	url, _ := newServer(t, map[string]route{"/?mode=json&include=all": {body: []byte(goFeed)}})
	setVar(t, &goDLURL, url)
	v, err := env.ResolveSpec(context.Background(), &GoInstaller{}, "latest")
	if err != nil || v != "1.26.2" {
		t.Fatalf("latest = %q, %v; want 1.26.2", v, err)
	}
	if v, err := env.ResolveSpec(context.Background(), &GoInstaller{}, "1.26"); err != nil || v != "1.26.2" {
		t.Fatalf("1.26 = %q, %v", v, err)
	}
}

func TestGoAsset(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  "go1.26.2.darwin-arm64.tar.gz",
		{"linux", "amd64"}:   "go1.26.2.linux-amd64.tar.gz",
		{"linux", "arm"}:     "go1.26.2.linux-armv6l.tar.gz",
		{"windows", "amd64"}: "go1.26.2.windows-amd64.zip",
		{"windows", "arm64"}: "go1.26.2.windows-arm64.zip",
	}
	for p, want := range cases {
		if got := goAsset(p[0], p[1], "1.26.2"); got != want {
			t.Errorf("%v: %s, want %s", p, got, want)
		}
	}
}

func TestGoInstallVerifiesAndHoists(t *testing.T) {
	skipWindows(t)
	setHost(t, "linux", "amd64")
	archive := tarGzBytes(t, []tarEntry{
		dir("go/"), dir("go/bin/"), reg("go/bin/go", "go"), reg("go/bin/gofmt", "gofmt"), reg("go/VERSION", "go1.26.2"),
	})
	feed := `[{"version":"go1.26.2","stable":true,"files":[{"filename":"go1.26.2.linux-amd64.tar.gz","sha256":"` + shaBytes(archive) + `"}]}]`
	url, _ := newServer(t, map[string]route{
		"/?mode=json":                  {body: []byte(feed)},
		"/go1.26.2.linux-amd64.tar.gz": {body: archive},
	})
	setVar(t, &goDLURL, url)
	dest := installInto(t, &GoInstaller{}, "1.26.2")
	if _, err := readFile(dest, "VERSION"); err != nil {
		t.Fatalf("go/ was not hoisted: %v", err)
	}
}

func TestGoInstallRejectsTamperedArchive(t *testing.T) {
	setHost(t, "linux", "amd64")
	feed := `[{"version":"go1.26.2","stable":true,"files":[{"filename":"go1.26.2.linux-amd64.tar.gz","sha256":"` + sha("original") + `"}]}]`
	url, _ := newServer(t, map[string]route{
		"/?mode=json":                  {body: []byte(feed)},
		"/go1.26.2.linux-amd64.tar.gz": {body: []byte("tampered")},
	})
	setVar(t, &goDLURL, url)
	err := (&GoInstaller{}).Install(context.Background(), env.InstallRequest{Version: "1.26.2", Dest: t.TempDir()})
	if err == nil {
		t.Fatal("tampered archive accepted")
	}
}
