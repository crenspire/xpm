package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

func TestSearchWhitespaceQueryIsMissing(t *testing.T) {
	withConfig(t, config.Config{})
	called := false
	var queries []string
	old := searchReport
	searchReport = func(q string, _ search.Options) (search.Report, error) {
		called = true
		queries = append(queries, q)
		return search.Report{Results: []search.Result{{Manager: pm.Npm, Name: "axios"}}}, nil
	}
	t.Cleanup(func() { searchReport = old })

	var code int
	errOut := captureStderr(t, func() {
		captureStdout(t, func() { code = cmdSearch([]string{"  ", "\t"}) })
	})
	if code != 1 || !strings.Contains(errOut, "missing package name") || called {
		t.Fatalf("code=%d called=%v stderr=%q, want 1, no registry call, missing package name", code, called, errOut)
	}

	captureStdout(t, func() { code = cmdSearch([]string{" axios "}) })
	if code != 0 || len(queries) != 1 || queries[0] != "axios" {
		t.Fatalf("code=%d queries=%q, want 0 and [axios]", code, queries)
	}
}

func TestParseInstallArgsRejectsValuedGlobal(t *testing.T) {
	for _, a := range []string{"--global=false", "--global=true", "-g=true", "-g=false"} {
		t.Run(a, func(t *testing.T) {
			_, err := parseInstallArgs([]string{"axios", a})
			if err == nil || !strings.Contains(err.Error(), "omit it for a local install") {
				t.Fatalf("err = %v, want a message containing %q", err, "omit it for a local install")
			}
		})
	}
	if _, err := parseInstallArgs([]string{"--bogus"}); err == nil || strings.Contains(err.Error(), "omit it") {
		t.Fatalf("other flags keep the generic message, got %v", err)
	}
}

func TestEnsureManagerDisabledAutoInstallUsesToInstallIt(t *testing.T) {
	withConfig(t, config.Config{AutoInstallPM: false})
	t.Cleanup(pm.SetLookPath(func(string) (string, error) { return "", errors.New("not found") }))
	var err error
	out := captureStdout(t, func() { err = ensureManager(pm.Pnpm) })
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(out, "To install it: ") || strings.Contains(out, "Hint:") {
		t.Fatalf("output = %q, want the To install it: prefix and no Hint:", out)
	}
}
