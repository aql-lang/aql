package eng

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The did-you-mean engine must suggest plausible near-misses and stay
// silent otherwise — every positive case here is paired with the
// negative (outside the cap, empty inputs, self-match) that must
// produce nothing.

func TestSuggestNamesBasics(t *testing.T) {
	cands := []string{"zephyr", "upper", "lower", "append"}
	if got := SuggestNames("zpehyr", cands); !reflect.DeepEqual(got, []string{"zephyr"}) {
		t.Errorf("transposition: got %v", got) // OSA transposition = 1 edit
	}
	if got := SuggestNames("uper", cands); !reflect.DeepEqual(got, []string{"upper"}) {
		t.Errorf("deletion: got %v", got)
	}
	// Nothing within the cap → no suggestion (never guess wildly).
	if got := SuggestNames("zzzzzz", cands); got != nil {
		t.Errorf("far name must suggest nothing, got %v", got)
	}
	// Empty inputs.
	if got := SuggestNames("", cands); got != nil {
		t.Errorf("empty name → nil, got %v", got)
	}
	if got := SuggestNames("x", nil); got != nil {
		t.Errorf("no candidates → nil, got %v", got)
	}
	// The name itself is never suggested.
	if got := SuggestNames("upper", []string{"upper"}); got != nil {
		t.Errorf("self must not be suggested, got %v", got)
	}
}

func TestSuggestNamesDistanceCapByLength(t *testing.T) {
	// len ≤ 4 → 1 edit only: two edits away must NOT be suggested.
	if got := SuggestNames("ab", []string{"abcd"}); got != nil {
		t.Errorf("2 edits on a short name must not match, got %v", got)
	}
	if got := SuggestNames("ab", []string{"abc"}); !reflect.DeepEqual(got, []string{"abc"}) {
		t.Errorf("1 edit on a short name must match, got %v", got)
	}
	// len ≤ 8 → 2 edits.
	if got := SuggestNames("filterr", []string{"filte"}); !reflect.DeepEqual(got, []string{"filte"}) {
		t.Errorf("2 edits on a mid name must match, got %v", got)
	}
	if got := SuggestNames("filterr", []string{"filt"}); got != nil {
		t.Errorf("3 edits on a mid name must not match, got %v", got)
	}
	// len > 8 → 3 edits.
	if got := SuggestNames("transformm", []string{"transfo"}); !reflect.DeepEqual(got, []string{"transfo"}) {
		t.Errorf("3 edits on a long name must match, got %v", got)
	}
}

func TestSuggestNamesRanking(t *testing.T) {
	// Case-only difference ranks before a closer-by-distance name; then
	// distance; then lexicographic. Max 3 results.
	got := SuggestNames("Upper", []string{"upperX", "upper", "uppex", "uppez", "uppey"})
	if len(got) != 3 {
		t.Fatalf("want 3 results, got %v", got)
	}
	if got[0] != "upper" {
		t.Errorf("case-only match must rank first, got %v", got)
	}
	// The remaining two are distance-1 ties resolved lexicographically.
	if got[1] > got[2] {
		t.Errorf("ties must sort lexicographically, got %v", got)
	}
}

func TestOsaDistanceWithinEdges(t *testing.T) {
	if d, ok := osaDistanceWithin("", "ab", 2); !ok || d != 2 {
		t.Errorf("empty a: %d %v", d, ok)
	}
	if _, ok := osaDistanceWithin("ab", "", 1); ok {
		t.Error("empty b beyond budget must not qualify")
	}
	if _, ok := osaDistanceWithin("abcdef", "zzzzzz", 2); ok {
		t.Error("row-min bail-out must fire")
	}
	if d, ok := osaDistanceWithin("acb", "abc", 1); !ok || d != 1 {
		t.Errorf("transposition costs 1: %d %v", d, ok)
	}
}

func TestSuggestionCandidates(t *testing.T) {
	var nilReg *Registry
	if got := nilReg.SuggestionCandidates(); got != nil {
		t.Errorf("nil registry → nil, got %d names", len(got))
	}
	r, rerr := NewRegistry()
	if rerr != nil {
		t.Fatalf("NewRegistry: %v", rerr)
	}
	r.Defs.Push("mydef", NewInteger(1))
	cands := r.SuggestionCandidates()
	want := map[string]bool{"mydef": false, "Integer": false, "true": false}
	for _, c := range cands {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("candidates must include %q", name)
		}
	}
}

