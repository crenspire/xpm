package env

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
)

// chdir is t.Chdir for Go < 1.24.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// isolate points HOME at a temp dir, chdirs into another (the repo root has
// its own .xpm-env) and returns a Manager rooted in a third. Progress
// output is discarded.
func isolate(t *testing.T) *Manager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	chdir(t, t.TempDir())
	cfg := config.Config{Env: config.EnvConfig{Enabled: true, Path: t.TempDir()}}
	m, err := NewManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.SetOutput(io.Discard)
	return m
}

func TestValidateVersionSpec(t *testing.T) {
	ok := []string{"20.11.0", "lts", "latest", "stable", "1.22rc1", "8.3", "21.0.2+13", "1.75.0-beta.1"}
	bad := []string{"", "..", "../x", "a/b", `a\b`, "1..2", " 1", "-1", ".hidden", strings.Repeat("9", 65)}
	for _, v := range ok {
		if err := ValidateVersionSpec(v); err != nil {
			t.Errorf("ValidateVersionSpec(%q) = %v, want nil", v, err)
		}
	}
	for _, v := range bad {
		if err := ValidateVersionSpec(v); err == nil {
			t.Errorf("ValidateVersionSpec(%q) = nil, want error", v)
		}
	}
}

func TestValidateRuntimeName(t *testing.T) {
	for _, n := range []string{"node", "python", "go", "php"} {
		if err := ValidateRuntimeName(n); err != nil {
			t.Errorf("%q: %v", n, err)
		}
	}
	for _, n := range []string{"", "..", "../x", "Node", "no de", "node/x"} {
		if err := ValidateRuntimeName(n); err == nil {
			t.Errorf("ValidateRuntimeName(%q) = nil, want error", n)
		}
	}
}

func TestRemoveVersionRejectsDangerousInput(t *testing.T) {
	m := isolate(t)
	for _, v := range []string{"18.0.0", "20.0.0"} {
		if err := os.MkdirAll(filepath.Join(m.GetRuntimesPath(), fakeRT, v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := [][2]string{{fakeRT, ""}, {fakeRT, ".."}, {fakeRT, "."}, {"../x", "1"}, {"", "1"}}
	for _, c := range cases {
		if err := RemoveVersion(context.Background(), m, c[0], c[1]); err == nil {
			t.Errorf("RemoveVersion(%q, %q) = nil, want error", c[0], c[1])
		}
	}
	for _, v := range []string{"18.0.0", "20.0.0"} {
		if _, err := os.Stat(filepath.Join(m.GetRuntimesPath(), fakeRT, v)); err != nil {
			t.Fatalf("node %s was deleted by a rejected call", v)
		}
	}
	if err := RemoveVersion(context.Background(), m, fakeRT, "18.0.0"); err != nil {
		t.Fatalf("legit removal failed: %v", err)
	}
}

func TestLocalEnvRejectsPathLikeVersion(t *testing.T) {
	m := isolate(t)
	if err := os.WriteFile(".xpm-env", []byte("node=../../../../tmp/evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := m.ActiveVersion("node")
	if err == nil || !strings.Contains(err.Error(), `invalid node version "../../../../tmp/evil"`) {
		t.Fatalf("ActiveVersion = %+v, %v; want an invalid-version error", a, err)
	}
	if errors.Is(err, ErrNoVersion) || errors.Is(err, ErrNotInstalled) {
		t.Fatal("an invalid entry must fail closed, not fall back")
	}
	if strings.Contains(a.Version, "..") {
		t.Fatalf("returned path-like version %q", a.Version)
	}
}

func TestShimTemplateGuardsVersion(t *testing.T) {
	code, err := generateShimCode("node", "node", "/opt/xpm env")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "shim.go", code, 0); err != nil {
		t.Fatalf("generated shim is not valid Go: %v", err)
	}
	if !strings.Contains(code, "!safeVersion(version)") {
		t.Fatal("shim does not validate the resolved version before using it as a path")
	}
}
