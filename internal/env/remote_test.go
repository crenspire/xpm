package env

import (
	"fmt"
	"strings"
	"testing"
)

func TestFormatRemoteShowsNewestTwenty(t *testing.T) {
	var versions []string
	for i := 30; i >= 1; i-- {
		versions = append(versions, fmt.Sprintf("1.%d.0", i))
	}
	out := FormatRemote("go", versions)
	if !strings.Contains(out, "(showing the newest 20 of 30 versions)") {
		t.Fatalf("missing count line:\n%s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if first := strings.TrimSpace(lines[3]); first != "1.30.0" {
		t.Fatalf("first listed = %q, want 1.30.0", first)
	}
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "1.11.0" {
		t.Fatalf("last listed = %q, want 1.11.0", last)
	}
}
