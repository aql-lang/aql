package native

import (
	"errors"
	"testing"
)

// Finding E — the interpreter's `for` must error on a NON-concrete bound the
// same way the VM's OpForSetup does (for_error), instead of silently coercing
// it to zero and running the loop zero times. Without the fix a DepScalar /
// carrier count looped zero times in the interpreter while the compiled VM
// raised for_error — a silent divergence.
func TestForCountRejectsNonConcrete(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	body := NewList([]Value{NewWord("i")})

	// Negative: a non-concrete count (a bare carrier matched to the Integer
	// slot) is a for_error, not a silent zero-iteration run.
	_, err = forCountHandler([]Value{NewCarrier(TInteger), body}, nil, nil, r)
	if err == nil {
		t.Fatal("non-concrete count returned no error (silent zero-iteration)")
	}
	if ae := asBoruErr(err); ae == nil || ae.Code != "for_error" {
		t.Fatalf("non-concrete count error = %v, want for_error", err)
	}

	// Positive: a concrete count still runs.
	if _, err := forCountHandler([]Value{NewInteger(3), body}, nil, nil, r); err != nil {
		t.Fatalf("concrete count errored: %v", err)
	}
}

// Finding E — parseRange rejects a non-concrete bound (matching the VM) rather
// than swallowing the coercion error and treating it as zero.
func TestParseRangeRejectsNonConcrete(t *testing.T) {
	// Negative: a carrier element is not a concrete integer.
	if _, _, _, err := parseRange([]Value{NewCarrier(TInteger)}); err == nil {
		t.Fatal("parseRange accepted a non-concrete bound")
	}
	if _, _, _, err := parseRange([]Value{NewInteger(0), NewCarrier(TInteger)}); err == nil {
		t.Fatal("parseRange accepted a non-concrete end bound")
	}
	// Positive: concrete bounds still parse.
	start, end, step, err := parseRange([]Value{NewInteger(2), NewInteger(8), NewInteger(2)})
	if err != nil || start != 2 || end != 8 || step != 2 {
		t.Fatalf("parseRange concrete = (%d,%d,%d) err=%v, want (2,8,2) nil", start, end, step, err)
	}
}

// asBoruErr unwraps a BoruError for code assertions; nil if not one.
func asBoruErr(err error) *BoruError {
	var ae *BoruError
	if errors.As(err, &ae) {
		return ae
	}
	return nil
}
