package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestModelActionKeys drives the p/v/x keys against a fake api and
// executes the returned commands, covering actionCmd and selected.
func TestModelActionKeys(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)
	m := newModel(newAPIClient(ts.URL, ""))
	m.services = []serviceEntity{{Name: "registry", State: "running"}}

	for key, action := range map[string]string{"p": "pause", "r": "resume", "x": "stop"} {
		_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd == nil {
			t.Fatalf("%q should produce an action command", key)
		}
		msg, ok := cmd().(actionResultMsg)
		if !ok {
			t.Fatalf("%q cmd returned %T, want actionResultMsg", key, msg)
		}
		if msg.action != action || msg.name != "registry" || msg.err != nil {
			t.Errorf("%q result = %+v", key, msg)
		}
		if f.lastBody["action"] != action {
			t.Errorf("%q: server saw action %q", key, f.lastBody["action"])
		}
	}
}

func TestModelActionKeysNoSelection(t *testing.T) {
	m := newModel(nil) // no services: selected() is !ok
	for _, key := range []string{"p", "r", "x"} {
		_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd != nil {
			t.Errorf("%q with no services should be a no-op", key)
		}
	}
}

func TestModelArrowKeys(t *testing.T) {
	m := newModel(nil)
	m.services = []serviceEntity{{Name: "a"}, {Name: "b"}}
	m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor after down = %d, want 1", m.cursor)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor after up = %d, want 0", m.cursor)
	}
}

func TestModelTickTriggersPoll(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)
	m := newModel(newAPIClient(ts.URL, ""))
	next, cmd := m.Update(tickMsg{})
	if next == nil || cmd == nil {
		t.Fatal("tick should schedule a poll+tick batch")
	}
}

func TestModelInitReturnsBatch(t *testing.T) {
	m := newModel(newAPIClient("http://127.0.0.1:1", ""))
	if m.Init() == nil {
		t.Error("Init should return the initial poll batch")
	}
}

func TestModelPollCmdSuccessAndError(t *testing.T) {
	f := &fakeAPI{}
	ts := f.start(t)

	ok := newModel(newAPIClient(ts.URL, ""))
	msg := ok.pollCmd()()
	sm, isSvc := msg.(servicesMsg)
	if !isSvc {
		t.Fatalf("pollCmd returned %T, want servicesMsg", msg)
	}
	if len(sm.services) != 1 || sm.server == nil {
		t.Errorf("servicesMsg = %+v", sm)
	}

	bad := newModel(newAPIClient("http://127.0.0.1:1", ""))
	if _, isErr := bad.pollCmd()().(errMsg); !isErr {
		t.Error("pollCmd against a dead api should return errMsg")
	}
}

func TestModelViewStoppedAndUnknownStates(t *testing.T) {
	m := newModel(nil)
	m.services = []serviceEntity{
		{Name: "a", State: "stopped"},
		{Name: "b", State: "starting"},
	}
	m.server = &serverInfo{PID: 7, Version: "v", UptimeSeconds: 1, ServiceCount: 2}
	out := m.View()
	for _, want := range []string{"stopped", "starting", "pid=7"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q:\n%s", want, out)
		}
	}
}

func TestModelViewEmptyServices(t *testing.T) {
	m := newModel(nil)
	if out := m.View(); !strings.Contains(out, "no services") {
		t.Errorf("empty View should note no services:\n%s", out)
	}
}

func TestModelStatusLineShownWithoutError(t *testing.T) {
	m := newModel(nil)
	m.status = "pause a: ok"
	if out := m.View(); !strings.Contains(out, "pause a: ok") {
		t.Errorf("View should include the status line:\n%s", out)
	}
}

func TestFormatMetadataEmpty(t *testing.T) {
	if got := formatMetadata(nil); got != "" {
		t.Errorf("formatMetadata(nil) = %q, want empty", got)
	}
}

func TestMax0(t *testing.T) {
	if max0(-3) != 0 || max0(0) != 0 || max0(4) != 4 {
		t.Error("max0 clamps negatives to zero")
	}
}
