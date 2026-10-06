// Package osv queries the OSV.dev vulnerability database.
package osv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// DefaultBaseURL is the OSV.dev API.
const DefaultBaseURL = "https://api.osv.dev"

const (
	userAgent       = "xpm (+https://github.com/crenspire/xpm)"
	maxBatchQueries = 1000
	maxPages        = 10
	maxInFlight     = 8
	maxBatchBody    = 16 << 20
	maxDetailsBody  = 4 << 20
	maxErrBody      = 512
)

// Package is one package version to check.
type Package struct {
	Ecosystem string // OSV ecosystem name: npm, PyPI, Packagist, crates.io, Go, Maven
	Name      string
	Version   string
}

// Vuln is one vulnerability, with details when they could be fetched.
type Vuln struct {
	ID       string   `json:"id"`
	Aliases  []string `json:"aliases,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Severity string   `json:"severity,omitempty"` // database_specific.severity (e.g. HIGH), else ""
	Fixed    []string `json:"fixed,omitempty"`    // "fixed" events for this package's ecosystem+name, sorted, de-duplicated
}

// Client talks to OSV.dev.
type Client struct {
	BaseURL string       // "" means DefaultBaseURL
	HTTP    *http.Client // nil means a client without its own timeout (ctx bounds every call)
}

type pkgJSON struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
}

type query struct {
	Package   pkgJSON `json:"package"`
	Version   string  `json:"version"`
	PageToken string  `json:"page_token,omitempty"`
}

type batchResponse struct {
	Results []struct {
		Vulns []struct {
			ID string `json:"id"`
		} `json:"vulns"`
		NextPageToken string `json:"next_page_token"`
	} `json:"results"`
}

type details struct {
	ID               string   `json:"id"`
	Summary          string   `json:"summary"`
	Aliases          []string `json:"aliases"`
	DatabaseSpecific struct {
		Severity any `json:"severity"`
	} `json:"database_specific"`
	Affected []struct {
		Package pkgJSON `json:"package"`
		Ranges  []struct {
			Events []struct {
				Fixed string `json:"fixed"`
			} `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

func (c *Client) base() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(c.BaseURL, "/")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{}
}

// do performs a request and returns the body (capped at limit) on 200.
func (c *Client) do(ctx context.Context, method, u string, body []byte, limit int64) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
		return nil, fmt.Errorf("osv: %s %s: status %d: %s", method, u, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("osv: %s %s: response exceeds %d bytes", method, u, limit)
	}
	return b, nil
}

func (c *Client) postBatch(ctx context.Context, qs []query) (*batchResponse, error) {
	body, err := json.Marshal(map[string]any{"queries": qs})
	if err != nil {
		return nil, err
	}
	b, err := c.do(ctx, http.MethodPost, c.base()+"/v1/querybatch", body, maxBatchBody)
	if err != nil {
		return nil, err
	}
	var out batchResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("osv: decode querybatch response: %w", err)
	}
	if len(out.Results) != len(qs) {
		return nil, fmt.Errorf("osv: querybatch returned %d results for %d queries", len(out.Results), len(qs))
	}
	return &out, nil
}

func toQuery(p Package) query {
	v := p.Version
	if p.Ecosystem == "Go" {
		v = strings.TrimPrefix(v, "v")
	}
	return query{Package: pkgJSON{Ecosystem: p.Ecosystem, Name: p.Name}, Version: v}
}

// QueryBatch returns, for each package (same order), the vulnerabilities
// affecting it. It sends POST /v1/querybatch in chunks of at most 1000
// queries, follows per-query next_page_token pages, then fetches each
// distinct vulnerability's details with GET /v1/vulns/{id} (at most 8 in
// flight). A failed details fetch keeps the vulnerability with its ID only
// (detailsErr counts them); a failed batch query returns an error.
func (c *Client) QueryBatch(ctx context.Context, pkgs []Package) (results [][]Vuln, detailsErrs int, err error) {
	if len(pkgs) == 0 {
		return [][]Vuln{}, 0, nil
	}
	ids := make([][]string, len(pkgs))
	for start := 0; start < len(pkgs); start += maxBatchQueries {
		end := min(start+maxBatchQueries, len(pkgs))
		qs := make([]query, 0, end-start)
		for _, p := range pkgs[start:end] {
			qs = append(qs, toQuery(p))
		}
		resp, err := c.postBatch(ctx, qs)
		if err != nil {
			return nil, 0, err
		}
		for i, r := range resp.Results {
			idx := start + i
			for _, v := range r.Vulns {
				ids[idx] = append(ids[idx], v.ID)
			}
			token := r.NextPageToken
			for page := 1; token != "" && page < maxPages; page++ {
				q := qs[i]
				q.PageToken = token
				pr, err := c.postBatch(ctx, []query{q})
				if err != nil {
					return nil, 0, err
				}
				for _, v := range pr.Results[0].Vulns {
					ids[idx] = append(ids[idx], v.ID)
				}
				token = pr.Results[0].NextPageToken
			}
		}
	}

	det, detailsErrs := c.fetchDetails(ctx, ids)
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}

	results = make([][]Vuln, len(pkgs))
	for i, list := range ids {
		seen := map[string]bool{}
		out := make([]Vuln, 0, len(list))
		for _, id := range list {
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, buildVuln(id, det[id], pkgs[i]))
		}
		sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
		results[i] = out
	}
	return results, detailsErrs, nil
}

// fetchDetails fetches each distinct ID once; failed IDs are absent from the map.
func (c *Client) fetchDetails(ctx context.Context, ids [][]string) (map[string]*details, int) {
	distinct := map[string]bool{}
	var order []string
	for _, l := range ids {
		for _, id := range l {
			if !distinct[id] {
				distinct[id] = true
				order = append(order, id)
			}
		}
	}
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		sem    = make(chan struct{}, maxInFlight)
		det    = make(map[string]*details, len(order))
		failed int
	)
	for _, id := range order {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			d, err := c.getDetails(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				return
			}
			det[id] = d
		}(id)
	}
	wg.Wait()
	return det, failed
}

func (c *Client) getDetails(ctx context.Context, id string) (*details, error) {
	b, err := c.do(ctx, http.MethodGet, c.base()+"/v1/vulns/"+url.PathEscape(id), nil, maxDetailsBody)
	if err != nil {
		return nil, err
	}
	var d details
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("osv: decode details for %s: %w", id, err)
	}
	return &d, nil
}

func buildVuln(id string, d *details, p Package) Vuln {
	v := Vuln{ID: id}
	if d == nil {
		return v
	}
	v.Summary = d.Summary
	v.Aliases = d.Aliases
	if s, ok := d.DatabaseSpecific.Severity.(string); ok {
		v.Severity = s
	}
	fixed := map[string]bool{}
	for _, a := range d.Affected {
		if a.Package.Ecosystem != p.Ecosystem || a.Package.Name != p.Name {
			continue
		}
		for _, r := range a.Ranges {
			for _, e := range r.Events {
				if e.Fixed != "" {
					fixed[e.Fixed] = true
				}
			}
		}
	}
	for f := range fixed {
		v.Fixed = append(v.Fixed, f)
	}
	sort.Strings(v.Fixed)
	return v
}
