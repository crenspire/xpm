package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/crenspire/xpm/internal/deps"
	"github.com/crenspire/xpm/internal/osv"
	"github.com/crenspire/xpm/internal/search"
)

// osvBaseURL is the OSV.dev API root; a seam for tests.
var osvBaseURL = osv.DefaultBaseURL

const defaultAuditTimeout = 30 * time.Second

// auditArgs is `xpm audit`'s command line.
type auditArgs struct {
	JSON, Workspace bool
	Timeout         time.Duration
}

// parseAuditTimeout accepts a Go duration ("30s", "2m") or a bare number of
// seconds, and requires a positive result.
func parseAuditTimeout(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		n, nerr := strconv.ParseFloat(s, 64)
		if nerr != nil {
			return 0, fmt.Errorf("invalid --timeout %q: use a duration such as 30s or a number of seconds", s)
		}
		d = time.Duration(n * float64(time.Second))
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid --timeout %q: must be greater than zero", s)
	}
	return d, nil
}

// parseAuditArgs parses the flags; the command takes no positional arguments.
func parseAuditArgs(args []string) (auditArgs, error) {
	a := auditArgs{Timeout: defaultAuditTimeout}
	var timeout string
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&a.JSON, "json", false, "print the result as JSON")
	fs.StringVar(&timeout, "timeout", "", "time limit for the OSV.dev queries (default 30s)")
	fs.BoolVar(&a.Workspace, "workspace", false, "combine all workspace projects")
	fs.BoolVar(&a.Workspace, "w", false, "shorthand for --workspace")
	if err := fs.Parse(args); err != nil {
		return auditArgs{}, errFlagReported{err}
	}
	if fs.NArg() > 0 {
		return auditArgs{}, fmt.Errorf("audit takes no arguments, got %q", fs.Arg(0))
	}
	if timeout != "" {
		d, err := parseAuditTimeout(timeout)
		if err != nil {
			return auditArgs{}, err
		}
		a.Timeout = d
	}
	return a, nil
}

// auditVuln is one vulnerability of a package in the JSON output.
type auditVuln struct {
	ID       string   `json:"id"`
	Aliases  []string `json:"aliases"`
	Summary  string   `json:"summary"`
	Severity string   `json:"severity"`
	Fixed    []string `json:"fixed"`
}

// auditPackage is one vulnerable package.
type auditPackage struct {
	Ecosystem string      `json:"ecosystem"`
	Name      string      `json:"name"`
	Version   string      `json:"version"`
	Vulns     []auditVuln `json:"vulns"`
}

// auditUnchecked is a dependency that was not sent to OSV.
type auditUnchecked struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Reason    string `json:"reason"`
}

// auditReport is the JSON document of `xpm audit`.
type auditReport struct {
	Scanned    int              `json:"scanned"`
	Vulnerable []auditPackage   `json:"vulnerable"`
	Unchecked  []auditUnchecked `json:"unchecked"`
}

// cleanStrings sanitizes each string for the terminal; never returns nil.
func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, search.SanitizeText(s))
	}
	return out
}

// cmdAudit checks the locked dependencies against OSV.dev.
// Exit status: 1 if any vulnerability is found, 0 if none; usage errors, an
// unreadable project and an OSV failure exit 2.
func cmdAudit(args []string) int {
	a, err := parseAuditArgs(args)
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

	list := deps.All(g)
	rep := auditReport{Vulnerable: []auditPackage{}, Unchecked: []auditUnchecked{}}
	var pkgs []osv.Package
	var checked []deps.Dep
	for _, d := range list {
		eco, ok := deps.OSVEcosystem(d.Ecosystem)
		switch {
		case !ok:
			rep.Unchecked = append(rep.Unchecked, auditUnchecked{d.Ecosystem, d.Name, d.Version, "no OSV ecosystem for this ecosystem"})
		case !deps.Pinned(d.Version):
			rep.Unchecked = append(rep.Unchecked, auditUnchecked{d.Ecosystem, d.Name, d.Version, "no locked version"})
		default:
			pkgs = append(pkgs, osv.Package{Ecosystem: eco, Name: d.Name, Version: d.Version})
			checked = append(checked, d)
		}
	}
	rep.Scanned = len(checked)

	if len(pkgs) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), a.Timeout)
		defer cancel()
		client := osv.Client{BaseURL: osvBaseURL}
		results, detailsErrs, qerr := client.QueryBatch(ctx, pkgs)
		if qerr != nil {
			fmt.Fprintf(os.Stderr, "error: OSV.dev query failed: %s\n", search.SanitizeText(qerr.Error()))
			return 2
		}
		if detailsErrs > 0 {
			fmt.Fprintf(os.Stderr, "warning: details for %d vulnerabilities could not be fetched; IDs are listed\n", detailsErrs)
		}
		for i, d := range checked {
			if i >= len(results) || len(results[i]) == 0 {
				continue
			}
			p := auditPackage{Ecosystem: d.Ecosystem, Name: search.SanitizeText(d.Name), Version: search.SanitizeText(d.Version), Vulns: []auditVuln{}}
			for _, v := range results[i] {
				p.Vulns = append(p.Vulns, auditVuln{
					ID:       search.SanitizeText(v.ID),
					Aliases:  cleanStrings(v.Aliases),
					Summary:  search.SanitizeText(v.Summary),
					Severity: search.SanitizeText(v.Severity),
					Fixed:    cleanStrings(v.Fixed),
				})
			}
			rep.Vulnerable = append(rep.Vulnerable, p)
		}
	}
	sort.SliceStable(rep.Vulnerable, func(i, j int) bool {
		a, b := rep.Vulnerable[i], rep.Vulnerable[j]
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})
	sort.SliceStable(rep.Unchecked, func(i, j int) bool {
		a, b := rep.Unchecked[i], rep.Unchecked[j]
		if a.Ecosystem != b.Ecosystem {
			return a.Ecosystem < b.Ecosystem
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})

	if a.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
	} else {
		printAudit(rep, len(list))
	}
	noteUnchecked(len(rep.Unchecked))
	if len(rep.Vulnerable) > 0 {
		return 1
	}
	return 0
}

// printAudit writes the human report; total is the number of dependencies
// found (checked or not).
func printAudit(rep auditReport, total int) {
	switch {
	case total == 0:
		fmt.Println("No dependencies found.")
		return
	case len(rep.Vulnerable) == 0:
		fmt.Printf("No known vulnerabilities in %d packages.\n", rep.Scanned)
		return
	}
	vulns := 0
	for _, p := range rep.Vulnerable {
		fmt.Printf("%s@%s (%s)\n", p.Name, p.Version, p.Ecosystem)
		for _, v := range p.Vulns {
			vulns++
			line := "  " + v.ID
			if len(v.Aliases) > 0 {
				aliases := v.Aliases
				if len(aliases) > 3 {
					aliases = aliases[:3]
				}
				line += " (" + strings.Join(aliases, ", ") + ")"
			}
			if v.Severity != "" {
				line += "  [" + v.Severity + "]"
			}
			if v.Summary != "" {
				line += "  " + v.Summary
			}
			if len(v.Fixed) > 0 {
				line += "  (fixed in: " + strings.Join(v.Fixed, ", ") + ")"
			}
			fmt.Println(line)
		}
	}
	fmt.Printf("Found %d vulnerabilities in %d packages (%d packages scanned).\n", vulns, len(rep.Vulnerable), rep.Scanned)
}
