package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPConfig holds configuration for HTTP requests.
type HTTPConfig struct {
	// Timeout is the maximum duration for a single request.
	Timeout time.Duration
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration
	// RetryBackoff multiplies the delay for each subsequent retry.
	RetryBackoff float64
}

// DefaultHTTPConfig returns the default HTTP configuration.
func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		Timeout:      4 * time.Second,
		MaxRetries:   2,
		RetryDelay:   500 * time.Millisecond,
		RetryBackoff: 2.0,
	}
}

// httpConfig is the active HTTP configuration.
var httpConfig = DefaultHTTPConfig()

// SetHTTPConfig sets the HTTP configuration for search requests.
func SetHTTPConfig(cfg HTTPConfig) {
	httpConfig = cfg
	httpClient.Timeout = cfg.Timeout
}

// GetHTTPConfig returns the current HTTP configuration.
func GetHTTPConfig() HTTPConfig {
	return httpConfig
}

// isRetryableError checks if an error is retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Network timeouts are retryable
	if err == context.DeadlineExceeded {
		return true
	}

	// Connection errors are retryable
	errStr := err.Error()
	retryableMessages := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"timeout",
		"temporary failure",
	}

	for _, msg := range retryableMessages {
		if contains(errStr, msg) {
			return true
		}
	}

	return false
}

// contains checks if s contains substr (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

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
