package search

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
)

// writeGoEnv writes a go env file in a temp dir and points GOENV at it, so
// the real user file is never read.
func writeGoEnv(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", p)
	return p
}

func clearGoProcessEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GOPROXY", "")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "")
}

func TestGoEnvPath(t *testing.T) {
	t.Setenv("GOENV", "/some/where/env")
	if got := goEnvPath(); got != "/some/where/env" {
		t.Errorf("GOENV set: %q", got)
	}
	t.Setenv("GOENV", "off")
	if got := goEnvPath(); got != "" {
		t.Errorf("GOENV=off: %q, want no file", got)
	}
	t.Setenv("GOENV", "")
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir: %v", err)
	}
	if got, want := goEnvPath(), filepath.Join(dir, "go", "env"); got != want {
		t.Errorf("default: %q, want %q", got, want)
	}
}

func TestGoModuleIsPrivateReadsGoEnvFile(t *testing.T) {
	clearGoProcessEnv(t)
	writeGoEnv(t, "# written by go env -w\n\nGOPRIVATE=corp.example,*.internal\r\nGONOPROXY=\"proxyless.example\"\nGOTOOLCHAIN=local\n")
	for mod, want := range map[string]bool{
		"corp.example/team/mod": true,
		"git.internal/x":        true,
		"proxyless.example/y":   true,
		"golang.org/x/text":     false,
	} {
		if got := GoModuleIsPrivate(mod); got != want {
			t.Errorf("GoModuleIsPrivate(%q) = %v, want %v", mod, got, want)
		}
	}
	// A non-empty process variable wins over the file.
	t.Setenv("GOPRIVATE", "other.example")
	if GoModuleIsPrivate("corp.example/team/mod") {
		t.Error("process GOPRIVATE did not override the go env file")
	}
	if !GoModuleIsPrivate("other.example/z") {
		t.Error("process GOPRIVATE ignored")
	}
}

func TestGoModuleIsPrivateMissingOrOffFile(t *testing.T) {
	clearGoProcessEnv(t)
	t.Setenv("GOENV", filepath.Join(t.TempDir(), "missing"))
	if GoModuleIsPrivate("corp.example/a") {
		t.Error("missing file: module reported private")
	}
	p := writeGoEnv(t, "GOPRIVATE=corp.example\n")
	if !GoModuleIsPrivate("corp.example/a") {
		t.Fatalf("file %s not read", p)
	}
	t.Setenv("GOENV", "off")
	if GoModuleIsPrivate("corp.example/a") {
		t.Error("GOENV=off: file still read")
	}
}

func TestLatestVersionsGoEnvFileProxySettings(t *testing.T) {
	var calls int32
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `{"Version":"v1.0.0"}`)
	})
	q := []LatestQuery{{pm.GoMod, "corp.example/team/mod"}}
	for _, content := range []string{
		"GOPRIVATE=corp.example\n",
		"GONOPROXY=corp.example/team\n",
		"GOPROXY=off\n",
		"GOPROXY=file:///srv/goproxy,direct\n",
	} {
		writeGoEnv(t, content)
		if r := LatestVersions(q, Options{})[0]; !errors.Is(r.Err, ErrNotChecked) || r.Found {
			t.Errorf("%q: %+v", content, r)
		}
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("%d requests made, want 0", n)
	}
}

func TestLatestVersionsNonHTTPProxyNotChecked(t *testing.T) {
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	})
	q := []LatestQuery{{pm.GoMod, "example.com/mod"}}
	for _, proxy := range []string{"file:///srv/goproxy", "ftp://proxy.example", "proxy.example"} {
		t.Setenv("GOPROXY", proxy)
		r := LatestVersions(q, Options{})[0]
		if !errors.Is(r.Err, ErrNotChecked) {
			t.Errorf("GOPROXY=%s: %+v", proxy, r)
		}
	}
}

func TestLatestVersionsPrivateSkipsCache(t *testing.T) {
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Version":"v1.0.0"}`)
	})
	defer SetCacheDir(t.TempDir())()
	q := []LatestQuery{{pm.GoMod, "corp.example/mod"}}
	if r := LatestVersions(q, Options{})[0]; !r.Found {
		t.Fatalf("first lookup = %+v", r)
	}
	// Now the module is private: the cached answer must not be shown.
	t.Setenv("GOPRIVATE", "corp.example")
	if r := LatestVersions(q, Options{})[0]; !errors.Is(r.Err, ErrNotChecked) || r.Found {
		t.Errorf("private after caching = %+v", r)
	}
}
