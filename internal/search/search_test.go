package search

import (
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
)

// TestEnabled verifies the Enabled function correctly checks option flags.
func TestEnabled(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		id   pm.ID
		want bool
	}{
		{
			name: "nil enable map returns true",
			opts: Options{Enable: nil},
			id:   pm.Npm,
			want: true,
		},
		{
			name: "missing key returns true",
			opts: Options{Enable: map[pm.ID]bool{pm.Pip: false}},
			id:   pm.Npm,
			want: true,
		},
		{
			name: "explicit true returns true",
			opts: Options{Enable: map[pm.ID]bool{pm.Npm: true}},
			id:   pm.Npm,
			want: true,
		},
		{
			name: "explicit false returns false",
			opts: Options{Enable: map[pm.ID]bool{pm.Npm: false}},
			id:   pm.Npm,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Enabled(tc.opts, tc.id)
			if got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSearchEverywhereOptions tests that SearchEverywhere respects options.
func TestSearchEverywhereOptions(t *testing.T) {
	// Test with all ecosystems disabled
	opts := Options{
		Enable: map[pm.ID]bool{
			pm.Npm:      false,
			pm.Pip:      false,
			pm.Composer: false,
			pm.Cargo:    false,
			pm.Maven:    false,
		},
	}

	results, err := SearchEverywhere("test", opts)
	if err != nil {
		t.Errorf("SearchEverywhere() error = %v", err)
	}

	// Should return empty since all are disabled (and no network calls)
	if len(results) != 0 {
		t.Errorf("SearchEverywhere() with all disabled returned %d results, want 0", len(results))
	}
}

// TestResultFields verifies Result struct field access.
func TestResultFields(t *testing.T) {
	result := Result{
		Manager: pm.Npm,
		Name:    "lodash",
		Info:    "A modern JavaScript utility library",
		Extra: map[string]string{
			"version": "4.17.21",
		},
	}

	if result.Manager != pm.Npm {
		t.Errorf("Result.Manager = %v, want %v", result.Manager, pm.Npm)
	}
	if result.Name != "lodash" {
		t.Errorf("Result.Name = %v, want lodash", result.Name)
	}
	if result.Info != "A modern JavaScript utility library" {
		t.Errorf("Result.Info = %v, want description", result.Info)
	}
	if result.Extra["version"] != "4.17.21" {
		t.Errorf("Result.Extra[version] = %v, want 4.17.21", result.Extra["version"])
	}
}

// TestDefaultTimeout verifies the default timeout constant.
func TestDefaultTimeout(t *testing.T) {
	if DefaultTimeout.Seconds() != 4 {
		t.Errorf("DefaultTimeout = %v, want 4s", DefaultTimeout)
	}
}

// TestURLInjectionPrevention tests that URL injection attacks are prevented.
func TestURLInjectionPrevention(t *testing.T) {
	tests := []struct {
		name     string
		pkg      string
		wantErr  bool
		contains string
	}{
		{
			name:     "path traversal attempt",
			pkg:      "../../etc/passwd",
			wantErr:  true,
			contains: "invalid package name",
		},
		{
			name:     "query injection attempt",
			pkg:      "package?evil=param",
			wantErr:  true,
			contains: "invalid package name",
		},
		{
			name:     "fragment injection",
			pkg:      "package#fragment",
			wantErr:  true,
			contains: "invalid package name",
		},
		{
			name:     "very long package name",
			pkg:      string(make([]byte, 300)), // 300 chars
			wantErr:  true,
			contains: "too long",
		},
		{
			name:    "valid package name",
			pkg:     "lodash",
			wantErr: false,
		},
		{
			name:    "npm scoped name",
			pkg:     "@types/node",
			wantErr: false,
		},
		{
			name:    "composer vendor/name",
			pkg:     "monolog/monolog",
			wantErr: false,
		},
		{
			name:     "embedded whitespace",
			pkg:      "foo bar",
			wantErr:  true,
			contains: "invalid package name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePackageNameForURL(tc.pkg)
			if tc.wantErr {
				if err == nil {
					t.Errorf("validatePackageNameForURL() expected error for %q, got nil", tc.pkg)
				} else if tc.contains != "" && !strings.Contains(err.Error(), tc.contains) {
					t.Errorf("validatePackageNameForURL() error = %v, want error containing %q", err, tc.contains)
				}
			} else {
				if err != nil {
					t.Errorf("validatePackageNameForURL() unexpected error for %q: %v", tc.pkg, err)
				}
			}
		})
	}
}
