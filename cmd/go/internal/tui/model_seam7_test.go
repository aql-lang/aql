package tui

import "testing"

// TestS7n_TickCmdFiresAfterInterval invokes the tea.Cmd returned by
// tickCmd, which blocks until refreshInterval elapses and then calls
// the inner closure that produces a tickMsg — the closure body
// (model.go) isn't exercised just by driving Update(tickMsg{}), which
// only ever receives an already-constructed message.
func TestS7n_TickCmdFiresAfterInterval(t *testing.T) {
	m := newModel(nil)
	msg := m.tickCmd()()
	if _, ok := msg.(tickMsg); !ok {
		t.Errorf("tickCmd() produced %T, want tickMsg", msg)
	}
}

// TestS7n_UpdateUnknownMsgFallsThrough drives Update's final
// "return m, nil" fallback for a message type matching none of the
// switch cases.
func TestS7n_UpdateUnknownMsgFallsThrough(t *testing.T) {
	m := newModel(nil)
	type unknownMsg struct{}

	next, cmd := m.Update(unknownMsg{})
	if cmd != nil {
		t.Errorf("Update(unknownMsg) cmd = %v, want nil", cmd)
	}
	if got, ok := next.(*model); !ok || got != m {
		t.Errorf("Update(unknownMsg) model = %v, want the same *model unchanged", next)
	}
}
