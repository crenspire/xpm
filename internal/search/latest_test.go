package search

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

func latestFake(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Cleanup(SetEndpoints(Endpoints{
		Npm:         srv.URL + "/npm",
		PyPI:        srv.URL + "/pypi",
		Packagist:   srv.URL + "/packagist",
		CratesAPI:   srv.URL + "/crates-api",
		CratesIndex: srv.URL + "/crates-index",
		MavenSearch: srv.URL + "/maven/select",
		GoProxy:     srv.URL + "/goproxy",
	}))
	t.Cleanup(SetCacheDir(""))
	t.Setenv("GOPROXY", "")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "")
}

func TestLatestVersionsPerRegistry(t *testing.T) {
	var goPath atomic.Value
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/npm/left-pad/latest":
			fmt.Fprint(w, `{"version":"1.3.0"}`)
		case "/pypi/Requests/json":
			fmt.Fprint(w, `{"info":{"name":"requests","version":"2.32.3"}}`)
		case "/packagist/p2/monolog/monolog.json":
			fmt.Fprint(w, `{"packages":{"monolog/monolog":[{"version":"3.9.0-beta1"},{"version":"3.8.1"},{"version":"3.8.0"}]}}`)
		case "/crates-index/se/rd/serde":
			fmt.Fprint(w, `{"name":"serde","vers":"1.0.214","yanked":false}`+"\n")
		case "/maven/select":
			fmt.Fprint(w, `{"response":{"numFound":1,"docs":[{"id":"com.google.guava:guava","g":"com.google.guava","a":"guava","latestVersion":"33.0.0"}]}}`)
		case "/crates-api/crates/serde":
			fmt.Fprint(w, `{"crate":{"name":"serde"}}`)
		default:
			goPath.Store(r.URL.EscapedPath())
			fmt.Fprint(w, `{"Version":"v1.4.2"}`)
		}
	})
	qs := []LatestQuery{
		{pm.GoMod, "github.com/BurntSushi/toml"},
		{pm.Npm, "left-pad"},
		{pm.Pip, "Requests"},
		{pm.Composer, "monolog/monolog"},
		{pm.Cargo, "serde"},
		{pm.Maven, "com.google.guava:guava"},
	}
	want := []string{"v1.4.2", "1.3.0", "2.32.3", "3.8.1", "1.0.214", "33.0.0"}
	got := LatestVersions(qs, Options{})
	if len(got) != len(qs) {
		t.Fatalf("got %d results", len(got))
	}
	for i, r := range got {
		if r.LatestQuery != qs[i] || r.Err != nil || !r.Found || r.Version != want[i] {
			t.Errorf("result %d = %+v, want version %s", i, r, want[i])
		}
	}
	if goPath.Load() != "/goproxy/github.com/!burnt!sushi/toml/@latest" {
		t.Errorf("go proxy path = %v", goPath.Load())
	}
}

func TestLatestVersionsNotFoundAndErrors(t *testing.T) {
	var npmCalls, pipCalls int32
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/npm/gone/latest":
			atomic.AddInt32(&npmCalls, 1)
			http.NotFound(w, r)
		case "/goproxy/example.com/x/@latest":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			atomic.AddInt32(&pipCalls, 1)
			http.NotFound(w, r)
		}
	})
	got := LatestVersions([]LatestQuery{
		{pm.Npm, "gone"},
		{pm.GoMod, "example.com/x"},
		{pm.Pip, "requests"},
		{pm.Yarn, "x"},
	}, Options{Enable: map[pm.ID]bool{pm.Pip: false}})
	if got[0].Found || got[0].Err != nil {
		t.Errorf("404: %+v", got[0])
	}
	if got[1].Err == nil || got[1].Found {
		t.Errorf("500: %+v", got[1])
	}
	if !errors.Is(got[2].Err, ErrRegistryDisabled) || atomic.LoadInt32(&pipCalls) != 0 {
		t.Errorf("disabled: %+v calls=%d", got[2], pipCalls)
	}
	if got[3].Err == nil {
		t.Errorf("unknown manager: %+v", got[3])
	}
}

func TestLatestVersionsMavenRequiresExactCoordinates(t *testing.T) {
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"response":{"numFound":1,"docs":[{"id":"other:artifact","g":"other","a":"artifact","latestVersion":"1.0"}]}}`)
	})
	got := LatestVersions([]LatestQuery{{pm.Maven, "g:artifact"}}, Options{})
	if got[0].Found || got[0].Err != nil {
		t.Fatalf("result = %+v", got[0])
	}
}

func TestLatestVersionsConcurrencyCap(t *testing.T) {
	var cur, peak int32
	var mu sync.Mutex
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&cur, 1)
		mu.Lock()
		if n > peak {
			peak = n
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		fmt.Fprint(w, `{"version":"1.0.0"}`)
	})
	qs := make([]LatestQuery, 30)
	for i := range qs {
		qs[i] = LatestQuery{pm.Npm, fmt.Sprintf("pkg%d", i)}
	}
	for i, r := range LatestVersions(qs, Options{}) {
		if !r.Found || r.Err != nil {
			t.Errorf("result %d = %+v", i, r)
		}
	}
	if peak > latestConcurrency || peak < 2 {
		t.Errorf("max in flight = %d, want 2..%d", peak, latestConcurrency)
	}
}

func TestLatestVersionsTimeout(t *testing.T) {
	latestFake(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	start := time.Now()
	got := LatestVersions([]LatestQuery{{pm.Npm, "slow"}}, Options{Timeout: 50 * time.Millisecond})
	if !errors.Is(got[0].Err, ErrRegistryTimeout) {
		t.Errorf("err = %v", got[0].Err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("took %v", time.Since(start))
	}
}

func TestLatestVersionsComposerLargeBody(t *testing.T) {
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/packagist/p2/aws/aws-sdk-php.json" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"minified":"x","packages":{"other/pkg":[{"version":"9.9.9"}],"aws/aws-sdk-php":[{"version":"3.5.0-RC1"},{"version":"3.4.0"}`)
		pad := strings.Repeat("x", 1000)
		for i := 0; i < 1500; i++ {
			fmt.Fprintf(w, `,{"version":"2.%d.0","description":%q}`, i, pad)
		}
		fmt.Fprint(w, `]}}`)
	})
	got := LatestVersions([]LatestQuery{{pm.Composer, "AWS/aws-sdk-php"}}, Options{})
	if got[0].Err != nil || !got[0].Found || got[0].Version != "3.4.0" {
		t.Fatalf("result = %+v", got[0])
	}
}

