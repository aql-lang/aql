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

	// The happy path: a conforming identity parser echoes its source. This is
	// also the CALLBACK lane — a real boru body, offered to its compiled unit
	// before CallBoru.
	fid := fdFn(t, r, `fn [[source:Any opts:Map] [Any] [source]]`)
	out, err := parseFnDispatchHandler([]native.Value{fid, native.NewString("s"), opts}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("fn dispatch: identity parser failed: out=%v err=%v", out, err)
	}
	if got, cerr := out[0].AsConcreteString(); cerr != nil || got != "s" {
		t.Errorf("fn dispatch: identity parser = %v, want 's'", out[0])
	}
}

// TestFnDispatchTokenRunLane pins the two shapes that still need the TOKEN RUN
// — the sub-engine step of `fn source opts end` — now that a conforming parser
// takes either the delegation lane or the callback lane. Both raise exactly what
// the interpreter raises for the same call, which is the property that lets the
// run stay a backstop rather than a third behaviour.
func TestFnDispatchTokenRunLane(t *testing.T) {
	r := mcovReg(t)
	opts := s7bMap()

	// (1) An OVER-WIDE signature. ParseLangFnSigWhy admits extra params after
	// the [source opts] prefix, so a three-param parser passes the contract
	// check and then matches nothing against the handler's two operands —
	// MatchFnSig answers nil, the token run steps `fn source opts end`, the fn
	// applies to nothing, and all THREE values survive as data. That is the
	// single-result guard's real reachable case.
	fwide := fdFn(t, r, `fn [[source:Any opts:Map extra:Integer] [Any] [source]]`)
	if _, err := parseFnDispatchHandler([]native.Value{fwide, native.NewString("s"), opts}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected one result, got 3") {
		t.Errorf("fn dispatch: an over-wide parser signature should fail the single-result guard, got %v", err)
	}

	// (2) A DELEGATION wrapper whose inner native does not resolve. The shape
	// is what Parse.parser mints — unnamed params, a body of one Word — but the
	// word names nothing in the wrapper's registry, so parseFnNativeApply
	// declines and the token run reports the missing word.
	deleg := native.NewFunction(native.FnDefInfo{
		Name:     "fd_missing_inner_xyz",
		Registry: r,
		Signatures: []native.Signature{{
			Params:     []native.FnParam{{Type: native.TAny}, {Type: native.TMap}},
			Returns:    []*native.Type{native.TAny},
			Impl:       native.Boru([]native.Value{native.NewWord("fd_missing_inner_xyz")}),
			BarrierPos: -1,
		}},
	})
	if _, err := parseFnDispatchHandler([]native.Value{deleg, native.NewString("s"), opts}, nil, nil, r); err == nil {
		t.Error("fn dispatch: a delegation wrapper with no inner native should error")
	}

	// (3) A conforming signature the OPERANDS do not fit: the declared
	// source:String against an Integer. Neither lane matches, and the token run
	// raises the interpreter's own dispatch error.
	fstr := fdFn(t, r, `fn [[source:String opts:Map] [Any] [source]]`)
	if _, err := parseFnDispatchHandler([]native.Value{fstr, native.NewInteger(5), opts}, nil, nil, r); err == nil {
		t.Error("fn dispatch: a String-source parser given an Integer should error")
	}
}
