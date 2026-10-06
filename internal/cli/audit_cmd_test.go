package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// osvFake is a fake OSV.dev: every queried package named in vulns is
// vulnerable to the listed IDs; details come from details.
type osvFake struct {
	mu      sync.Mutex
	queries []map[string]any // package queries seen in batch requests
	vulns   map[string][]string
	details map[string]string // id -> details JSON
	status  int
	hang    bool
}

func (f *osvFake) handler(w http.ResponseWriter, r *http.Request) {
	if f.hang {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		return
	}
	if f.status != 0 {
		http.Error(w, "boom", f.status)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/querybatch") {
		var req struct {
			Queries []map[string]any `json:"queries"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		f.mu.Lock()
		f.queries = append(f.queries, req.Queries...)
		f.mu.Unlock()
		type res struct {
			Vulns []map[string]string `json:"vulns"`
		}
		out := struct {
			Results []res `json:"results"`
		}{}
		for _, q := range req.Queries {
			pkg, _ := q["package"].(map[string]any)
			name, _ := pkg["name"].(string)
			var rs res
			for _, id := range f.vulns[name] {
				rs.Vulns = append(rs.Vulns, map[string]string{"id": id})
			}
			out.Results = append(out.Results, rs)
		}
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if d, ok := f.details[id]; ok {
		_, _ = io.WriteString(w, d)
		return
	}
	http.NotFound(w, r)
}

func newOSVFake(t *testing.T, f *osvFake) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	old := osvBaseURL
	osvBaseURL = srv.URL
	t.Cleanup(func() { osvBaseURL = old; srv.Close() })
}

const debugDetails = `{"id":"GHSA-aaaa-bbbb-cccc","aliases":["CVE-2017-16137"],"summary":"ReDoS in debug",
"database_specific":{"severity":"HIGH"},
"affected":[{"package":{"ecosystem":"npm","name":"debug"},"ranges":[{"type":"SEMVER","events":[{"introduced":"0"},{"fixed":"2.6.10"}]}]}]}`

func runAudit(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	stdout = captureStdout(t, func() {
		stderr = captureStderr(t, func() { code = cmdAudit(args) })
	})
	return code, stdout, stderr
}

func TestAuditVulnerableExit1(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock, "requirements.txt": "requests>=2.0\n"})
	f := &osvFake{vulns: map[string][]string{"debug": {"GHSA-aaaa-bbbb-cccc"}}, details: map[string]string{"GHSA-aaaa-bbbb-cccc": debugDetails}}
	newOSVFake(t, f)
	code, stdout, stderr := runAudit(t)
	if code != 1 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"debug@2.6.9 (node)", "GHSA-aaaa-bbbb-cccc", "(CVE-2017-16137)", "[HIGH]", "ReDoS in debug", "(fixed in: 2.6.10)",
		"Found 1 vulnerabilities in 1 packages (3 packages scanned)."} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stderr, "note: 1 dependency was not checked") {
		t.Errorf("stderr = %q", stderr)
	}
	// Only pinned packages are queried, node maps to npm.
	if len(f.queries) != 3 {
		t.Fatalf("queries = %v", f.queries)
	}
	for _, q := range f.queries {
		pkg := q["package"].(map[string]any)
		if pkg["ecosystem"] != "npm" || pkg["name"] == "requests" {
			t.Errorf("query = %v", q)
		}
	}
}

func TestAuditClean(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	newOSVFake(t, &osvFake{})
	code, stdout, stderr := runAudit(t)
	if code != 0 || !strings.Contains(stdout, "No known vulnerabilities in 3 packages.") {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestAuditNoDependencies(t *testing.T) {
	graphProject(t, map[string]string{"README.md": "x"})
	code, stdout, _ := runAudit(t)
	if code != 0 || !strings.Contains(stdout, "No dependencies found.") {
		t.Fatalf("exit %d stdout %q", code, stdout)
	}
}

func TestAuditOSVFailureExit2(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	newOSVFake(t, &osvFake{status: 500})
	code, stdout, stderr := runAudit(t)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "error: OSV.dev query failed: ") {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestAuditTimeout(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	newOSVFake(t, &osvFake{hang: true})
	start := time.Now()
	code, _, stderr := runAudit(t, "--timeout", "50ms")
	if code != 2 || !strings.Contains(stderr, "OSV.dev query failed") {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
}

func TestAuditBadTimeoutAndUsage(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	newOSVFake(t, &osvFake{})
	for _, args := range [][]string{{"--timeout", "0"}, {"--timeout", "abc"}, {"--timeout", "-5s"}, {"extra"}, {"--bogus"}} {
		if code, stdout, _ := runAudit(t, args...); code != 2 || stdout != "" {
			t.Errorf("%v: exit %d stdout %q", args, code, stdout)
		}
	}
	// A bare number is seconds, and flags may follow no positionals only.
	if code, _, stderr := runAudit(t, "--timeout", "20"); code != 0 {
		t.Errorf("--timeout 20: exit %d stderr %q", code, stderr)
	}
}

func TestAuditJSONShape(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock, "requirements.txt": "requests>=2.0\n"})
	newOSVFake(t, &osvFake{vulns: map[string][]string{"debug": {"GHSA-aaaa-bbbb-cccc"}}, details: map[string]string{"GHSA-aaaa-bbbb-cccc": debugDetails}})
	code, stdout, stderr := runAudit(t, "--json")
	if code != 1 {
		t.Fatalf("exit %d stderr %q", code, stderr)
	}
	var out struct {
		Scanned    int
		Vulnerable []struct {
			Ecosystem, Name, Version string
			Vulns                    []struct {
				ID, Summary, Severity string
				Aliases, Fixed        []string
			}
		}
		Unchecked []struct{ Ecosystem, Name, Version, Reason string }
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("%v\n%s", err, stdout)
	}
	if out.Scanned != 3 || len(out.Vulnerable) != 1 || out.Vulnerable[0].Name != "debug" || out.Vulnerable[0].Ecosystem != "node" || out.Vulnerable[0].Version != "2.6.9" {
		t.Errorf("out = %+v", out)
	}
	v := out.Vulnerable[0].Vulns[0]
	if v.ID != "GHSA-aaaa-bbbb-cccc" || v.Severity != "HIGH" || v.Aliases[0] != "CVE-2017-16137" || v.Fixed[0] != "2.6.10" {
		t.Errorf("vuln = %+v", v)
	}
	if len(out.Unchecked) != 1 || out.Unchecked[0].Name != "requests" || out.Unchecked[0].Ecosystem != "python" || out.Unchecked[0].Reason == "" {
		t.Errorf("unchecked = %+v", out.Unchecked)
	}

	newOSVFake(t, &osvFake{})
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	_, stdout, _ = runAudit(t, "--json")
	if !strings.Contains(stdout, `"vulnerable": []`) || !strings.Contains(stdout, `"unchecked": []`) {
		t.Errorf("arrays must not be null:\n%s", stdout)
	}
}

func TestAuditSanitizesOSVText(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	d := `{"id":"GHSA-x","summary":"bad\u001b[31m red\u0007 text","affected":[]}`
	newOSVFake(t, &osvFake{vulns: map[string][]string{"debug": {"GHSA-x"}}, details: map[string]string{"GHSA-x": d}})
	code, stdout, _ := runAudit(t)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if strings.ContainsAny(stdout, "\x1b\x07") {
		t.Errorf("control characters leaked: %q", stdout)
	}
	if !strings.Contains(stdout, "GHSA-x") || strings.Contains(stdout, "[HIGH]") || strings.Contains(stdout, "fixed in") {
		t.Errorf("stdout:\n%s", stdout)
	}
}

func TestAuditDetailsWarning(t *testing.T) {
	graphProject(t, map[string]string{"package-lock.json": demoLock})
	newOSVFake(t, &osvFake{vulns: map[string][]string{"debug": {"GHSA-gone"}}})
	code, stdout, stderr := runAudit(t)
	if code != 1 || !strings.Contains(stdout, "GHSA-gone") || !strings.Contains(stderr, "warning: details for 1 vulnerabilities could not be fetched; IDs are listed") {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
}

func TestNoteUncheckedPlural(t *testing.T) {
	got := captureStderr(t, func() { noteUnchecked(3); noteUnchecked(0) })
	if got != "note: 3 dependencies were not checked (no locked version, or no registry/OSV lookup for it; --json lists the reasons)\n" {
		t.Errorf("got %q", got)
	}
}
