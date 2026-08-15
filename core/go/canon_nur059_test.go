package core

import "testing"

// NUR059: canon gains SOURCE-form renderers for kinds that previously fell
// through to Value.String's debug spelling. The `/`-modifier half is the
// worst of them — `foo/r` and `foo/2` both rendered `word(foo)`, so the
// modifier was not mis-spelled but DROPPED, and re-parsing the canon
// yielded a different program.
//
// The parser-side round-trip is pinned in parser/spec/parse.tsv, which both
// ports run; these cover the renderer arms from core's own suite.

func TestNUR059WordModifierRenders(t *testing.T) {
	cases := []struct {
		w    WordInfo
		want string
	}{
		{WordInfo{Name: "foo", ArgCount: -1}, "foo"},
		{WordInfo{Name: "foo", ArgCount: 2}, "foo/2"},
		{WordInfo{Name: "foo", ArgCount: -1, ForceRef: true}, "foo/r"},
		{WordInfo{Name: "foo", ArgCount: -1, ForceUsurp: true}, "foo/u"},
		{WordInfo{Name: "foo", ArgCount: -1, ForceStack: true}, "foo/s"},
		{WordInfo{Name: "foo", ArgCount: -1, ForceForward: true}, "foo/f"},
		// Combinations render in the canonical order — digits, f|s, u, r —
		// whatever order they were written in, so the render re-parses and
		// canon stays a usable sort key.
		{WordInfo{Name: "foo", ArgCount: -1, ForceUsurp: true, ForceRef: true}, "foo/ur"},
		{WordInfo{Name: "foo", ArgCount: 3, ForceStack: true, ForceRef: true}, "foo/3sr"},
		// f and s are exclusive; f wins the switch, which is the arm order
		// scanWordModifier's own validation makes unreachable from source.
		{WordInfo{Name: "foo", ArgCount: -1, ForceForward: true, ForceStack: true}, "foo/f"},
	}
	for _, c := range cases {
		got := CanonValue(NewValueRaw(TWord, c.w))
		if c.want == "foo" {
			// A BARE word keeps its existing debug spelling: rendering every
			// word bare is a far larger change and a different question,
			// which this record does not decide.
			if got == "foo" {
				t.Errorf("a bare word must keep its existing spelling, got %q", got)
			}
			continue
		}
		if got != c.want {
			t.Errorf("canon %+v = %q, want %q", c.w, got, c.want)
		}
	}
}

func TestNUR059SugarAngleRenders(t *testing.T) {
	angle := NewSugar(SugarInfo{
		Kind:  SugarAngle,
		Name:  "Box",
		Items: []Value{NewWord("Integer")},
	})
	if got := CanonValue(angle); got != "Box<Integer>" {
		t.Errorf("angle sugar canon = %q, want Box<Integer>", got)
	}
	multi := NewSugar(SugarInfo{
		Kind:  SugarAngle,
		Name:  "Pair",
		Items: []Value{NewWord("Integer"), NewWord("String")},
	})
	if got := CanonValue(multi); got != "Pair<Integer String>" {
		t.Errorf("multi-arg angle canon = %q, want Pair<Integer String>", got)
	}
}

// The NEGATIVE half, and the reason it is a negative: the other sugar kinds
// were tried and withdrawn. A per-row fixpoint check showed `+m'src'`
// rendering `+m<src>` — SugarInfo does not retain the source delimiter, so
// the render re-parses with a stray `>` — and `w/t` rendering `[w/q]/t`,
// which does not parse at all. A spelling that does not round-trip is worse
// than the debug form, because it looks like source. They keep the
// fallback, and this pins that they do.
func TestNUR059OtherSugarKindsKeepTheFallback(t *testing.T) {
	for _, info := range []SugarInfo{
		{Kind: SugarMini, Name: "m", Src: "src"},
		{Kind: SugarTypeBound, Items: []Value{NewList([]Value{NewAtom("w")})}},
		{Kind: SugarLambda},
		{Kind: SugarForceArity, N: 2},
	} {
		got := CanonValue(NewSugar(info))
		if _, handled := canonSugar(info); handled {
			t.Errorf("%v must NOT claim a source spelling (got %q)", info.Kind, got)
		}
	}
}

func TestNUR059ParenGroupRenders(t *testing.T) {
	pe := NewParenExpr([]Value{NewInteger(1), NewWord("add"), NewInteger(2)})
	if got := CanonValue(pe); got != "(1 add 2)" {
		t.Errorf("paren canon = %q, want (1 add 2)", got)
	}
}
