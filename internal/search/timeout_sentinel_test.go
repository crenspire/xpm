package search

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

// timeoutNetErr is a net.Error that reports a timeout, like net/http's
// client timeout.
type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "i/o timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return true }

func singleFailure(t *testing.T, fn func(ctx context.Context) ([]Result, error)) error {
	t.Helper()
	rep := fanOut([]registryCall{{id: pm.Npm, timeout: 50 * time.Millisecond, fn: fn}})
	if len(rep.Unavailable) != 1 {
		t.Fatalf("Unavailable = %+v, want exactly one", rep.Unavailable)
	}
	return rep.Unavailable[0].Err
}

func TestFanOutCtxDeadlineReportedAsTimeout(t *testing.T) {
	err := singleFailure(t, func(ctx context.Context) ([]Result, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if !errors.Is(err, ErrRegistryTimeout) {
		t.Fatalf("err = %v, want ErrRegistryTimeout", err)
	}
}

func TestFanOutNetTimeoutIsSentinel(t *testing.T) {
	err := singleFailure(t, func(context.Context) ([]Result, error) {
		return nil, timeoutNetErr{}
	})
	if !errors.Is(err, ErrRegistryTimeout) {
		t.Fatalf("err = %v, want ErrRegistryTimeout", err)
	}
}

func TestFanOutOtherErrorIsNotSentinel(t *testing.T) {
	err := singleFailure(t, func(context.Context) ([]Result, error) {
		return nil, errors.New("boom")
	})
	if errors.Is(err, ErrRegistryTimeout) {
		t.Fatalf("err = %v, must not be ErrRegistryTimeout", err)
	}
}

func TestSearchReportPipFreeTextIsNotUnavailable(t *testing.T) {
	var reqs int32
	fakeRegistry(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqs, 1)
		http.NotFound(w, r)
	})
	got, err := searchPipMultiple(context.Background(), "http client")
	if got != nil || err != nil {
		t.Fatalf("searchPipMultiple = (%v, %v), want (nil, nil)", got, err)
	}
	withMultiLookups(t, []multiLookup{{id: pm.Pip, fn: searchPipMultiple}})
	rep, err := SearchReport("http client", Options{})
	if err != nil || len(rep.Unavailable) != 0 || len(rep.Results) != 0 {
		t.Fatalf("SearchReport = (%+v, %v), want empty report", rep, err)
	}
	if n := atomic.LoadInt32(&reqs); n != 0 {
		t.Fatalf("made %d requests, want 0", n)
	}
}

func TestAsTimeout(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		in          error
		wantTimeout bool
		wantSame    bool
	}{
		{"nil", nil, false, true},
		{"deadline exceeded", context.DeadlineExceeded, true, false},
		{"wrapped deadline exceeded", fmt.Errorf("x: %w", context.DeadlineExceeded), true, false},
		{"net timeout", timeoutNetErr{}, true, false},
		{"other error", boom, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := asTimeout(tc.in)
			if is := errors.Is(got, ErrRegistryTimeout); is != tc.wantTimeout {
				t.Fatalf("asTimeout(%v) timeout = %v, want %v", tc.in, is, tc.wantTimeout)
			}
			if tc.wantSame && !errors.Is(got, tc.in) && (got != nil || tc.in != nil) {
				t.Fatalf("asTimeout(%v) = %v, want unchanged", tc.in, got)
			}
			if tc.wantTimeout && !errors.Is(got, tc.in) {
				t.Fatalf("asTimeout(%v) = %v, lost the original error", tc.in, got)
			}
		})
	}
}
