package cli

import (
	"runtime/debug"
	"testing"
)

// withVersion sets Version and the build-info seam for one test.
func withVersion(t *testing.T, v string, info *debug.BuildInfo, ok bool) {
	t.Helper()
	oldV, oldRead := Version, readBuildInfo
	t.Cleanup(func() { Version, readBuildInfo = oldV, oldRead })
	Version = v
	readBuildInfo = func() (*debug.BuildInfo, bool) { return info, ok }
}

func TestVersionStringPrefersLdflags(t *testing.T) {
	withVersion(t, "1.4.0", &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, true)
	if got := versionString(); got != "1.4.0" {
		t.Errorf("versionString() = %q, want 1.4.0 (ldflags wins)", got)
	}
}

func TestVersionStringFromBuildInfo(t *testing.T) {
	cases := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{"module version", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, true, "0.1.0"},
		{"pseudo version", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.1-0.20261008120000-abcdef123456"}}, true, "0.1.1-0.20261008120000-abcdef123456"},
		{"devel", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true, defaultVersion},
		{"empty", &debug.BuildInfo{Main: debug.Module{Version: ""}}, true, defaultVersion},
		{"no build info", nil, false, defaultVersion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withVersion(t, defaultVersion, tc.info, tc.ok)
			if got := versionString(); got != tc.want {
				t.Errorf("versionString() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVersionStringStripsLeadingV(t *testing.T) {
	withVersion(t, "v0.1.0", nil, false)
	if got := versionString(); got != "0.1.0" {
		t.Errorf("versionString() = %q, want 0.1.0", got)
	}
}
