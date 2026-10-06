package env

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
)

// isolate chdirs into a fresh temp dir (the repo root has its own .xpm-env)
// and returns a Manager rooted in another temp dir.
func isolate(t *testing.T) *Manager {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	cfg := config.Config{Env: config.EnvConfig{Enabled: true, Path: t.TempDir()}}
	m, err := NewManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
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
		if err := os.MkdirAll(filepath.Join(m.GetRuntimesPath(), "node", v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := [][2]string{{"node", ""}, {"node", ".."}, {"node", "."}, {"../x", "1"}, {"", "1"}}
	for _, c := range cases {
		if err := RemoveVersion(m, c[0], c[1]); err == nil {
			t.Errorf("RemoveVersion(%q, %q) = nil, want error", c[0], c[1])
		}
	}
	for _, v := range []string{"18.0.0", "20.0.0"} {
		if _, err := os.Stat(filepath.Join(m.GetRuntimesPath(), "node", v)); err != nil {
			t.Fatalf("node %s was deleted by a rejected call", v)
		}
	}
	if err := RemoveVersion(m, "node", "18.0.0"); err != nil {
		t.Fatalf("legit removal failed: %v", err)
	}
}

func TestLocalEnvIgnoresPathLikeVersion(t *testing.T) {
	m := isolate(t)
	if err := os.WriteFile(".xpm-env", []byte("node=../../../../tmp/evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, _ := m.GetActiveVersion("node"); strings.Contains(v, "..") {
		t.Fatalf("GetActiveVersion returned path-like version %q from .xpm-env", v)
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
