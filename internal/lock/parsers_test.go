package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountPackagesFixtures(t *testing.T) {
	cases := []struct {
		fixture string // under testdata/counts
		file    string // lockfile name that selects the counter
		want    int
	}{
		{"package-lock.json", "package-lock.json", 3}, // root "" excluded, nested node_modules counted
		{"yarn.lock", "yarn.lock", 3},                 // a multi-range header counts once
		{"yarn-berry.lock", "yarn.lock", 2},           // __metadata is not a package
		{"pnpm-v6.yaml", "pnpm-lock.yaml", 2},
		{"pnpm-v9.yaml", "pnpm-lock.yaml", 2}, // v9 keys have no leading "/"; snapshots ignored
		{"bun.lock", "bun.lock", 3},           // JSONC: trailing commas and // comments
		{"uv.lock", "uv.lock", 2},             // the editable project itself is not counted
		{"poetry.lock", "poetry.lock", 2},
		{"Cargo.lock", "Cargo.lock", 2},
		{"composer.lock", "composer.lock", 3},
		{"Pipfile.lock", "Pipfile.lock", 3},
		{"go.sum", "go.sum", 3}, // h1 + /go.mod lines of one version count once
		{"gradle.lockfile", "gradle.lockfile", 3},
		{"requirements.lock", "requirements.lock", 2}, // -e and --hash lines skipped
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "counts", c.fixture))
			if err != nil {
				t.Fatal(err)
			}
			got, err := countPackages(c.file, data)
			if err != nil {
				t.Fatalf("countPackages(%s): %v", c.file, err)
			}
			if got != c.want {
				t.Errorf("countPackages(%s) = %d, want %d", c.file, got, c.want)
			}
		})
	}
}

func TestCountPackagesMalformedNeverPanics(t *testing.T) {
	inputs := []string{
		"",
		"{",
		"\x00\xff\xfe",
		`{"packages": [1, 2]}`,
		`{"packages": {"a": 1,}`,
		"\"unterminated",
		"/* unterminated comment",
		"{\"a\": \"\\",
		"[[package]]\nname = ",
		"packages:\n  - [",
		"packages: 7\n",
		"lockfileVersion: '9.0'\npackages:\n  a: [\n",
	}
	for _, spec := range SupportedLockfiles {
		for _, in := range inputs {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("countPackages(%s, %q) panicked: %v", spec.File, in, r)
					}
				}()
				n, _ := countPackages(spec.File, []byte(in))
				if n < 0 {
					t.Errorf("countPackages(%s, %q) = %d, want >= 0", spec.File, in, n)
				}
			}()
		}
	}
}

func TestCountPackagesMalformedStructuredFormatsReportError(t *testing.T) {
	for _, file := range []string{"package-lock.json", "pnpm-lock.yaml", "bun.lock", "composer.lock", "poetry.lock", "uv.lock", "Pipfile.lock", "Cargo.lock"} {
		n, err := countPackages(file, []byte("{[ not valid in any format"))
		if err == nil {
			t.Errorf("countPackages(%s, garbage) error = nil, want parse error", file)
		}
		if n != 0 {
			t.Errorf("countPackages(%s, garbage) = %d, want 0", file, n)
		}
	}
}

func TestCountBunLockbIsReportedNotEstimated(t *testing.T) {
	n, err := countPackages("bun.lockb", make([]byte, 5000))
	if n != 0 || err == nil {
		t.Fatalf("countPackages(bun.lockb) = %d, %v; want 0 and an explanation", n, err)
	}
}

func TestStripJSONCKeepsStrings(t *testing.T) {
	in := `{"url": "https://x/y", "a": "b,}", /* c */ "d": [1,2,],}`
	want := `{"url": "https://x/y", "a": "b,}",  "d": [1,2]}`
	if got := string(stripJSONC([]byte(in))); got != want {
		t.Fatalf("stripJSONC = %q, want %q", got, want)
	}
}
