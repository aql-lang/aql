package modules

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// fdFn constructs a real fn VALUE by running the given `fn […]` source — the
// same values the recorded `parse <fn>` dispatch receives at run time.
func fdFn(t *testing.T, r *native.Registry, src string) native.Value {
	t.Helper()
	out := mcovRun(t, r, src)
	if len(out) != 1 {
		t.Fatalf("fn construction: expected 1 value, got %d", len(out))
	}
	return out[0]
}

// TestFnDispatchArms drives parseFnDispatchHandler's defensive arms directly
// — the compiled corpus covers the happy and bad-signature paths end-to-end
// (module-parselang.tsv §10 computed rows); these pin the arms a recorded
// call shape cannot reach in the corpus: a non-function operand, a run error
// inside the dispatched parser, and a drifted result count.
func TestFnDispatchArms(t *testing.T) {
	r := mcovReg(t)
	opts := s7bMap()

	// Non-FnDefInfo operand → parse_error.
	if _, err := parseFnDispatchHandler([]native.Value{native.NewInteger(5), native.NewString("s"), opts}, nil, nil, r); err == nil {
		t.Error("fn dispatch: a non-function operand should error")
	}

	// A fn without the standard [source opts] prefix → parse_bad_signature.
	bad := fdFn(t, r, `fn [[a:Integer] [Integer] [a]]`)
	if _, err := parseFnDispatchHandler([]native.Value{bad, native.NewString("s"), opts}, nil, nil, r); err == nil {
		t.Error("fn dispatch: a non-parser signature should error")
	}

	// A conforming fn whose body raises → the run error propagates.
	ferr := fdFn(t, r, `fn [[source:Any opts:Map] [Any] [fd_undefined_word_xyz]]`)
	if _, err := parseFnDispatchHandler([]native.Value{ferr, native.NewString("s"), opts}, nil, nil, r); err == nil {
		t.Error("fn dispatch: a run error should propagate")
	}

	// A multi-return signature → parse_bad_signature (the single-result
	// contract is enforced up front; the downstream count arm is a
	// covergate-allowed drift guard).
	ftwo := fdFn(t, r, `fn [[source:Any opts:Map] [Any Any] [1 2]]`)
	if _, err := parseFnDispatchHandler([]native.Value{ftwo, native.NewString("s"), opts}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "parse_bad_signature") {
		t.Errorf("fn dispatch: a multi-return signature should be parse_bad_signature, got %v", err)
	}

	// The happy path: a conforming identity parser echoes its source.
	fid := fdFn(t, r, `fn [[source:Any opts:Map] [Any] [source]]`)
	out, err := parseFnDispatchHandler([]native.Value{fid, native.NewString("s"), opts}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("fn dispatch: identity parser failed: out=%v err=%v", out, err)
	}
	if got, cerr := out[0].AsConcreteString(); cerr != nil || got != "s" {
		t.Errorf("fn dispatch: identity parser = %v, want 's'", out[0])
	}
}