func TestLatestVersionsComposerEdgeCases(t *testing.T) {
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packagist/p2/v/prerelease.json":
			fmt.Fprint(w, `{"packages":{"v/prerelease":[{"version":"2.0.0-beta2"},{"version":"2.0.0-a1"},{"version":"dev-main"}]}}`)
		case "/packagist/p2/v/patch.json":
			fmt.Fprint(w, `{"packages":{"v/patch":[{"version":"1.0.0-p1"},{"version":"1.0.0"}]}}`)
		default:
			http.NotFound(w, r)
		}
	})
	got := LatestVersions([]LatestQuery{
		{pm.Composer, "v/missing"},
		{pm.Composer, "v/prerelease"},
		{pm.Composer, "v/patch"},
		{pm.Composer, "noslash"},
	}, Options{})
	if got[0].Found || got[0].Err != nil {
		t.Errorf("404: %+v", got[0])
	}
	if !got[1].Found || got[1].Version != "2.0.0-beta2" {
		t.Errorf("no-stable fallback: %+v", got[1])
	}
	if !got[2].Found || got[2].Version != "1.0.0-p1" {
		t.Errorf("patch is stable: %+v", got[2])
	}
	if got[3].Err == nil || !strings.Contains(got[3].Err.Error(), "invalid package name") {
		t.Errorf("noslash: %+v", got[3])
	}
}

func TestLatestVersionsGoEdgeCases(t *testing.T) {
	var calls int32
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		switch r.URL.Path {
		case "/goproxy/example.com/gone/@latest":
			http.Error(w, "gone", http.StatusGone)
		case "/goproxy/example.com/empty/@latest":
			fmt.Fprint(w, `{"Version":""}`)
		default:
			http.NotFound(w, r)
		}
	})
	got := LatestVersions([]LatestQuery{{pm.GoMod, "example.com/gone"}, {pm.GoMod, "example.com/empty"}}, Options{})
	for i, r := range got {
		if r.Found || r.Err != nil {
			t.Errorf("result %d = %+v", i, r)
		}
	}
}

func TestLatestVersionsGoPrivateAndProxyEnv(t *testing.T) {
	var calls int32
	var lastPath atomic.Value
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		lastPath.Store(r.URL.Path)
		fmt.Fprint(w, `{"Version":"v1.0.0"}`)
	})
	q := []LatestQuery{{pm.GoMod, "corp.example/team/mod"}}

	t.Setenv("GOPRIVATE", "corp.example")
	if r := LatestVersions(q, Options{})[0]; !errors.Is(r.Err, ErrNotChecked) || r.Found {
		t.Errorf("GOPRIVATE: %+v", r)
	}
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "corp.example/team")
	if r := LatestVersions(q, Options{})[0]; !errors.Is(r.Err, ErrNotChecked) {
		t.Errorf("GONOPROXY: %+v", r)
	}
	t.Setenv("GONOPROXY", "")
	t.Setenv("GOPROXY", "off")
	if r := LatestVersions(q, Options{})[0]; !errors.Is(r.Err, ErrNotChecked) {
		t.Errorf("GOPROXY=off: %+v", r)
	}
	t.Setenv("GOPROXY", "direct")
	if r := LatestVersions(q, Options{})[0]; !errors.Is(r.Err, ErrNotChecked) {
		t.Errorf("GOPROXY=direct: %+v", r)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("%d requests made, want 0", n)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath.Store("other" + r.URL.Path)
		fmt.Fprint(w, `{"Version":"v2.0.0"}`)
	}))
	defer other.Close()
	t.Setenv("GOPROXY", other.URL+"/,direct")
	r := LatestVersions(q, Options{})[0]
	if r.Err != nil || r.Version != "v2.0.0" || lastPath.Load() != "other/corp.example/team/mod/@latest" {
		t.Errorf("GOPROXY list: %+v path=%v", r, lastPath.Load())
	}
}

func TestLatestVersionsComposerCacheUsesLatestDir(t *testing.T) {
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"packages":{"v/p":[{"version":"1.2.3"}]}}`)
	})
	dir := t.TempDir()
	defer SetCacheDir(dir)()
	if r := LatestVersions([]LatestQuery{{pm.Composer, "v/p"}}, Options{})[0]; !r.Found {
		t.Fatalf("result = %+v", r)
	}
	if _, err := os.Stat(cachePath(filepath.Join(dir, "latest"), pm.Composer, "v/p")); err != nil {
		t.Errorf("cache entry not under latest/: %v", err)
	}
	if _, err := os.Stat(cachePath(dir, pm.Composer, "v/p")); err == nil {
		t.Error("version-less search cache key was written")
	}
}
