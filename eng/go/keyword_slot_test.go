package eng

// KEYWORD slots: a /q signature position carrying a concrete Atom
// pattern admits exactly one literal word, matched binding-agnostically
// by token NAME (patternsOk) — the mechanism behind def's
// `[name/q fn/q sigs]` form. These tests pin the matcher branch and the
// plan-scan capture gate (capturesForwardToken) directly, mirroring the
// white-box style of engine_seam7_barrier_test.go.

import "testing"

// kwSig builds a def-shaped keyword signature: [Atom/q, Atom/q=lit, List].
func kwSig(lit string) *Signature {
	return &Signature{
		Args:       []*Type{TAtom, TAtom, TList},
		QuoteArgs:  map[int]bool{0: true, 1: true},
		Patterns:   map[int]Value{1: NewAtom(lit)},
		BarrierPos: -1,
	}
}

// --- patternsOk keyword branch ---------------------------------------------

// patternsTape lays out [name-word, tok] and reports patternsOk with both
// positions forward-matched.
func kwPatternsOk(t *testing.T, sig *Signature, tok Value) bool {
	t.Helper()
	tape := NewTape([]Value{NewWord("g"), tok}, stackHeadroom)
	return patternsOk(sig, []int{0, 1}, tape, 2, nil)
}

func TestKeywordSlotMatchesLiteralWord(t *testing.T) {
	// The raw token NAME matches — even though `fn`-like words are
	// normally Defs-bound to a dispatching value, /q capture is
	// binding-agnostic and the pattern must never see the binding.
	if !kwPatternsOk(t, kwSig("fn"), NewWord("fn")) {
		t.Error("keyword slot must match the literal word token by name")
	}
}

func TestKeywordSlotMatchesAtom(t *testing.T) {
	// A stack/arrival position may already hold the converted Atom.
	if !kwPatternsOk(t, kwSig("fn"), NewAtom("fn")) {
		t.Error("keyword slot must match an Atom of the pattern's name")
	}
}

func TestKeywordSlotRejectsOtherWord(t *testing.T) {
	if kwPatternsOk(t, kwSig("fn"), NewWord("gen")) {
		t.Error("keyword slot must reject a word with a different name")
	}
}

func TestKeywordSlotRejectsNonWord(t *testing.T) {
	// A value (a pre-evaluated group's Function, a literal) can never
	// satisfy a keyword slot.
	if kwPatternsOk(t, kwSig("fn"), NewInteger(7)) {
		t.Error("keyword slot must reject a non-word, non-atom value")
	}
}

// --- capturesForwardToken ----------------------------------------------------

func kwFnDef(lit string) *FnDefInfo {
	return &FnDefInfo{Signatures: []Signature{*kwSig(lit)}}
}

func TestCapturesForwardTokenKeywordMatch(t *testing.T) {
	if !capturesForwardToken(kwFnDef("fn"), 1, NewWord("fn")) {
		t.Error("the matching keyword token is captured, not a barrier")
	}
}

func TestCapturesForwardTokenKeywordMiss(t *testing.T) {
	if capturesForwardToken(kwFnDef("fn"), 1, NewWord("add")) {
		t.Error("a non-matching word keeps its barrier treatment")
	}
}

func TestCapturesForwardTokenUnpatternedQuote(t *testing.T) {
	// Position 0 is a plain /q slot — captures ANY word, as before.
	if !capturesForwardToken(kwFnDef("fn"), 0, NewWord("anything")) {
		t.Error("an unpatterned /q slot captures unconditionally")
	}
}

func TestCapturesForwardTokenRawSlot(t *testing.T) {
	fn := &FnDefInfo{Signatures: []Signature{{
		Args:       []*Type{TAny},
		RawParens:  map[int]bool{0: true},
		BarrierPos: -1,
	}}}
	if !capturesForwardToken(fn, 0, NewWord("w")) {
		t.Error("raw/form/type slots capture unconditionally")
	}
}

func TestCapturesForwardTokenNoCapture(t *testing.T) {
	fn := &FnDefInfo{Signatures: []Signature{{
		Args:       []*Type{TInteger},
		BarrierPos: -1,
	}}}
	if capturesForwardToken(fn, 0, NewWord("w")) {
		t.Error("a plain typed slot never captures a word structurally")
	}
}
