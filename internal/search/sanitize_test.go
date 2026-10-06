package search

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

func TestSanitizeText(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"plain", "plain", "plain"},
		{"printable unicode", "héllo 世界", "héllo 世界"},
		{"csi", "\x1b[31mred\x1b[0m", "red"},
		{"osc bel", "a\x1b]0;title\x07b", "ab"},
		{"osc st", "a\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\b", "alinkb"},
		{"dcs", "a\x1bPdata\x1b\\b", "ab"},
		{"lone esc", "a\x1bcd", "ad"},
		{"c1 csi", "\u009b31mX", "X"},
		{"whitespace controls", "line1\nline2\ttab\r", "line1 line2 tab"},
		{"other c0 and del", "a\x00b\x7fc\x08d", "abcd"},
		{"invalid utf8", "a\xffb", "a�b"},
		{"unterminated csi", "a\x1b[31", "a"},
		{"osc c1 st", "a\x1b]0;t\u009cvisible", "avisible"},
		{"osc 8-bit st", "a\u009d0;t\u009cvisible", "avisible"},
		{"unterminated osc", "a\x1b]0;title", "a"},
		{"c1 osc", "a\u009d0;t\u0007b", "ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeText(tc.in); got != tc.want {
				t.Fatalf("SanitizeText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFanOutSanitizesRegistryText(t *testing.T) {
	shared := map[string]string{"version": "1.0\x1b[0m"}
	rep := fanOut([]registryCall{{id: pm.Npm, timeout: time.Second, fn: func(context.Context) ([]Result, error) {
		return []Result{{Manager: pm.Npm, Name: "pkg\x1b[1m", Info: "\x1b[2Jevil", Extra: shared}}, nil
	}}})
	if len(rep.Results) != 1 {
		t.Fatalf("Results = %+v, want one", rep.Results)
	}
	r := rep.Results[0]
	if r.Name != "pkg" || r.Info != "evil" || r.Extra["version"] != "1.0" {
		t.Fatalf("result = %q %q %q, want sanitized", r.Name, r.Info, r.Extra["version"])
	}
	if shared["version"] != "1.0\x1b[0m" {
		t.Fatalf("shared Extra map was mutated: %q", shared["version"])
	}
}

func TestFanOutSanitizesRegistryErrors(t *testing.T) {
	sentinel := errors.New("boom")
	err := singleFailure(t, func(context.Context) ([]Result, error) {
		return nil, errors.Join(errors.New("bad \x1b]0;pwn\x07response"), sentinel)
	})
	if got := err.Error(); got != "bad response boom" {
		t.Fatalf("Err.Error() = %q, want %q", got, "bad response boom")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("sanitized error must still wrap the original")
	}
}
