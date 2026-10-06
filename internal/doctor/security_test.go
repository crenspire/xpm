package doctor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeAudit replaces lookPath and runCommand. installed lists binaries on
// PATH; every run returns out (a testdata/audit fixture name, or raw output
// when it does not end in .json/.ndjson), code and err, and is recorded.
type fakeAudit struct {
	installed map[string]bool
	out       string
	code      int
	err       error
	calls     []string
}

func withFakeAudit(t *testing.T, f *fakeAudit) {
	t.Helper()
	oldLook, oldRun := lookPath, runCommand
	lookPath = func(name string) (string, error) {
		if f.installed[name] {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}
	runCommand = func(dir, name string, args ...string) ([]byte, int, error) {
		f.calls = append(f.calls, filepath.Base(dir)+": "+name+" "+strings.Join(args, " "))
		out := []byte(f.out)
		if strings.HasSuffix(f.out, ".json") || strings.HasSuffix(f.out, ".ndjson") {
			data, err := os.ReadFile(filepath.Join("testdata", "audit", f.out))
			if err != nil {
				t.Fatal(err)
			}
			out = data
		}
		return out, f.code, f.err
	}
	t.Cleanup(func() { lookPath, runCommand = oldLook, oldRun })
}

// projectWith creates a temp project holding the named (empty unless given)
// files and returns its directory and scan.
func projectWith(t *testing.T, files map[string]string) (string, ProjectScanResult) {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, ScanProject(dir)
}

func TestRunSecurityAuditClassifiesToolOutput(t *testing.T) {
	cases := []struct {
		name      string
		files     map[string]string
		installed string
		out       string
		code      int
		runErr    error
		want      SecurityResult
		wantCall  string
	}{
		{
			name: "npm vulnerable exits 1 with valid JSON", files: map[string]string{"package.json": "{}", "package-lock.json": "{}"},
			installed: "npm", out: "npm-v2-vulnerable.json", code: 1,
			want:     SecurityResult{Ecosystem: "node", Tool: "npm audit", Status: AuditVulnerable, Vulnerabilities: 2, HighSeverity: 1, MediumSeverity: 1, Summary: "2 vulnerabilities (1 high, 1 moderate)"},
			wantCall: "npm audit --json",
		},
		{
			name: "npm clean", files: map[string]string{"package.json": "{}", "package-lock.json": "{}"},
			installed: "npm", out: "npm-v2-clean.json", code: 0,
			want: SecurityResult{Ecosystem: "node", Tool: "npm audit", Status: AuditOK, Summary: "no known vulnerabilities"},
		},
		{
			name: "npm error JSON is unavailable, not OK", files: map[string]string{"package.json": "{}"},
			installed: "npm", out: "npm-error-enolock.json", code: 1,
			want: SecurityResult{Ecosystem: "node", Tool: "npm audit", Status: AuditUnavailable, Summary: "unavailable: audit failed: ENOLOCK This command requires an existing lockfile."},
		},
		{
			name: "npm prints nothing", files: map[string]string{"package.json": "{}"},
			installed: "npm", out: "", code: 1,
			want: SecurityResult{Ecosystem: "node", Tool: "npm audit", Status: AuditUnavailable, Summary: "unavailable: could not parse audit output: unexpected end of JSON input"},
		},
		{
			name: "npm valid JSON but crash exit code", files: map[string]string{"package.json": "{}"},
			installed: "npm", out: "npm-v2-clean.json", code: 134,
			want: SecurityResult{Ecosystem: "node", Tool: "npm audit", Status: AuditUnavailable, Summary: "unavailable: npm audit exited with status 134"},
		},
		{
			name: "npm cannot start", files: map[string]string{"package.json": "{}"},
			installed: "npm", runErr: errors.New("fork/exec npm: permission denied"),
			want: SecurityResult{Ecosystem: "node", Tool: "npm audit", Status: AuditUnavailable, Summary: "unavailable: fork/exec npm: permission denied"},
		},
		{
			name: "npm not installed", files: map[string]string{"package.json": "{}"},
			want: SecurityResult{Ecosystem: "node", Tool: "npm audit", Status: AuditNotInstalled, Summary: "npm not installed"},
		},
		{
			name: "pnpm", files: map[string]string{"package.json": "{}", "pnpm-lock.yaml": "lockfileVersion: '9.0'\n"},
			installed: "pnpm", out: "pnpm-vulnerable.json", code: 1,
			want:     SecurityResult{Ecosystem: "node", Tool: "pnpm audit", Status: AuditVulnerable, Vulnerabilities: 1, HighSeverity: 1, Summary: "1 vulnerabilities (1 high)"},
			wantCall: "pnpm audit --json",
		},
		{
			name: "yarn v1 bitmask exit", files: map[string]string{"package.json": "{}", "yarn.lock": "# yarn lockfile v1\n"},
			installed: "yarn", out: "yarn-v1-vulnerable.ndjson", code: 18,
			want: SecurityResult{Ecosystem: "node", Tool: "yarn audit", Status: AuditVulnerable, Vulnerabilities: 2, HighSeverity: 1, LowSeverity: 1, Summary: "2 vulnerabilities (1 high, 1 low)"},
		},
		{
			name: "yarn without summary", files: map[string]string{"package.json": "{}", "yarn.lock": "# yarn lockfile v1\n"},
			installed: "yarn", out: "{\"type\":\"error\",\"data\":\"Unexpected token\"}\n", code: 1,
			want: SecurityResult{Ecosystem: "node", Tool: "yarn audit", Status: AuditUnavailable, Summary: "unavailable: yarn audit printed no auditSummary"},
		},
		{
			name: "yarn berry is not run", files: map[string]string{"package.json": "{}", "yarn.lock": "__metadata:\n  version: 8\n"},
			installed: "yarn",
			want:      SecurityResult{Ecosystem: "node", Tool: "yarn audit", Status: AuditUnavailable, Summary: "unavailable: yarn 2+ projects are audited with `yarn npm audit`, whose output xpm does not read"},
		},
		{
			name: "bun", files: map[string]string{"package.json": "{}", "bun.lock": "{}"},
			installed: "bun", out: "bun-vulnerable.json", code: 1,
			want: SecurityResult{Ecosystem: "node", Tool: "bun audit", Status: AuditVulnerable, Vulnerabilities: 1, HighSeverity: 1, Summary: "1 vulnerabilities (1 high)"},
		},
		{
			name: "pip-audit object format", files: map[string]string{"requirements.txt": "requests==2.25.0\n"},
			installed: "pip-audit", out: "pip-audit-object.json", code: 1,
			want:     SecurityResult{Ecosystem: "python", Tool: "pip-audit", Status: AuditVulnerable, Vulnerabilities: 1, LowSeverity: 1, Summary: "1 vulnerabilities (1 low) (1 not audited)"},
			wantCall: "pip-audit -r requirements.txt -f json",
		},
		{
			name: "pip-audit that skipped every dependency is unavailable, not OK", files: map[string]string{"requirements.txt": "-e .\n"},
			installed: "pip-audit", out: "pip-audit-all-skipped.json", code: 0,
			want: SecurityResult{Ecosystem: "python", Tool: "pip-audit", Status: AuditUnavailable, Summary: "unavailable: pip-audit audited no dependencies (2 skipped)"},
		},
		{
			name: "pip-audit with no dependencies is unavailable", files: map[string]string{"requirements.txt": "\n"},
			installed: "pip-audit", out: `{"dependencies": [], "fixes": []}`, code: 0,
			want: SecurityResult{Ecosystem: "python", Tool: "pip-audit", Status: AuditUnavailable, Summary: "unavailable: pip-audit audited no dependencies"},
		},
		{
			name: "pip-audit clean but partly skipped says so", files: map[string]string{"requirements.txt": "certifi\n"},
			installed: "pip-audit", out: "pip-audit-clean-partly-skipped.json", code: 0,
			want: SecurityResult{Ecosystem: "python", Tool: "pip-audit", Status: AuditOK, Summary: "no known vulnerabilities (1 not audited)"},
		},
		{
			name: "bun bare object is clean", files: map[string]string{"package.json": "{}", "bun.lock": "{}"},
			installed: "bun", out: "{}", code: 0,
			want: SecurityResult{Ecosystem: "node", Tool: "bun audit", Status: AuditOK, Summary: "no known vulnerabilities"},
		},
		{
			name: "pip-audit legacy array", files: map[string]string{"pyproject.toml": "[project]\nname = \"x\"\n"},
			installed: "pip-audit", out: "pip-audit-legacy.json", code: 1,
			want:     SecurityResult{Ecosystem: "python", Tool: "pip-audit", Status: AuditVulnerable, Vulnerabilities: 2, LowSeverity: 2, Summary: "2 vulnerabilities (2 low) (audited the active Python environment)"},
			wantCall: "pip-audit -f json",
		},
		{
			name: "pip-audit failure", files: map[string]string{"requirements.txt": "x\n"},
			installed: "pip-audit", out: "", code: 1,
			want: SecurityResult{Ecosystem: "python", Tool: "pip-audit", Status: AuditUnavailable, Summary: "unavailable: could not parse pip-audit output: unexpected end of JSON input"},
		},
		{
			name: "composer vulnerable", files: map[string]string{"composer.json": "{}", "composer.lock": "{}"},
			installed: "composer", out: "composer-vulnerable.json", code: 1,
			want: SecurityResult{Ecosystem: "php", Tool: "composer audit", Status: AuditVulnerable, Vulnerabilities: 2, HighSeverity: 1, MediumSeverity: 1, Summary: "2 vulnerabilities (1 high, 1 moderate)"},
		},
		{
			name: "composer clean uses an empty array", files: map[string]string{"composer.json": "{}", "composer.lock": "{}"},
			installed: "composer", out: "composer-clean.json", code: 0,
			want: SecurityResult{Ecosystem: "php", Tool: "composer audit", Status: AuditOK, Summary: "no known vulnerabilities"},
		},
		{
			name: "cargo audit", files: map[string]string{"Cargo.toml": "[package]\nname = \"x\"\n"},
			installed: "cargo-audit", out: "cargo-audit-vulnerable.json", code: 1,
			want:     SecurityResult{Ecosystem: "rust", Tool: "cargo audit", Status: AuditVulnerable, Vulnerabilities: 1, LowSeverity: 1, Summary: "1 vulnerabilities (1 low)"},
			wantCall: "cargo audit --json",
		},
		{
			name: "cargo-audit missing", files: map[string]string{"Cargo.toml": "[package]\nname = \"x\"\n"},
			installed: "cargo",
			want:      SecurityResult{Ecosystem: "rust", Tool: "cargo audit", Status: AuditNotInstalled, Summary: "cargo-audit not installed"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeAudit{installed: map[string]bool{c.installed: true}, out: c.out, code: c.code, err: c.runErr}
			withFakeAudit(t, f)
			dir, scan := projectWith(t, c.files)
			got := RunSecurityAudit(dir, scan)
			if len(got) != 1 {
				t.Fatalf("results = %+v, want exactly one", got)
			}
			if !reflect.DeepEqual(got[0], c.want) {
				t.Errorf("result =\n  %+v\nwant\n  %+v", got[0], c.want)
			}
			if c.wantCall != "" {
				want := filepath.Base(dir) + ": " + c.wantCall
				if len(f.calls) != 1 || f.calls[0] != want {
					t.Errorf("calls = %q, want [%q] (run in the project dir)", f.calls, want)
				}
			}
			if c.want.Status == AuditNotInstalled && len(f.calls) != 0 {
				t.Errorf("a missing tool was run: %q", f.calls)
			}
		})
	}
}

func TestAuditParsersNeverPanicOnMalformedOutput(t *testing.T) {
	parsers := map[string]func([]byte) (severities, error){
		"npm": parseNpmAudit, "yarn": parseYarnAudit, "bun": parseBunAudit,
		"pip": parsePipAudit, "composer": parseComposerAudit, "cargo": parseCargoAudit,
	}
	inputs := []string{"", "null", "[]", "{}", "{", "[1,2]", `{"metadata": 3}`, `{"advisories": 7}`, `{"advisories": [[{"severity": 1}]]}`, `{"vulnerabilities": []}`, "\x00\xff", `{"dependencies": null}`}
	for name, parse := range parsers {
		for _, in := range inputs {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s parser panicked on %q: %v", name, in, r)
					}
				}()
				s, err := parse([]byte(in))
				if err == nil && s.total != 0 {
					t.Errorf("%s parser invented %d vulnerabilities from %q", name, s.total, in)
				}
			}()
		}
	}
}

func TestUnavailableAuditIsAWarningNeverOK(t *testing.T) {
	results := []SecurityResult{{Status: AuditUnavailable}, {Status: AuditNotInstalled}, {Status: AuditOK}}
	ok, vulnerable, unavailable := CountSecurityIssues(results)
	if ok != 1 || vulnerable != 0 || unavailable != 2 {
		t.Fatalf("CountSecurityIssues = %d %d %d, want 1 0 2", ok, vulnerable, unavailable)
	}
	if HasSecurityIssues(results) {
		t.Fatal("HasSecurityIssues = true for audits that did not run")
	}
}
