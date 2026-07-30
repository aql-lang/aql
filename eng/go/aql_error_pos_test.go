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

// AqlErrorAt is the positioned twin of AqlError: it carries the caller's
// SrcPos through to the rendered arrow, where AqlError reports Row 0 and
// renders "source position unknown". The pair is what lets a handler blame
// the argument that actually failed instead of shrugging.
func TestRegistryAqlErrorAtCarriesPosition(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.Source = "one\ntwo three\n"

	at := r.AqlErrorAt("read_error", "read: boom", "read", SrcPos{Row: 2, Col: 5})
	if !strings.Contains(at.Error(), "--> 2:5") {
		t.Errorf("AqlErrorAt lost the position:\n%s", at.Error())
	}
	// The negative half: the unpositioned constructor must still say so,
	// rather than text-searching for a plausible location.
	plain := r.AqlError("read_error", "read: boom", "read")
	if !strings.Contains(plain.Error(), "source position unknown") {
		t.Errorf("AqlError should report an unknown position:\n%s", plain.Error())
	}
}

// AqlErrorHintAt carries both the hint and the position.
func TestRegistryAqlErrorHintAtCarriesBoth(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	e := r.AqlErrorHintAt("write_error", "write: boom", "write", "check the path", SrcPos{Row: 7, Col: 2})
	msg := e.Error()
	if !strings.Contains(msg, "--> 7:2") {
		t.Errorf("lost the position:\n%s", msg)
	}
	if !strings.Contains(msg, "check the path") {
		t.Errorf("lost the hint:\n%s", msg)
	}
}

// Both constructors tolerate a nil registry, like their unpositioned
// siblings — a handler that errors before its registry is wired must not
// turn a diagnosable failure into a panic (ADR-005).
func TestRegistryAqlErrorAtNilRegistry(t *testing.T) {
	var r *Registry
	if e := r.AqlErrorAt("c", "d", "w", SrcPos{Row: 1, Col: 1}); e == nil {
		t.Error("AqlErrorAt on a nil registry returned nil")
	}
	if e := r.AqlErrorHintAt("c", "d", "w", "h", SrcPos{Row: 1, Col: 1}); e == nil {
		t.Error("AqlErrorHintAt on a nil registry returned nil")
	}
}
