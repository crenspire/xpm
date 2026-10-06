package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/crenspire/xpm/internal/deps"
	"github.com/crenspire/xpm/internal/graph"
	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// latestVersions looks up the newest registry version of each query; a seam
// for tests.
var latestVersions = search.LatestVersions

// Row statuses of `xpm outdated`.
const (
	statusOutdated    = "outdated"
	statusCurrent     = "current"
	statusUnavailable = "unavailable"
	statusNotFound    = "not found"
)

// outdatedArgs is `xpm outdated`'s command line.
type outdatedArgs struct {
	JSON, Workspace, All bool
}

// parseOutdatedArgs parses the flags; the command takes no positional
// arguments.
func parseOutdatedArgs(args []string) (outdatedArgs, error) {
	var a outdatedArgs
	fs := flag.NewFlagSet("outdated", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&a.JSON, "json", false, "print the result as JSON")
	fs.BoolVar(&a.All, "all", false, "check every locked package, not only direct dependencies")
	fs.BoolVar(&a.Workspace, "workspace", false, "combine all workspace projects")
	fs.BoolVar(&a.Workspace, "w", false, "shorthand for --workspace")
	if err := fs.Parse(args); err != nil {
		return outdatedArgs{}, errFlagReported{err}
	}
	// Flags may follow positionals; reject any positional once parsed.
	if fs.NArg() > 0 {
		return outdatedArgs{}, fmt.Errorf("outdated takes no arguments, got %q", fs.Arg(0))
	}
	return a, nil
}

// outdatedRow is one checked dependency in the JSON output.
type outdatedRow struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// uncheckedRow is one dependency that was not looked up.
type uncheckedRow struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Current   string `json:"current"`
	Reason    string `json:"reason"`
}

// outdatedReport is the JSON document of `xpm outdated`.
type outdatedReport struct {
	Dependencies []outdatedRow  `json:"dependencies"`
	Unchecked    []uncheckedRow `json:"unchecked"`
}

// cmdOutdated lists dependencies whose registry has a newer version.
// Exit status: 1 if any is outdated, else 2 if a lookup failed or no
// dependency could be checked at all, else 0; usage errors and an unreadable
// project exit 2.
func cmdOutdated(args []string) int {
	a, err := parseOutdatedArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		if !errors.As(err, new(errFlagReported)) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		return 2
	}

	g, err := loadDepGraph(a.Workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	list := deps.Direct(g)
	if a.All {
		list = deps.All(g)
	}
	rep, total := checkOutdated(list)

	if a.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
	}
	return finishOutdated(rep, total, a.JSON)
}

// loadDepGraph extracts the dependency graph of the current directory (all
// workspace projects when workspace is set), warning on stderr. It never
// returns a nil graph without an error.
func loadDepGraph(workspace bool) (*graph.DepGraph, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	opts := graph.ExtractOptions{
		Warn: func(msg string) { fmt.Fprintf(os.Stderr, "warning: %s\n", msg) },
	}
	var g *graph.DepGraph
	if workspace {
		g, err = workspaceGraph(cwd, opts)
	} else {
		g, err = extractGraph(cwd, opts)
	}
	if err != nil {
		return nil, err
	}
	if g == nil {
		g = graph.NewGraph()
	}
	return g, nil
}

// Why dependencies end up in "unchecked", per command.
const (
	outdatedUncheckedWhy = "no locked version, or no registry lookup for it"
	auditUncheckedWhy    = "no locked version, no OSV ecosystem, or a private Go module"
)

// plural returns one when n is 1, else many.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// noteUnchecked prints the stderr note shared by outdated and audit for n
// dependencies that were not looked up; why lists the command's reasons.
func noteUnchecked(n int, why string) {
	if n > 0 {
		fmt.Fprintf(os.Stderr, "note: %d %s not checked (%s; --json lists the reasons)\n", n, plural(n, "dependency was", "dependencies were"), why)
	}
}

// noteNothingChecked reports on stderr that the project has n dependencies
// and none of them could be checked (the command then exits 2).
func noteNothingChecked(n int, why string) {
	fmt.Fprintf(os.Stderr, "error: nothing could be checked: %d %s not checked (%s); see --json for the reasons\n", n, plural(n, "dependency was", "dependencies were"), why)
}

