package engine

import (
	"fmt"
	"testing"
)

func TestFlexibleMatchPositional(t *testing.T) {
	// Positional match should always be preferred.
	vals := []Value{NewAtom("x"), NewList(nil)}
	ordered, ok := flexibleMatch(vals, []Type{TAtom, TList})
	if !ok {
		t.Fatal("expected positional match")
	}
	if ordered[0].AsAtom() != "x" {
		t.Errorf("expected atom x at [0], got %v", ordered[0])
	}
}

func TestFlexibleMatchNoPermutation(t *testing.T) {
	// Values in wrong positional order should NOT match — no permutation.
	vals := []Value{NewList(nil), NewAtom("x")}
	_, ok := flexibleMatch(vals, []Type{TAtom, TList})
	if ok {
		t.Fatal("expected no match — arguments must not be permuted")
	}
}

func TestFlexibleMatchNoMatch(t *testing.T) {
	// No valid permutation exists.
	vals := []Value{NewAtom("a"), NewAtom("b")}
	_, ok := flexibleMatch(vals, []Type{TAtom, TList})
	if ok {
		t.Fatal("expected no match for incompatible types")
	}
}

func TestFlexibleMatchPrefersLeastDisplacement(t *testing.T) {
	// When multiple permutations match, prefer fewest displacements.
	// [atom, atom, list] with types [atom, atom, list] — positional wins (0 displacements).
	vals := []Value{NewAtom("a"), NewAtom("b"), NewList(nil)}
	types := []Type{TAtom, TAtom, TList}
	ordered, ok := flexibleMatch(vals, types)
	if !ok {
		t.Fatal("expected match")
	}
	// Positional match: atoms should stay in original order.
	if ordered[0].AsAtom() != "a" {
		t.Errorf("[0] expected atom a, got %s", ordered[0].AsAtom())
	}
	if ordered[1].AsAtom() != "b" {
		t.Errorf("[1] expected atom b, got %s", ordered[1].AsAtom())
	}
}

// --- signatureScore tests ---

func TestSignatureScoreZeroArgs(t *testing.T) {
	sig := Signature{Args: nil}
	if got := SignatureScore(&sig); got != 0 {
		t.Errorf("zero-arg score = %d, want 0", got)
	}
}

func TestSignatureScoreArgCountDominates(t *testing.T) {
	// 1 arg of any (specificity 1) = 101
	sig1 := Signature{Args: []Type{TAny}}
	// 2 args of any = 202
	sig2 := Signature{Args: []Type{TAny, TAny}}
	s1, s2 := SignatureScore(&sig1), SignatureScore(&sig2)
	if s2 <= s1 {
		t.Errorf("2-arg score %d should be > 1-arg score %d", s2, s1)
	}
}

func TestSignatureScoreSpecificityBreaksTie(t *testing.T) {
	// [integer, integer] specificity=2+2=4, total=204
	sigNarrow := Signature{Args: []Type{TInteger, TInteger}}
	// [scalar, scalar] specificity=1+1=2, total=202
	sigWide := Signature{Args: []Type{TScalar, TScalar}}
	sn, sw := SignatureScore(&sigNarrow), SignatureScore(&sigWide)
	if sn <= sw {
		t.Errorf("narrow score %d should be > wide score %d", sn, sw)
	}
}

func TestSignatureScoreMixedSpecificity(t *testing.T) {
	// [integer, any] = 200 + 2 + 1 = 203
	sig1 := Signature{Args: []Type{TInteger, TAny}}
	// [any, any] = 200 + 1 + 1 = 202
	sig2 := Signature{Args: []Type{TAny, TAny}}
	s1, s2 := SignatureScore(&sig1), SignatureScore(&sig2)
	if s1 <= s2 {
		t.Errorf("[integer,any] score %d should be > [any,any] score %d", s1, s2)
	}
}

