package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeRegistry starts an httptest server, points every registry base URL at
// it (each under its own path prefix) and restores them afterwards.
func fakeRegistry(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	restore := SetEndpoints(Endpoints{
		Npm:         srv.URL + "/npm",
		PyPI:        srv.URL + "/pypi",
		Packagist:   srv.URL + "/packagist",
		CratesAPI:   srv.URL + "/crates-api",
		CratesIndex: srv.URL + "/crates-index",
		MavenSearch: srv.URL + "/maven/select",
	})
	t.Cleanup(restore)
}

var bg = context.Background()

func TestExistsInPipFoundUsesCanonicalName(t *testing.T) {
	var gotPath string
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"info":{"name":"requests","summary":"HTTP for Humans.","version":"2.32.3"}}`)
	})
	r, err := existsInPip(bg, "Requests")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/pypi/Requests/json" {
		t.Errorf("path = %q, want /pypi/Requests/json", gotPath)
	}
	if r == nil || r.Name != "requests" || r.Extra["version"] != "2.32.3" || r.Info != "HTTP for Humans." {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInPipNotFound(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if r, err := existsInPip(bg, "nope"); r != nil || err != nil {
		t.Fatalf("404 must be (nil, nil), got (%+v, %v)", r, err)
	}
}

func TestExistsInPipDecodeErrorNamesPackage(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"info":`) })
	_, err := existsInPip(bg, "requests")
	if err == nil || !strings.Contains(err.Error(), "pypi: decode requests") {
		t.Fatalf("err = %v, want it to name the registry and package", err)
	}
}

func TestStatusErrorWithEmptyBodyHasNoTrailingColon(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	_, err := existsInPip(bg, "requests")
	if err == nil || err.Error() != "pypi registry returned status 503" {
		t.Fatalf("err = %q, want %q", err, "pypi registry returned status 503")
	}
}

func TestExistsInComposerReturnsFirstHit(t *testing.T) {
	var gotQuery string
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		fmt.Fprint(w, `{"results":[{"name":"monolog/monolog","description":"Logging"},{"name":"x/monolog-ext"}]}`)
	})
	r, err := existsInComposer(bg, "monolog")
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "monolog" {
		t.Errorf("q = %q", gotQuery)
	}
	if r == nil || r.Name != "monolog/monolog" || r.Info != "Logging" {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInComposerNoHits(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"results":[]}`) })
	if r, err := existsInComposer(bg, "zzz"); r != nil || err != nil {
		t.Fatalf("no hits must be (nil, nil), got (%+v, %v)", r, err)
	}
}

func TestExistsInComposerDecodeErrorNamesPackage(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `<html>`) })
	_, err := existsInComposer(bg, "monolog")
	if err == nil || !strings.Contains(err.Error(), "packagist: decode monolog") {
		t.Fatalf("err = %v", err)
	}
}

func TestExistsInMavenBuildsCoordinates(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/maven/select" || r.URL.Query().Get("q") != "guava" {
			t.Errorf("unexpected request %s", r.URL)
		}
		fmt.Fprint(w, `{"response":{"docs":[{"id":"com.google.guava:guava","g":"com.google.guava","a":"guava","latestVersion":"33.3.1-jre"}]}}`)
	})
	r, err := existsInMaven(bg, "guava")
	if err != nil {
		t.Fatal(err)
	}
	if r == nil || r.Name != "com.google.guava:guava" || r.Extra["group"] != "com.google.guava" ||
		r.Extra["artifact"] != "guava" || r.Extra["version"] != "33.3.1-jre" {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInMavenNoDocs(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"response":{"docs":[]}}`) })
	if r, err := existsInMaven(bg, "zzz"); r != nil || err != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", r, err)
	}
}

func TestExistsInMavenDecodeErrorNamesPackage(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{`) })
	if _, err := existsInMaven(bg, "guava"); err == nil || !strings.Contains(err.Error(), "maven: decode guava") {
		t.Fatalf("err = %v", err)
	}
}

func TestMultiSearchSendsUserAgent(t *testing.T) {
	var uas []string
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		uas = append(uas, r.UserAgent())
		switch {
		case strings.HasPrefix(r.URL.Path, "/npm/"):
			fmt.Fprint(w, `{"objects":[{"package":{"name":"axios","version":"1.7.9"}}]}`)
		case strings.HasPrefix(r.URL.Path, "/crates-api/"):
			fmt.Fprint(w, `{"crates":[{"name":"serde","max_version":"2.0.0-rc.1","default_version":"1.0.215"}]}`)
		case strings.HasPrefix(r.URL.Path, "/packagist/"):
			fmt.Fprint(w, `{"results":[{"name":"monolog/monolog"}]}`)
		default:
			fmt.Fprint(w, `{"response":{"docs":[]}}`)
		}
	})
	npm, err := SearchNpmPackages(bg, "axios", 5)
	if err != nil || len(npm) != 1 || npm[0].Name != "axios" {
		t.Fatalf("npm: %+v %v", npm, err)
	}
	crates, err := SearchCratesIO(bg, "serde", 5)
	if err != nil || len(crates) != 1 || crates[0].Extra["version"] != "1.0.215" {
		t.Fatalf("crates: %+v %v (want the stable default_version)", crates, err)
	}
	if _, err := SearchPackagistPackages(bg, "monolog", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := SearchMavenCentral(bg, "guava", 5); err != nil {
		t.Fatal(err)
	}
	for _, ua := range uas {
		if !strings.HasPrefix(ua, "xpm") {
			t.Fatalf("User-Agent = %q, want xpm/... (crates.io answers 403 to Go's default UA)", ua)
		}
	}
}
