package env

import (
	"sort"
	"strconv"
	"strings"
)

// Version is a parsed runtime version such as "1.22.3", "1.22rc1",
// "21.0.12.1+1" or "20.11.0-beta.1".
type Version struct {
	Nums  []int    // numeric release components, any count
	Pre   []string // prerelease identifiers; nil for a release
	Build []string // build metadata identifiers ("+1")
}

// IsPrerelease reports whether v carries prerelease identifiers.
func (v Version) IsPrerelease() bool { return len(v.Pre) > 0 }

// ParseVersion parses s tolerantly. A leading "v" is ignored. The numeric
// part is one or more dot-separated integers; a prerelease follows either
// "-" or directly a letter ("1.22rc1"); build metadata follows "+".
func ParseVersion(s string) (Version, bool) {
	var v Version
	s = strings.TrimPrefix(s, "v")
	i := 0
	for {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i || j-i > 9 {
			return Version{}, false
		}
		n, err := strconv.Atoi(s[i:j])
		if err != nil {
			return Version{}, false
		}
		v.Nums = append(v.Nums, n)
		i = j
		if i+1 < len(s) && s[i] == '.' && s[i+1] >= '0' && s[i+1] <= '9' {
			i++
			continue
		}
		break
	}
	rest := s[i:]
	pre, build, hasBuild := strings.Cut(rest, "+")
	if hasBuild {
		ids, ok := splitIdentifiers(build)
		if !ok {
			return Version{}, false
		}
		v.Build = ids
	}
	if pre != "" {
		if pre[0] == '-' {
			pre = pre[1:]
		} else if !isLetter(pre[0]) {
			return Version{}, false
		}
		ids, ok := splitIdentifiers(pre)
		if !ok {
			return Version{}, false
		}
		v.Pre = ids
	}
	return v, true
}

// splitIdentifiers splits "rc.10", "beta-1" or "rc1" into identifiers,
// breaking on '.', '-' and on letter/digit boundaries.
func splitIdentifiers(s string) ([]string, bool) {
	if s == "" {
		return nil, false
	}
	var ids []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '-' }) {
		start := 0
		for k := 1; k <= len(part); k++ {
			if k == len(part) || isDigit(part[k]) != isDigit(part[k-1]) {
				ids = append(ids, part[start:k])
				start = k
			}
		}
	}
	if len(ids) == 0 {
		return nil, false
	}
	for _, id := range ids {
		for k := 0; k < len(id); k++ {
			if !isDigit(id[k]) && !isLetter(id[k]) {
				return nil, false
			}
		}
	}
	return ids, true
}

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// compareIdentifiers compares identifier lists: numeric ones numerically,
// numeric < alphanumeric, otherwise lexically; a shorter equal prefix is lower.
func compareIdentifiers(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareIdentifier(a[i], b[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(a), len(b))
}

func compareIdentifier(a, b string) int {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	switch {
	case aerr == nil && berr == nil:
		return compareInt(an, bn)
	case aerr == nil:
		return -1
	case berr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Compare orders versions: numeric components (missing ones count as 0),
// then release > prerelease, then prerelease identifiers, then the number of
// components, then build metadata.
func (v Version) Compare(o Version) int {
	for i := 0; i < len(v.Nums) || i < len(o.Nums); i++ {
		var a, b int
		if i < len(v.Nums) {
			a = v.Nums[i]
		}
		if i < len(o.Nums) {
			b = o.Nums[i]
		}
		if c := compareInt(a, b); c != 0 {
			return c
		}
	}
	switch {
	case v.IsPrerelease() && !o.IsPrerelease():
		return -1
	case !v.IsPrerelease() && o.IsPrerelease():
		return 1
	}
	if c := compareIdentifiers(v.Pre, o.Pre); c != 0 {
		return c
	}
	if c := compareInt(len(v.Nums), len(o.Nums)); c != 0 {
		return c
	}
	return compareIdentifiers(v.Build, o.Build)
}

// CompareVersions compares two version strings. Unparseable strings sort
// below every parseable one and lexically among themselves.
func CompareVersions(a, b string) int {
	va, aok := ParseVersion(a)
	vb, bok := ParseVersion(b)
	switch {
	case aok && bok:
		return va.Compare(vb)
	case aok:
		return 1
	case bok:
		return -1
	}
	return strings.Compare(a, b)
}

// SortVersionsDesc sorts versions newest first, in place.
func SortVersionsDesc(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) > 0 })
}

// MatchesSpec reports whether version v satisfies spec. "latest" (and "")
// match everything. Otherwise spec's numeric components must be an exact
// prefix of v's ("20" matches 20.11.0 but not 200.1.0; "1.2" does not
// match 1.20.3). A spec with a prerelease or build part must match exactly.
func MatchesSpec(spec, v string) bool {
	if spec == "" || spec == "latest" {
		_, ok := ParseVersion(v)
		return ok
	}
	sv, ok := ParseVersion(spec)
	if !ok {
		return false
	}
	vv, ok := ParseVersion(v)
	if !ok || len(sv.Nums) > len(vv.Nums) {
		return false
	}
	for i, n := range sv.Nums {
		if vv.Nums[i] != n {
			return false
		}
	}
	if sv.IsPrerelease() || len(sv.Build) > 0 {
		return len(sv.Nums) == len(vv.Nums) &&
			compareIdentifiers(sv.Pre, vv.Pre) == 0 &&
			(len(sv.Build) == 0 || compareIdentifiers(sv.Build, vv.Build) == 0)
	}
	return true
}

// HighestMatch returns the highest version satisfying spec. Prereleases are
// considered only when includePrerelease is true.
func HighestMatch(spec string, versions []string, includePrerelease bool) (string, bool) {
	best := ""
	var bestV Version
	for _, s := range versions {
		if !MatchesSpec(spec, s) {
			continue
		}
		v, _ := ParseVersion(s)
		if v.IsPrerelease() && !includePrerelease {
			continue
		}
		if best == "" || v.Compare(bestV) > 0 {
			best, bestV = s, v
		}
	}
	return best, best != ""
}
