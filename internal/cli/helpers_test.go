package cli

import (
	"io"
	"os"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/search"
)

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	defer func() {
		os.Stdout = old
	}()
	fn()
	w.Close()
	return <-out
}

// withConfig replaces the package-level cfg for one test.
func withConfig(t *testing.T, c config.Config) {
	t.Helper()
	old := cfg
	cfg = c
	t.Cleanup(func() { cfg = old })
}

// withLookupReport fakes the exact-name registry fan-out.
func withLookupReport(t *testing.T, rep search.Report, err error) {
	t.Helper()
	old := lookupReport
	lookupReport = func(string, search.Options) (search.Report, error) { return rep, err }
	t.Cleanup(func() { lookupReport = old })
}

// withSearchReport fakes the multi-result registry fan-out.
func withSearchReport(t *testing.T, rep search.Report, err error) {
	t.Helper()
	old := searchReport
	searchReport = func(string, search.Options) (search.Report, error) { return rep, err }
	t.Cleanup(func() { searchReport = old })
}
