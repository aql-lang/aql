package parser

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
)

// valuesEqual compares two eng.Value instances for equality.
func valuesEqual(a, b eng.Value) bool {
	if !a.Parent.Equal(b.Parent) {
		return false
	}
	switch {
	case eng.IsWord(a):
		aw, _ := eng.AsWord(a)
		bw, _ := eng.AsWord(b)
		return aw.Name == bw.Name &&
			aw.ArgCount == bw.ArgCount &&
			aw.ForceStack == bw.ForceStack &&
			aw.ForceForward == bw.ForceForward
	case eng.IsOpenParen(a):
		return true
	case a.Parent.Matches(eng.TString):
		as, _ := eng.AsString(a)
		bs, _ := eng.AsString(b)
		return as == bs
	case a.Parent.Matches(eng.TInteger):
		an, _ := eng.AsInteger(a)
		bn, _ := eng.AsInteger(b)
		return an == bn
	default:
		return a.String() == b.String()
	}
}

func assertParse(t *testing.T, input string, want []eng.Value) {
	t.Helper()
	got, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", input, err)
	}
	if len(got) != len(want) {
		t.Fatalf("Parse(%q) got %d values, want %d\n  got:  %v\n  want: %v",
			input, len(got), len(want), got, want)
	}
	for i := range got {
		if !valuesEqual(got[i], want[i]) {
			t.Errorf("Parse(%q)[%d] = %s, want %s", input, i, got[i], want[i])
		}
	}
}

func assertParseError(t *testing.T, input string) {
	t.Helper()
	_, err := Parse(input)
	if err == nil {
		t.Fatalf("Parse(%q) expected error, got nil", input)
	}
}

// --- Basic literal tests ---

func TestParseEmpty(t *testing.T) {
	assertParse(t, "", nil)
}

func TestParseSingleInteger(t *testing.T) {
	assertParse(t, "1", []eng.Value{eng.NewInteger(1)})
}

func TestParseZero(t *testing.T) {
	assertParse(t, "0", []eng.Value{eng.NewInteger(0)})
}

func TestParseNegativeInteger(t *testing.T) {
	assertParse(t, "-5", []eng.Value{eng.NewInteger(-5)})
}

func TestParseLargeInteger(t *testing.T) {
	assertParse(t, "999", []eng.Value{eng.NewInteger(999)})
}

func TestParseMultipleIntegers(t *testing.T) {
	assertParse(t, "1 2 3", []eng.Value{
		eng.NewInteger(1),
		eng.NewInteger(2),
		eng.NewInteger(3),
	})
}

func TestParseQuotedStringDouble(t *testing.T) {
	assertParse(t, `"hello"`, []eng.Value{eng.NewString("hello")})
}

func TestParseQuotedStringSingle(t *testing.T) {
	assertParse(t, `'world'`, []eng.Value{eng.NewString("world")})
}

func TestParseEmptyQuotedString(t *testing.T) {
	assertParse(t, `""`, []eng.Value{eng.NewString("")})
}

func TestParseQuotedStringWithSpaces(t *testing.T) {
	assertParse(t, `"hello world"`, []eng.Value{eng.NewString("hello world")})
}

// --- Word tests ---

func TestParseSingleWord(t *testing.T) {
	assertParse(t, "upper", []eng.Value{eng.NewWord("upper")})
}

func TestParseUnknownWord(t *testing.T) {
	assertParse(t, "foo", []eng.Value{eng.NewWord("foo")})
}

func TestParseEndKeyword(t *testing.T) {
	assertParse(t, "end", []eng.Value{eng.NewEnd()})
}

func TestParseMultipleWords(t *testing.T) {
	assertParse(t, "a b c", []eng.Value{
		eng.NewWord("a"),
		eng.NewWord("b"),
		eng.NewWord("c"),
	})
}

// --- Mixed token tests ---

func TestParsePrefixExpression(t *testing.T) {
	// a upper → two words: the engine resolves unknown "a" to a string
	assertParse(t, "a upper", []eng.Value{
		eng.NewWord("a"),
		eng.NewWord("upper"),
	})
}

func TestParseForwardExpression(t *testing.T) {
	// lower B → two words
	assertParse(t, "lower B", []eng.Value{
		eng.NewWord("lower"),
		eng.NewWord("B"),
	})
}

func TestParsePrefixArithmetic(t *testing.T) {
	// 1 2 add → two integers then word
	assertParse(t, "1 2 add", []eng.Value{
		eng.NewInteger(1),
		eng.NewInteger(2),
		eng.NewWord("add"),
	})
}

func TestParseInfixArithmetic(t *testing.T) {
	// 1 add 2
	assertParse(t, "1 add 2", []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
	})
}

func TestParseChainedArithmetic(t *testing.T) {
	assertParse(t, "1 add 2 add 3", []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
		eng.NewWord("add"),
		eng.NewInteger(3),
	})
}

func TestParseMixedOperators(t *testing.T) {
	// 2 add 3 mul 4
	assertParse(t, "2 add 3 mul 4", []eng.Value{
		eng.NewInteger(2),
		eng.NewWord("add"),
		eng.NewInteger(3),
		eng.NewWord("mul"),
		eng.NewInteger(4),
	})
}

func TestParseStringThenWord(t *testing.T) {
	// "hello" upper → string then word
	assertParse(t, `"hello" upper`, []eng.Value{
		eng.NewString("hello"),
		eng.NewWord("upper"),
	})
}

func TestParseSetWithEnd(t *testing.T) {
	// set foo 99 end
	assertParse(t, "set foo 99 end", []eng.Value{
		eng.NewWord("set"),
		eng.NewWord("foo"),
		eng.NewInteger(99),
		eng.NewEnd(),
	})
}

// --- Parentheses ---

func TestParseSimpleParens(t *testing.T) {
	// (1 add 2)
	assertParse(t, "(1 add 2)", []eng.Value{
		eng.NewOpenParen(),
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
		eng.NewCloseParen(),
	})
}

func TestParseNestedParens(t *testing.T) {
	// (1 add (2 mul 3))
	assertParse(t, "(1 add (2 mul 3))", []eng.Value{
		eng.NewOpenParen(),
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewOpenParen(),
		eng.NewInteger(2),
		eng.NewWord("mul"),
		eng.NewInteger(3),
		eng.NewCloseParen(),
		eng.NewCloseParen(),
	})
}

func TestParseAdjacentParens(t *testing.T) {
	// (1)(2) — no space between groups
	assertParse(t, "(1)(2)", []eng.Value{
		eng.NewOpenParen(),
		eng.NewInteger(1),
		eng.NewCloseParen(),
		eng.NewOpenParen(),
		eng.NewInteger(2),
		eng.NewCloseParen(),
	})
}

func TestParseParenAroundWord(t *testing.T) {
	// (add)
	assertParse(t, "(add)", []eng.Value{
		eng.NewOpenParen(),
		eng.NewWord("add"),
		eng.NewCloseParen(),
	})
}

// --- Word modifier tests ---

func TestParseArgCountModifier(t *testing.T) {
	// lower/1
	assertParse(t, "lower/1", []eng.Value{
		eng.NewWordModified("lower", 1, false, false),
	})
}

func TestParseForceForwardModifier(t *testing.T) {
	// lower/f
	assertParse(t, "lower/f", []eng.Value{
		eng.NewWordModified("lower", -1, false, true),
	})
}

func TestParseForceStackModifier(t *testing.T) {
	// lower/s
	assertParse(t, "lower/s", []eng.Value{
		eng.NewWordModified("lower", -1, true, false),
	})
}

