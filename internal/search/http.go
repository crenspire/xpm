package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxMetadataBytes caps every registry response body xpm decodes.
const maxMetadataBytes = 1 << 20 // 1 MiB

const userAgent = "xpm (+https://github.com/crenspire/xpm)"

// httpGet is the single entry point for registry GETs: shared client
// (with its timeout), ctx for cancellation, identifying User-Agent and a
// JSON Accept header.
func httpGet(ctx context.Context, rawURL string) (*http.Response, error) {
	return httpGetAccept(ctx, rawURL, "application/json")
}

// httpGetAccept is httpGet with a caller-chosen Accept header (the crates.io
// sparse index serves text/plain).
func httpGetAccept(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	return httpClient.Do(req)
}

// statusError builds an error for a non-2xx response, including at most
// 512 bytes of the body so HTML error pages don't flood the terminal.
// An empty body yields no trailing ": ".
func statusError(registry string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		return fmt.Errorf("%s registry returned status %d", registry, resp.StatusCode)
	}
	return fmt.Errorf("%s registry returned status %d: %s", registry, resp.StatusCode, msg)
}
