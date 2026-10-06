package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// candidate is one way to satisfy `xpm install <query>`: a registry hit and
// the tool that would install it (Result.Manager).
type candidate struct {
	Result search.Result
	// Via is the project file that selected the tool ("yarn.lock"), or "".
	Via string
}

// buildCandidates turns registry hits into install choices. Inside an
// ecosystem the project's lock/build files narrow the tool: with yarn.lock,
// the npm hit is installed with yarn. Across ecosystems nothing is narrowed:
// a name found on npm and on PyPI is always the user's choice.
func buildCandidates(results []search.Result, project map[pm.Ecosystem][]pm.ProjectFile) []candidate {
	var out []candidate
	for _, r := range results {
		files := project[pm.EcosystemForManager(r.Manager)]
		if len(files) == 0 {
			out = append(out, candidate{Result: r})
			continue
		}
		for _, f := range files {
			c := candidate{Result: r, Via: f.Name}
			c.Result.Manager = f.Manager
			out = append(out, c)
		}
	}
	return out
}

// sortCandidates orders candidates by the user's prefer list. Ties keep
// their registry order, so the result is deterministic.
func sortCandidates(cands []candidate, prefer []string) {
	order := preferOrderMap(prefer)
	sort.SliceStable(cands, func(i, j int) bool {
		return order[string(cands[i].Result.Manager)] < order[string(cands[j].Result.Manager)]
	})
}

// candidateLabel is how a candidate appears in the selection prompt.
func candidateLabel(c candidate) string {
	label := fmt.Sprintf("%s (%s)", c.Result.Name, c.Result.Manager)
	if c.Via != "" {
		label = fmt.Sprintf("%s (%s, from %s)", c.Result.Name, c.Result.Manager, c.Via)
	}
	if v := c.Result.Extra["version"]; v != "" {
		label += " @ " + v
	}
	if c.Result.Info != "" {
		label += " - " + c.Result.Info
	}
	return label
}

// installSpec returns what to hand the adapter: the registry's name for the
// package (Composer/Maven hits can differ from the query) and a copy of its
// extra info. A version is passed only when the user asked for one; Maven
// and Gradle keep the registry's latest because they print a snippet that
// needs a concrete version.
func installSpec(c candidate, requestedVersion string) (name string, extra map[string]string) {
	extra = make(map[string]string, len(c.Result.Extra)+1)
	for k, v := range c.Result.Extra {
		extra[k] = v
	}
	switch {
	case requestedVersion != "":
		extra["version"] = requestedVersion
	case c.Result.Manager == pm.Maven || c.Result.Manager == pm.Gradle:
	default:
		delete(extra, "version")
	}
	return c.Result.Name, extra
}

// isGoModulePath reports whether s looks like a Go module path
// (github.com/gin-gonic/gin, golang.org/x/term, gopkg.in/yaml.v3): a first
// element containing a dot (a domain) followed by at least one more element.
// npm scopes (@a/b), Composer names (vendor/pkg) and Maven coordinates
// (g:a) never match.
func isGoModulePath(s string) bool {
	first, rest, ok := strings.Cut(s, "/")
	return ok && rest != "" && !strings.HasPrefix(s, "@") &&
		strings.Contains(first, ".") && !strings.Contains(s, ":")
}

// goModuleCandidate installs a module path with go modules; no registry is
// consulted (`go get` resolves and verifies it via the module proxy).
func goModuleCandidate(path string) candidate {
	return candidate{Result: search.Result{Manager: pm.GoMod, Name: path, Extra: map[string]string{"module": path}}}
}

// needsRenameConfirmation reports whether installing c would run a tool on a
// name the user did not type (a fuzzy registry hit such as `axioss` ->
// `axios`). Maven and Gradle only print a snippet, so they never need it.
func needsRenameConfirmation(query string, c candidate) bool {
	if c.Result.Manager == pm.Maven || c.Result.Manager == pm.Gradle {
		return false
	}
	return !strings.EqualFold(query, c.Result.Name)
}

// nonInteractivePick chooses a candidate without asking. It picks only when
// all candidates share one ecosystem (the list is already ordered by prefer),
// or when the prefer list puts exactly one candidate strictly first.
func nonInteractivePick(cands []candidate, prefer []string) (candidate, bool) {
	if len(cands) == 0 {
		return candidate{}, false
	}
	same := true
	eco := pm.EcosystemForManager(cands[0].Result.Manager)
	for _, c := range cands[1:] {
		if pm.EcosystemForManager(c.Result.Manager) != eco {
			same = false
			break
		}
	}
	if same {
		return cands[0], true
	}
	order := preferOrderMap(prefer)
	best, ties := -1, 0
	for i, c := range cands {
		switch o := order[string(c.Result.Manager)]; {
		case best == -1 || o < order[string(cands[best].Result.Manager)]:
			best, ties = i, 1
		case o == order[string(cands[best].Result.Manager)]:
			ties++
		}
	}
	if ties == 1 {
		return cands[best], true
	}
	return candidate{}, false
}

// candidateChoice is a candidate as listed when xpm refuses to choose.
func candidateChoice(c candidate) string {
	label := fmt.Sprintf("%s (%s", c.Result.Manager, ecosystemTitle(pm.EcosystemForManager(c.Result.Manager)))
	if c.Via != "" {
		label += ", via " + c.Via
	}
	return label + ")"
}

func ecosystemTitle(e pm.Ecosystem) string {
	switch e {
	case pm.EcosystemNode:
		return "Node"
	case pm.EcosystemPython:
		return "Python"
	case pm.EcosystemJava:
		return "Java"
	}
	return string(e)
}
