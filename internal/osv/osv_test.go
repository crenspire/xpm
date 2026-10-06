package osv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type batchReq struct {
	Queries []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		Version   string `json:"version"`
		PageToken string `json:"page_token"`
	} `json:"queries"`
}

func decodeBatch(t *testing.T, r *http.Request) batchReq {
	t.Helper()
	var req batchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("decode request: %v", err)
	}
	return req
}

func TestQueryBatchSingleWithDetails(t *testing.T) {
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/querybatch":
			gotHeaders = r.Header.Clone()
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			req := decodeBatch(t, r)
			if len(req.Queries) != 2 {
				t.Errorf("queries = %d", len(req.Queries))
			}
			_, _ = fmt.Fprint(w, `{"results":[{"vulns":[{"id":"GHSA-b","modified":"x"},{"id":"GHSA-a","modified":"x"}]},{}]}`)
		case "/v1/vulns/GHSA-a":
			_, _ = fmt.Fprint(w, `{"id":"GHSA-a","summary":"bad","aliases":["CVE-1"],"database_specific":{"severity":"HIGH"},
"affected":[{"package":{"ecosystem":"npm","name":"lodash"},"ranges":[{"events":[{"introduced":"0"},{"fixed":"4.17.21"}]}]},
{"package":{"ecosystem":"npm","name":"lodash"},"ranges":[{"events":[{"fixed":"4.17.21"},{"fixed":"3.0.1"}]}]},
{"package":{"ecosystem":"npm","name":"other"},"ranges":[{"events":[{"fixed":"9.9.9"}]}]}]}`)
		case "/v1/vulns/GHSA-b":
			_, _ = fmt.Fprint(w, `{"id":"GHSA-b","database_specific":{"severity":5}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	res, derrs, err := c.QueryBatch(context.Background(), []Package{
		{"npm", "lodash", "4.17.0"}, {"npm", "left-pad", "1.0.0"},
	})
	if err != nil || derrs != 0 {
		t.Fatalf("err=%v derrs=%d", err, derrs)
	}
	if len(res) != 2 || len(res[0]) != 2 || len(res[1]) != 0 {
		t.Fatalf("res = %+v", res)
	}
	want := Vuln{ID: "GHSA-a", Aliases: []string{"CVE-1"}, Summary: "bad", Severity: "HIGH", Fixed: []string{"3.0.1", "4.17.21"}}
	if !reflect.DeepEqual(res[0][0], want) {
		t.Errorf("got %+v want %+v", res[0][0], want)
	}
	if res[0][1].ID != "GHSA-b" || res[0][1].Severity != "" {
		t.Errorf("b = %+v", res[0][1])
	}
	if gotHeaders.Get("Content-Type") != "application/json" || gotHeaders.Get("Accept") != "application/json" ||
		gotHeaders.Get("User-Agent") != "xpm (+https://github.com/crenspire/xpm)" {
		t.Errorf("headers = %v", gotHeaders)
	}
}

func TestGoVersionWithoutV(t *testing.T) {
	var got batchReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/querybatch" {
			got = decodeBatch(t, r)
			_, _ = fmt.Fprint(w, `{"results":[{"vulns":[{"id":"GO-1"}]}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"GO-1","affected":[{"package":{"ecosystem":"Go","name":"example.com/m"},"ranges":[{"events":[{"fixed":"v1.2.3"},{"fixed":"1.2.3"}]}]}]}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	res, _, err := c.QueryBatch(context.Background(), []Package{{"Go", "example.com/m", "v1.0.0"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Queries[0].Version != "1.0.0" {
		t.Errorf("version = %q", got.Queries[0].Version)
	}
	if len(res[0]) != 1 || len(res[0][0].Fixed) == 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestChunking(t *testing.T) {
	var mu sync.Mutex
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/querybatch" {
			_, _ = fmt.Fprint(w, `{"id":"V"}`)
			return
		}
		req := decodeBatch(t, r)
		mu.Lock()
		sizes = append(sizes, len(req.Queries))
		mu.Unlock()
		var parts []string
		for _, q := range req.Queries {
			parts = append(parts, fmt.Sprintf(`{"vulns":[{"id":"ID-%s"}]}`, q.Package.Name))
		}
		_, _ = fmt.Fprintf(w, `{"results":[%s]}`, strings.Join(parts, ","))
	}))
	defer srv.Close()
	pkgs := make([]Package, 2500)
	for i := range pkgs {
		pkgs[i] = Package{"npm", fmt.Sprintf("p%d", i), "1.0.0"}
	}
	res, _, err := (&Client{BaseURL: srv.URL}).QueryBatch(context.Background(), pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sizes, []int{1000, 1000, 500}) {
		t.Errorf("sizes = %v", sizes)
	}
	for i := range pkgs {
		if len(res[i]) != 1 || res[i][0].ID != "ID-"+pkgs[i].Name {
			t.Fatalf("res[%d] = %+v", i, res[i])
		}
	}
}

func TestPagination(t *testing.T) {
	var mu sync.Mutex
	var tokens []string
	var queryCounts []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/querybatch" {
			_, _ = fmt.Fprint(w, `{"id":"x"}`)
			return
		}
		req := decodeBatch(t, r)
		mu.Lock()
		tokens = append(tokens, req.Queries[0].PageToken)
		queryCounts = append(queryCounts, len(req.Queries))
		mu.Unlock()
		if req.Queries[0].PageToken == "" {
			_, _ = fmt.Fprint(w, `{"results":[{"vulns":[{"id":"A"}],"next_page_token":"tok"}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"results":[{"vulns":[{"id":"B"}]}]}`)
	}))
	defer srv.Close()
	res, _, err := (&Client{BaseURL: srv.URL}).QueryBatch(context.Background(), []Package{{"npm", "a", "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tokens, []string{"", "tok"}) || !reflect.DeepEqual(queryCounts, []int{1, 1}) {
		t.Errorf("tokens=%v counts=%v", tokens, queryCounts)
	}
	if len(res[0]) != 2 || res[0][0].ID != "A" || res[0][1].ID != "B" {
		t.Errorf("res = %+v", res)
	}
}

func TestBatchServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = fmt.Fprint(w, "boom")
	}))
	defer srv.Close()
	_, _, err := (&Client{BaseURL: srv.URL}).QueryBatch(context.Background(), []Package{{"npm", "a", "1"}})
	if err == nil || !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestResultsLengthMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"results":[{}]}`)
	}))
	defer srv.Close()
	_, _, err := (&Client{BaseURL: srv.URL}).QueryBatch(context.Background(), []Package{{"npm", "a", "1"}, {"npm", "b", "1"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDetails404KeepsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/querybatch" {
			_, _ = fmt.Fprint(w, `{"results":[{"vulns":[{"id":"GHSA-x"}]}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	res, derrs, err := (&Client{BaseURL: srv.URL}).QueryBatch(context.Background(), []Package{{"npm", "a", "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if derrs != 1 || len(res[0]) != 1 || res[0][0].ID != "GHSA-x" || res[0][0].Summary != "" {
		t.Errorf("derrs=%d res=%+v", derrs, res)
	}
}

func TestContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"results":[{}]}`)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := (&Client{BaseURL: srv.URL}).QueryBatch(ctx, []Package{{"npm", "a", "1"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEmptyInputNoRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request")
	}))
	defer srv.Close()
	res, derrs, err := (&Client{BaseURL: srv.URL}).QueryBatch(context.Background(), nil)
	if err != nil || derrs != 0 || len(res) != 0 {
		t.Fatalf("res=%v derrs=%d err=%v", res, derrs, err)
	}
}
