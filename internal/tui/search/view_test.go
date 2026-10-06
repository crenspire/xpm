package search

import (
	"strings"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

func TestRenderResultStripsControlSequences(t *testing.T) {
	out := renderResult(search.Result{Manager: pm.Npm, Name: "x\x1b]0;t\x07y", Info: "d\x1b]0;t\x07e"}, false, 100)
	if !strings.Contains(out, "xy") || !strings.Contains(out, "de") {
		t.Fatalf("renderResult = %q, want sanitized name xy and info de", out)
	}
	if strings.Contains(out, "\x1b]") {
		t.Fatalf("renderResult = %q, contains an OSC sequence", out)
	}
}
