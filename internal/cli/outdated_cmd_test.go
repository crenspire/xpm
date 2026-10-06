package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/graph"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

const outdatedGoMod = "module example.com/app\n\ngo 1.22\n\nrequire golang.org/x/text v0.3.0\n"

// fakeLatest replaces latestVersions with a lookup table (name -> version or
// error) and records every call's queries.
func fakeLatest(t *testing.T, versions map[string]string, errs map[string]error) *[][]search.LatestQuery {
	t.Helper()
	var calls [][]search.LatestQuery
	old := latestVersions
	latestVersions = func(qs []search.LatestQuery, _ search.Options) []search.LatestResult {
		calls = append(calls, qs)
		out := make([]search.LatestResult, len(qs))
		for i, q := range qs {
			r := search.LatestResult{LatestQuery: q}
			if err := errs[q.Name]; err != nil {
				r.Err = err
			} else if v, ok := versions[q.Name]; ok {
				r.Version, r.Found = v, true
			}
			out[i] = r
		}
		return out
	}
	t.Cleanup(func() { latestVersions = old })
	return &calls
}

func runOutdated(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() { code = cmdOutdated(args) })
	})
	return code, stdout, stderr
}

func TestOutdatedTableAndExit1(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock, "go.mod": outdatedGoMod})
	calls := fakeLatest(t, map[string]string{"debug": "4.0.0", "ms": "2.1.3", "golang.org/x/text": "v0.14.0"}, nil)
	code, stdout, stderr := runOutdated(t)
	if code != 1 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if len(*calls) != 1 || len((*calls)[0]) != 3 {
		t.Fatalf("calls = %v, want one call with 3 queries", *calls)
	}
	seen := map[string]pm.ID{}
	for _, q := range (*calls)[0] {
		seen[q.Name] = q.Manager
	}
	if seen["debug"] != pm.Npm || seen["ms"] != pm.Npm || seen["golang.org/x/text"] != pm.GoMod {
		t.Errorf("queries = %v", seen)
	}
	for _, want := range []string{"ECOSYSTEM", "PACKAGE", "CURRENT", "LATEST", "STATUS"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 { // header, go row, node row, summary
		t.Fatalf("stdout:\n%s", stdout)
	}
	if f := strings.Fields(lines[1]); strings.Join(f, " ") != "go golang.org/x/text v0.3.0 v0.14.0 outdated" {
		t.Errorf("row 1 = %q", lines[1])
	}
	if f := strings.Fields(lines[2]); strings.Join(f, " ") != "node debug 2.6.9 4.0.0 outdated" {
		t.Errorf("row 2 = %q", lines[2])
	}
	if lines[3] != "2 outdated, 1 up to date" {
		t.Errorf("summary = %q", lines[3])
	}
	if strings.Contains(stdout, "ms ") {
		t.Errorf("current package listed:\n%s", stdout)
	}
}

