package env

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// signalWriter collects output and closes seen once it contains want, so a
// test can wait for "Waiting for another xpm process" instead of sleeping.
type signalWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	want string
	seen chan struct{}
	once sync.Once
}

func newSignalWriter(want string) *signalWriter {
	return &signalWriter{want: want, seen: make(chan struct{})}
}

func (w *signalWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if strings.Contains(w.buf.String(), w.want) {
		w.once.Do(func() { close(w.seen) })
	}
	return n, err
}

func (w *signalWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *signalWriter) wait(t *testing.T) {
	t.Helper()
	select {
	case <-w.seen:
	case <-time.After(5 * time.Second):
		t.Fatalf("never printed %q; output %q", w.want, w.String())
	}
}

// namedFake is a second test runtime with its own name and binaries.
type namedFake struct {
	*fakeInstaller
	name string
	bins []string
}

func (f namedFake) Name() string          { return f.name }
func (f namedFake) BinaryPaths() []string { return f.bins }

func TestConcurrentGlobalDefaultsAllSurvive(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rt := fmt.Sprintf("rt%02d", i)
			if i%2 == 0 {
				errs <- m.afterInstall(context.Background(), rt, "1.0.0", "", "")
				return
			}
			errs <- m.withStateLock(context.Background(), func() error { return m.SetGlobalVersion(rt, "2.0.0") })
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	active, err := m.loadActiveVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != n {
		t.Fatalf("active.json kept %d of %d runtimes: %v", len(active), n, active)
	}
}

func TestCreateShimsConcurrentRuntimesKeepEveryShim(t *testing.T) {
	skipOnWindows(t)
	useFake(t, &fakeInstaller{})
	const other = "xpmfakeb"
	RegisterInstaller(other, namedFake{&fakeInstaller{}, other, []string{"bin/fakeb"}})
	t.Cleanup(func() { unregisterInstaller(other) })
	for i := 0; i < 10; i++ {
		m := isolate(t)
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		wg.Add(2)
		go func() { // process A installs xpmfake, then reshims
			defer wg.Done()
			if err := writeFakeBinaries(filepath.Join(m.GetRuntimesPath(), fakeRT, "1.0.0")); err != nil {
				errs <- err
				return
			}
			errs <- CreateShims(m)
		}()
		go func() { // process B installs xpmfakeb, then reshims
			defer wg.Done()
			if err := os.MkdirAll(filepath.Join(m.GetRuntimesPath(), other, "1.0.0", "bin"), 0o755); err != nil {
				errs <- err
				return
			}
			if err := os.WriteFile(filepath.Join(m.GetRuntimesPath(), other, "1.0.0", "bin", "fakeb"), []byte("#!/bin/sh\n"), 0o755); err != nil {
				errs <- err
				return
			}
			errs <- CreateShims(m)
		}()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: %v", i, err)
			}
		}
		for _, name := range []string{"fakebin", "fakebin2", "fakeb"} {
			if _, err := os.Lstat(filepath.Join(m.GetShimsPath(), name)); err != nil {
				t.Fatalf("round %d: shim %s missing: %v", i, name, err)
			}
		}
	}
}

func TestCreateShimsPruneSkipsInFlightNames(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	useFake(t, &fakeInstaller{})
	installFake(t, m, "1.0.0", "")
	keep := []string{".hidden", "fakebin.tmp-0123456789ab"}
	for _, n := range append([]string{"stale"}, keep...) {
		if err := os.WriteFile(filepath.Join(m.GetShimsPath(), n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := CreateShims(m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(m.GetShimsPath(), "stale")); err == nil {
		t.Fatal("stale shim not pruned")
	}
	for _, n := range keep {
		if _, err := os.Lstat(filepath.Join(m.GetShimsPath(), n)); err != nil {
			t.Fatalf("%s was pruned: %v", n, err)
		}
	}
}

func TestRemoveVersionHoldsTheRuntimeLock(t *testing.T) {
	skipOnWindows(t)
	m := isolate(t)
	out := newSignalWriter("Waiting for another xpm process changing xpmfake...")
	m.SetOutput(out)
	useFake(t, &fakeInstaller{})
	dir := installFake(t, m, "1.0.0", "")
	installFake(t, m, "2.0.0", "")
	unlock, err := lockFile(context.Background(), filepath.Join(filepath.Dir(dir), ".lock"), func() {})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- RemoveVersion(context.Background(), m, fakeRT, "1.0.0") }()
	out.wait(t)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("removed while another process held the lock: %v", err)
	}
	unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("remove never got the lock")
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("not removed")
	}
}
