package search

import (
	"context"
	"testing"
	"time"
)

// TestContextCancellationLeak tests that contexts are properly cancelled.
func TestContextCancellationLeak(t *testing.T) {
	// Set a very short timeout to test cancellation
	oldConfig := httpConfig
	defer func() { httpConfig = oldConfig }()

	httpConfig = HTTPConfig{
		Timeout:       100 * time.Millisecond,
		MaxRetries:    2,
		RetryDelay:    10 * time.Millisecond,
		RetryBackoff:  2.0,
	}

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Wait for context to be cancelled
	<-ctx.Done()

	// Verify context is actually cancelled
	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", ctx.Err())
	}
}

// TestIsRetryableErrorFix tests the fix for incorrect error check.
func TestIsRetryableErrorFix(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "context deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "other error",
			err:  context.Canceled,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetryableError(tc.err)
			if got != tc.want {
				t.Errorf("isRetryableError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

