package search

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// cratesNamePattern is what crates.io accepts as a crate name. Anything else
// (e.g. "@types/node") cannot be a crate, so it is "not found" with no request.
var cratesNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// cratesDescriptionTimeout caps the best-effort API call for the
// description (the API's tail is long; 1.2 s is common). The index answers
// existence and version; an API stall must never turn a found crate into a
// timeout, so the call also stops descriptionMargin before the lookup's own
// deadline.
var cratesDescriptionTimeout = 2 * time.Second

const descriptionMargin = 200 * time.Millisecond

// maxIndexBytes caps an index file. The biggest real ones are a few MB; the
// file is streamed line by line, never held in memory.
const maxIndexBytes = 16 << 20

// cratesIndexPath returns the sparse-index path of a crate (lowercased):
// 1 char -> 1/a, 2 -> 2/ab, 3 -> 3/a/abc, else ab/cd/abcd...
func cratesIndexPath(name string) string {
	n := strings.ToLower(name)
	switch len(n) {
	case 1:
		return "1/" + n
	case 2:
		return "2/" + n
	case 3:
		return "3/" + n[:1] + "/" + n
	default:
		return n[:2] + "/" + n[2:4] + "/" + n
	}
}

// existsInCrates checks crates.io via the sparse index (CDN, ~50 ms warm;
// 404 = no such crate). The version is the highest non-yanked stable
// release (highest non-yanked prerelease if there is no stable one). The
// description comes from the API, best-effort. Returns (nil, nil) if the
// crate doesn't exist or every version is yanked.
func existsInCrates(ctx context.Context, pkg string) (*Result, error) {
	if !cratesNamePattern.MatchString(pkg) {
		return nil, nil
	}
	u := cratesIndexURL + "/" + cratesIndexPath(pkg)
	logx.Info("query crates.io index: %s", u)
	resp, err := httpGetAccept(ctx, u, "text/plain, application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("crates.io index", resp)
	}
	name, version, err := latestFromIndex(io.LimitReader(resp.Body, maxIndexBytes))
	if err != nil {
		return nil, fmt.Errorf("crates.io index: %s: %w", pkg, err)
	}
	if version == "" {
		return nil, nil
	}
	return &Result{
		Manager: pm.Cargo,
		Name:    name,
		Info:    cratesDescription(ctx, name),
		Extra:   map[string]string{"version": version},
	}, nil
}

// latestFromIndex reads newline-delimited index entries and returns the
// crate's canonical name and its best version. Publish order is not version
// order (0.8.x patches ship after 0.9.0), so versions are compared, not
// taken from the last line.
func latestFromIndex(r io.Reader) (name, version string, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	var bestStable, bestAny string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e struct {
			Name   string `json:"name"`
			Vers   string `json:"vers"`
			Yanked bool   `json:"yanked"`
		}
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return "", "", fmt.Errorf("bad index entry: %w", err)
		}
		if e.Name != "" {
			name = e.Name
		}
		if e.Yanked || e.Vers == "" {
			continue
		}
		if bestAny == "" || semverLess(bestAny, e.Vers) {
			bestAny = e.Vers
		}
		if !isPrerelease(e.Vers) && (bestStable == "" || semverLess(bestStable, e.Vers)) {
			bestStable = e.Vers
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if bestStable != "" {
		return name, bestStable, nil
	}
	return name, bestAny, nil
}

// cratesDescriptionBudget is how long the description call may take: at
// most cratesDescriptionTimeout, and never past ctx's deadline minus
// descriptionMargin.
func cratesDescriptionBudget(ctx context.Context) time.Duration {
	budget := cratesDescriptionTimeout
	if dl, ok := ctx.Deadline(); ok {
		if left := time.Until(dl) - descriptionMargin; left < budget {
			budget = left
		}
	}
	return budget
}

// cratesDescription fetches the crate's description from the API, giving
// up silently when cratesDescriptionBudget runs out.
func cratesDescription(ctx context.Context, name string) string {
	budget := cratesDescriptionBudget(ctx)
	if budget <= 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	u := fmt.Sprintf("%s/crates/%s?include=default_version", cratesAPIURL, url.PathEscape(name))
	resp, err := httpGet(ctx, u)
	if err != nil {
		logx.Info("crates.io description for %s unavailable: %v", name, err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var data struct {
		Crate struct {
			Description string `json:"description"`
		} `json:"crate"`
	}
	if err := decodeJSON("crates.io", name, resp.Body, &data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.Crate.Description)
}

// isPrerelease reports whether a semver string has a pre-release part.
func isPrerelease(v string) bool {
	core, _, _ := strings.Cut(v, "+")
	return strings.Contains(core, "-")
}

// semverLess reports a < b for MAJOR.MINOR.PATCH[-pre][+build] versions.
// Non-numeric core parts count as 0; prereleases of the same core compare
// lexically and sort before the release.
func semverLess(a, b string) bool {
	ac, ap := splitSemver(a)
	bc, bp := splitSemver(b)
	for i := range ac {
		if ac[i] != bc[i] {
			return ac[i] < bc[i]
		}
	}
	if ap == "" || bp == "" {
		return ap != "" && bp == ""
	}
	return ap < bp
}

func splitSemver(v string) (core [3]int, pre string) {
	v, _, _ = strings.Cut(v, "+")
	v, pre, _ = strings.Cut(v, "-")
	for i, p := range strings.SplitN(v, ".", 3) {
		core[i], _ = strconv.Atoi(p)
	}
	return core, pre
}