func TestParseArgCountAndForwardModifier(t *testing.T) {
	// lower/1f
	assertParse(t, "lower/1f", []eng.Value{
		eng.NewWordModified("lower", 1, false, true),
	})
}

func TestParseArgCountAndStackModifier(t *testing.T) {
	// lower/1s
	assertParse(t, "lower/1s", []eng.Value{
		eng.NewWordModified("lower", 1, true, false),
	})
}

func TestParseArgCountZero(t *testing.T) {
	// dup/0
	assertParse(t, "dup/0", []eng.Value{
		eng.NewWordModified("dup", 0, false, false),
	})
}

func TestParseArgCountTwo(t *testing.T) {
	// set/2
	assertParse(t, "set/2", []eng.Value{
		eng.NewWordModified("set", 2, false, false),
	})
}

func TestParseModifierInExpression(t *testing.T) {
	// B lower/f → word then modified word
	assertParse(t, "B lower/f", []eng.Value{
		eng.NewWord("B"),
		eng.NewWordModified("lower", -1, false, true),
	})
}

// --- /q quote-suffix modifier ---

func TestParseQuoteSuffix(t *testing.T) {
	// foo/q → Atom(foo), the canonical short form of (quote foo)
	assertParse(t, "foo/q", []eng.Value{
		eng.NewAtom("foo"),
	})
}

func TestParseQuoteSuffixOnRegisteredName(t *testing.T) {
	// dup/q → Atom(dup) — /q produces an atom even when the bare name
	// would dispatch as a function. Resolution happens at parse time.
	assertParse(t, "dup/q", []eng.Value{
		eng.NewAtom("dup"),
	})
}

func TestParseQuoteSuffixOnReservedLikeName(t *testing.T) {
	// true/q → Atom(true) — /q applies to any word, including
	// reserved-looking names; the suffix wins over the literal path.
	assertParse(t, "true/q", []eng.Value{
		eng.NewAtom("true"),
	})
}

func TestParseQuoteSuffixInExpression(t *testing.T) {
	// foo/q bar/q → two atoms
	assertParse(t, "foo/q bar/q", []eng.Value{
		eng.NewAtom("foo"),
		eng.NewAtom("bar"),
	})
}

// --- Stacked modifiers: order independence ---

func TestParseStackedQuoteAfterStack(t *testing.T) {
	// foo/sq == foo/qs == foo/q — q dominates; result is an Atom.
	assertParse(t, "foo/sq", []eng.Value{
		eng.NewAtom("foo"),
	})
}

func TestParseStackedQuoteBeforeStack(t *testing.T) {
	assertParse(t, "foo/qs", []eng.Value{
		eng.NewAtom("foo"),
	})
}

func TestParseStackedQuoteAfterForward(t *testing.T) {
	assertParse(t, "foo/fq", []eng.Value{
		eng.NewAtom("foo"),
	})
}

func TestParseStackedQuoteBeforeForward(t *testing.T) {
	assertParse(t, "foo/qf", []eng.Value{
		eng.NewAtom("foo"),
	})
}

func TestParseStackedQuoteAndDigits(t *testing.T) {
	// digits and q in any order produce an Atom — modifiers other than
	// q are accepted syntactically but ignored for atoms.
	assertParse(t, "foo/1q", []eng.Value{
		eng.NewAtom("foo"),
	})
	assertParse(t, "foo/q1", []eng.Value{
		eng.NewAtom("foo"),
	})
}

func TestParseStackedAllModifiers(t *testing.T) {
	// All three modifier flavours (digits, s, q) in arbitrary orders
	// produce the same atom — order independent including the barrier
	// number.
	want := []eng.Value{eng.NewAtom("foo")}
	for _, src := range []string{"foo/2qs", "foo/qs2", "foo/sq2", "foo/s2q", "foo/q2s", "foo/2sq"} {
		assertParse(t, src, want)
	}
}

func TestParseForwardSuffixBeforeDigits(t *testing.T) {
	// f and digits in either order produce the same word — /f1 == /1f.
	assertParse(t, "lower/f1", []eng.Value{
		eng.NewWordModified("lower", 1, false, true),
	})
}

func TestParseStackSuffixBeforeDigits(t *testing.T) {
	assertParse(t, "lower/s2", []eng.Value{
		eng.NewWordModified("lower", 2, true, false),
	})
}

// --- /r ref-suffix modifier ---

func TestParseRefSuffix(t *testing.T) {
	// foo/r → ref-word, short form of (ref foo). The kernel resolves
	// the binding at execution time without invoking it.
	assertParse(t, "foo/r", []eng.Value{
		eng.NewWordRef("foo"),
	})
}

func TestParseRefSuffixInExpression(t *testing.T) {
	// Multiple ref-words on one line round-trip independently.
	assertParse(t, "add/r mul/r", []eng.Value{
		eng.NewWordRef("add"),
		eng.NewWordRef("mul"),
	})
}

func TestParseRefAndQuoteMutuallyExclusive(t *testing.T) {
	// /q and /r express different intents (data vs. resolved value),
	// so combining them is a parse-level rejection — the whole token
	// reverts to a plain word.
	assertParse(t, "foo/qr", []eng.Value{
		eng.NewWord("foo/qr"),
	})
	assertParse(t, "foo/rq", []eng.Value{
		eng.NewWord("foo/rq"),
	})
}

func TestParseRefIgnoresShapeModifiers(t *testing.T) {
	// /r short-circuits dispatch, so /s, /f, and digit modifiers are
	// accepted syntactically but have no effect on the emitted value.
	assertParse(t, "foo/rs", []eng.Value{
		eng.NewWordRef("foo"),
	})
	assertParse(t, "foo/rf", []eng.Value{
		eng.NewWordRef("foo"),
	})
	assertParse(t, "foo/2r", []eng.Value{
		eng.NewWordRef("foo"),
	})
}

func TestParseRefDuplicateRejected(t *testing.T) {
	assertParse(t, "foo/rr", []eng.Value{
		eng.NewWord("foo/rr"),
	})
}

func TestParseModifiersForwardAndStackMutuallyExclusive(t *testing.T) {
	// f and s in the same suffix are invalid — the whole token falls
	// back to a plain word with the slash in its name.
	assertParse(t, "foo/fs", []eng.Value{
		eng.NewWord("foo/fs"),
	})
}

func TestParseModifiersDuplicateRejected(t *testing.T) {
	// Duplicate modifiers reset the whole token to a plain word.
	assertParse(t, "foo/qq", []eng.Value{
		eng.NewWord("foo/qq"),
	})
	assertParse(t, "foo/ff", []eng.Value{
		eng.NewWord("foo/ff"),
	})
	assertParse(t, "foo/ss", []eng.Value{
		eng.NewWord("foo/ss"),
	})
	// Digits may not appear in two runs separated by a letter — the
	// argCount is a single contiguous number.
	assertParse(t, "foo/1q1", []eng.Value{
		eng.NewWord("foo/1q1"),
	})
}

func TestParseMultiDigitArgCount(t *testing.T) {
	// Multi-digit numbers form a single contiguous argCount.
	assertParse(t, "foo/12", []eng.Value{
		eng.NewWordModified("foo", 12, false, false),
	})
}

// --- String vs word distinction ---

func TestParseQuotedFunctionName(t *testing.T) {
	// "upper" → string, not a word; upper → word
	assertParse(t, `"upper" upper`, []eng.Value{
		eng.NewString("upper"),
		eng.NewWord("upper"),
	})
}

func TestParseQuotedEnd(t *testing.T) {
	// "end" → string (not the end keyword)
	assertParse(t, `"end"`, []eng.Value{eng.NewString("end")})
}

