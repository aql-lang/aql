package eng

import (
	"strings"
	"testing"
)

// TestAqlErrorRendersKnownPosition checks that a known source position is
// rendered as the jsonic-style arrow.
func TestAqlErrorRendersKnownPosition(t *testing.T) {
	e := makeAqlErrorAt("type_error", "boom", "w", "", "", SrcPos{Row: 4, Col: 7})
	msg := e.Error()
	if !strings.Contains(msg, "\n  --> 4:7") {
		t.Errorf("expected arrow `--> 4:7`, got:\n%s", msg)
	}
	if strings.Contains(msg, "unknown") {
		t.Errorf("known position should not render 'unknown', got:\n%s", msg)
	}
}

// TestAqlErrorRendersUnknownPosition checks that a position-less error says
// so explicitly rather than guessing a location by text-searching the word.
func TestAqlErrorRendersUnknownPosition(t *testing.T) {
	// "boom" appears in the source, but with no SrcPos there is NO
	// text-search fallback — the error states the position is unknown.
	e := makeAqlErrorAt("type_error", "boom happened", "boom", "x boom y\nboom z", "", SrcPos{})
	msg := e.Error()
	if !strings.Contains(msg, "--> source position unknown") {
		t.Errorf("expected 'source position unknown', got:\n%s", msg)
	}
	if strings.Contains(msg, "1:") || strings.Contains(msg, "2:") {
		t.Errorf("position-less error must not invent a row:col, got:\n%s", msg)
	}
}
