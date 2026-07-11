package eng

import (
	"strings"
	"testing"
)

// The explain pass runs only after a dispatch has already failed; these
// tests pin its verdicts per failure kind, its ranking, and the honesty
// rules (skip fallbacks, skip overloads it cannot blame) — paired with
// the negative cases (nil fn, quoted slots, carrier patterns) that must
// NOT produce a verdict.

func sigOf(types ...*Type) Signature {
	return Signature{Args: types, BarrierPos: -1}
}

func TestExplainCandidatesNilAndFallback(t *testing.T) {
	if got := explainCandidates(nil, nil); got != nil {
		t.Errorf("nil fn → nil, got %v", got)
	}
	fn := &FnDefInfo{Signatures: []Signature{{Fallback: true, BarrierPos: 0}}}
	if got := explainCandidates(fn, []Value{NewInteger(1)}); len(got) != 0 {
		t.Errorf("fallback sigs must be skipped, got %v", got)
	}
}

func TestExplainCandidatesArity(t *testing.T) {
	// (String) probed with 2 args: arity failure, score 0 (Integer
	// misses the leading String slot).
	fn := &FnDefInfo{Signatures: []Signature{sigOf(TString)}}
	written := []Value{NewInteger(1), NewInteger(2)}
	fails := explainCandidates(fn, written)
	if len(fails) != 1 || fails[0].Kind != candArity || fails[0].SlotIdx != -1 {
		t.Fatalf("want one arity failure, got %+v", fails)
	}
	if fails[0].Score != 0 {
		t.Errorf("score = %d, want 0", fails[0].Score)
	}
	// (Integer, Integer, Integer) probed with 2 matching args: the
	// leading matches count toward the score.
	fn3 := &FnDefInfo{Signatures: []Signature{sigOf(TInteger, TInteger, TInteger)}}
	fails = explainCandidates(fn3, written)
	if len(fails) != 1 || fails[0].Kind != candArity || fails[0].Score != 2 {
		t.Fatalf("want arity failure with score 2, got %+v", fails)
	}
}

func TestExplainCandidatesSlotType(t *testing.T) {
	fn := &FnDefInfo{Signatures: []Signature{sigOf(TString, TInteger)}}
	written := []Value{NewString("x"), NewString("y")}
	fails := explainCandidates(fn, written)
	if len(fails) != 1 {
		t.Fatalf("want one failure, got %+v", fails)
	}
	f := fails[0]
	if f.Kind != candSlotType || f.SlotIdx != 1 || !f.Expected.Equal(TInteger) || f.Score != 1 {
		t.Errorf("verdict = %+v, want slot 1 expected Integer score 1", f)
	}
	if s, _ := AsString(f.Got); s != "y" {
		t.Errorf("Got = %v, want 'y'", f.Got)
	}
}

func TestExplainCandidatesRanking(t *testing.T) {
	// Two overloads: the (String, Integer) one matches the leading slot
	// (score 1) and must rank above the (Integer, Integer) one (score 0).
	fn := &FnDefInfo{Signatures: []Signature{
		sigOf(TInteger, TInteger),
		sigOf(TString, TInteger),
	}}
	written := []Value{NewString("x"), NewString("y")}
	fails := explainCandidates(fn, written)
	if len(fails) != 2 {
		t.Fatalf("want two failures, got %+v", fails)
	}
	if fails[0].Score != 1 || fails[1].Score != 0 {
		t.Errorf("ranking = [%d %d], want nearest-first [1 0]", fails[0].Score, fails[1].Score)
	}
}

func TestExplainCandidatesQuotedSlotAdmits(t *testing.T) {
	// A QuoteArgs slot binds a bare name without a type match — it can
	// never be the blamed slot, and it counts as admitting in the arity
	// score.
	sig := sigOf(TAtom, TInteger)
	sig.QuoteArgs = map[int]bool{0: true}
	fn := &FnDefInfo{Signatures: []Signature{sig}}
	written := []Value{NewInteger(9), NewString("y")}
	fails := explainCandidates(fn, written)
	if len(fails) != 1 || fails[0].Kind != candSlotType || fails[0].SlotIdx != 1 {
		t.Fatalf("quoted slot must be skipped; want slot-1 verdict, got %+v", fails)
	}
	// Arity path: quoted slot counts as a leading match.
	fnA := &FnDefInfo{Signatures: []Signature{sig}}
	fails = explainCandidates(fnA, []Value{NewInteger(9)})
	if len(fails) != 1 || fails[0].Kind != candArity || fails[0].Score != 1 {
		t.Fatalf("want arity failure scoring the quoted slot, got %+v", fails)
	}
}

func TestExplainCandidatesFullAdmitSkipped(t *testing.T) {
	// Every slot admits the tuple positionally: the probe cannot blame
	// this overload and must stay silent rather than fabricate a reason.
	fn := &FnDefInfo{Signatures: []Signature{sigOf(TInteger, TInteger)}}
	written := []Value{NewInteger(1), NewInteger(2)}
	if fails := explainCandidates(fn, written); len(fails) != 0 {
		t.Errorf("fully-admitting overload must be skipped, got %+v", fails)
	}
}

func TestExplainCandidatesPatternFailures(t *testing.T) {
	// Value-literal pattern: slot type admits, pattern rejects.
	lit := sigOf(TInteger)
	litPat := NewInteger(0)
	lit.Patterns = map[int]Value{0: litPat}
	fn := &FnDefInfo{Signatures: []Signature{lit}}
	fails := explainCandidates(fn, []Value{NewInteger(5)})
	if len(fails) != 1 || fails[0].Kind != candPattern || fails[0].SlotIdx != 0 {
		t.Fatalf("want a pattern verdict, got %+v", fails)
	}
	// The matching value passes the pattern — no verdict.
	if fails := explainCandidates(fn, []Value{NewInteger(0)}); len(fails) != 0 {
		t.Errorf("pattern-passing tuple must not be blamed, got %+v", fails)
	}

	// Negation pattern ((tnot List) — fn's triple form): a List operand
	// is rejected via the direct unifyNegation path.
	neg := sigOf(TAny)
	neg.Patterns = map[int]Value{0: NewNegation(NewTypeLiteral(TList))}
	fnNeg := &FnDefInfo{Signatures: []Signature{neg}}
	fails = explainCandidates(fnNeg, []Value{NewList([]Value{NewInteger(1)})})
	if len(fails) != 1 || fails[0].Kind != candPattern {
		t.Fatalf("want a negation-pattern verdict, got %+v", fails)
	}
	if fails := explainCandidates(fnNeg, []Value{NewInteger(1)}); len(fails) != 0 {
		t.Errorf("negation-passing tuple must not be blamed, got %+v", fails)
	}
}

func TestPatternRejectsEdges(t *testing.T) {
	// Carrier pattern (a check-mode placeholder for a computed pattern)
	// can never reject.
	carrier := NewCarrier(TInteger)
	if patternRejects(carrier, NewInteger(1)) {
		t.Error("carrier pattern must not reject")
	}
	// Structural map pattern: open (subset) matching.
	mkMap := func(kv ...any) Value {
		m := NewOrderedMap()
		for i := 0; i < len(kv); i += 2 {
			m.Set(kv[i].(string), kv[i+1].(Value))
		}
		return NewMap(m)
	}
	pat := mkMap("kind", NewString("api"))
	yes := mkMap("kind", NewString("api"), "x", NewInteger(1))
	no := mkMap("kind", NewString("db"))
	if patternRejects(pat, yes) {
		t.Error("open map match must admit a superset map")
	}
	if !patternRejects(pat, no) {
		t.Error("open map match must reject a contradicting map")
	}
}

// --- diag_msg.go builders ---------------------------------------------------

func TestNoMatchDetailAndSuggestionTexts(t *testing.T) {
	if got := noMatchDetail("pad"); got != "cannot call `pad` — no signature matches the arguments" {
		t.Errorf("noMatchDetail = %q", got)
	}
	if got := insufficientArgsDetail("f", 1); !strings.Contains(got, "1 forward argument but") {
		t.Errorf("singular detail = %q", got)
	}
	if got := insufficientArgsDetail("f", 2); !strings.Contains(got, "2 forward arguments but") {
		t.Errorf("plural detail = %q", got)
	}
	if got := forwardParensSuggestion("f"); !strings.Contains(got, "(f …)") {
		t.Errorf("parens suggestion = %q", got)
	}
	if got := describeSuggestion("pad"); got != "see `aql describe pad` for its signatures and examples" {
		t.Errorf("describe suggestion = %q", got)
	}
}

func TestCallArgsNote(t *testing.T) {
	if got := callArgsNote(nil); got != "" {
		t.Errorf("empty tuple → empty note, got %q", got)
	}
	one := callArgsNote([]Value{NewInteger(99)})
	if one != "the argument was 99 (an Integer)" {
		t.Errorf("one arg = %q", one)
	}
	three := callArgsNote([]Value{NewString("x"), NewInteger(2), NewBoolean(true)})
	if three != "the arguments were 'x' (a ProperString), 2 (an Integer) and true (a Boolean)" {
		t.Errorf("three args = %q", three)
	}
}