func TestOutdatedAllCurrent(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": singleDepLock("demo", "left-pad", "1.3.0")})
	fakeLatest(t, map[string]string{"left-pad": "1.3.0"}, nil)
	code, stdout, _ := runOutdated(t)
	if code != 0 || stdout != "All 1 dependencies are up to date.\n" {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
}

func TestOutdatedNoDependencies(t *testing.T) {
	graphProject(t, map[string]string{"README.md": "x"})
	calls := fakeLatest(t, nil, nil)
	code, stdout, _ := runOutdated(t)
	if code != 0 || stdout != "No dependencies found.\n" || len(*calls) != 0 {
		t.Errorf("exit %d, stdout %q, calls %v", code, stdout, *calls)
	}
}

func TestOutdatedUnavailableExit2(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	fakeLatest(t, map[string]string{"ms": "2.1.3"}, map[string]error{"debug": errors.New("timeout\x1b[31m")})
	code, stdout, _ := runOutdated(t)
	if code != 2 {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
	if !strings.Contains(stdout, "unavailable") || !strings.Contains(stdout, "1 could not be checked") {
		t.Errorf("stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "\x1b") {
		t.Errorf("unsanitized text: %q", stdout)
	}
}

func TestOutdatedOutdatedBeatsUnavailable(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	fakeLatest(t, map[string]string{"ms": "9.0.0"}, map[string]error{"debug": errors.New("boom")})
	if code, _, _ := runOutdated(t); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}

func TestOutdatedNotFoundKeepsExit0(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": singleDepLock("demo", "secret-pkg", "1.0.0")})
	fakeLatest(t, nil, nil)
	code, stdout, _ := runOutdated(t)
	if code != 0 || !strings.Contains(stdout, "not found") || !strings.Contains(stdout, "0 outdated, 0 up to date, 1 not found\n") || strings.Contains(stdout, "could not be checked") {
		t.Errorf("exit %d, stdout:\n%s", code, stdout)
	}
}

func TestOutdatedUncheckedErrorsAreNotUnavailable(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock, "go.mod": outdatedGoMod})
	fakeLatest(t, map[string]string{"debug": "2.6.9", "ms": "2.1.3"}, map[string]error{
		"golang.org/x/text": fmt.Errorf("%w: private module", search.ErrNotChecked),
	})
	code, stdout, stderr := runOutdated(t, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var out struct {
		Dependencies []struct{ Name string }
		Unchecked    []struct{ Name, Reason string }
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Dependencies) != 2 || len(out.Unchecked) != 1 || out.Unchecked[0].Name != "golang.org/x/text" || out.Unchecked[0].Reason == "" {
		t.Errorf("json = %s", stdout)
	}
	if !strings.Contains(stderr, "note: 1 dependency was not checked") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestOutdatedJSON(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": singleDepLock("demo", "left-pad", "1.0.0")})
	fakeLatest(t, map[string]string{"left-pad": "1.3.0"}, nil)
	code, stdout, _ := runOutdated(t, "--json")
	if code != 1 {
		t.Errorf("exit %d", code)
	}
	var out struct {
		Dependencies []map[string]string `json:"dependencies"`
		Unchecked    []map[string]string `json:"unchecked"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	want := map[string]string{"ecosystem": "node", "name": "left-pad", "current": "1.0.0", "latest": "1.3.0", "status": "outdated"}
	if len(out.Dependencies) != 1 || fmt.Sprint(out.Dependencies[0]) != fmt.Sprint(want) {
		t.Errorf("dependencies = %v", out.Dependencies)
	}
	if out.Unchecked == nil || len(out.Unchecked) != 0 || !strings.Contains(stdout, `"unchecked": []`) {
		t.Errorf("unchecked should be [], stdout %s", stdout)
	}
}

func TestOutdatedJSONEmptyArrays(t *testing.T) {
	graphProject(t, map[string]string{"README.md": "x"})
	fakeLatest(t, nil, nil)
	code, stdout, _ := runOutdated(t, "--json")
	if code != 0 || !strings.Contains(stdout, `"dependencies": []`) || !strings.Contains(stdout, `"unchecked": []`) {
		t.Errorf("exit %d, stdout %s", code, stdout)
	}
}

func TestOutdatedUnpinnedIsUnchecked(t *testing.T) {
	graphProject(t, map[string]string{"requirements.txt": "requests>=2.0\n", "go.mod": outdatedGoMod})
	calls := fakeLatest(t, map[string]string{"golang.org/x/text": "v0.3.0"}, nil)
	code, stdout, stderr := runOutdated(t, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if len(*calls) != 1 || len((*calls)[0]) != 1 {
		t.Errorf("calls = %v", *calls)
	}
	var out struct {
		Unchecked []struct{ Ecosystem, Name, Current string }
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Unchecked) != 1 || out.Unchecked[0].Name != "requests" || out.Unchecked[0].Ecosystem != "python" {
		t.Errorf("unchecked = %+v", out.Unchecked)
	}
	if !strings.Contains(stderr, "note: 1 dependency was not checked (no locked version, or no registry lookup for it; --json lists the reasons)") || strings.Contains(stderr, "OSV") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestOutdatedUsageErrors(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	calls := fakeLatest(t, nil, nil)
	for _, args := range [][]string{{"--bogus"}, {"react"}, {"--json", "react"}} {
		if code, _, _ := runOutdated(t, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("registry queried on usage error")
	}
}

func TestOutdatedAllIncludesTransitive(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	calls := fakeLatest(t, map[string]string{"debug": "2.6.9", "ms": "2.1.3"}, nil)
	_, stdout, _ := runOutdated(t, "--all")
	if !strings.Contains(stdout, "ms") || !strings.Contains(stdout, "2.0.0") {
		t.Errorf("transitive ms@2.0.0 missing:\n%s", stdout)
	}
	if len(*calls) != 1 || len((*calls)[0]) != 2 {
		t.Errorf("queries not de-duplicated: %v", *calls)
	}
}

func TestOutdatedExtractionErrorExit2(t *testing.T) {
	graphProject(t, map[string]string{"README.md": "x"})
	old := extractGraph
	extractGraph = func(string, graph.ExtractOptions) (*graph.DepGraph, error) { return nil, errors.New("unreadable") }
	t.Cleanup(func() { extractGraph = old })
	code, _, stderr := runOutdated(t)
	if code != 2 || !strings.Contains(stderr, "unreadable") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

func TestOutdatedNothingCheckedExit2(t *testing.T) {
	graphProject(t, map[string]string{"requirements.txt": "requests>=2.0\nflask\n"})
	calls := fakeLatest(t, nil, nil)
	code, stdout, stderr := runOutdated(t)
	if code != 2 {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
	if stdout != "" || !strings.Contains(stderr, "error: nothing could be checked: 2 dependencies were not checked (no locked version, or no registry lookup for it); see --json") {
		t.Errorf("stdout %q stderr %q", stdout, stderr)
	}
	if strings.Contains(stderr, "note:") {
		t.Errorf("note repeated: %q", stderr)
	}
	if len(*calls) != 0 {
		t.Errorf("calls = %v", *calls)
	}
	// Lookups skipped on purpose (private Go module) also leave nothing checked.
	graphProject(t, map[string]string{"go.mod": outdatedGoMod})
	fakeLatest(t, nil, map[string]error{"golang.org/x/text": fmt.Errorf("%w: private module", search.ErrNotChecked)})
	code, stdout, _ = runOutdated(t, "--json")
	if code != 2 || !strings.Contains(stdout, `"unchecked": [`) {
		t.Errorf("--json: exit %d stdout %s", code, stdout)
	}
}

func TestOutdatedSummaryCountsNotFoundSeparately(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	fakeLatest(t, map[string]string{"debug": "9.0.0"}, nil) // both ms rows: not found
	_, stdout, _ := runOutdated(t, "--all")
	if !strings.Contains(stdout, "1 outdated, 0 up to date, 2 not found\n") {
		t.Errorf("stdout:\n%s", stdout)
	}
}