func TestParseQuotedNumber(t *testing.T) {
	// "1" → string, not an integer
	assertParse(t, `"1"`, []eng.Value{eng.NewString("1")})
}

// --- Whitespace handling ---

func TestParseExtraSpaces(t *testing.T) {
	assertParse(t, "1  2", []eng.Value{
		eng.NewInteger(1),
		eng.NewInteger(2),
	})
}

func TestParseLeadingTrailingSpaces(t *testing.T) {
	assertParse(t, "  1  ", []eng.Value{eng.NewInteger(1)})
}

func TestParseTabs(t *testing.T) {
	assertParse(t, "1\t2", []eng.Value{
		eng.NewInteger(1),
		eng.NewInteger(2),
	})
}

func TestParseWhitespaceOnly(t *testing.T) {
	assertParse(t, "   ", nil)
}

// --- Comment handling ---

func TestParseHashComment(t *testing.T) {
	// 1 # this is a comment
	assertParse(t, "1 # this is a comment", []eng.Value{
		eng.NewInteger(1),
	})
}

func TestParseSlashComment(t *testing.T) {
	// 1 // inline comment
	assertParse(t, "1 // inline comment", []eng.Value{
		eng.NewInteger(1),
	})
}

func TestParseCommentOnly(t *testing.T) {
	assertParse(t, "# just a comment", nil)
}

// --- Value keywords disabled (treated as words) ---

func TestParseTrueAsWord(t *testing.T) {
	assertParse(t, "true", []eng.Value{eng.NewWord("true")})
}

func TestParseFalseAsWord(t *testing.T) {
	assertParse(t, "false", []eng.Value{eng.NewWord("false")})
}

func TestParseNullAsWord(t *testing.T) {
	assertParse(t, "null", []eng.Value{eng.NewWord("null")})
}

// --- Full expression tests ---

func TestParseFullPrefixExpression(t *testing.T) {
	// "hello" upper → string then word (engine would call upper on the string)
	assertParse(t, `"hello" upper`, []eng.Value{
		eng.NewString("hello"),
		eng.NewWord("upper"),
	})
}

func TestParseFullInfixWithParens(t *testing.T) {
	// 2 mul (3 add 4)
	assertParse(t, "2 mul (3 add 4)", []eng.Value{
		eng.NewInteger(2),
		eng.NewWord("mul"),
		eng.NewOpenParen(),
		eng.NewInteger(3),
		eng.NewWord("add"),
		eng.NewInteger(4),
		eng.NewCloseParen(),
	})
}

func TestParseForthPrimitives(t *testing.T) {
	// 1 dup swap drop
	assertParse(t, "1 dup swap drop", []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("dup"),
		eng.NewWord("swap"),
		eng.NewWord("drop"),
	})
}

func TestParseStorageSetGet(t *testing.T) {
	// set x 10 end get x
	assertParse(t, "set x 10 end get x", []eng.Value{
		eng.NewWord("set"),
		eng.NewWord("x"),
		eng.NewInteger(10),
		eng.NewEnd(),
		eng.NewWord("get"),
		eng.NewWord("x"),
	})
}

func TestParseMixedLiteralsAndWords(t *testing.T) {
	// 42 "hello" foo 7
	assertParse(t, `42 "hello" foo 7`, []eng.Value{
		eng.NewInteger(42),
		eng.NewString("hello"),
		eng.NewWord("foo"),
		eng.NewInteger(7),
	})
}

// --- Error cases ---

func TestParseUnterminatedString(t *testing.T) {
	assertParseError(t, `"hello`)
}

func TestParseUnterminatedSingleQuote(t *testing.T) {
	assertParseError(t, `'hello`)
}

// --- Multiline tests ---

func TestParseNewlines(t *testing.T) {
	assertParse(t, "1\nadd\n2", []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
	})
}

func TestParseCRLF(t *testing.T) {
	assertParse(t, "1\r\nadd\r\n2", []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
	})
}

func TestParseBlankLines(t *testing.T) {
	assertParse(t, "1\n\n\nadd\n\n2", []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
	})
}

func TestParseMultilineWithTabs(t *testing.T) {
	assertParse(t, "\t1\n\tadd\n\t2", []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
	})
}

func TestParseMultilineWithComments(t *testing.T) {
	src := "1 # first value\nadd # operator\n2 # second value"
	assertParse(t, src, []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
	})
}

func TestParseMultilineScript(t *testing.T) {
	src := `
		set x 10 end
		set y 20 end
		get x
		add
		get y
	`
	assertParse(t, src, []eng.Value{
		eng.NewWord("set"),
		eng.NewWord("x"),
		eng.NewInteger(10),
		eng.NewEnd(),
		eng.NewWord("set"),
		eng.NewWord("y"),
		eng.NewInteger(20),
		eng.NewEnd(),
		eng.NewWord("get"),
		eng.NewWord("x"),
		eng.NewWord("add"),
		eng.NewWord("get"),
		eng.NewWord("y"),
	})
}

// --- Typed list tests (list.child) ---

func TestParseTypedListString(t *testing.T) {
	// [:String] → typed list with child type string
	assertParse(t, "[:String]", []eng.Value{
		eng.NewTypedList(eng.NewTypeLiteral(eng.TString)),
	})
}

func TestParseTypedListNumber(t *testing.T) {
	// [:Number] → typed list with child type number
	assertParse(t, "[:Number]", []eng.Value{
		eng.NewTypedList(eng.NewTypeLiteral(eng.TNumber)),
	})
}

func TestParseTypedListBoolean(t *testing.T) {
	assertParse(t, "[:Boolean]", []eng.Value{
		eng.NewTypedList(eng.NewTypeLiteral(eng.TBoolean)),
	})
}

func TestParseTypedListAny(t *testing.T) {
	assertParse(t, "[:Any]", []eng.Value{
		eng.NewTypedList(eng.NewTypeLiteral(eng.TAny)),
	})
}

func TestParseTypedListMap(t *testing.T) {
	// [:{x:Number}] → typed list with child type {x:Number}
	got, err := Parse("[:{x:Number}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !eng.IsTypedList(got[0]) {
		t.Fatalf("expected typed list, got %s", got[0])
	}
	ct0a, _ := eng.AsChildType(got[0])
	child := ct0a.Child
	if !child.Parent.Equal(eng.TMap) {
		t.Errorf("expected child type map, got %s", child.Parent)
	}
	m, _ := eng.AsMap(child)
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatalf("expected key 'x' in child map")
	}
	if !xVal.Equal(eng.TNumber) {
		t.Errorf("expected x to be number type, got %s (TestParseTypedListMap)", xVal)
	}
}

func TestParseTypedListNested(t *testing.T) {
	// [:[:String]] → typed list of typed lists of strings
	assertParse(t, "[:[:String]]", []eng.Value{
		eng.NewTypedList(eng.NewTypedList(eng.NewTypeLiteral(eng.TString))),
	})
}

func TestParseTypedListDeepNested(t *testing.T) {
	// [:[:[:Number]]] → three levels deep
	assertParse(t, "[:[:[:Number]]]", []eng.Value{
		eng.NewTypedList(eng.NewTypedList(eng.NewTypedList(eng.NewTypeLiteral(eng.TNumber)))),
	})
}

func TestParseTypedListInExpression(t *testing.T) {
	// 1 [:String] → integer then typed list
	assertParse(t, "1 [:String]", []eng.Value{
		eng.NewInteger(1),
		eng.NewTypedList(eng.NewTypeLiteral(eng.TString)),
	})
}

