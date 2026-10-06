package env

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestActiveVersionNothingConfigured(t *testing.T) {
	m := isolate(t)
	if _, err := m.ActiveVersion(fakeRT); !errors.Is(err, ErrNoVersion) {
		t.Fatalf("err = %v, want ErrNoVersion", err)
	}
}

func TestActiveVersionPrefersNearestXpmEnvWithTheKey(t *testing.T) {
	m := isolate(t)
	for _, v := range []string{"18.19.0", "20.9.0", "20.11.1", "200.1.0"} {
		installFake(t, m, v, "")
	}
	if err := m.SetGlobalVersion(fakeRT, "18.19.0"); err != nil {
		t.Fatal(err)
	}
	root, _ := os.Getwd()
	if err := os.WriteFile(filepath.Join(root, ".xpm-env"), []byte("# pins\n"+fakeRT+"=20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// A nearer .xpm-env without the key must not stop the walk.
	if err := os.WriteFile(filepath.Join(root, "a", ".xpm-env"), []byte("othert=1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, sub)

	a, err := m.ActiveVersion(fakeRT)
	if err != nil {
		t.Fatal(err)
	}
	want := Active{Version: "20.11.1", Source: filepath.Join(root, ".xpm-env")}
	if a != want {
		t.Fatalf("got %+v, want %+v", a, want)
	}

	chdir(t, t.TempDir())
	a, err = m.ActiveVersion(fakeRT)
	if err != nil || a.Version != "18.19.0" || !a.Global || a.Source != m.GetActivePath() {
		t.Fatalf("global: got %+v, %v", a, err)
	}
}

func TestActiveVersionAliasesAndMissing(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "20.11.1", "lts")
	installFake(t, m, "22.1.0", "")
	cases := map[string]string{"lts": "20.11.1", "latest": "22.1.0", "22": "22.1.0", "20.11.1": "20.11.1"}
	for spec, want := range cases {
		if err := os.WriteFile(".xpm-env", []byte(fakeRT+"="+spec+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		a, err := m.ActiveVersion(fakeRT)
		if err != nil || a.Version != want {
			t.Errorf("%s: got %+v, %v; want %s", spec, a, err, want)
		}
	}
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=19\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := m.ActiveVersion(fakeRT)
	if !errors.Is(err, ErrNotInstalled) || a.Version != "19" || !strings.HasSuffix(a.Source, ".xpm-env") {
		t.Fatalf("got %+v, %v; want ErrNotInstalled with the raw value and source", a, err)
	}
}

func TestActiveVersionIgnoresStagingDirs(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "1.0.0", "")
	if err := os.MkdirAll(filepath.Join(m.GetRuntimesPath(), fakeRT, ".tmp-2.0.0-123", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := m.InstalledVersions(fakeRT)
	if err != nil || len(got) != 1 || got[0] != "1.0.0" {
		t.Fatalf("InstalledVersions = %v, %v", got, err)
	}
}

func TestInstalledVersionsIgnoresLegacyAliasDirs(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "20.11.1", "")
	for _, legacy := range []string{"latest", "lts"} { // pre-P5 layouts
		if err := writeFakeBinaries(filepath.Join(m.GetRuntimesPath(), fakeRT, legacy)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=latest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := m.ActiveVersion(fakeRT)
	if err != nil || a.Version != "20.11.1" {
		t.Fatalf("got %+v, %v; a legacy dir named latest must not be used", a, err)
	}
}

func TestSetGlobalVersionIsSortedJSONWithNewline(t *testing.T) {
	m := isolate(t)
	if err := m.SetGlobalVersion("node", "20.11.0"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetGlobalVersion("go", "1.22.3"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.GetActivePath())
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"go\": \"1.22.3\",\n  \"node\": \"20.11.0\"\n}\n"
	if string(data) != want {
		t.Fatalf("active.json = %q, want %q", data, want)
	}
	entries, _ := os.ReadDir(m.GetEnvPath())
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestUpdateEnvContent(t *testing.T) {
	cases := []struct{ in, key, val, want string }{
		{"", "node", "20.11.0", "node=20.11.0\n"},
		{"# my pins\n\ngo=1.22.0\n", "node", "20", "# my pins\n\ngo=1.22.0\nnode=20\n"},
		{"go=1.22.0", "node", "20", "go=1.22.0\nnode=20\n"},
		{"# c\n  node = 18\ngo=1.22.0\nnode=16\n", "node", "20", "# c\n  node =20\ngo=1.22.0\nnode=16\n"},
		{"#node=1\nnode=2\r\n", "node", "3", "#node=1\nnode=3\r\n"},
	}
	for _, c := range cases {
		if got := updateEnvContent(c.in, c.key, c.val); got != c.want {
			t.Errorf("updateEnvContent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseEnvFileFirstOccurrenceWins(t *testing.T) {
	got := parseEnvFile("# x=1\n node = 18 \nnode=20\nbad line\n=3\ngo=\n")
	if len(got) != 1 || got["node"] != "18" {
		t.Fatalf("got %v", got)
	}
}

func TestUseVersionWritesExactInstalledVersion(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "20.9.0", "")
	installFake(t, m, "20.11.1", "")
	if err := os.WriteFile(".xpm-env", []byte("# keep me\ngo=1.22.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	a, err := UseVersion(m, fakeRT, "20", false)
	if err != nil || a.Version != "20.11.1" || a.Global {
		t.Fatalf("UseVersion = %+v, %v", a, err)
	}
	data, _ := os.ReadFile(".xpm-env")
	if string(data) != "# keep me\ngo=1.22.0\n"+fakeRT+"=20.11.1\n" {
		t.Fatalf(".xpm-env = %q", data)
	}
	if fi, _ := os.Stat(".xpm-env"); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600 kept", fi.Mode().Perm())
	}

	a, err = UseVersion(m, fakeRT, "20.9.0", true)
	if err != nil || !a.Global || a.Source != m.GetActivePath() {
		t.Fatalf("global UseVersion = %+v, %v", a, err)
	}
	if g, _ := m.GlobalVersion(fakeRT); g != "20.9.0" {
		t.Fatalf("active.json has %q", g)
	}

	if _, err := UseVersion(m, fakeRT, "21", false); err == nil || !strings.Contains(err.Error(), "xpm env install "+fakeRT+"@21") {
		t.Fatalf("not-installed error = %v", err)
	}
}

func TestRemoveVersionRefusesActiveAndGlobal(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "1.0.0", "")
	installFake(t, m, "2.0.0", "")
	installFake(t, m, "3.0.0", "")
	if err := m.SetGlobalVersion(fakeRT, "1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".xpm-env", []byte(fakeRT+"=2.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := RemoveVersion(ctx, m, fakeRT, "2.0.0"); err == nil || !strings.Contains(err.Error(), "active here") {
		t.Fatalf("removing the local pin: %v", err)
	}
	if err := RemoveVersion(ctx, m, fakeRT, "1.0.0"); err == nil || !strings.Contains(err.Error(), "global default") {
		t.Fatalf("removing the global default: %v", err)
	}
	if err := RemoveVersion(ctx, m, fakeRT, "3.0.0"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(m.GetRuntimesPath(), fakeRT))
	if len(entries) != 2 {
		t.Fatalf("left %d entries, want 2 (no .tmp-old-*)", len(entries))
	}
}

func TestListInstalledSemverOrder(t *testing.T) {
	m := isolate(t)
	for _, v := range []string{"1.9.0", "1.10.0", "1.2.0"} {
		installFake(t, m, v, "")
	}
	got, err := ListInstalled(m)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, v := range got[fakeRT] {
		names = append(names, v.Version)
	}
	if strings.Join(names, " ") != "1.10.0 1.9.0 1.2.0" {
		t.Fatalf("order = %v", names)
	}
}

func TestWriteThroughSymlinkedStateFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	m := isolate(t)
	tdir := t.TempDir()
	realEnv := filepath.Join(tdir, "env-real")
	if err := os.WriteFile(realEnv, []byte("# mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realEnv, ".xpm-env"); err != nil {
		t.Fatal(err)
	}
	if _, err := SetLocalVersion(".", fakeRT, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(".xpm-env"); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".xpm-env no longer a symlink: %v %v", fi, err)
	}
	data, _ := os.ReadFile(realEnv)
	if string(data) != "# mine\n"+fakeRT+"=1.0.0\n" {
		t.Fatalf("target = %q", data)
	}
	if fi, _ := os.Stat(realEnv); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}

	realActive := filepath.Join(tdir, "active-real.json")
	if err := os.WriteFile(realActive, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realActive, m.GetActivePath()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetGlobalVersion(fakeRT, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(m.GetActivePath()); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("active.json no longer a symlink: %v %v", fi, err)
	}
	if data, _ := os.ReadFile(realActive); !strings.Contains(string(data), "2.0.0") {
		t.Fatalf("target = %q", data)
	}
}

func TestRemoveVersionRefusesWhenActiveJSONCorrupt(t *testing.T) {
	m := isolate(t)
	installFake(t, m, "1.0.0", "")
	if err := os.WriteFile(m.GetActivePath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RemoveVersion(context.Background(), m, fakeRT, "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("err = %v, want refusal naming the corrupt file", err)
	}
	if _, serr := os.Stat(filepath.Join(m.GetRuntimesPath(), fakeRT, "1.0.0")); serr != nil {
		t.Fatalf("version was removed: %v", serr)
	}
}
