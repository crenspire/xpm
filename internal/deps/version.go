package deps

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// Compare compares two versions of a package in ecosystem: -1, 0 or +1.
// Go versions use golang.org/x/mod/semver (a missing "v" is added).
// Everything else uses a loose semver-like order: an optional leading "v"
// is dropped, the release part is split on "." into numeric fields compared
// numerically (missing fields are 0, so 1.2 == 1.2.0; a non-numeric field
// compares as a string after all numeric ones), and a version with a
// pre-release suffix (after the first "-", or a PEP 440 "a"/"b"/"rc"/".dev"
// marker directly after a numeric field) sorts before the same release
// without it. Pre-release identifiers compare dot-separated: numeric
// identifiers numerically (rc.2 < rc.10), others lexically. Build metadata
// after "+" is ignored.
func Compare(ecosystem, a, b string) int {
	if ecosystem == "go" {
		return semver.Compare(goVersion(a), goVersion(b))
	}
	ra, pa := splitVersion(a)
	rb, pb := splitVersion(b)
	if c := compareRelease(ra, rb); c != 0 {
		return c
	}
	switch {
	case pa == "" && pb == "":
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	}
	return comparePre(pa, pb)
}

func goVersion(v string) string {
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// pepMarker finds a PEP 440 pre-release marker (a, b, rc, dev) directly
// after the numeric release fields.
var pepMarker = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?((?:a|b|rc|dev)(?:\d.*)?)$`)

// splitVersion drops the leading "v" and build metadata and separates the
// release part from the pre-release suffix.
func splitVersion(v string) (release, pre string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	if m := pepMarker.FindStringSubmatch(v); m != nil {
		return m[1], m[2]
	}
	return v, ""
}

func compareRelease(a, b string) int {
	fa, fb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(fa) || i < len(fb); i++ {
		x, y := "0", "0"
		if i < len(fa) {
			x = fa[i]
		}
		if i < len(fb) {
			y = fb[i]
		}
		if c := compareIdent(x, y); c != 0 {
			return c
		}
	}
	return 0
}

// compareIdent compares two identifiers: numeric ones numerically and
// before non-numeric ones, which compare as strings.
func compareIdent(x, y string) int {
	nx, errX := strconv.ParseUint(x, 10, 64)
	ny, errY := strconv.ParseUint(y, 10, 64)
	switch {
	case errX == nil && errY == nil:
		return cmpInt(nx, ny)
	case errX == nil:
		return -1
	case errY == nil:
		return 1
	}
	return strings.Compare(x, y)
}

func cmpInt(x, y uint64) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// comparePre compares pre-release suffixes identifier by identifier; with
// all shared identifiers equal, the one with more identifiers is greater.
func comparePre(a, b string) int {
	ia, ib := preIdents(a), preIdents(b)
	for i := 0; i < len(ia) && i < len(ib); i++ {
		if c := compareIdent(ia[i], ib[i]); c != 0 {
			return c
		}
	}
	return cmpInt(uint64(len(ia)), uint64(len(ib)))
}

// preIdents splits a pre-release suffix on "." and at letter/digit
// boundaries, so that PEP 440 "rc2" and "rc10" order as rc.2 < rc.10.
func preIdents(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ".") {
		start := 0
		for i := 1; i <= len(part); i++ {
			if i == len(part) || isDigit(part[i]) != isDigit(part[i-1]) {
				out = append(out, part[start:i])
				start = i
			}
		}
	}
	return out
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
