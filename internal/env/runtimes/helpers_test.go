package runtimes

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	progress = io.Discard
	os.Exit(m.Run())
}

// route is one canned response of a fake server; status 0 means 200.
type route struct {
	body   []byte
	status int
}

// newServer serves routes keyed by "path" or "path?query" (query must
// match exactly when given). Unknown paths are 404s. It records requests.
func newServer(t *testing.T, routes map[string]route) (string, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r)
		rt, ok := routes[r.URL.Path+"?"+r.URL.RawQuery]
		if !ok {
			rt, ok = routes[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		if rt.status != 0 {
			w.WriteHeader(rt.status)
		}
		_, _ = w.Write(rt.body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &seen
}

// setVar swaps a package variable for one test.
func setVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// zipBytes builds a zip in memory; names ending in "/" are directories.
func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func reg(name, body string) tarEntry { return tarEntry{name: name, body: body, typ: tar.TypeReg} }
