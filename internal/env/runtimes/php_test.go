package runtimes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/env"
)

func phpServer(t *testing.T) {
	t.Helper()
	url, _ := newServer(t, map[string]route{
		"/releases/index.php?json&version=8": {body: []byte(`{"version":"8.5.11","date":"24 Sep 2026"}`)},
		"/releases/index.php?json&version=7": {body: []byte(`{"version":"7.4.33"}`)},
	})
	setVar(t, &phpReleasesURL, url+"/releases/index.php")
}

func TestPHPResolve(t *testing.T) {
	phpServer(t)
	p := &PHPInstaller{}
	ctx := context.Background()
	for spec, want := range map[string]string{"latest": "8.5", "8": "8.5", "7": "7.4", "8.3": "8.3"} {
		if v, err := env.ResolveSpec(ctx, p, spec); err != nil || v != want {
			t.Errorf("%s: %q, %v; want %s", spec, v, err, want)
		}
	}
	_, err := p.Resolve(ctx, "8.3.12")
	if err == nil || err.Error() != "PHP is installed per minor version through Homebrew; use php@8.3" {
		t.Fatalf("patch: %v", err)
	}
	got, err := p.ListRemote(ctx)
	if err != nil || strings.Join(got, " ") != "8.5 8.4 8.3 8.2 8.1 8.0 7.4" {
		t.Fatalf("ListRemote = %v, %v", got, err)
	}
}

func TestPHPNeedsBrew(t *testing.T) {
	setHost(t, "linux", "amd64")
	if err := (&PHPInstaller{}).Install(context.Background(), installReq(t, "8.3")); !errors.Is(err, errPHPNeedsBrew) {
		t.Fatalf("linux: %v", err)
	}
	setHost(t, "darwin", "arm64")
	setVar(t, &lookPath, func(string) (string, error) { return "", errors.New("not found") })
	err := (&PHPInstaller{}).Install(context.Background(), installReq(t, "8.3"))
	if err == nil || err.Error() != "PHP needs Homebrew on macOS (https://brew.sh); on Linux install PHP with your system package manager" {
		t.Fatalf("no brew: %v", err)
	}
}

func TestPHPInstallLinksHomebrewBinary(t *testing.T) {
	skipWindows(t)
	setHost(t, "darwin", "arm64")
	prefix := t.TempDir()
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "bin", "php"), []byte("php"), 0o755); err != nil {
		t.Fatal(err)
	}
	setVar(t, &lookPath, func(string) (string, error) { return "/opt/homebrew/bin/brew", nil })
	var ran []string
	setVar(t, &runCmd, func(_ context.Context, name string, args, _ []string) error {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return nil
	})
	setVar(t, &cmdOutput, func(_ context.Context, name string, args ...string) ([]byte, error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return []byte(prefix + "\n"), nil
	})
	dest := installInto(t, &PHPInstaller{}, "8.3")
	want := "/opt/homebrew/bin/brew install shivammathur/php/php@8.3|/opt/homebrew/bin/brew --prefix shivammathur/php/php@8.3"
	if strings.Join(ran, "|") != want {
		t.Fatalf("ran %v", ran)
	}
	if target, _ := os.Readlink(filepath.Join(dest, "bin", "php")); target != filepath.Join(prefix, "bin", "php") {
		t.Fatalf("bin/php -> %q", target)
	}
}
