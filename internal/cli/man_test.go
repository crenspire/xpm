package cli

import (
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func lineWith(out, name string) string {
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(ansiRe.ReplaceAllString(l, ""))
		if len(f) > 0 && strings.TrimSuffix(f[0], ",") == name {
			return l
		}
	}
	return ""
}

func TestUsageListsEveryDispatchedCommand(t *testing.T) {
	out := captureStdout(t, usage)
	for _, c := range []string{"install", "ci", "run", "which", "list", "update", "remove", "info", "search", "lock", "cache", "cc", "cg", "graph", "workspaces", "env", "doctor", "config", "version", "help", "man"} {
		if !strings.Contains(out, c) {
			t.Errorf("usage missing %q", c)
		}
	}
}

func TestUsageMarksExperimental(t *testing.T) {
	out := captureStdout(t, usage)
	for _, c := range []string{"env", "graph", "lock", "workspaces", "cache"} {
		l := lineWith(out, c)
		if l == "" || !strings.Contains(l, "(experimental)") {
			t.Errorf("%s line not marked experimental: %q", c, l)
		}
	}
	for _, c := range []string{"install", "which", "search"} {
		l := lineWith(out, c)
		if l == "" || strings.Contains(l, "(experimental)") {
			t.Errorf("%s line wrong: %q", c, l)
		}
	}
}

func TestManListMatchesUsage(t *testing.T) {
	u := captureStdout(t, usage)
	m := captureStdout(t, listCommands)
	for _, c := range commandTable {
		ul, ml := lineWith(u, c.name), lineWith(m, c.name)
		if ul == "" || ul != ml {
			t.Errorf("%s: usage %q vs man %q", c.name, ul, ml)
		}
	}
}

func TestNormalizeCommandAliases(t *testing.T) {
	for in, want := range map[string]string{"cc": "cache", "cg": "cache", "i": "install", "-V": "version", "--version": "version", "-v": "version", "-h": "help", "--help": "help", "rm": "remove", "ci": "ci"} {
		if got := normalizeCommand(in); got != want {
			t.Errorf("normalizeCommand(%q)=%q want %q", in, got, want)
		}
	}
}
