package search

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withNpmServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	orig := npmRegistryURL
	npmRegistryURL = srv.URL
	t.Cleanup(func() { npmRegistryURL = orig })
}

func TestExistsInNpmUsesLatestEndpoint(t *testing.T) {
	var gotPath, gotUA string
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA = r.URL.EscapedPath(), r.UserAgent()
		fmt.Fprint(w, `{"name":"@types/node","version":"22.1.0","description":"TS defs"}`)
	})

	r, err := existsInNpm("@types/node")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/@types%2Fnode/latest" {
		t.Errorf("path = %q, want /@types%%2Fnode/latest", gotPath)
	}
	if !strings.HasPrefix(gotUA, "xpm") {
		t.Errorf("User-Agent = %q, want xpm/...", gotUA)
	}
	if r == nil || r.Extra["version"] != "22.1.0" || r.Info != "TS defs" || r.Name != "@types/node" {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInNpmNotFound(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `"Not Found"`, http.StatusNotFound)
	})
	r, err := existsInNpm("nope")
	if err != nil || r != nil {
		t.Fatalf("404 must be (nil, nil), got (%+v, %v)", r, err)
	}
}

func TestExistsInNpmServerErrorIsTruncated(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, strings.Repeat("x", 10_000))
	})
	_, err := existsInNpm("lodash")
	if err == nil || len(err.Error()) > 700 {
		t.Fatalf("want a short status error, got len=%d err=%v", len(fmt.Sprint(err)), err)
	}
}

func TestExistsInNpmCapsBodySize(t *testing.T) {
	withNpmServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"1.0.0","description":"`+strings.Repeat("a", 2<<20)+`"}`)
	})
	if _, err := existsInNpm("huge"); err == nil {
		t.Fatal("a body over maxMetadataBytes must fail to decode, got nil error")
	}
}
