package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelHandleKeyQuit(t *testing.T) {
	m := newModel(nil)
	for _, key := range []string{"q", "ctrl+c"} {
		_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if key == "ctrl+c" {
			_, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
		}
		if cmd == nil {
			t.Errorf("%q should produce a Quit cmd", key)
			continue
		}
		// Calling the cmd should return tea.QuitMsg.
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("%q cmd returned %T, want tea.QuitMsg", key, msg)
		}
	}
}

func TestModelCursorMovement(t *testing.T) {
	m := newModel(nil)
	m.services = []serviceEntity{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	}
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}

	// Down past end is clamped.
	for i := 0; i < 5; i++ {
		m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	if m.cursor != 2 {
		t.Errorf("after 5 down: cursor = %d, want 2", m.cursor)
	}

	// Up past 0 is clamped.
	for i := 0; i < 5; i++ {
		m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	}
	if m.cursor != 0 {
		t.Errorf("after 5 up: cursor = %d, want 0", m.cursor)
	}
}

func TestModelViewRendersServices(t *testing.T) {
	m := newModel(nil)
	m.services = []serviceEntity{
		{Name: "registry", State: "running", Metadata: map[string]string{"addr": ":8080"}},
		{Name: "lsp", State: "paused", Metadata: map[string]string{"mode": "tcp"}},
	}
	out := m.View()
	for _, want := range []string{"registry", "lsp", "running", "paused"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q: %s", want, out)
		}
	}
}

func TestModelServicesMsgUpdatesAndClampsCursor(t *testing.T) {
	m := newModel(nil)
	m.services = []serviceEntity{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.cursor = 2

	// Shrink the list; cursor should clamp.
	next, _ := m.Update(servicesMsg{services: []serviceEntity{{Name: "a"}}})
	got := next.(*model)
	if got.cursor != 0 {
		t.Errorf("cursor = %d, want 0 after shrinking to 1 service", got.cursor)
	}
}

func TestModelErrorMessageShows(t *testing.T) {
	m := newModel(nil)
	next, _ := m.Update(errMsg{err: testErr("boom")})
	got := next.(*model)
	if got.err != "boom" {
		t.Errorf("err = %q, want boom", got.err)
	}
	if !strings.Contains(got.View(), "boom") {
		t.Errorf("view should include error: %s", got.View())
	}
}

func TestModelActionResultMessageShows(t *testing.T) {
	m := newModel(nil)
	next, _ := m.Update(actionResultMsg{name: "a", action: "pause", err: nil})
	got := next.(*model)
	if !strings.Contains(got.status, "pause a: ok") {
		t.Errorf("status = %q", got.status)
	}

	next, _ = next.Update(actionResultMsg{name: "a", action: "pause", err: testErr("nope")})
	got = next.(*model)
	if !strings.Contains(got.status, "pause a: nope") {
		t.Errorf("status = %q", got.status)
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }
