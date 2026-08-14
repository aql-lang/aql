package native

// The await RESULT MODEL, exercised directly. The shapes it can prove come
// from doAwait's own statement order, and the two declines (a non-concrete
// parallels list, a mode whose residual is the winning branch's whole
// residual) are not reachable through the spec corpus — a run always has a
// concrete list, and `first` / `any` are non-deterministic. So they are
// driven here, against the same functions the handler dispatches through.

import "testing"

// wantResidual asserts a one-value residual whose carrier node is want.
func wantResidual(t *testing.T, label string, got []Value, want *Type) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("%s: got %d residual values, want 1", label, len(got))
	}
	if got[0].Parent != want {
		t.Errorf("%s: residual %v, want %v", label, got[0].Parent, want)
	}
}

func TestAwaitResidualDeclines(t *testing.T) {
	// A non-concrete parallels list: doAwait raises before it reads a mode,
	// so there is no residual to model.
	wantResidual(t, "carrier parallels", awaitResidual("all", NewCarrier(TList)), TAny)

	// first / any hand back the WINNING branch's whole residual — any type
	// at any arity — and an unknown mode raises. Neither is modellable.
	for _, mode := range []string{"first", "any", "bogus"} {
		wantResidual(t, "mode "+mode,
			awaitResidual(mode, NewList([]Value{NewList(nil)})), TAny)
	}
}

func TestAwaitResidualEmptyListPrecedesTheMode(t *testing.T) {
	// doAwait returns the empty List BEFORE the mode switch, so an empty
	// parallels list models as a List for every mode — including one with
	// no runner at all. This is the ordering the mirror has to honour.
	for _, mode := range []string{"all", "full", "first", "any", "bogus"} {
		wantResidual(t, "empty/"+mode, awaitResidual(mode, NewList(nil)), TList)
	}
}

// The emptiness predicate is shared by the mirror and the result model, and
// this is the test that keeps them shared: two copies of it drifted once
// already, and in the MIRROR the drift flags an error-severity diagnostic on
// a program doAwait runs to completion.
func TestAwaitParallelsEmptyMatchesTheHandler(t *testing.T) {
	// A NIL payload is empty, exactly as doAwait's len(Slice()) reads it.
	if !awaitParallelsEmpty(NewList(nil)) {
		t.Error("nil-backed list: not empty, but doAwait returns the empty List for it")
	}
	if !awaitParallelsEmpty(NewList([]Value{})) {
		t.Error("zero-length list: not empty")
	}
	if awaitParallelsEmpty(NewList([]Value{NewList(nil)})) {
		t.Error("one-branch list reads as empty — the mode switch would be skipped")
	}
	// A non-list cannot be asked for a length; the gate keeps it out, and the
	// predicate must not claim emptiness for it either.
	if awaitParallelsEmpty(NewInteger(5)) {
		t.Error("non-list reads as empty")
	}
}

func TestAwaitResidualKnownModes(t *testing.T) {
	nonEmpty := NewList([]Value{NewList(nil)})

	// full has no error escape: always a List, and a list of MAPS.
	full := awaitResidual("full", nonEmpty)
	wantResidual(t, "full", full, TList)
	ci, ok := full[0].Data.(ChildTypeInfo)
	if !ok || ci.Child.Parent != TMap {
		t.Errorf("full: element carrier %+v (typed=%v), want a Map", full[0].Data, ok)
	}

	// all is List-or-Error, named as a dynamic union so the claim does not
	// distribute over dispatch (a strict disjunct refuses `… get 0`).
	all := awaitResidual("all", nonEmpty)
	if len(all) != 1 {
		t.Fatalf("all: got %d residual values, want 1", len(all))
	}
	if !all[0].Dynamic {
		t.Errorf("all: residual is strict; a strict disjunct false-positives on list consumers")
	}
	di, err := AsDisjunct(all[0])
	if err != nil {
		t.Fatalf("all: residual is not a disjunct: %v", err)
	}
	// The alternatives are bare type NODES, so the node is ValueType, not
	// Parent — the same discrimination typeCovered makes before it decomposes
	// a disjunct. Compared with Equal, not pointer identity: ValueType hands
	// back the address of a by-value type literal, which is exactly the
	// non-canonical pointer CanonicalType exists to resolve.
	if len(di.Alternatives) != 2 ||
		!ValueType(di.Alternatives[0]).Equal(TList) ||
		!ValueType(di.Alternatives[1]).Equal(TError) {
		t.Errorf("all: alternatives %v, want [List Error]", di.Alternatives)
	}
}

func TestAwaitOptsReturnsDeclinesUnreadableOptions(t *testing.T) {
	// The mode has to be READ before it can select a shape: an options
	// carrier resolves nothing, and must NOT fall through to the "all"
	// default (that default belongs to a map that is present and simply
	// missing a mode key).
	wantResidual(t, "options carrier",
		awaitOptsReturns([]Value{NewCarrier(TOptions), NewList(nil)}, nil), TAny)

	// Short args cannot reach the reader either. A ReturnsFn is called with
	// the matched operands, so this is defensive — but the rule is that a
	// handler-side index is guarded, not that it is provably safe.
	wantResidual(t, "short args", awaitOptsReturns(nil, nil), TAny)
	wantResidual(t, "short args (1-arg form)", awaitDefaultReturns(nil, nil), TAny)
}

func TestAwaitModeNameFallsBackToDefault(t *testing.T) {
	if got := awaitModeName(NewString("full"), "all"); got != "full" {
		t.Errorf("String mode: %q, want full", got)
	}
	if got := awaitModeName(NewAtom("first"), "all"); got != "first" {
		t.Errorf("Atom mode: %q, want first", got)
	}
	// Neither String nor Atom: the handler keeps its default rather than
	// raising, and the model has to inherit that.
	if got := awaitModeName(NewInteger(5), "all"); got != "all" {
		t.Errorf("Integer mode: %q, want the default all", got)
	}
}

func TestAwaitRunnerTableMatchesTheMirror(t *testing.T) {
	for _, mode := range []string{"all", "full", "first", "any"} {
		if awaitRunner(mode) == nil {
			t.Errorf("awaitRunner(%q) is nil — the mirror would flag a mode that runs", mode)
		}
	}
	if awaitRunner("bogus") != nil {
		t.Error("awaitRunner(bogus) is non-nil — the mirror would miss a mode that raises")
	}
}
