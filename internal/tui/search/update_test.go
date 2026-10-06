package search

import (
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/crenspire/xpm/internal/pm"
	"github.com/crenspire/xpm/internal/search"
)

func TestInstallManagersFor(t *testing.T) {
	for id, want := range map[pm.ID]string{
		pm.Npm:   "[npm yarn pnpm bun]",
		pm.Pip:   "[pip poetry pipenv]",
		pm.Maven: "[maven gradle]",
		pm.Cargo: "[cargo]",
	} {
		if got := fmt.Sprint(installManagersFor(id)); got != want {
			t.Errorf("installManagersFor(%s) = %s, want %s", id, got, want)
		}
	}
}

func queryModel(t *testing.T) model {
	t.Helper()
	m := NewModel("", search.Options{}, UIOptions{DebounceMs: 1})
	m.registryMode = false
	return m
}

func press(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(model), cmd
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestQueryModeTypesQJK(t *testing.T) {
	m := queryModel(t)
	for _, k := range []string{"q", "j", "k"} {
		var cmd tea.Cmd
		m, cmd = press(t, m, runes(k))
		if isQuit(cmd) {
			t.Fatalf("typing %q quit the search", k)
		}
	}
	if m.query != "qjk" {
		t.Fatalf("query = %q, want qjk", m.query)
	}
}

func TestRegistryModeTypingQStartsTheQuery(t *testing.T) {
	m := NewModel("", search.Options{}, UIOptions{DebounceMs: 1})
	m, cmd := press(t, m, runes("q"))
	if isQuit(cmd) || m.registryMode || m.query != "q" {
		t.Fatalf("query=%q registryMode=%v quit=%v", m.query, m.registryMode, isQuit(cmd))
	}
}

func TestEscAndCtrlCQuit(t *testing.T) {
	for _, k := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		if _, cmd := press(t, queryModel(t), tea.KeyMsg{Type: k}); !isQuit(cmd) {
			t.Errorf("%v did not quit", k)
		}
	}
}

func TestArrowsAndCtrlKeysNavigate(t *testing.T) {
	m := queryModel(t)
	m.results = []search.Result{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlN})
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
}

func TestOnlyTheLatestKeystrokeSearches(t *testing.T) {
	var searched []string
	old := runSearch
	runSearch = func(q string, _ search.Options) ([]search.Result, error) {
		searched = append(searched, q)
		return []search.Result{{Manager: pm.Npm, Name: q}}, nil
	}
	t.Cleanup(func() { runSearch = old })

	m := queryModel(t)
	m, first := press(t, m, runes("a"))
	m, second := press(t, m, runes("x"))

	m, cmd := press(t, m, first().(debounceMsg))
	if cmd != nil {
		t.Fatal("the debounce of a superseded keystroke started a search")
	}
	m, cmd = press(t, m, second().(debounceMsg))
	if cmd == nil {
		t.Fatal("the latest keystroke did not start a search")
	}
	reply := cmd().(searchMsg)
	if len(searched) != 1 || searched[0] != "ax" {
		t.Fatalf("searched %v, want only [ax]", searched)
	}

	// A late reply for an older query must not overwrite newer results.
	stale := searchMsg{seq: reply.seq - 1, results: []search.Result{{Name: "a"}}}
	m, _ = press(t, m, stale)
	if len(m.results) != 0 || !m.loading {
		t.Fatalf("stale results applied: %+v", m.results)
	}
	m, _ = press(t, m, reply)
	if len(m.results) != 1 || m.results[0].Name != "ax" || m.loading {
		t.Fatalf("results = %+v loading=%v", m.results, m.loading)
	}
}

func TestBackspaceRemovesAWholeCharacter(t *testing.T) {
	m := queryModel(t)
	m.query = "café"
	m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.query != "caf" {
		t.Fatalf("query = %q, want caf", m.query)
	}
}

func TestTruncateByRune(t *testing.T) {
	got := truncate("日本語のパッケージ名", 5)
	if got != "日本..." || !utf8.ValidString(got) {
		t.Fatalf("truncate = %q", got)
	}
	if truncate("héllo", 10) != "héllo" || truncate("héllo", 0) != "" {
		t.Fatal("short strings must be unchanged and max 0 must give empty")
	}
}

func TestUIOptionsAreUsed(t *testing.T) {
	m := NewModel("", search.Options{}, UIOptions{DebounceMs: 50, PageSize: 10})
	m.height = 50
	if m.debounce != 50*time.Millisecond || m.visibleRows() != 10 {
		t.Fatalf("debounce=%v rows=%d, want 50ms and 10", m.debounce, m.visibleRows())
	}
	m = NewModel("", search.Options{}, UIOptions{})
	m.height = 50
	if m.debounce != defaultDebounce || m.visibleRows() != 45 {
		t.Fatalf("defaults: debounce=%v rows=%d", m.debounce, m.visibleRows())
	}
}
