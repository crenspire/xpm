package search

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCratesIndexPath(t *testing.T) {
	for name, want := range map[string]string{
		"a": "1/a", "ab": "2/ab", "abc": "3/a/abc", "Serde": "se/rd/serde", "tokio": "to/ki/tokio",
	} {
		t.Run(name, func(t *testing.T) {
			if got := cratesIndexPath(name); got != want {
				t.Errorf("cratesIndexPath(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

func TestSemverLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.0.9", "1.0.10", true},
		{"0.9.0", "0.8.5", false},
		{"2.0.0-rc.1", "2.0.0", true},
		{"2.0.0", "2.0.0-rc.1", false},
		{"1.0.0+build.1", "1.0.0", false},
		{"1.2.3", "1.2.3", false},
	} {
		if got := semverLess(c.a, c.b); got != c.want {
			t.Errorf("semverLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

const serdeIndex = `{"name":"serde","vers":"1.0.214","deps":[],"cksum":"x","features":{},"yanked":false}
{"name":"serde","vers":"1.0.215","deps":[],"cksum":"x","features":{},"yanked":true}
{"name":"serde","vers":"2.0.0-rc.1","deps":[],"cksum":"x","features":{},"yanked":false}
{"name":"serde","vers":"0.9.16","deps":[],"cksum":"x","features":{},"yanked":false}
`

func TestExistsInCratesUsesSparseIndex(t *testing.T) {
	var indexPath string
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/crates-index/"):
			indexPath = r.URL.Path
			fmt.Fprint(w, serdeIndex)
		case r.URL.Path == "/crates-api/crates/serde":
			fmt.Fprint(w, `{"crate":{"name":"serde","description":"A serialization framework"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	r, err := existsInCrates(bg, "serde")
	if err != nil {
		t.Fatal(err)
	}
	if indexPath != "/crates-index/se/rd/serde" {
		t.Errorf("index path = %q", indexPath)
	}
	// 1.0.215 is yanked, 2.0.0-rc.1 is a prerelease, 0.9.16 was published last but is older.
	if r == nil || r.Name != "serde" || r.Extra["version"] != "1.0.214" || r.Info != "A serialization framework" {
		t.Fatalf("result = %+v", r)
	}
}

func TestExistsInCratesNotFoundSkipsAPI(t *testing.T) {
	var apiCalls int32
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/crates-api/") {
			atomic.AddInt32(&apiCalls, 1)
		}
		http.NotFound(w, r)
	})
	if r, err := existsInCrates(bg, "no-such-crate"); r != nil || err != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", r, err)
	}
	if atomic.LoadInt32(&apiCalls) != 0 {
		t.Fatal("the API must not be called when the index says 404")
	}
}

func TestExistsInCratesAPIStallStillFound(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/crates-api/") {
			select { // stall like crates.io's 46 s tail
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return
		}
		fmt.Fprint(w, serdeIndex)
	})
	// The fan-out gives each registry a deadline; the crate must come back
	// before it, without a description, instead of timing out.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	r, err := existsInCrates(ctx, "serde")
	if err != nil || r == nil || r.Extra["version"] != "1.0.214" {
		t.Fatalf("an API stall must not hide a found crate: (%+v, %v)", r, err)
	}
	if r.Info != "" {
		t.Errorf("Info = %q, want empty when the API stalls", r.Info)
	}
	if ctx.Err() != nil {
		t.Fatal("existsInCrates returned after the caller's deadline")
	}
}

func TestCratesDescriptionBudget(t *testing.T) {
	if got := cratesDescriptionBudget(context.Background()); got != cratesDescriptionTimeout {
		t.Errorf("no deadline: budget = %v, want %v", got, cratesDescriptionTimeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if got := cratesDescriptionBudget(ctx); got > time.Second-descriptionMargin {
		t.Errorf("budget %v ignores the caller's deadline", got)
	}
}

func TestExistsInCratesAllYankedIsNotFound(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"gone","vers":"0.1.0","yanked":true}`+"\n")
	})
	if r, err := existsInCrates(bg, "gone"); r != nil || err != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", r, err)
	}
}

func TestExistsInCratesInvalidNameMakesNoRequest(t *testing.T) {
	var calls int32
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&calls, 1) })
	for _, name := range []string{"@types/node", "monolog/monolog", "com.google.guava:guava", "1abc"} {
		if r, err := existsInCrates(bg, name); r != nil || err != nil {
			t.Errorf("%q: got (%+v, %v), want (nil, nil)", name, r, err)
		}
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("%d requests for names that cannot be crates", calls)
	}
}

func TestExistsInCratesBadIndexLineIsAnError(t *testing.T) {
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "{not json\n") })
	if _, err := existsInCrates(bg, "serde"); err == nil || !strings.Contains(err.Error(), "crates.io index: serde") {
		t.Fatalf("err = %v", err)
	}
}
