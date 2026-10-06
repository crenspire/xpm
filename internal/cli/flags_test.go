package cli

import (
	"reflect"
	"testing"
)

func TestSplitGlobalFlags(t *testing.T) {
	tests := []struct {
		name        string
		raw         []string
		wantArgs    []string
		wantVerbose bool
		wantVersion bool
	}{
		{"no args", []string{}, nil, false, false},
		{"long version", []string{"--version"}, nil, false, true},
		{"short V", []string{"-V"}, nil, false, true},
		{"lone -v is version", []string{"-v"}, nil, false, true},
		{"-v before command is verbose", []string{"-v", "install", "axios"}, []string{"install", "axios"}, true, false},
		{"--verbose before command", []string{"--verbose", "which", "x"}, []string{"which", "x"}, true, false},
		{"script args keep --version", []string{"run", "test", "--", "--version"}, []string{"run", "test", "--", "--version"}, false, false},
		{"script args keep -v", []string{"run", "build", "--", "-v"}, []string{"run", "build", "--", "-v"}, false, false},
		{"flag after command is not global", []string{"graph", "-V"}, []string{"graph", "-V"}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args, verbose, version := splitGlobalFlags(tc.raw)
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tc.wantArgs)
			}
			if verbose != tc.wantVerbose {
				t.Errorf("verbose = %v, want %v", verbose, tc.wantVerbose)
			}
			if version != tc.wantVersion {
				t.Errorf("showVersion = %v, want %v", version, tc.wantVersion)
			}
		})
	}
}
