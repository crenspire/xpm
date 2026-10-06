package cli

import (
	"testing"

	"github.com/crenspire/xpm/internal/pm"
)

// TestParsePackageVersionEdgeCases tests edge cases in parsePackageVersion.
func TestParsePackageVersionEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion string
	}{
		{
			name:        "empty string",
			input:       "",
			wantName:    "",
			wantVersion: "",
		},
		{
			name:        "just @",
			input:       "@",
			wantName:    "",
			wantVersion: "",
		},
		{
			name:        "package with @ at end",
			input:       "axios@",
			wantName:    "axios",
			wantVersion: "",
		},
		{
			name:        "scoped package",
			input:       "@scope/package@1.0.0",
			wantName:    "@scope/package",
			wantVersion: "1.0.0",
		},
		{
			name:        "scoped package without version",
			input:       "@scope/package",
			wantName:    "@scope/package",
			wantVersion: "",
		},
		{
			name:        "regular package with version",
			input:       "axios@1.0.0",
			wantName:    "axios",
			wantVersion: "1.0.0",
		},
		{
			name:        "package without version",
			input:       "axios",
			wantName:    "axios",
			wantVersion: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotVersion := parsePackageVersion(tc.input)
			if gotName != tc.wantName {
				t.Errorf("parsePackageVersion(%q) name = %q, want %q", tc.input, gotName, tc.wantName)
			}
			if gotVersion != tc.wantVersion {
				t.Errorf("parsePackageVersion(%q) version = %q, want %q", tc.input, gotVersion, tc.wantVersion)
			}
		})
	}
}

// TestCommandInjectionPrevention tests that command injection is prevented.
func TestCommandInjectionPrevention(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		manager pm.ID
		wantErr bool
	}{
		{
			name:    "package with semicolon",
			pkg:     "package; rm -rf /",
			manager: pm.Npm,
			wantErr: true,
		},
		{
			name:    "package with pipe",
			pkg:     "package | cat /etc/passwd",
			manager: pm.Npm,
			wantErr: true,
		},
		{
			name:    "package with backtick",
			pkg:     "package`rm -rf /`",
			manager: pm.Npm,
			wantErr: true,
		},
		{
			name:    "valid package name",
			pkg:     "lodash",
			manager: pm.Npm,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := pm.ValidatePackageName(tc.pkg, tc.manager)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ValidatePackageName(%q, %v) expected error, got nil", tc.pkg, tc.manager)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePackageName(%q, %v) unexpected error: %v", tc.pkg, tc.manager, err)
				}
			}
		})
	}
}