func TestParseTypedListMapChild(t *testing.T) {
	// [:{a:String,b:Number}] → typed list with multi-key map child
	got, err := Parse("[:{a:String,b:Number}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !eng.IsTypedList(got[0]) {
		t.Fatalf("expected 1 typed list, got %v", got)
	}
	ct0b, _ := eng.AsChildType(got[0])
	child0b := ct0b.Child
	m, _ := eng.AsMap(child0b)
	if m.Len() != 2 {
		t.Errorf("expected 2 keys, got %d", m.Len())
	}
}

// --- Typed map tests (map.child) ---

func TestParseTypedMapString(t *testing.T) {
	// {:String} → typed map with child type string
	assertParse(t, "{:String}", []eng.Value{
		eng.NewTypedMap(eng.NewTypeLiteral(eng.TString)),
	})
}

func TestParseTypedMapNumber(t *testing.T) {
	// {:Number} → typed map with child type number
	assertParse(t, "{:Number}", []eng.Value{
		eng.NewTypedMap(eng.NewTypeLiteral(eng.TNumber)),
	})
}

func TestParseTypedMapBoolean(t *testing.T) {
	assertParse(t, "{:Boolean}", []eng.Value{
		eng.NewTypedMap(eng.NewTypeLiteral(eng.TBoolean)),
	})
}

func TestParseTypedMapAny(t *testing.T) {
	assertParse(t, "{:Any}", []eng.Value{
		eng.NewTypedMap(eng.NewTypeLiteral(eng.TAny)),
	})
}

func TestParseTypedMapList(t *testing.T) {
	// {:[:Number]} → typed map with child type [:Number]
	assertParse(t, "{:[:Number]}", []eng.Value{
		eng.NewTypedMap(eng.NewTypedList(eng.NewTypeLiteral(eng.TNumber))),
	})
}

func TestParseTypedMapNested(t *testing.T) {
	// {:{:String}} → typed map of typed maps of strings
	assertParse(t, "{:{:String}}", []eng.Value{
		eng.NewTypedMap(eng.NewTypedMap(eng.NewTypeLiteral(eng.TString))),
	})
}

func TestParseTypedMapDeepNested(t *testing.T) {
	// {:{:{:Number}}} → three levels deep
	assertParse(t, "{:{:{:Number}}}", []eng.Value{
		eng.NewTypedMap(eng.NewTypedMap(eng.NewTypedMap(eng.NewTypeLiteral(eng.TNumber)))),
	})
}

func TestParseTypedMapInExpression(t *testing.T) {
	// 1 {:String} → integer then typed map
	assertParse(t, "1 {:String}", []eng.Value{
		eng.NewInteger(1),
		eng.NewTypedMap(eng.NewTypeLiteral(eng.TString)),
	})
}

func TestParseTypedMapConcreteChild(t *testing.T) {
	// {:{x:Number}} → typed map with map child type
	got, err := Parse("{:{x:Number}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !eng.IsTypedMap(got[0]) {
		t.Fatalf("expected 1 typed map, got %v", got)
	}
	ct0c, _ := eng.AsChildType(got[0])
	child0c := ct0c.Child
	if !child0c.Parent.Equal(eng.TMap) {
		t.Errorf("expected child type map, got %s", child0c.Parent)
	}
	m, _ := eng.AsMap(child0c)
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatalf("expected key 'x' in child map")
	}
	if !xVal.Equal(eng.TNumber) {
		t.Errorf("expected x to be number type, got %s (TestParseTypedMapConcreteChild)", xVal)
	}
}

// --- Word list (explicit bracket) tests ---

func TestParseExplicitList(t *testing.T) {
	// [1 add 2] → single list value containing words
	got, err := Parse("[1 add 2]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !got[0].Parent.Equal(eng.TList) {
		t.Fatalf("expected list, got %s", got[0].Parent)
	}
	_lst, _ := eng.AsList(got[0])
	elems := _lst.Slice()
	if len(elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(elems))
	}
}