// checkOutdated looks up the latest version of every checkable dependency in
// list. It returns the report (sorted) and the number of dependencies given.
func checkOutdated(list []deps.Dep) (outdatedReport, int) {
	rep := outdatedReport{Dependencies: []outdatedRow{}, Unchecked: []uncheckedRow{}}
	type key struct {
		m    pm.ID
		name string
	}
	var queries []search.LatestQuery
	queued := map[key]bool{}
	var checkable []deps.Dep
	for _, d := range list {
		m, ok := deps.Manager(d.Ecosystem)
		switch {
		case !ok:
			rep.Unchecked = append(rep.Unchecked, uncheckedRow{d.Ecosystem, d.Name, d.Version, "no registry for this ecosystem"})
		case !deps.Pinned(d.Version):
			rep.Unchecked = append(rep.Unchecked, uncheckedRow{d.Ecosystem, d.Name, d.Version, "no locked version"})
		default:
			checkable = append(checkable, d)
			if k := (key{m, d.Name}); !queued[k] {
				queued[k] = true
				queries = append(queries, search.LatestQuery{Manager: m, Name: d.Name})
			}
		}
	}

	results := map[key]search.LatestResult{}
	if len(queries) > 0 {
		for _, r := range latestVersions(queries, search.OptionsFromConfig(cfg)) {
			results[key{r.Manager, r.Name}] = r
		}
	}
	for _, d := range checkable {
		m, _ := deps.Manager(d.Ecosystem)
		r, answered := results[key{m, d.Name}]
		switch {
		case !answered:
			rep.Unchecked = append(rep.Unchecked, uncheckedRow{d.Ecosystem, d.Name, d.Version, "no registry answer"})
		case errors.Is(r.Err, search.ErrNotChecked) || errors.Is(r.Err, search.ErrRegistryDisabled):
			rep.Unchecked = append(rep.Unchecked, uncheckedRow{d.Ecosystem, d.Name, d.Version, search.SanitizeText(r.Err.Error())})
		default:
			row := outdatedRow{Ecosystem: d.Ecosystem, Name: d.Name, Current: d.Version}
			switch {
			case r.Err != nil:
				row.Status, row.Error = statusUnavailable, search.SanitizeText(r.Err.Error())
			case !r.Found:
				row.Status = statusNotFound
			default:
				row.Latest = search.SanitizeText(r.Version)
				row.Status = statusCurrent
				if deps.Compare(d.Ecosystem, d.Version, r.Version) < 0 {
					row.Status = statusOutdated
				}
			}
			rep.Dependencies = append(rep.Dependencies, row)
		}
	}
	sort.SliceStable(rep.Dependencies, func(i, j int) bool {
		a, b := rep.Dependencies[i], rep.Dependencies[j]
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Current < b.Current
	})
	sort.SliceStable(rep.Unchecked, func(i, j int) bool {
		a, b := rep.Unchecked[i], rep.Unchecked[j]
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Current < b.Current
	})
	return rep, len(list)
}

// finishOutdated prints the human report (unless asJSON), the stderr note,
// and returns the exit status.
func finishOutdated(rep outdatedReport, total int, asJSON bool) int {
	if total > 0 && len(rep.Dependencies) == 0 {
		// The project has dependencies but none got a registry answer.
		noteNothingChecked(len(rep.Unchecked), outdatedUncheckedWhy)
		return 2
	}
	var outdated, current, unavailable, notFound int
	for _, r := range rep.Dependencies {
		switch r.Status {
		case statusOutdated:
			outdated++
		case statusCurrent:
			current++
		case statusNotFound:
			notFound++
		default:
			unavailable++
		}
	}

	if !asJSON {
		switch {
		case total == 0:
			fmt.Println("No dependencies found.")
		case outdated == 0 && unavailable == 0 && notFound == 0:
			fmt.Printf("All %d dependencies are up to date.\n", len(rep.Dependencies))
		default:
			tw := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ECOSYSTEM\tPACKAGE\tCURRENT\tLATEST\tSTATUS")
			for _, r := range rep.Dependencies {
				if r.Status == statusCurrent {
					continue
				}
				latest := r.Latest
				if latest == "" {
					latest = "-"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Ecosystem, search.SanitizeText(r.Name), search.SanitizeText(r.Current), latest, r.Status)
			}
			_ = tw.Flush()
			summary := fmt.Sprintf("%d outdated, %d up to date", outdated, current)
			if notFound > 0 {
				summary += fmt.Sprintf(", %d not found", notFound)
			}
			if unavailable > 0 {
				summary += fmt.Sprintf(", %d could not be checked", unavailable)
			}
			fmt.Println(summary)
		}
	}
	noteUnchecked(len(rep.Unchecked), outdatedUncheckedWhy)

	switch {
	case outdated > 0:
		return 1
	case unavailable > 0:
		return 2
	}
	return 0
}
