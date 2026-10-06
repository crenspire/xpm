package cli

import (
	"strings"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

// Seams for tests: the registry fan-outs.
var (
	lookupReport = search.SearchEverywhereReport
	searchReport = search.SearchReport
)

// registryStatus says, for the registries that did not return a result,
// which answered "not here" and which could not answer at all.
type registryStatus struct {
	NotFound    []pm.ID
	Unavailable []search.RegistryFailure
}

// classify sorts the enabled registries without a result into NotFound and
// Unavailable, in search.Registries order.
func classify(rep search.Report, opts search.Options) registryStatus {
	found := map[pm.ID]bool{}
	for _, r := range rep.Results {
		found[r.Manager] = true
	}
	failed := map[pm.ID]bool{}
	for _, f := range rep.Unavailable {
		failed[f.Manager] = true
	}
	st := registryStatus{Unavailable: rep.Unavailable}
	for _, id := range search.Registries {
		if search.Enabled(opts, id) && !found[id] && !failed[id] {
			st.NotFound = append(st.NotFound, id)
		}
	}
	return st
}

// managerName is the display name of a manager ("pip (Python)").
func managerName(id pm.ID) string {
	if meta, ok := pm.MetaFor(id); ok {
		return meta.Name
	}
	return string(id)
}

// describeFailure is the one-word-ish reason a registry is unavailable. Error
// text can carry registry response bodies, so it is sanitized for the terminal.
func describeFailure(f search.RegistryFailure) string {
	if f.TimedOut() {
		return "timed out"
	}
	return search.SanitizeText(f.Err.Error())
}

// formatAvailability renders the "Not found in" and "Unavailable" sections
// (empty when there is nothing to say).
func formatAvailability(st registryStatus) string {
	var b strings.Builder
	if len(st.NotFound) > 0 {
		b.WriteString("\nNot found in:\n")
		for _, id := range st.NotFound {
			b.WriteString("- " + managerName(id) + "\n")
		}
	}
	if len(st.Unavailable) > 0 {
		b.WriteString("\nUnavailable (results may be incomplete):\n")
		for _, f := range st.Unavailable {
			b.WriteString("- " + managerName(f.Manager) + ": " + describeFailure(f) + "\n")
		}
	}
	return b.String()
}
