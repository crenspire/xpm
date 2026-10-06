package search

import (
	"context"
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
