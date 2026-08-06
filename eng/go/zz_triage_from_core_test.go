package eng

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	"strings"
	"testing"
)

// Test blocks re-homed by compiler-driven triage at the carve.

func TestBestEffortNoMatch(t *testing.T) {
	r := pnmRegistry(t, []Signature{pnmSig(-1, TInteger, TString), {Fallback: true}})
	fn := r.Lookup("pnmw")
	window := []Value{NewBoolean(true), NewBoolean(false)}
	if bestEffortNoMatch(r, nil, "pnmw", window, seam7Dbg, 0) != nil {
		t.Error("a nil fn must build no alt")
	}
	if bestEffortNoMatch(r, fn, "pnmw", nil, seam7Dbg, 0) != nil {
		t.Error("an empty window must build no alt")
	}
	// A mixed-arity table: another arity's collection could match at run
	// time, so the interpreter is not proven to fail — no alt.
	rMixed := pnmRegistry(t, []Signature{pnmSig(-1, TInteger, TString), pnmSig(-1, TInteger, TString, TBoolean)})
	if bestEffortNoMatch(rMixed, rMixed.Lookup("pnmw"), "pnmw", window, seam7Dbg, 0) != nil {
		t.Error("a mixed-arity table must build no alt")
	}
	alt := bestEffortNoMatch(r, fn, "pnmw", window, seam7Dbg, 0)
	if alt == nil || alt.Code != "signature_error" {
		t.Fatalf("alt = %v, want the rich signature_error", alt)
	}
	if !strings.Contains(alt.Error(), "the arguments were true (a Boolean) and false (a Boolean)") {
		t.Errorf("the alt must render the live window:\n%v", alt)
	}
	if alt.Row != seam7Dbg[0].Row {
		t.Errorf("alt Row = %d, want the stamped debug pos %d", alt.Row, seam7Dbg[0].Row)
	}
}