func TestTypeArticle(t *testing.T) {
	cases := map[string]string{"Integer": "an", "String": "a", "Atom": "an", "": "a", "int": "an", "atom": "an"}
	for name, want := range cases {
		if got := typeArticle(name); got != want {
			t.Errorf("typeArticle(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCandidateNotes(t *testing.T) {
	if got := candidateNotes("f", nil, 3, 1); got != nil {
		t.Errorf("no failures → nil, got %v", got)
	}
	// Arity verdicts across supplied counts (none / 1 was / N were) and
	// singular takes-1.
	one := sigOf(TString)
	two := sigOf(TString, TInteger)
	fails := []CandidateFailure{{Sig: &one, Kind: candArity, SlotIdx: -1}}
	if got := candidateNotes("f", fails, 1, 0); !strings.Contains(got[0], "takes 1 argument, but none were supplied") {
		t.Errorf("zero-supplied = %q", got[0])
	}
	fails = []CandidateFailure{{Sig: &two, Kind: candArity, SlotIdx: -1}}
	if got := candidateNotes("f", fails, 1, 1); !strings.Contains(got[0], "takes 2 arguments, but 1 was supplied") {
		t.Errorf("one-supplied = %q", got[0])
	}
	if got := candidateNotes("f", fails, 1, 3); !strings.Contains(got[0], "takes 2 arguments, but 3 were supplied") {
		t.Errorf("three-supplied = %q", got[0])
	}
	// Slot verdict.
	fails = []CandidateFailure{{Sig: &two, Kind: candSlotType, SlotIdx: 1,
		Expected: TInteger, Got: NewString("y")}}
	got := candidateNotes("f", fails, 1, 2)
	if !strings.Contains(got[0], "candidate `f (String, Integer)` — argument 2: expected Integer, got 'y' (a ProperString)") {
		t.Errorf("slot verdict = %q", got[0])
	}
	// Pattern verdict.
	fails = []CandidateFailure{{Sig: &one, Kind: candPattern, SlotIdx: 0,
		Got: NewInteger(5), Pattern: NewInteger(0)}}
	got = candidateNotes("f", fails, 1, 1)
	if !strings.Contains(got[0], "argument 1: 5 does not satisfy its declared pattern 0") {
		t.Errorf("pattern verdict = %q", got[0])
	}
	// Truncation: 5 failures over 6 overloads → 3 shown + "…and 3 more".
	many := make([]CandidateFailure, 5)
	for i := range many {
		many[i] = CandidateFailure{Sig: &one, Kind: candArity, SlotIdx: -1}
	}
	got = candidateNotes("f", many, 6, 0)
	if len(got) != 4 || got[3] != "…and 3 more signatures" {
		t.Errorf("truncation notes = %v", got)
	}
	// Singular hidden count.
	got = candidateNotes("f", many, 4, 0)
	if got[3] != "…and 1 more signature" {
		t.Errorf("singular hidden = %v", got)
	}
}

// --- engine noMatchError assembly -------------------------------------------

func TestNoMatchErrorExpectedFallback(t *testing.T) {
	// Every overload fully admits the tuple positionally (the real
	// failure was collection order): no candidate verdict is possible,
	// so the compact signature overview note stands in.
	fn := &FnDefInfo{Signatures: []Signature{sigOf(TInteger, TInteger)}}
	e := engWithTape(t, []Value{NewInteger(1), NewInteger(2)}, 2)
	ae := e.noMatchError("f", fn, []Value{NewInteger(1), NewInteger(2)}, SrcPos{}, "")
	if ae.Detail != noMatchDetail("f") {
		t.Errorf("Detail = %q", ae.Detail)
	}
	found := false
	for _, n := range ae.Notes {
		if strings.HasPrefix(n, "expected: f ") {
			found = true
		}
	}
	if !found {
		t.Errorf("want the signature-overview fallback note, got %v", ae.Notes)
	}
	if len(ae.Suggestions) != 0 {
		t.Errorf("no suggestions expected, got %v", ae.Suggestions)
	}
}

func TestNoMatchErrorDescribeSuggestion(t *testing.T) {
	// ≥4 overloads → the describe pointer rides as a suggestion and the
	// candidate list truncates.
	fn := &FnDefInfo{Signatures: []Signature{
		sigOf(TString), sigOf(TInteger, TInteger), sigOf(TFloat, TFloat),
		sigOf(TString, TString), sigOf(TList),
	}}
	e := engWithTape(t, []Value{NewBoolean(true)}, 1)
	ae := e.noMatchError("f", fn, []Value{NewBoolean(true)}, SrcPos{}, "")
	if len(ae.Suggestions) != 1 || !strings.Contains(ae.Suggestions[0].Message, "aql describe f") {
		t.Errorf("want the describe suggestion, got %v", ae.Suggestions)
	}
	joined := strings.Join(ae.Notes, "\n")
	if !strings.Contains(joined, "more signature") {
		t.Errorf("want a truncation note, got %v", ae.Notes)
	}
}

func TestNoMatchErrorForwardParensSuggestion(t *testing.T) {
	// A forward-collecting word with no swap reading gets the parens fix.
	sig := sigOf(TInteger)
	sig.BarrierPos = 1
	fn := &FnDefInfo{Signatures: []Signature{sig}}
	e := engWithTape(t, nil, 0)
	ae := e.noMatchError("f", fn, nil, SrcPos{}, "")
	if len(ae.Suggestions) != 1 || !strings.Contains(ae.Suggestions[0].Message, "(f …)") {
		t.Errorf("want the forward-parens suggestion, got %v", ae.Suggestions)
	}
	if len(ae.Notes) == 0 || !strings.Contains(ae.Notes[0], "takes 1 argument, but none were supplied") {
		t.Errorf("want the arity verdict, got %v", ae.Notes)
	}
}
