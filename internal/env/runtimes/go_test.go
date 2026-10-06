package runtimes

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

const goFeed = `[
 {"version":"go1.27rc3","stable":false},
 {"version":"go1.26.2","stable":true},
 {"version":"go1.26.2","stable":true},
 {"version":"go1.26.1","stable":true}
]`

func TestStableGoVersionsSkipsPrereleases(t *testing.T) {
	got, err := stableGoVersions([]byte(goFeed))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.26.2", "1.26.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestGoLatestVersionSkipsReleaseCandidate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(goFeed))
	}))
	defer srv.Close()
	old := goDLURL
	goDLURL = srv.URL
	t.Cleanup(func() { goDLURL = old })

	v, err := (&GoInstaller{}).GetLatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.26.2" {
		t.Fatalf("latest = %q, want 1.26.2", v)
	}
}
