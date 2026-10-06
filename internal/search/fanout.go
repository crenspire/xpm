package search

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// lookupDeadline bounds the whole fan-out. Registry APIs have long tails
// (crates.io has been measured at 46s, Maven search stalls without ever
// answering), so a registry that misses the deadline is reported as timed
// out instead of holding up the answer.
var lookupDeadline = 2500 * time.Millisecond

var (
	// ErrAllRegistriesFailed is returned when every enabled registry errored
	// or timed out, so callers can tell "offline" from "no such package".
	ErrAllRegistriesFailed = errors.New("all registries failed")
	// ErrRegistryTimeout marks a registry that missed the deadline.
	ErrRegistryTimeout = errors.New("registry did not answer in time")
)

// RegistryFailure is a registry that errored or did not answer in time.
type RegistryFailure struct {
	Manager pm.ID
	Err     error
}

// TimedOut reports whether the registry missed its deadline (as opposed to
// answering with an error).
func (f RegistryFailure) TimedOut() bool {
	return errors.Is(f.Err, ErrRegistryTimeout) || errors.Is(f.Err, context.DeadlineExceeded)
}

// Report is the outcome of asking several registries: what they found, and
// which ones could not answer. "Not in Unavailable and not in Results"
// means the registry answered "no such package".
type Report struct {
	Results     []Result
	Unavailable []RegistryFailure
}

// UnavailableIDs returns the managers of r.Unavailable in table order.
func (r Report) UnavailableIDs() []pm.ID {
	ids := make([]pm.ID, 0, len(r.Unavailable))
	for _, f := range r.Unavailable {
		ids = append(ids, f.Manager)
	}
	return ids
}

// registryCall is one registry's part of a fan-out. fn must honour ctx,
// which expires after timeout.
type registryCall struct {
	id      pm.ID
	timeout time.Duration
	fn      func(ctx context.Context) ([]Result, error)
}

// fanOut runs calls concurrently and waits at most for the longest call
// timeout. Results keep call order. A call that has not answered by then is
// reported as ErrRegistryTimeout and its request is cancelled.
func fanOut(calls []registryCall) Report {
	var deadline time.Duration
	for _, c := range calls {
		if c.timeout > deadline {
			deadline = c.timeout
		}
	}
	type outcome struct {
		res []Result
		err error
	}
	type indexed struct {
		i int
		outcome
	}
	// Buffered so goroutines that finish after the deadline never block.
	ch := make(chan indexed, len(calls))
	ctxs := make([]context.CancelFunc, 0, len(calls))
	defer func() {
		for _, cancel := range ctxs {
			cancel()
		}
	}()
	for i, c := range calls {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		ctxs = append(ctxs, cancel)
		go func(i int, c registryCall, ctx context.Context) {
			res, err := c.fn(ctx)
			ch <- indexed{i, outcome{res, err}}
		}(i, c, ctx)
	}

	outcomes := make([]outcome, len(calls))
	for i := range outcomes {
		outcomes[i] = outcome{err: ErrRegistryTimeout}
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()
collect:
	for received := 0; received < len(calls); received++ {
		select {
		case o := <-ch:
			outcomes[o.i] = o.outcome
		case <-timer.C:
			break collect
		}
	}

	var rep Report
	for i, o := range outcomes {
		if o.err != nil {
			logx.Info("registry %s failed: %v", calls[i].id, o.err)
			rep.Unavailable = append(rep.Unavailable, RegistryFailure{Manager: calls[i].id, Err: o.err})
			continue
		}
		rep.Results = append(rep.Results, o.res...)
	}
	return rep
}

// runReport fans out calls and turns "every registry failed" into an error
// wrapping ErrAllRegistriesFailed. The Report is returned either way.
func runReport(calls []registryCall) (Report, error) {
	if len(calls) == 0 {
		return Report{}, nil
	}
	rep := fanOut(calls)
	if len(rep.Unavailable) == len(calls) {
		errs := make([]error, 0, len(rep.Unavailable))
		for _, f := range rep.Unavailable {
			errs = append(errs, fmt.Errorf("%s: %w", f.Manager, f.Err))
		}
		return rep, fmt.Errorf("%w: %w", ErrAllRegistriesFailed, errors.Join(errs...))
	}
	return rep, nil
}

// SearchEverywhereReport checks every enabled registry concurrently for an
// exact package name and returns within lookupDeadline, reporting which
// registries could not answer. If all of them fail the error wraps
// ErrAllRegistriesFailed.
func SearchEverywhereReport(pkg string, opts Options) (Report, error) {
	cacheDir := lookupCacheDir
	var calls []registryCall
	for _, l := range exactLookups {
		if !Enabled(opts, l.id) {
			continue
		}
		calls = append(calls, registryCall{id: l.id, timeout: lookupDeadline, fn: func(ctx context.Context) ([]Result, error) {
			res, err := cachedLookup(ctx, cacheDir, l, pkg)
			if err != nil || res == nil {
				return nil, err
			}
			return []Result{*res}, nil
		}})
	}
	return runReport(calls)
}

// SearchEverywhere is SearchEverywhereReport without the per-registry
// failures: results in table order, or an error if every registry failed.
func SearchEverywhere(pkg string, opts Options) ([]Result, error) {
	rep, err := SearchEverywhereReport(pkg, opts)
	if err != nil {
		return nil, err
	}
	return rep.Results, nil
}