func TestSignatureScoreDeepType(t *testing.T) {
	// A 3-level type like "number/integer/positive" has specificity 3
	deep, err := NewType("Number/Integer/Positive")
	if err != nil {
		t.Fatal(err)
	}
	sigDeep := Signature{Args: []Type{deep}}
	sigShallow := Signature{Args: []Type{TInteger}} // specificity 2
	sd, ss := SignatureScore(&sigDeep), SignatureScore(&sigShallow)
	if sd <= ss {
		t.Errorf("deep type score %d should be > shallow type score %d", sd, ss)
	}
}

// --- RankSignatures tests ---

func TestRankSignaturesEmpty(t *testing.T) {
	result := RankSignatures(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestRankSignaturesSingle(t *testing.T) {
	sigs := []Signature{{Args: []Type{TAny}}}
	result := RankSignatures(sigs)
	if len(result) != 1 || result[0] != 0 {
		t.Errorf("expected [0], got %v", result)
	}
}

func TestRankSignaturesLongerFirst(t *testing.T) {
	sigs := []Signature{
		{Args: []Type{TAny}},             // score 101
		{Args: []Type{TAny, TAny, TAny}}, // score 303
		{Args: []Type{TAny, TAny}},       // score 202
	}
	ranked := RankSignatures(sigs)
	// Best first: 3-arg(idx=1), 2-arg(idx=2), 1-arg(idx=0)
	want := []int{1, 2, 0}
	for i, idx := range ranked {
		if idx != want[i] {
			t.Errorf("rank[%d] = %d, want %d (ranked=%v)", i, idx, want[i], ranked)
		}
	}
}

func TestRankSignaturesNarrowerFirst(t *testing.T) {
	sigs := []Signature{
		{Args: []Type{TScalar, TScalar}},   // 202
		{Args: []Type{TInteger, TInteger}}, // 204
		{Args: []Type{TAny, TAny}},         // 202 (tie with scalar)
	}
	ranked := RankSignatures(sigs)
	// Best first: integer(1), then scalar(0) and any(2) tied — stable order
	if ranked[0] != 1 {
		t.Errorf("expected narrowest (integer,integer) first, got index %d", ranked[0])
	}
}

func TestRankSignaturesLengthBeatsSpecificity(t *testing.T) {
	// 3 args of any (score 303) beats 2 args of deep types (score ~206)
	deep, err := NewType("Number/Integer/Positive")
	if err != nil {
		t.Fatal(err)
	}
	sigs := []Signature{
		{Args: []Type{deep, deep}},        // 200 + 3+3 = 206
		{Args: []Type{TAny, TAny, TAny}},  // 300 + 1+1+1 = 303
	}
	ranked := RankSignatures(sigs)
	if ranked[0] != 1 {
		t.Errorf("3-arg should beat 2-arg deep, got ranked=%v", ranked)
	}
}

func TestRankSignaturesStableForEqualScores(t *testing.T) {
	// Two sigs with identical scores: stable sort preserves order.
	sigs := []Signature{
		{Args: []Type{TString}}, // 101
		{Args: []Type{TAtom}},   // 101
	}
	ranked := RankSignatures(sigs)
	// Equal scores, stable sort → original order preserved
	if ranked[0] != 0 || ranked[1] != 1 {
		t.Errorf("expected stable order [0,1], got %v", ranked)
	}
}

func TestRankSignatures4To7Args(t *testing.T) {
	// Verify scoring works correctly for larger signatures.
	sigs := []Signature{
		{Args: []Type{TAny, TAny, TAny, TAny}},                                       // 4*100+4 = 404
		{Args: []Type{TAny, TAny, TAny, TAny, TAny}},                                 // 5*100+5 = 505
		{Args: []Type{TAny, TAny, TAny, TAny, TAny, TAny}},                           // 6*100+6 = 606
		{Args: []Type{TAny, TAny, TAny, TAny, TAny, TAny, TAny}},                     // 7*100+7 = 707
		{Args: []Type{TInteger, TInteger, TInteger, TInteger, TInteger}},             // 5*100+10 = 510
	}
	ranked := RankSignatures(sigs)
	// 7-arg(3), 6-arg(2), 5-int(4), 5-any(1), 4-any(0)
	want := []int{3, 2, 4, 1, 0}
	for i, idx := range ranked {
		if idx != want[i] {
			t.Errorf("rank[%d] = %d, want %d (ranked=%v)", i, idx, want[i], ranked)
		}
	}
}

// --- MatchSignature priority tests ---

func dummyHandler(args []Value) ([]Value, error) { return args, nil }

func TestMatchSignaturePrefersMostSpecific(t *testing.T) {
	sigs := []Signature{
		{Args: []Type{TAny, TAny}, Handler: dummyHandler},
		{Args: []Type{TInteger, TInteger}, Handler: dummyHandler},
		{Args: []Type{TScalar, TScalar}, Handler: dummyHandler},
	}
	SortSignatures(sigs)
	stack := []Value{NewInteger(1), NewInteger(2)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	// integer,integer has highest specificity for integer args
	if !m.Sig.Args[0].Equal(TInteger) || !m.Sig.Args[1].Equal(TInteger) {
		t.Errorf("expected [integer,integer] match, got %v", m.Sig.Args)
	}
}

func TestMatchSignaturePrefersLonger(t *testing.T) {
	sigs := []Signature{
		{Args: []Type{TAny}, Handler: dummyHandler},
		{Args: []Type{TAny, TAny, TAny}, Handler: dummyHandler},
		{Args: []Type{TAny, TAny}, Handler: dummyHandler},
	}
	SortSignatures(sigs)
	stack := []Value{NewInteger(1), NewString("x"), NewBoolean(true)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	if m.Sig.TotalArgs() != 3 {
		t.Errorf("expected 3-arg match, got %d args", m.Sig.TotalArgs())
	}
}

func TestMatchSignatureArgCountFilter(t *testing.T) {
	sigs := []Signature{
		{Args: []Type{TAny}, Handler: dummyHandler},
		{Args: []Type{TAny, TAny}, Handler: dummyHandler},
		{Args: []Type{TAny, TAny, TAny}, Handler: dummyHandler},
	}
	stack := []Value{NewInteger(1), NewInteger(2), NewInteger(3)}

	// Force 2-arg match via modifier
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: 2})
	if m == nil {
		t.Fatal("expected match")
	}
	if m.Sig.TotalArgs() != 2 {
		t.Errorf("expected 2-arg match with ArgCount filter, got %d", m.Sig.TotalArgs())
	}
}

func TestMatchSignatureSubtypeMatchesParent(t *testing.T) {
	// integer is a subtype of number; both match number pattern
	sigs := []Signature{
		{Args: []Type{TNumber}, Handler: dummyHandler},
	}
	stack := []Value{NewInteger(42)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("integer should match number pattern")
	}
}

func TestMatchSignatureParentDoesNotMatchChild(t *testing.T) {
	// A plain number value should NOT match an integer-only signature
	sigs := []Signature{
		{Args: []Type{TInteger}, Handler: dummyHandler},
	}
	// Create a value with type "number" (not "number/integer")
	v := NewInteger(42)
	v.VType = TNumber // override to plain number
	stack := []Value{v}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m != nil {
		t.Error("plain number should not match integer-only signature")
	}
}

func TestMatchSignatureNoMatchInsufficientStack(t *testing.T) {
	sigs := []Signature{
		{Args: []Type{TAny, TAny, TAny}, Handler: dummyHandler},
	}
	stack := []Value{NewInteger(1), NewInteger(2)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m != nil {
		t.Error("should not match 3-arg sig with only 2 stack values")
	}
}

func TestMatchSignatureExtraStackIgnored(t *testing.T) {
	sigs := []Signature{
		{Args: []Type{TInteger}, Handler: dummyHandler},
	}
	stack := []Value{NewString("extra"), NewString("extra2"), NewInteger(42)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("should match top-of-stack integer")
	}
	if m.Args[0].AsInteger() != 42 {
		t.Errorf("expected arg 42, got %v", m.Args[0])
	}
}

func TestMatchSignatureNarrowVsWideHierarchy(t *testing.T) {
	// Test with multiple specificity levels:
	// boolean/true (3 parts) vs boolean (1 part) vs any (1 part)
	sigs := []Signature{
		{Args: []Type{TAny}, Handler: dummyHandler},
		{Args: []Type{TBoolean}, Handler: dummyHandler},
		{Args: []Type{TBooleanTrue}, Handler: dummyHandler},
	}
	SortSignatures(sigs)
	stack := []Value{NewBoolean(true)}
	m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
	if m == nil {
		t.Fatal("expected match")
	}
	if !m.Sig.Args[0].Equal(TBooleanTrue) {
		t.Errorf("expected boolean/true match, got %v", m.Sig.Args[0])
	}
}

// --- Score value tests for specific arg counts ---

func TestSignatureScoreValues(t *testing.T) {
	tests := []struct {
		name  string
		args  []Type
		score int
	}{
		{"empty", nil, 0},
		{"1 any", []Type{TAny}, 101},
		{"1 integer", []Type{TInteger}, 103},
		{"2 any", []Type{TAny, TAny}, 202},
		{"2 integer", []Type{TInteger, TInteger}, 206},
		{"2 scalar", []Type{TScalar, TScalar}, 202},
		{"3 any", []Type{TAny, TAny, TAny}, 303},
		{"3 mixed", []Type{TInteger, TString, TAny}, 306},
		{"4 any", []Type{TAny, TAny, TAny, TAny}, 404},
		{"5 any", []Type{TAny, TAny, TAny, TAny, TAny}, 505},
		{"6 any", []Type{TAny, TAny, TAny, TAny, TAny, TAny}, 606},
		{"7 any", []Type{TAny, TAny, TAny, TAny, TAny, TAny, TAny}, 707},
		{"7 integer", []Type{TInteger, TInteger, TInteger, TInteger, TInteger, TInteger, TInteger}, 721},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := Signature{Args: tt.args}
			got := SignatureScore(&sig)
			if got != tt.score {
				t.Errorf("score = %d, want %d", got, tt.score)
			}
		})
	}
}

// --- MatchSignature with varying arg counts (1-7) ---

func TestMatchSignaturePriorityByArgCount(t *testing.T) {
	// Register signatures from 1 to 7 args, all TAny.
	// The longest matching signature should always win.
	for targetLen := 1; targetLen <= 7; targetLen++ {
		t.Run(fmt.Sprintf("%d_args", targetLen), func(t *testing.T) {
			var sigs []Signature
			for n := 1; n <= 7; n++ {
				args := make([]Type, n)
				for i := range args {
					args[i] = TAny
				}
				sigs = append(sigs, Signature{Args: args, Handler: dummyHandler})
			}
			SortSignatures(sigs)

			stack := make([]Value, targetLen)
			for i := range stack {
				stack[i] = NewInteger(int64(i + 1))
			}

			m := MatchSignature(sigs, stack, WordInfo{ArgCount: -1})
			if m == nil {
				t.Fatal("expected match")
			}
			if m.Sig.TotalArgs() != targetLen {
				t.Errorf("with %d stack values, expected %d-arg match, got %d",
					targetLen, targetLen, m.Sig.TotalArgs())
			}
		})
	}
}

func TestMaxArgsLimit(t *testing.T) {
	// Exactly MaxArgs should be fine.
	args := make([]Type, MaxArgs)
	for i := range args {
		args[i] = TAny
	}
	sig := Signature{Args: args, Handler: dummyHandler}
	if sig.TotalArgs() != MaxArgs {
		t.Fatalf("expected %d args, got %d", MaxArgs, sig.TotalArgs())
	}
}

func TestMaxArgsExceededReturnsError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	args := make([]Type, MaxArgs+1)
	for i := range args {
		args[i] = TAny
	}
	r.Register("toobig", Signature{Args: args, Handler: dummyHandler})
	if r.Err() == nil {
		t.Fatal("expected error for signature exceeding MaxArgs")
	}
}
