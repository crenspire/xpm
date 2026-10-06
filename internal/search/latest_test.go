package search

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	var cur, max int32
	var mu sync.Mutex
	latestFake(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&cur, 1)
		mu.Lock()
		if n > max {
			max = n
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
	if max > latestConcurrency || max < 2 {
		t.Errorf("max in flight = %d, want 2..%d", max, latestConcurrency)
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