func TestDidYouMeanMessageForms(t *testing.T) {
	if got := DidYouMean("zz", nil); got != "" {
		t.Errorf("no match → empty, got %q", got)
	}
	if got := didYouMeanMessage([]string{"a"}); got != "did you mean `a`?" {
		t.Errorf("one = %q", got)
	}
	if got := didYouMeanMessage([]string{"a", "b"}); got != "did you mean `a` or `b`?" {
		t.Errorf("two = %q", got)
	}
	if got := didYouMeanMessage([]string{"a", "b", "c"}); got != "did you mean `a`, `b`, or `c`?" {
		t.Errorf("three = %q", got)
	}
	if got := DidYouMean("zpehyr", []string{"zephyr"}); got != "did you mean `zephyr`?" {
		t.Errorf("end-to-end = %q", got)
	}
}

// --- engine wiring ----------------------------------------------------------

func TestUndefinedWordErrorSuggestions(t *testing.T) {
	e := engWithTape(t, nil, 0)
	e.registry.Defs.Push("zephyr", NewInteger(1))
	ae := e.undefinedWordError("zpehyr", SrcPos{Row: 1, Col: 1})
	if ae.Code != "undefined_word" || ae.Detail != "undefined word: zpehyr" {
		t.Fatalf("head = %q %q", ae.Code, ae.Detail)
	}
	rendered := ae.Error()
	if !strings.Contains(rendered, "did you mean `zephyr`?") {
		t.Errorf("missing did-you-mean:\n%s", rendered)
	}
	// A def-bound near-miss is not a builtin — no describe pointer.
	if strings.Contains(rendered, "aql describe") {
		t.Errorf("describe pointer must need a builtin match:\n%s", rendered)
	}
}

func TestUndefinedWordErrorDescribeForBuiltin(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.tape = NewTape(nil, stackHeadroom)
	// covRegistry registers natives; pick one and typo it.
	names := r.RegisteredWordNames()
	if len(names) == 0 {
		t.Skip("no registered words in fixture registry")
	}
	name := ""
	for _, n := range names {
		if r.IsBuiltinWord(n) && len(n) >= 3 {
			name = n
			break
		}
	}
	if name == "" {
		t.Skip("no builtin word to typo")
	}
	typo := name + "x"
	ae := e.undefinedWordError(typo, SrcPos{})
	rendered := ae.Error()
	if !strings.Contains(rendered, "did you mean `"+name+"`") {
		t.Fatalf("missing did-you-mean for %q:\n%s", typo, rendered)
	}
	if !strings.Contains(rendered, "aql describe "+name) {
		t.Errorf("builtin near-miss must add the describe pointer:\n%s", rendered)
	}
}

func TestUndefinedWordCheckDiagSuggestions(t *testing.T) {
	e := engWithTape(t, nil, 0)
	e.registry.Defs.Push("counter", NewInteger(1))
	d := e.undefinedWordCheckDiag("countr", SrcPos{Row: 2, Col: 3})
	if d.Code != "undefined_word" || d.Row != 2 || d.Col != 3 {
		t.Fatalf("diag = %+v", d)
	}
	if len(d.Suggestions) == 0 || !strings.Contains(d.Suggestions[0].Message, "`counter`") {
		t.Errorf("check diag must carry the did-you-mean, got %+v", d.Suggestions)
	}
	// No plausible match → no suggestions at all.
	d = e.undefinedWordCheckDiag("qqqqqqqq", SrcPos{})
	if len(d.Suggestions) != 0 {
		t.Errorf("far name must carry no suggestions, got %+v", d.Suggestions)
	}
}

// TestForkSuggestionsSafeAgainstParentRegister pins the fork isolation
// contract for the did-you-mean candidate enumeration: a concurrent
// fork's failure path iterates ITS OWN builtinWords snapshot, so a
// host Register call on the parent can never fault the iteration (the
// fatal "concurrent map iteration and map write" a timer callback hit
// when its undefined-word error raced (*AQL).Register).
func TestForkSuggestionsSafeAgainstParentRegister(t *testing.T) {
	r := covRegistry(t, nil)
	fork := r.ForkConcurrent()
	done := make(chan struct{})
	go func() {
		defer close(done)
		e := NewTop(fork)
		for i := 0; i < 300; i++ {
			_ = e.didYouMeanSuggestions("xyzzy-nope")
		}
	}()
	for i := 0; i < 300; i++ {
		r.Register(fmt.Sprintf("host-word-%d", i), Signature{BarrierPos: 0})
	}
	<-done
	// The fork's snapshot predates the registrations: none of the new
	// names are candidates there, while the parent sees them all.
	if fork.IsBuiltinWord("host-word-0") {
		t.Error("fork snapshot must not see post-fork registrations")
	}
	if !r.IsBuiltinWord("host-word-0") {
		t.Error("parent must see its own registration")
	}
}
