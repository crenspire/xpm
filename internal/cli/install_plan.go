package cli

import (
	"fmt"
	"regexp"
	"slices"
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
	// Picked means the user chose it from a menu that showed its full
	// name, so a closest match needs no second confirmation.
	Picked bool
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
// A Packagist package named <q>/<q> (monolog -> monolog/monolog) is the
// package the user typed.
func needsRenameConfirmation(query string, c candidate) bool {
	switch {
	case c.Result.Manager == pm.Maven || c.Result.Manager == pm.Gradle:
		return false
	case c.Result.Manager == pm.Composer:
		return !strings.EqualFold(query, c.Result.Name) && !strings.EqualFold(query+"/"+query, c.Result.Name)
	case pm.EcosystemForManager(c.Result.Manager) == pm.EcosystemPython:
		return pep503Name(query) != pep503Name(c.Result.Name)
	}
	return !strings.EqualFold(query, c.Result.Name)
}

var pep503Separators = regexp.MustCompile(`[-_.]+`)

// pep503Name is the PyPI-normalised form of a project name, under which
// `Flask_SQLAlchemy` and `flask-sqlalchemy` are the same package.
func pep503Name(name string) string {
	return pep503Separators.ReplaceAllString(strings.ToLower(name), "-")
}

// isExact reports whether c is the package the user typed rather than a
// registry's closest match (Packagist and Maven searches return their first
// hit, often unrelated: axios -> swlib/saber). A Maven or Gradle hit is
// exact when its full coordinate is the query, or when its artifactId is
// the query with the same case (Maven Central has `...royale.framework:
// Express`, which is not npm's express) and it is not an npm/web-asset
// repackage (org.mvnpm*, org.webjars*), which reuse npm names. A Packagist
// hit is exact when its name, or <q>/<q>, is the query in any case.
func isExact(query string, c candidate) bool {
	if c.Result.Manager == pm.Maven || c.Result.Manager == pm.Gradle {
		name := c.Result.Name
		if strings.EqualFold(name, query) {
			return true
		}
		group := strings.ToLower(name[:max(strings.LastIndex(name, ":"), 0)])
		if strings.HasPrefix(group, "org.mvnpm") || strings.HasPrefix(group, "org.webjars") {
			return false
		}
		return name[strings.LastIndex(name, ":")+1:] == query
	}
	return !needsRenameConfirmation(query, c)
}

// splitExact separates exact candidates from closest matches, keeping order.
func splitExact(query string, cands []candidate) (exact, fuzzy []candidate) {
	for _, c := range cands {
		if isExact(query, c) {
			exact = append(exact, c)
		} else {
			fuzzy = append(fuzzy, c)
		}
	}
	return exact, fuzzy
}

// ecosystemKey groups tools that install the same packages: npm, yarn,
// pnpm and bun share "node"; composer is "php", cargo "rust", go modules
// "go". The keys are also what projectEcosystems returns.
func ecosystemKey(id pm.ID) string {
	if eco := pm.EcosystemForManager(id); eco != "" {
		return string(eco)
	}
	switch id {
	case pm.Composer:
		return "php"
	case pm.Cargo:
		return "rust"
	case pm.GoMod:
		return "go"
	}
	return "manager:" + string(id)
}

// ecosystemTitle is how an ecosystem key reads in messages ("PHP").
func ecosystemTitle(key string) string {
	switch key {
	case "node":
		return "Node"
	case "python":
		return "Python"
	case "java":
		return "Java"
	case "php":
		return "PHP"
	case "rust":
		return "Rust"
	case "go":
		return "Go"
	}
	return strings.TrimPrefix(key, "manager:")
}

// projectEcosystems returns the ecosystems (ecosystemKey values) that dir
// is a project for, in a fixed order: any project file counts
// (package.json or a Node lockfile, requirements.txt/pyproject.toml/
// Pipfile/poetry.lock, composer.json, Cargo.toml, go.mod, pom.xml,
// build.gradle(.kts)). It returns nil outside a project.
func projectEcosystems(dir string) []string {
	var keys []string
	add := func(k string) {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	for _, t := range detectProjectTargetsIn(dir) {
		add(ecosystemKey(t.PMs[0]))
	}
	for _, f := range pm.ProjectManagers(dir) {
		add(ecosystemKey(f[0].Manager))
	}
	return keys
}

// installAction is what decideInstall settles on.
type installAction int

const (
	actionPick   installAction = iota // install Pick
	actionMenu                        // let the user choose from Menu
	actionRefuse                      // no terminal and no safe choice: Err
)

// installInputs is everything decideInstall looks at.
type installInputs struct {
	Query       string
	Cands       []candidate // from buildCandidates, ordered by sortCandidates
	ProjectEcos []string    // projectEcosystems of the current directory
	Prefer      []string
	Unavailable []pm.ID // registries that did not answer
	Interactive bool    // a menu can be shown
	Global      bool    // a global install: the current project does not decide
}

// installDecision is decideInstall's answer. Note (with actionPick) is a
// line to print before installing, or "".
type installDecision struct {
	Action installAction
	Pick   candidate
	Note   string
	Menu   []candidate
	Labels []string
	Err    error
}

// decideInstall chooses the candidate for `xpm install <query>`, for
// terminal and non-terminal runs alike ("project first"):
//
//   - Inside a project, an exact hit in the project's ecosystem is installed
//     with the project's tool, even when other ecosystems have namesakes or
//     their registries did not answer. Exact hits in two of the project's
//     ecosystems, no exact hit in them, or the project's own registry not
//     answering mean a menu, or a refusal without a terminal.
//   - Outside a project, and for global installs anywhere, exact hits from one ecosystem (or, without any
//     exact hit, the one registry that answered with a closest match) are
//     installed; "prefer" settles exact hits from several ecosystems when
//     it puts one strictly first; anything else is a menu or a refusal. A
//     registry that did not answer forbids any guess without a terminal.
//
// A closest match picked here is still confirmed (or refused) by
// installCandidate.
func decideInstall(in installInputs) installDecision {
	if len(in.ProjectEcos) > 0 && !in.Global {
		return decideInProject(in)
	}
	return decideOutsideProject(in)
}

func decideInProject(in installInputs) installDecision {
	inProject := func(c candidate) bool { return slices.Contains(in.ProjectEcos, ecosystemKey(c.Result.Manager)) }
	var down []pm.ID
	for _, id := range in.Unavailable {
		if slices.Contains(in.ProjectEcos, ecosystemKey(id)) {
			down = append(down, id)
		}
	}
	if len(down) > 0 {
		return menuOrRefuse(in, refuseGuessWhenUnavailable(down))
	}
	exact, _ := splitExact(in.Query, in.Cands)
	var own []candidate
	for _, c := range exact {
		if inProject(c) {
			own = append(own, c)
		}
	}
	switch ecos := ecosystemsOf(own); len(ecos) {
	case 0:
		return menuOrRefuse(in, fmt.Errorf("no exact match for %q in this %s registry; found: %s. Run xpm inside the project the package belongs to, or install it by its exact name (vendor/package for Composer, group:artifact for Maven and Gradle, a module path such as github.com/spf13/cobra for Go)",
			in.Query, projectPhrase(in.ProjectEcos), choiceList(in.Query, orderMenu(in))))
	case 1:
		pick := own[0] // ordered by prefer
		if in.Interactive && len(managersOf(own)) > 1 {
			// Several lockfiles in one ecosystem: prefer decides, or the user.
			c, ok := strictlyPreferred(own, in.Prefer)
			if !ok {
				return menu(in)
			}
			pick = c
		}
		key := ecosystemKey(pick.Result.Manager)
		return installDecision{Action: actionPick, Pick: pick,
			Note: fmt.Sprintf("Using %s for this %s project%s.", pick.Result.Manager, ecosystemTitle(key), alsoFound(exact, key, "("))}
	default:
		return menuOrRefuse(in, fmt.Errorf("%s exists in several of this project's ecosystems: %s. Run xpm in a terminal to choose",
			own[0].Result.Name, choiceList(in.Query, own)))
	}
}

func decideOutsideProject(in installInputs) installDecision {
	if len(in.Unavailable) > 0 {
		return menuOrRefuse(in, refuseGuessWhenUnavailable(in.Unavailable))
	}
	exact, _ := splitExact(in.Query, in.Cands)
	pool := exact
	if len(pool) == 0 {
		pool = in.Cands
	}
	if len(ecosystemsOf(pool)) == 1 {
		return installDecision{Action: actionPick, Pick: pool[0]}
	}
	if len(exact) == 0 {
		return menuOrRefuse(in, fmt.Errorf("no package is named exactly %q; closest matches: %s. Re-run with the exact name",
			in.Query, choiceList(in.Query, pool)))
	}
	if c, ok := strictlyPreferred(exact, in.Prefer); ok {
		return installDecision{Action: actionPick, Pick: c,
			Note: fmt.Sprintf("Using %s (\"prefer\" in config%s).", c.Result.Manager, alsoFound(exact, ecosystemKey(c.Result.Manager), "; "))}
	}
	where := " or run inside a project"
	if in.Global {
		where = "" // a project does not choose a global install
	}
	return menuOrRefuse(in, fmt.Errorf("%s exists in several ecosystems: %s. Set \"prefer\" in config (e.g. xpm config set prefer %s)%s",
		exact[0].Result.Name, choiceList(in.Query, exact), exact[0].Result.Manager, where))
}

// menuOrRefuse shows the menu on a terminal and refuses with err otherwise.
func menuOrRefuse(in installInputs, err error) installDecision {
	if in.Interactive {
		return menu(in)
	}
	return installDecision{Action: actionRefuse, Err: err}
}

func menu(in installInputs) installDecision {
	items := orderMenu(in)
	labels := make([]string, len(items))
	for i, c := range items {
		labels[i] = candidateLabel(c)
		if !isExact(in.Query, c) {
			labels[i] += " (closest match)"
		}
	}
	return installDecision{Action: actionMenu, Menu: items, Labels: labels}
}

// orderMenu lists the candidates of the project's ecosystems first (exact,
// then closest matches), then exact candidates elsewhere, then closest
// matches elsewhere, keeping prefer order within each group.
func orderMenu(in installInputs) []candidate {
	var groups [4][]candidate
	for _, c := range in.Cands {
		g := 2
		if slices.Contains(in.ProjectEcos, ecosystemKey(c.Result.Manager)) {
			g = 0
		}
		if !isExact(in.Query, c) {
			g++
		}
		groups[g] = append(groups[g], c)
	}
	return slices.Concat(groups[0], groups[1], groups[2], groups[3])
}

// strictlyPreferred returns the candidate whose tool the prefer list puts
// strictly before every other candidate's tool.
func strictlyPreferred(cands []candidate, prefer []string) (candidate, bool) {
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
	if ties != 1 {
		return candidate{}, false
	}
	return cands[best], true
}

// ecosystemsOf returns the distinct ecosystem keys of cands, in order.
func ecosystemsOf(cands []candidate) []string {
	var keys []string
	for _, c := range cands {
		if k := ecosystemKey(c.Result.Manager); !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// managersOf returns the distinct tools of cands, in order.
func managersOf(cands []candidate) []pm.ID {
	var ids []pm.ID
	for _, c := range cands {
		if !slices.Contains(ids, c.Result.Manager) {
			ids = append(ids, c.Result.Manager)
		}
	}
	return ids
}

// alsoFound is ", also found: Node, Rust" style text naming the ecosystems
// other than key that have an exact candidate, opened with sep ("(" or
// "; ") and closed with ")" when sep is "("; "" when there are none.
func alsoFound(exact []candidate, key, sep string) string {
	var titles []string
	for _, k := range ecosystemsOf(exact) {
		if k != key {
			titles = append(titles, ecosystemTitle(k))
		}
	}
	if len(titles) == 0 {
		return ""
	}
	text := "also found: " + strings.Join(titles, ", ")
	if sep == "(" {
		return " (" + text + ")"
	}
	return sep + text
}

// projectPhrase names the project's ecosystems: "PHP project's" or
// "project's (Go, Node)".
func projectPhrase(ecos []string) string {
	if len(ecos) == 1 {
		return ecosystemTitle(ecos[0]) + " project's"
	}
	titles := make([]string, len(ecos))
	for i, k := range ecos {
		titles[i] = ecosystemTitle(k)
	}
	return "project's (" + strings.Join(titles, ", ") + ")"
}

// choiceList lists candidates for a refusal; a closest match shows the
// name it would install.
func choiceList(query string, cands []candidate) string {
	choices := make([]string, len(cands))
	for i, c := range cands {
		choices[i] = candidateChoice(c)
		if !isExact(query, c) {
			choices[i] += ": " + c.Result.Name + " (closest match)"
		}
	}
	return strings.Join(choices, ", ")
}

// candidateChoice is a candidate as listed when xpm refuses to choose:
// "npm (Node, via package-lock.json)", or "composer (PHP)".
func candidateChoice(c candidate) string {
	details := []string{ecosystemTitle(ecosystemKey(c.Result.Manager))}
	if c.Via != "" {
		details = append(details, "via "+c.Via)
	}
	return fmt.Sprintf("%s (%s)", c.Result.Manager, strings.Join(details, ", "))
}