func TestParseListWithStrings(t *testing.T) {
	// ["hello" "world"] → list of strings
	got, err := Parse(`["hello" "world"]`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	_lst, _ := eng.AsList(got[0])
	elems := _lst.Slice()
	if len(elems) != 2 {
		t.Errorf("expected 2 elements, got %d", len(elems))
	}
}

// --- Data list (list inside map) tests ---

func TestParseMapWithList(t *testing.T) {
	// {x:[1,2,3]} → map with list value in data context
	got, err := Parse(`{x:[1,2,3]}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !got[0].Parent.Equal(eng.TMap) {
		t.Fatalf("expected map, got %s", got[0].Parent)
	}
	m, _ := eng.AsMap(got[0])
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x'")
	}
	if !xVal.Parent.Equal(eng.TList) {
		t.Errorf("expected list, got %s", xVal.Parent)
	}
	_lst, _ := eng.AsList(xVal)
	elems := _lst.Slice()
	if len(elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(elems))
	}
}

func TestParseMapWithBooleans(t *testing.T) {
	// {a:true,b:false} → map with word values (word context)
	got, err := Parse(`{a:true,b:false}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	aVal, _ := m.Get("a")
	aw, _ := eng.AsWord(aVal)
	if !eng.IsWord(aVal) || aw.Name != "true" {
		t.Errorf("expected word(true), got %s", aVal)
	}
	bVal, _ := m.Get("b")
	bw, _ := eng.AsWord(bVal)
	if !eng.IsWord(bVal) || bw.Name != "false" {
		t.Errorf("expected word(false), got %s", bVal)
	}
}

func TestParseMapWithNull(t *testing.T) {
	// {x:null} → in jsonic data context, bare "null" is Text
	got, err := Parse(`{x:null}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	_, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x'")
	}
}

func TestParseMapWithTypeName(t *testing.T) {
	// {x:Number} → map with type literal in data context
	got, err := Parse(`{x:Number}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	xVal, _ := m.Get("x")
	if !xVal.Equal(eng.TNumber) {
		t.Errorf("expected number type literal, got %s", xVal)
	}
}

func TestParseMapWithNestedMap(t *testing.T) {
	// {a:{b:1}} → nested map in data context
	got, err := Parse(`{a:{b:1}}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	aVal, _ := m.Get("a")
	if !aVal.Parent.Equal(eng.TMap) {
		t.Errorf("expected nested map, got %s", aVal.Parent)
	}
}

// --- preprocessParens tests ---

func TestParseParensInString(t *testing.T) {
	// "(hello)" → the parens are inside a string, not structural
	assertParse(t, `"(hello)"`, []eng.Value{eng.NewString("(hello)")})
}

func TestParseNoParens(t *testing.T) {
	// No parens — preprocessParens should be a no-op
	assertParse(t, "1 2 3", []eng.Value{
		eng.NewInteger(1),
		eng.NewInteger(2),
		eng.NewInteger(3),
	})
}

// --- Escape in string within parens ---

func TestParseEscapeInParenString(t *testing.T) {
	// Parentheses inside escaped strings in preprocessParens
	got, err := Parse(`"a\"b" 1`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) < 1 {
		t.Fatalf("expected at least 1 value, got %d", len(got))
	}
}

// --- Semicolon token tests ---

func TestParseSemicolonAsEnd(t *testing.T) {
	// ";" should parse as the word "end"
	assertParse(t, "1 add 2; 99", []eng.Value{
		eng.NewInteger(1),
		eng.NewWord("add"),
		eng.NewInteger(2),
		eng.NewEnd(),
		eng.NewInteger(99),
	})
}

func TestParseSemicolonStandalone(t *testing.T) {
	assertParse(t, ";", []eng.Value{
		eng.NewEnd(),
	})
}

func TestParseSemicolonAdjacentToWord(t *testing.T) {
	// "foo;bar" — semicolon is a fixed token, so it splits the text
	assertParse(t, "foo;bar", []eng.Value{
		eng.NewWord("foo"),
		eng.NewEnd(),
		eng.NewWord("bar"),
	})
}

// (expandDottedWord tests removed — dot notation is now handled by
// simple token conversion: . → get, ! . → getr)

// --- Boolean and nil as top-level values ---

func TestParseBooleanTopLevel(t *testing.T) {
	// Test bool and nil through convertTopLevelValue
	// jsonic with Lex=false means true/false are Text, but let's test
	// nil handling by parsing a map with nil value
	got, err := Parse("{x:null}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Data context: nested map with booleans and nil ---

func TestParseDataMapWithNil(t *testing.T) {
	// {a:null,b:true,c:false,d:1}
	got, err := Parse("{a:null,b:true,c:false,d:1}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	// b should be word "true" (word context)
	bVal, _ := m.Get("b")
	bw2, _ := eng.AsWord(bVal)
	if !eng.IsWord(bVal) || bw2.Name != "true" {
		t.Errorf("expected word(true), got %s", bVal)
	}
	// c should be word "false" (word context)
	cVal, _ := m.Get("c")
	cw2, _ := eng.AsWord(cVal)
	if !eng.IsWord(cVal) || cw2.Name != "false" {
		t.Errorf("expected word(false), got %s", cVal)
	}
}

// --- Data context: nested structures ---

func TestParseDataMapWithNestedList(t *testing.T) {
	// {x:{y:[1,2,3]}} — nested map with data list inside
	got, err := Parse("{x:{y:[1,2,3]}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	xVal, _ := m.Get("x")
	if !xVal.Parent.Equal(eng.TMap) {
		t.Fatalf("expected nested map, got %s", xVal.Parent)
	}
	inner, _ := eng.AsMap(xVal)
	yVal, _ := inner.Get("y")
	if !yVal.Parent.Equal(eng.TList) {
		t.Errorf("expected list, got %s", yVal.Parent)
	}
}

// --- Map containing typed list child ---

func TestParseMapWithTypedListValue(t *testing.T) {
	// {x:[:Number]} → map key 'x' has typed list value
	got, err := Parse("{x:[:Number]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- List in data context with bool/nil ---

func TestParseDataListWithBoolAndNil(t *testing.T) {
	// {x:[true,false,null,1]} → data list with bool and nil
	got, err := Parse("{x:[true,false,null,1]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	xVal, _ := m.Get("x")
	_lst, _ := eng.AsList(xVal)
	elems := _lst.Slice()
	if len(elems) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(elems))
	}
	// With word context in lists, true/false become words (resolved at runtime),
	// and null becomes a word (resolved to atom at runtime).
	ew0, _ := eng.AsWord(elems[0])
	ew1, _ := eng.AsWord(elems[1])
	ew2, _ := eng.AsWord(elems[2])
	if !eng.IsWord(elems[0]) || ew0.Name != "true" {
		t.Errorf("expected word(true), got %s", elems[0])
	}
	if !eng.IsWord(elems[1]) || ew1.Name != "false" {
		t.Errorf("expected word(false), got %s", elems[1])
	}
	if !eng.IsWord(elems[2]) || ew2.Name != "null" {
		t.Errorf("expected word(null), got %s", elems[2])
	}
}

// --- Decimal number tests ---

func TestParseDecimalNumber(t *testing.T) {
	// 1.5 → decimal (float64 path in convertTopLevelValue and floatToValue)
	got, err := Parse("1.5")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !got[0].Parent.Matches(eng.TDecimal) {
		t.Errorf("expected decimal type, got %s", got[0].Parent)
	}
}

func TestParseDecimalInExpression(t *testing.T) {
	// 1.5 add 2.3 → two decimals and a word
	got, err := Parse("1.5 add 2.3")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 values, got %d", len(got))
	}
	if !got[0].Parent.Matches(eng.TDecimal) {
		t.Errorf("expected decimal, got %s", got[0].Parent)
	}
}

func TestParseMapWithDecimal(t *testing.T) {
	// {x:1.5} → map with decimal in data context (float64 path in convertDataValue)
	got, err := Parse("{x:1.5}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	xVal, _ := m.Get("x")
	if !xVal.Parent.Matches(eng.TDecimal) {
		t.Errorf("expected decimal, got %s", xVal.Parent)
	}
}

// --- Nested list/map in word context ---

func TestParseListWithMap(t *testing.T) {
	// [{x:1}] → list containing a map (convertTopLevelValue MapRef path)
	got, err := Parse("[{x:1}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	_lst, _ := eng.AsList(got[0])
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
	if !elems[0].Parent.Equal(eng.TMap) {
		t.Errorf("expected map element, got %s", elems[0].Parent)
	}
}

func TestParseNestedList(t *testing.T) {
	// [[1,2],[3,4]] in data context — data list with nested lists
	got, err := Parse("{x:[[1,2],[3,4]]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	xVal, _ := m.Get("x")
	_lst, _ := eng.AsList(xVal)
	elems := _lst.Slice()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
}

// --- List with dotted word ---

func TestParseListWithDottedWord(t *testing.T) {
	// [foo.bar] → list with the dot chain grouped: ( foo get bar ) = 5
	// elements (OpenParen, foo, get, bar, CloseParen).
	got, err := Parse("[foo.bar]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	_lst, _ := eng.AsList(got[0])
	elems := _lst.Slice()
	if len(elems) != 5 {
		t.Fatalf("expected 5 elements ( foo get bar ), got %d", len(elems))
	}
}

// --- Map with single key (sortedKeys edge case) ---

func TestParseMapSingleKey(t *testing.T) {
	// {a:1} → sortedKeys with 1 key (no sorting needed)
	got, err := Parse("{a:1}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	if m.Len() != 1 {
		t.Errorf("expected 1 key, got %d", m.Len())
	}
}

// --- Word modifier edge cases ---

func TestParseUnrecognizedModifier(t *testing.T) {
	// foo/x → unrecognized modifier, treated as plain word "foo/x"
	assertParse(t, "foo/x", []eng.Value{eng.NewWord("foo/x")})
}

func TestParseSlashOnly(t *testing.T) {
	// foo/ → trailing slash with no modifier text (idx == len(name)-1),
	// so it's not matched by the modifier parsing
	assertParse(t, "foo/", []eng.Value{eng.NewWord("foo/")})
}

func TestParseEmptyDigitsEmptyRest(t *testing.T) {
	// This covers the case where modifier is just "/" at end of string
	// which doesn't trigger the modifier parsing (idx >= len(name)-1 check)
	assertParse(t, "x/", []eng.Value{eng.NewWord("x/")})
}

// --- Parens within different quote types ---

func TestParseSingleQuoteWithParens(t *testing.T) {
	// '(test)' → parens inside single-quoted string
	assertParse(t, "'(test)'", []eng.Value{eng.NewString("(test)")})
}

func TestParseBacktickWithParens(t *testing.T) {
	// `(test)` → parens inside backtick-quoted string
	assertParse(t, "`(test)`", []eng.Value{eng.NewString("(test)")})
}

// --- Data context: typed list inside data value ---

func TestParseDataMapWithTypedList(t *testing.T) {
	// {x:[:String]} → typed list in data context (ListRef.Child path in convertDataValue)
	got, err := Parse("{x:[:String]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Data context: typed map inside data value ---

func TestParseDataMapWithTypedMap(t *testing.T) {
	// {x:{:Number}} → typed map in data context (MapRef with child$ in convertDataValue)
	got, err := Parse("{x:{:Number}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Top-level dotted expansion ---

func TestParseDottedWordTopLevel(t *testing.T) {
	// foo.bar at top level — jsonic may parse this as a single text token
	// with a dot or as a map. Either way, it should not error.
	got, err := Parse("foo.bar")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) < 1 {
		t.Fatalf("expected at least 1 value, got %d", len(got))
	}
}

func TestParseDottedWordInExpression(t *testing.T) {
	// 1 foo.bar → 1 ( foo get bar ) = 6 values. The dot chain is grouped so
	// it binds to foo, not to the result of `1 foo`.
	got, err := Parse("1 foo.bar")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 6 {
		t.Fatalf("expected 6 values (1 ( foo get bar )), got %d: %v", len(got), got)
	}
}

// --- Map as top-level value ---

func TestParseTopLevelMap(t *testing.T) {
	// {a:1,b:2} as the only input → hits the MapRef branch in Parse
	got, err := Parse("{a:1,b:2}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !got[0].Parent.Equal(eng.TMap) {
		t.Errorf("expected map, got %s", got[0].Parent)
	}
}

// --- List pair syntax: maps inside lists ---

func TestParseListPairSyntax(t *testing.T) {
	// [x:1] → list with pair syntax creates map inside list
	// This hits map[string]any case in convertTopLevelValue
	got, err := Parse("[x:1]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

func TestParseListPairTypedMap(t *testing.T) {
	// [x:Number] → list with pair syntax, type name in value
	got, err := Parse("[x:Number]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Data context: map with string values ---

func TestParseMapWithStringValues(t *testing.T) {
	// {x:"hello"} → quoted string in data context (Text with quote)
	got, err := Parse(`{x:"hello"}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	xVal, _ := m.Get("x")
	xValS, _ := eng.AsString(xVal)
	if !xVal.Parent.Matches(eng.TString) || xValS != "hello" {
		t.Errorf("expected string hello, got %s", xVal)
	}
}

// --- Nested typed structures in data context ---

func TestParseDataNestedTypedMap(t *testing.T) {
	// {x:{:String},y:{:Number}} → multiple typed maps in data context
	got, err := Parse("{x:{:String},y:{:Number}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Data list with typed list child ---

func TestParseDataListWithTypedList(t *testing.T) {
	// {x:[[:String]]} → list containing a typed list in data context
	got, err := Parse("{x:[[:String]]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- convertTopLevelValue: bool path (only reachable from list-pair context) ---

func TestParseListWithBoolean(t *testing.T) {
	// [1, true, false] with booleans in list context
	got, err := Parse("[1, true, false]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Nil/null at top level ---

func TestParseNullValue(t *testing.T) {
	// null at top level → word "null" (with Lex=false)
	assertParse(t, "null", []eng.Value{eng.NewWord("null")})
}

// --- Data context: map with nil value via jsonic ---

func TestParseDataMapNilValue(t *testing.T) {
	// Testing data paths more thoroughly
	got, err := Parse("{a:1,b:hello,c:true,d:false,e:Number,f:Any}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	// Check type literal resolution in data context
	eVal, _ := m.Get("e")
	if !eVal.Equal(eng.TNumber) {
		t.Errorf("expected Number type literal, got %s", eVal)
	}
	fVal, _ := m.Get("f")
	if !fVal.Equal(eng.TAny) {
		t.Errorf("expected Any type literal, got %s", fVal)
	}
}

// --- List inside list (word context) ---

func TestParseListInsideList(t *testing.T) {
	// [[1,2]] → list containing a nested list in word context
	got, err := Parse("[[1,2]]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Empty map ---

func TestParseEmptyMap(t *testing.T) {
	got, err := Parse("{}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
}

// --- Map with many keys (sortedKeys with >1 key) ---

func TestParseMapManyKeys(t *testing.T) {
	got, err := Parse("{c:3,a:1,b:2}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	if m.Len() != 3 {
		t.Errorf("expected 3 keys, got %d", m.Len())
	}
}

// --- Decimal inside data list ---

func TestParseDataListWithDecimal(t *testing.T) {
	got, err := Parse("{x:[1.5,2.7]}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m, _ := eng.AsMap(got[0])
	xVal, _ := m.Get("x")
	_lst, _ := eng.AsList(xVal)
	elems := _lst.Slice()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
}

// --- preprocessParens: escape inside string with parens ---

func TestParseEscapeInStringWithParens(t *testing.T) {
	// Escape char inside a string when parens are also present
	// This covers the escape-in-string path in preprocessParens (lines 124-127)
	got, err := Parse(`"a\"b" (1)`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) < 1 {
		t.Fatalf("expected at least 1 value, got %d", len(got))
	}
}

// --- parseWord edge cases ---

func TestParseEmptyNameAfterModifier(t *testing.T) {
	// /1f → modifier on empty name, should error
	assertParseError(t, "/1f")
}

func TestParseSlashFModifier(t *testing.T) {
	// /f → empty name with forward modifier, should error
	assertParseError(t, "/f")
}

// --- convertTopLevelValue / convertDataValue unreachable cases ---
// These are defensive code paths for jsonic types not produced by
// the current configuration. We test the reachable paths thoroughly
// and accept the defensive branches as uncovered.

// --- Direct function tests for better coverage ---

func TestFloatToValueWholeNumber(t *testing.T) {
	// Whole number float → integer
	v := floatToValue(42.0)
	if !v.Parent.Matches(eng.TInteger) {
		t.Errorf("expected integer, got %s", v.Parent)
	}
}

func TestFloatToValueFractional(t *testing.T) {
	// Fractional float → decimal
	v := floatToValue(3.14)
	if !v.Parent.Matches(eng.TDecimal) {
		t.Errorf("expected decimal, got %s", v.Parent)
	}
}

func TestSortedKeysEmpty(t *testing.T) {
	keys := sortedKeys(map[string]any{})
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestSortedKeysSingle(t *testing.T) {
	keys := sortedKeys(map[string]any{"a": 1})
	if len(keys) != 1 || keys[0] != "a" {
		t.Errorf("expected [a], got %v", keys)
	}
}

func TestSortedKeysMultiple(t *testing.T) {
	keys := sortedKeys(map[string]any{"c": 3, "a": 1, "b": 2})
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("expected [a b c], got %v", keys)
	}
}

func TestResolveTextValueTypes(t *testing.T) {
	tests := []struct {
		input string
		check func(eng.Value) bool
	}{
		{"true", func(v eng.Value) bool { b, _ := eng.AsBoolean(v); return v.Parent.Matches(eng.TBoolean) && b }},
		{"false", func(v eng.Value) bool { b, _ := eng.AsBoolean(v); return v.Parent.Matches(eng.TBoolean) && !b }},
		{"Number", func(v eng.Value) bool { return v.Equal(eng.TNumber) }},
		{"String", func(v eng.Value) bool { return v.Equal(eng.TString) }},
		{"hello", func(v eng.Value) bool { s, _ := eng.AsAtom(v); return v.Parent.Matches(eng.TAtom) && s == "hello" }},
	}
	for _, tt := range tests {
		v := resolveTextValue(tt.input)
		if !tt.check(v) {
			t.Errorf("resolveTextValue(%q) = %s, unexpected", tt.input, v)
		}
	}
}

// --- Direct unexported function tests for defensive code paths ---

func TestConvertTopLevelValueBool(t *testing.T) {
	v, err := convertTopLevelValue(true)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := eng.AsBoolean(v)
	if !v.Parent.Matches(eng.TBoolean) || !b1 {
		t.Errorf("expected true, got %s", v)
	}
	v, err = convertTopLevelValue(false)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := eng.AsBoolean(v)
	if !v.Parent.Matches(eng.TBoolean) || b2 {
		t.Errorf("expected false, got %s", v)
	}
}

func TestConvertTopLevelValueNil(t *testing.T) {
	v, err := convertTopLevelValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !eng.IsAtom(v) {
		t.Fatalf("expected atom for null, got %s", v.Parent)
	}
	name, _ := eng.AsAtom(v)
	if name != "null" {
		t.Errorf("expected atom('null'), got atom(%q)", name)
	}
}

func TestConvertTopLevelValueUnsupported(t *testing.T) {
	_, err := convertTopLevelValue(struct{}{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestConvertDataValueBool(t *testing.T) {
	v, err := convertDataValue(true)
	if err != nil {
		t.Fatal(err)
	}
	b3, _ := eng.AsBoolean(v)
	if !v.Parent.Matches(eng.TBoolean) || !b3 {
		t.Errorf("expected true, got %s", v)
	}
}

func TestConvertDataValueNil(t *testing.T) {
	v, err := convertDataValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !eng.IsAtom(v) {
		t.Fatalf("expected atom for null, got %s", v.Parent)
	}
	name, _ := eng.AsAtom(v)
	if name != "null" {
		t.Errorf("expected atom('null'), got atom(%q)", name)
	}
}

func TestConvertDataValueUnsupported(t *testing.T) {
	_, err := convertDataValue(struct{}{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestConvertDataValueRawMap(t *testing.T) {
	// map[string]any without child$ key
	v, err := convertDataValue(map[string]any{"x": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Parent.Equal(eng.TMap) {
		t.Errorf("expected map, got %s", v.Parent)
	}
}

func TestConvertDataValueRawMapWithChild(t *testing.T) {
	// map[string]any with child$ key → typed map
	v, err := convertDataValue(map[string]any{"child$": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !eng.IsTypedMap(v) {
		t.Errorf("expected typed map, got %s", v)
	}
}

func TestConvertTopLevelValueRawMap(t *testing.T) {
	v, err := convertTopLevelValue(map[string]any{"x": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Parent.Equal(eng.TMap) {
		t.Errorf("expected map, got %s", v.Parent)
	}
}

func TestConvertTopLevelValueRawMapWithChild(t *testing.T) {
	v, err := convertTopLevelValue(map[string]any{"child$": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !eng.IsTypedMap(v) {
		t.Errorf("expected typed map, got %s", v)
	}
}

func TestParseWordDirectCoverage(t *testing.T) {
	// Test parseWord directly for various modifiers
	tests := []struct {
		input string
		name  string
		ok    bool
	}{
		{"hello", "hello", true},
		{"foo/2s", "foo", true},
		{"foo/0", "foo", true},
		{"foo/bad", "foo/bad", true}, // unrecognized modifier
		{"foo/", "foo/", true},       // slash at end not processed
	}
	for _, tt := range tests {
		v, err := parseWord(tt.input)
		if tt.ok && err != nil {
			t.Errorf("parseWord(%q) error: %v", tt.input, err)
		} else if !tt.ok && err == nil {
			t.Errorf("parseWord(%q) expected error", tt.input)
		}
		if tt.ok {
			vw, _ := eng.AsWord(v)
			if vw.Name != tt.name {
				t.Errorf("parseWord(%q) name = %q, want %q", tt.input, vw.Name, tt.name)
			}
		}
	}
}

// =============================================================================
// MapRef.Implicit — implicit pair syntax vs explicit maps
// =============================================================================

func TestParseImplicitMapInList(t *testing.T) {
	// [x:Integer] — pair syntax inside list produces an implicit map
	vals, err := Parse("[x:Integer]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	list := vals[0]
	if !list.Parent.Equal(eng.TList) {
		t.Fatalf("expected list, got %s", list.Parent)
	}
	_lst, _ := eng.AsList(list)
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element in list, got %d", len(elems))
	}
	if !elems[0].Parent.Equal(eng.TMap) {
		t.Fatalf("expected map element, got %s", elems[0].Parent)
	}
	m, _ := eng.AsMutableMap(elems[0])
	if !m.Implicit {
		t.Error("expected Implicit=true for pair syntax [x:Integer]")
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("expected key 'x', got %v", keys)
	}
}

func TestParseExplicitMapInList(t *testing.T) {
	// [{x:Integer}] — explicit map inside list is NOT implicit
	vals, err := Parse("[{x:Integer}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	list := vals[0]
	_lst, _ := eng.AsList(list)
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element in list, got %d", len(elems))
	}
	if !elems[0].Parent.Equal(eng.TMap) {
		t.Fatalf("expected map element, got %s", elems[0].Parent)
	}
	m, _ := eng.AsMutableMap(elems[0])
	if m.Implicit {
		t.Error("expected Implicit=false for explicit map [{x:Integer}]")
	}
}

func TestParseExplicitMapTopLevel(t *testing.T) {
	// {a:1} — explicit map at top level is NOT implicit
	vals, err := Parse("{a:1}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if !vals[0].Parent.Equal(eng.TMap) {
		t.Fatalf("expected map, got %s", vals[0].Parent)
	}
	m, _ := eng.AsMutableMap(vals[0])
	if m.Implicit {
		t.Error("expected Implicit=false for explicit map {a:1}")
	}
}

func TestParseOptionalFieldDisjunct(t *testing.T) {
	// {a?:Integer} → key "a" with value (Integer or None)
	vals, err := Parse("{a?:Integer}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	m, _ := eng.AsMap(vals[0])
	if m == nil {
		t.Fatalf("eng.AsMap() returned nil, value: %s (data: %T)", vals[0].String(), vals[0].Data)
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "a" {
		t.Errorf("expected key 'a', got %v", keys)
	}
	val, _ := m.Get("a")
	if !eng.IsDisjunct(val) {
		t.Fatalf("expected disjunct for optional field, got %s", val.String())
	}
	dj, _ := eng.AsDisjunct(val)
	alts := dj.Alternatives
	// `?:T` desugars to `disjunct(T, None, Absent)` — Absent is the
	// kernel type denoting "key not present", and the third
	// alternative is what makes the missing-key half of the rule
	// work via the type system rather than out-of-band metadata.
	if len(alts) != 3 {
		t.Fatalf("expected 3 alternatives, got %d", len(alts))
	}
	if !alts[1].Equal(eng.TNone) {
		t.Errorf("expected second alternative to be None, got %s", alts[1])
	}
	if !alts[2].Equal(eng.TAbsent) {
		t.Errorf("expected third alternative to be Absent, got %s", alts[2])
	}
}

func TestParseMapShorthand(t *testing.T) {
	// Each shorthand form must parse to the identical value tree as its
	// explicit `key:value` equivalent.
	cases := []struct{ shorthand, explicit string }{
		{"{foo}", "{foo:foo}"},                     // plain shorthand
		{"{foo/r}", "{foo:foo/r}"},                 // word modifier stays on the value
		{"{foo/q}", "{foo:foo/q}"},                 // /q → atom value
		{"{foo?}", "{foo?:foo}"},                   // optional shorthand
		{"{foo a:1 bar}", "{foo:foo a:1 bar:bar}"}, // mixed, keys sorted
		{"{a:{foo}}", "{a:{foo:foo}}"},             // nested
	}
	for _, c := range cases {
		sh, err := Parse(c.shorthand)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.shorthand, err)
		}
		ex, err := Parse(c.explicit)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.explicit, err)
		}
		if len(sh) != 1 || len(ex) != 1 {
			t.Fatalf("expected 1 value each for %q / %q, got %d / %d",
				c.shorthand, c.explicit, len(sh), len(ex))
		}
		if sh[0].String() != ex[0].String() {
			t.Errorf("shorthand %q = %s, want same as %q = %s",
				c.shorthand, sh[0].String(), c.explicit, ex[0].String())
		}
	}
}

func TestParseMapShorthandRejects(t *testing.T) {
	// Only unquoted identifiers trigger the shorthand; quoted keys and
	// non-identifier tokens remain parse errors.
	assertParseError(t, "{'foo'}")
	assertParseError(t, `{"foo"}`)
	assertParseError(t, "{123}")
}

func TestParseOptionalFieldMixed(t *testing.T) {
	// {a:Integer, b?:String} → "a" is plain, "b" is disjunct
	vals, err := Parse("{a:Integer, b?:String}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	m, _ := eng.AsMap(vals[0])
	if m == nil {
		t.Fatalf("eng.AsMap() returned nil")
	}
	aVal, _ := m.Get("a")
	if eng.IsDisjunct(aVal) {
		t.Errorf("expected 'a' to NOT be a disjunct, got %s", aVal.String())
	}
	bVal, _ := m.Get("b")
	if !eng.IsDisjunct(bVal) {
		t.Errorf("expected 'b' to be a disjunct, got %s", bVal.String())
	}
}

func TestParseOptionalFieldInList(t *testing.T) {
	// [x?:Integer] → implicit map with key "x" and disjunct value
	vals, err := Parse("[x?:Integer]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	list := vals[0]
	_lst, _ := eng.AsList(list)
	elems := _lst.Slice()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d: %s", len(elems), list.String())
	}
	m, _ := eng.AsMutableMap(elems[0])
	if m == nil {
		t.Fatalf("expected map element, got %s (data: %T)", elems[0].String(), elems[0].Data)
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("expected key 'x', got %v", keys)
	}
	val, _ := m.Get("x")
	if !eng.IsDisjunct(val) {
		t.Errorf("expected disjunct for x, got %s", val.String())
	}
}

func TestParseComputedKey(t *testing.T) {
	// {[x]:1} → key "x" with value 1
	vals, err := Parse("{[x]:1}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	m, _ := eng.AsMap(vals[0])
	if m == nil {
		t.Fatalf("eng.AsMap() returned nil, value: %s (data: %T)", vals[0].String(), vals[0].Data)
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("expected key 'x', got %v", keys)
	}
}

func TestParseComputedKeyMultiple(t *testing.T) {
	// {[a]:1, [b]:2} → keys "a" and "b"
	vals, err := Parse("{[a]:1, [b]:2}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	m, _ := eng.AsMap(vals[0])
	if m == nil {
		t.Fatalf("eng.AsMap() returned nil")
	}
	keys := m.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %v", keys)
	}
}

// --- String interpolation tests ---

func TestParseBacktickNoInterpolation(t *testing.T) {
	// Backtick string without ${} is just a plain string.
	assertParse(t, "`hello`", []eng.Value{eng.NewString("hello")})
}

func TestParseBacktickSimpleInterpolation(t *testing.T) {
	// `hello ${name}` produces an InterpString with 2 parts.
	got, err := Parse("`hello ${name}`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	if !eng.IsInterpString(got[0]) {
		t.Fatalf("expected InterpString, got %s", got[0].Parent)
	}
	parts, _ := eng.AsInterpString(got[0])
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	// Part 0: literal "hello "
	if parts[0].Expr != nil || parts[0].Lit != "hello " {
		t.Errorf("part 0: expected literal 'hello ', got %+v", parts[0])
	}
	// Part 1: expression [Word(name)]
	if parts[1].Expr == nil || len(parts[1].Expr) != 1 || !eng.IsWord(parts[1].Expr[0]) {
		t.Errorf("part 1: expected expression with Word(name), got %+v", parts[1])
	}
}

func TestParseBacktickMultipleInterpolations(t *testing.T) {
	// `${a} and ${b}` → [empty-lit, expr(a), " and ", expr(b), empty-lit]
	got, err := Parse("`${a} and ${b}`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !eng.IsInterpString(got[0]) {
		t.Fatalf("expected 1 InterpString value, got %d values", len(got))
	}
	parts, _ := eng.AsInterpString(got[0])
	// Parts: expr(a), " and ", expr(b)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
}

func TestParseBacktickExpressionInterpolation(t *testing.T) {
	// `result: ${1 add 2}` → InterpString with expression [1, Word(add), 2]
	got, err := Parse("`result: ${1 add 2}`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !eng.IsInterpString(got[0]) {
		t.Fatalf("expected 1 InterpString value")
	}
	parts, _ := eng.AsInterpString(got[0])
	// Parts: "result: ", expr(1 add 2), empty trailing
	if parts[0].Lit != "result: " {
		t.Errorf("expected literal 'result: ', got %q", parts[0].Lit)
	}
	if parts[1].Expr == nil || len(parts[1].Expr) != 3 {
		t.Errorf("expected expression with 3 values, got %+v", parts[1])
	}
}

func TestParseBacktickNestedBraces(t *testing.T) {
	// `${{a:1}}` → expression containing a map
	got, err := Parse("`${{a:1}}`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !eng.IsInterpString(got[0]) {
		t.Fatalf("expected 1 InterpString value")
	}
}

func TestParseBacktickUnclosedInterpolation(t *testing.T) {
	// `${name` → error: unclosed interpolation
	_, err := Parse("`${name`")
	if err == nil {
		t.Fatal("expected error for unclosed interpolation")
	}
}

func TestParseBacktickOnlyLiteral(t *testing.T) {
	// Backtick string with $ but no ${ is just a plain string.
	assertParse(t, "`price: $100`", []eng.Value{eng.NewString("price: $100")})
}

// --- Words starting with '-' (CLI flag style) ---
//
// `-h`, `--help`, `--limit` need to parse as Words so a CLI built
// on the engine (e.g. sdkgen go-cli) can register them as native AQL
// words and dispatch the same way as any other word. The grammar
// preserves the existing number-literal precedence — `-3.14`,
// `-42`, `+5` still tokenise as numbers because matchNumber only
// returns nil when the sign isn't followed by digits/`.`.

func TestParseSingleDashWord(t *testing.T) {
	assertParse(t, "-h", []eng.Value{eng.NewWord("-h")})
}

func TestParseDoubleDashWord(t *testing.T) {
	assertParse(t, "--help", []eng.Value{eng.NewWord("--help")})
}

func TestParseDashWordWithBareword(t *testing.T) {
	assertParse(t, "-h book", []eng.Value{
		eng.NewWord("-h"),
		eng.NewWord("book"),
	})
}

func TestParseDashWordWithIntegerArg(t *testing.T) {
	assertParse(t, "--limit 10", []eng.Value{
		eng.NewWord("--limit"),
		eng.NewInteger(10),
	})
}

func TestParseDashWordPreservesNegativeDecimal(t *testing.T) {
	// `-3.14` must still tokenise as Decimal, not Word("-3.14").
	assertParse(t, "-3.14", []eng.Value{eng.NewDecimal(-3.14)})
}

func TestParseDashWordPreservesNegativeInteger(t *testing.T) {
	assertParse(t, "-42", []eng.Value{eng.NewInteger(-42)})
}

func TestParseDashLetterDigitMix(t *testing.T) {
	// `-x5` is a Word (the leading `-` is followed by a non-digit).
	assertParse(t, "-x5", []eng.Value{eng.NewWord("-x5")})
}
