package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/crenspire/xpm/internal/logx"
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

// SetTimeoutForRegistry sets a custom timeout for a specific registry.
// This allows per-registry timeout configuration.
func SetTimeoutForRegistry(registry string, timeout time.Duration) {
	// Update the default timeout if this is the first registry configured
	// For now, we use a single timeout for all registries
	// Future enhancement: maintain per-registry timeouts
	httpConfig.Timeout = timeout
	httpClient.Timeout = timeout
}

// GetHTTPConfig returns the current HTTP configuration.
func GetHTTPConfig() HTTPConfig {
	return httpConfig
}

// retryableGet performs an HTTP GET with retry logic.
// It retries on network errors and 5xx status codes.
func retryableGet(url string) (*http.Response, error) {
	var lastErr error
	delay := httpConfig.RetryDelay

	// Create a single context with total timeout for all retries
	// Total timeout = per-request timeout * (max retries + 1) to allow for all attempts
	totalTimeout := httpConfig.Timeout * time.Duration(httpConfig.MaxRetries+1)
	ctx, cancel := context.WithTimeout(context.Background(), totalTimeout)
	defer cancel() // Ensure context is cancelled when function returns

	for attempt := 0; attempt <= httpConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			logx.Info("retrying request (attempt %d/%d) after %v", attempt, httpConfig.MaxRetries, delay)
			// Check if context is already cancelled before sleeping
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
			case <-time.After(delay):
				// Sleep completed
			}
			delay = time.Duration(float64(delay) * httpConfig.RetryBackoff)
		}

		// Check if context is cancelled before making request
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set a custom user agent
		req.Header.Set("User-Agent", "xpm (https://github.com/crenspire/xpm)")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			logx.Info("request failed: %v", err)
			// Check if we should retry (context not cancelled and not last attempt)
			if ctx.Err() != nil || attempt >= httpConfig.MaxRetries {
				break
			}
			continue
		}

		// Retry on 5xx errors
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
			logx.Info("server error %d, will retry", resp.StatusCode)
			// Check if we should retry (context not cancelled and not last attempt)
			if ctx.Err() != nil || attempt >= httpConfig.MaxRetries {
				break
			}
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", httpConfig.MaxRetries+1, lastErr)
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

// withTimeout wraps a function with a context timeout.
func withTimeout(timeout time.Duration, fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return fn(ctx)
}
