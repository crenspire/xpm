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
// after "+" is ignored and identifiers are lowercased.
//
// For java and python a trailing non-numeric field is a qualifier, ranked
// dev < alpha(a) < beta(b) < milestone(m) < rc(cr, c, pre, preview) <
// snapshot < release (final, ga, empty) < sp/post, with numbers compared
// numerically (Beta1, M2); so 5.3.20.RELEASE == 5.3.20 and
// 6.0.0.Beta1 < 6.0.0. Any other dotted qualifier sorts above the release.
//
// An empty version sorts lowest; callers should filter with Pinned first.
func Compare(ecosystem, a, b string) int {
	if a == "" || b == "" {
		if a == b {
			return 0
		}
		if a == "" {
			return -1
		}
		return 1
	}
	if ecosystem == "go" {
		return semver.Compare(goVersion(a), goVersion(b))
	}
	va, vb := parseVersion(ecosystem, a), parseVersion(ecosystem, b)
	if c := compareRelease(va.release, vb.release); c != 0 {
		return c
	}
	if va.rank != vb.rank {
		return cmpInt(va.rank, vb.rank)
	}
	return comparePre(va.ids, vb.ids)
}

// Qualifier ranks, low to high. rankPre is an unrecognised pre-release
// suffix (compared lexically); rankOther an unrecognised dotted qualifier,
// which sorts above the plain release.
const (
	rankPre = iota
	rankDev
	rankAlpha
	rankBeta
	rankMilestone
	rankRC
	rankSnapshot
	rankFinal
	rankPost
	rankOther
)

// qualifierRanks maps the Maven/OSGi and PEP 440 qualifier words (java and
// python only) to their rank. The empty word and final/release/ga are the
// plain release.
var qualifierRanks = map[string]int{
	"dev":   rankDev,
	"alpha": rankAlpha, "a": rankAlpha,
	"beta": rankBeta, "b": rankBeta,
	"milestone": rankMilestone, "m": rankMilestone,
	"rc": rankRC, "cr": rankRC, "c": rankRC, "pre": rankRC, "preview": rankRC,
	"snapshot": rankSnapshot,
	"":         rankFinal, "final": rankFinal, "release": rankFinal, "ga": rankFinal,
	"sp": rankPost, "post": rankPost,
}

type version struct {
	release []string
	rank    int
	ids     []string // identifiers after the qualifier word, compared per comparePre
}

func goVersion(v string) string {
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// pepMarker finds a PEP 440 pre/post-release marker directly after the
// numeric release fields.
var pepMarker = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?((?:alpha|beta|preview|pre|post|dev|rc|a|b|c)(?:\d.*)?)$`)

// parseVersion drops the leading "v" and build metadata, lowercases and
// separates the release fields from the qualifier or pre-release suffix.
func parseVersion(ecosystem, v string) version {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	release, pre := v, ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		release, pre = v[:i], v[i+1:]
	} else if m := pepMarker.FindStringSubmatch(v); m != nil && ecosystem == "python" {
		release, pre = m[1], m[2]
	}
	fields := strings.Split(release, ".")
	if ecosystem != "java" && ecosystem != "python" {
		return version{release: fields, rank: preRank(pre), ids: preIdents(pre)}
	}
	// A trailing non-numeric field is a qualifier (5.3.20.RELEASE, 6.0.0.Beta1).
	qual, dotQual := pre, false
	for k, f := range fields {
		if _, err := strconv.ParseUint(f, 10, 64); err != nil {
			qual = strings.Join(fields[k:], ".")
			if pre != "" {
				qual += "." + pre
			}
			fields, dotQual = fields[:k], true
			break
		}
	}
	ids := preIdents(qual)
	word := ""
	if len(ids) > 0 {
		word = ids[0]
	}
	if r, ok := qualifierRanks[word]; ok {
		if r == rankFinal {
			ids = nil
		} else if len(ids) > 0 {
			ids = ids[1:]
		}
		return version{release: fields, rank: r, ids: ids}
	}
	if dotQual {
		return version{release: fields, rank: rankOther, ids: ids}
	}
	if !dotQual && word != "" && strings.Trim(word, "0123456789") == "" {
		// A bare numeric suffix (1.0-1) is a post-release.
		return version{release: fields, rank: rankPost, ids: ids}
	}
	return version{release: fields, rank: preRank(pre), ids: ids}
}

// preRank is rankFinal for no suffix and rankPre for any pre-release.
func preRank(pre string) int {
	if pre == "" {
		return rankFinal
	}
	return rankPre
}

func compareRelease(fa, fb []string) int {
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

func cmpInt[T int | uint64](x, y T) int {
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
func comparePre(ia, ib []string) int {
	for i := 0; i < len(ia) && i < len(ib); i++ {
		if c := compareIdent(ia[i], ib[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(ia), len(ib))
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
