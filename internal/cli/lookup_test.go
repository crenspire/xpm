package cli

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/config"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

var timedOut = search.RegistryFailure{Manager: pm.Maven, Err: fmt.Errorf("x: %w", search.ErrRegistryTimeout)}

func TestClassifySeparatesNotFoundFromUnavailable(t *testing.T) {
	rep := search.Report{
		Results:     []search.Result{{Manager: pm.Npm, Name: "axios"}},
		Unavailable: []search.RegistryFailure{timedOut},
	}
	opts := search.Options{Enable: map[pm.ID]bool{pm.Composer: false}}
	st := classify(rep, opts)
	if fmt.Sprint(st.NotFound) != "[pip cargo]" {
		t.Fatalf("NotFound = %v, want [pip cargo] (npm found, composer disabled, maven unavailable)", st.NotFound)
	}
	if len(st.Unavailable) != 1 || st.Unavailable[0].Manager != pm.Maven {
		t.Fatalf("Unavailable = %+v", st.Unavailable)
	}
}

func TestWhichReportsUnavailableRegistriesSeparately(t *testing.T) {
	withConfig(t, config.Config{})
	withLookupReport(t, search.Report{
		Results: []search.Result{{Manager: pm.Npm, Name: "axios", Extra: map[string]string{"version": "1.7.9"}}},
		Unavailable: []search.RegistryFailure{
			{Manager: pm.Pip, Err: errors.New("pypi registry returned status 503")},
			timedOut,
		},
	}, nil)
	var code int
	out := captureStdout(t, func() { code = cmdWhich([]string{"axios"}) })
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	notFound := out[strings.Index(out, "Not found in:"):strings.Index(out, "Unavailable")]
	if strings.Contains(notFound, "maven") || strings.Contains(notFound, "pip") {
		t.Fatalf("unavailable registries listed as not found:\n%s", out)
	}
	for _, want := range []string{
		"- npm: axios @1.7.9",
		"- composer (PHP)\n- cargo (Rust)",
		"Unavailable (results may be incomplete):\n- pip (Python): pypi registry returned status 503\n- maven (Java): timed out",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestNoMatchExitsOne(t *testing.T) {
	withConfig(t, config.Config{})
	withLookupReport(t, search.Report{Unavailable: []search.RegistryFailure{timedOut}}, nil)
	withSearchReport(t, search.Report{}, nil)

	for name, run := range map[string]func() int{
		"which":   func() int { return cmdWhich([]string{"nope"}) },
		"info":    func() int { return cmdInfo([]string{"nope"}) },
		"install": func() int { return cmdInstall([]string{"nope"}) },
		"search":  func() int { return cmdSearch([]string{"nope"}) },
	} {
		var code int
		out := captureStdout(t, func() { code = run() })
		if code != 1 {
			t.Errorf("%s: exit %d, want 1 for no matches\n%s", name, code, out)
		}
	}
}

func TestAllRegistriesFailedExitsOne(t *testing.T) {
	withConfig(t, config.Config{})
	withLookupReport(t, search.Report{}, fmt.Errorf("%w: offline", search.ErrAllRegistriesFailed))
	var code int
	captureStdout(t, func() { code = cmdWhich([]string{"axios"}) })
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestSearchJoinsSeveralWordsIntoOneQuery(t *testing.T) {
	withConfig(t, config.Config{})
	var queries []string
	oldSearch := searchReport
	searchReport = func(q string, _ search.Options) (search.Report, error) {
		queries = append(queries, q)
		return search.Report{Results: []search.Result{{Manager: pm.Npm, Name: "react-router"}}}, nil
	}
	t.Cleanup(func() { searchReport = oldSearch })

	var code int
	captureStdout(t, func() { code = cmdSearch([]string{"react", "router"}) })
	if code != 0 || !reflect.DeepEqual(queries, []string{"react router"}) {
		t.Fatalf("code=%d queries=%q, want 0 and [\"react router\"]", code, queries)
	}
}

func TestSearchWithPipedStdinUsesPlainOutput(t *testing.T) {
	withConfig(t, config.Config{Interactive: true, SearchUI: config.SearchUIConfig{Enabled: true}})
	oldTTY := isInteractiveTerminal
	isInteractiveTerminal = func() bool { return false } // stdin is a pipe
	t.Cleanup(func() { isInteractiveTerminal = oldTTY })
	oldSearch := searchReport
	searchReport = func(string, search.Options) (search.Report, error) {
		return search.Report{Results: []search.Result{{Manager: pm.Npm, Name: "react"}}}, nil
	}
	t.Cleanup(func() { searchReport = oldSearch })

	var code int
	out := captureStdout(t, func() { code = cmdSearch([]string{"react"}) })
	if code != 0 || !strings.Contains(out, "react") {
		t.Fatalf("code=%d out=%q, want plain results", code, out)
	}
}

func TestFormatAvailabilityStripsControlSequences(t *testing.T) {
	st := registryStatus{Unavailable: []search.RegistryFailure{{Manager: pm.Npm, Err: errors.New("bad \x1b[2Jresponse\x1b]0;t\x07")}}}
	out := formatAvailability(st)
	if strings.Contains(out, "\x1b") || !strings.Contains(out, ": bad response\n") {
		t.Fatalf("formatAvailability = %q, want sanitized error text", out)
	}
}
