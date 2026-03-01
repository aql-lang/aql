package parser

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// valuesEqual compares two engine.Value instances for equality.
func valuesEqual(a, b engine.Value) bool {
	if !a.VType.Equal(b.VType) {
		return false
	}
	switch {
	case a.IsWord():
		aw, bw := a.AsWord(), b.AsWord()
		return aw.Name == bw.Name &&
			aw.ArgCount == bw.ArgCount &&
			aw.ForcePrefix == bw.ForcePrefix &&
			aw.ForceSuffix == bw.ForceSuffix
	case a.IsOpenParen():
		return true
	case a.VType.Matches(engine.TString):
		return a.AsString() == b.AsString()
	case a.VType.Matches(engine.TInteger):
		return a.AsInteger() == b.AsInteger()
	default:
		return a.String() == b.String()
	}
}

func assertParse(t *testing.T, input string, want []engine.Value) {
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
	assertParse(t, "1", []engine.Value{engine.NewInteger(1)})
}

func TestParseZero(t *testing.T) {
	assertParse(t, "0", []engine.Value{engine.NewInteger(0)})
}

func TestParseNegativeInteger(t *testing.T) {
	assertParse(t, "-5", []engine.Value{engine.NewInteger(-5)})
}

func TestParseLargeInteger(t *testing.T) {
	assertParse(t, "999", []engine.Value{engine.NewInteger(999)})
}

func TestParseMultipleIntegers(t *testing.T) {
	assertParse(t, "1 2 3", []engine.Value{
		engine.NewInteger(1),
		engine.NewInteger(2),
		engine.NewInteger(3),
	})
}

func TestParseQuotedStringDouble(t *testing.T) {
	assertParse(t, `"hello"`, []engine.Value{engine.NewString("hello")})
}

func TestParseQuotedStringSingle(t *testing.T) {
	assertParse(t, `'world'`, []engine.Value{engine.NewString("world")})
}

func TestParseEmptyQuotedString(t *testing.T) {
	assertParse(t, `""`, []engine.Value{engine.NewString("")})
}

func TestParseQuotedStringWithSpaces(t *testing.T) {
	assertParse(t, `"hello world"`, []engine.Value{engine.NewString("hello world")})
}

// --- Word tests ---

func TestParseSingleWord(t *testing.T) {
	assertParse(t, "upper", []engine.Value{engine.NewWord("upper")})
}

func TestParseUnknownWord(t *testing.T) {
	assertParse(t, "foo", []engine.Value{engine.NewWord("foo")})
}

func TestParseEndKeyword(t *testing.T) {
	assertParse(t, "end", []engine.Value{engine.NewWord("end")})
}

func TestParseMultipleWords(t *testing.T) {
	assertParse(t, "a b c", []engine.Value{
		engine.NewWord("a"),
		engine.NewWord("b"),
		engine.NewWord("c"),
	})
}

// --- Mixed token tests ---

func TestParsePrefixExpression(t *testing.T) {
	// a upper → two words: the engine resolves unknown "a" to a string
	assertParse(t, "a upper", []engine.Value{
		engine.NewWord("a"),
		engine.NewWord("upper"),
	})
}

func TestParseSuffixExpression(t *testing.T) {
	// lower B → two words
	assertParse(t, "lower B", []engine.Value{
		engine.NewWord("lower"),
		engine.NewWord("B"),
	})
}

func TestParsePrefixArithmetic(t *testing.T) {
	// 1 2 add → two integers then word
	assertParse(t, "1 2 add", []engine.Value{
		engine.NewInteger(1),
		engine.NewInteger(2),
		engine.NewWord("add"),
	})
}

func TestParseInfixArithmetic(t *testing.T) {
	// 1 add 2
	assertParse(t, "1 add 2", []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
	})
}

func TestParseChainedArithmetic(t *testing.T) {
	assertParse(t, "1 add 2 add 3", []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
		engine.NewWord("add"),
		engine.NewInteger(3),
	})
}

func TestParseMixedOperators(t *testing.T) {
	// 2 add 3 mul 4
	assertParse(t, "2 add 3 mul 4", []engine.Value{
		engine.NewInteger(2),
		engine.NewWord("add"),
		engine.NewInteger(3),
		engine.NewWord("mul"),
		engine.NewInteger(4),
	})
}

func TestParseStringThenWord(t *testing.T) {
	// "hello" upper → string then word
	assertParse(t, `"hello" upper`, []engine.Value{
		engine.NewString("hello"),
		engine.NewWord("upper"),
	})
}

func TestParseSetWithEnd(t *testing.T) {
	// set foo 99 end
	assertParse(t, "set foo 99 end", []engine.Value{
		engine.NewWord("set"),
		engine.NewWord("foo"),
		engine.NewInteger(99),
		engine.NewWord("end"),
	})
}

// --- Parentheses ---

func TestParseSimpleParens(t *testing.T) {
	// (1 add 2)
	assertParse(t, "(1 add 2)", []engine.Value{
		engine.NewWord("("),
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
		engine.NewWord(")"),
	})
}

func TestParseNestedParens(t *testing.T) {
	// (1 add (2 mul 3))
	assertParse(t, "(1 add (2 mul 3))", []engine.Value{
		engine.NewWord("("),
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewWord("("),
		engine.NewInteger(2),
		engine.NewWord("mul"),
		engine.NewInteger(3),
		engine.NewWord(")"),
		engine.NewWord(")"),
	})
}

func TestParseAdjacentParens(t *testing.T) {
	// (1)(2) — no space between groups
	assertParse(t, "(1)(2)", []engine.Value{
		engine.NewWord("("),
		engine.NewInteger(1),
		engine.NewWord(")"),
		engine.NewWord("("),
		engine.NewInteger(2),
		engine.NewWord(")"),
	})
}

func TestParseParenAroundWord(t *testing.T) {
	// (add)
	assertParse(t, "(add)", []engine.Value{
		engine.NewWord("("),
		engine.NewWord("add"),
		engine.NewWord(")"),
	})
}

// --- Word modifier tests ---

func TestParseArgCountModifier(t *testing.T) {
	// lower/1
	assertParse(t, "lower/1", []engine.Value{
		engine.NewWordModified("lower", 1, false, false),
	})
}

func TestParseForceSuffixModifier(t *testing.T) {
	// lower=
	assertParse(t, "lower=", []engine.Value{
		engine.NewWordModified("lower", -1, false, true),
	})
}

func TestParseForcePrefixModifier(t *testing.T) {
	// =lower
	assertParse(t, "=lower", []engine.Value{
		engine.NewWordModified("lower", -1, true, false),
	})
}

func TestParseArgCountAndSuffixModifier(t *testing.T) {
	// lower/1=
	assertParse(t, "lower/1=", []engine.Value{
		engine.NewWordModified("lower", 1, false, true),
	})
}

func TestParseArgCountZero(t *testing.T) {
	// dup/0
	assertParse(t, "dup/0", []engine.Value{
		engine.NewWordModified("dup", 0, false, false),
	})
}

func TestParseArgCountTwo(t *testing.T) {
	// set/2
	assertParse(t, "set/2", []engine.Value{
		engine.NewWordModified("set", 2, false, false),
	})
}

func TestParseModifierInExpression(t *testing.T) {
	// B lower= → word then modified word
	assertParse(t, "B lower=", []engine.Value{
		engine.NewWord("B"),
		engine.NewWordModified("lower", -1, false, true),
	})
}

// --- String vs word distinction ---

func TestParseQuotedFunctionName(t *testing.T) {
	// "upper" → string, not a word; upper → word
	assertParse(t, `"upper" upper`, []engine.Value{
		engine.NewString("upper"),
		engine.NewWord("upper"),
	})
}

func TestParseQuotedEnd(t *testing.T) {
	// "end" → string (not the end keyword)
	assertParse(t, `"end"`, []engine.Value{engine.NewString("end")})
}

func TestParseQuotedNumber(t *testing.T) {
	// "1" → string, not an integer
	assertParse(t, `"1"`, []engine.Value{engine.NewString("1")})
}

// --- Whitespace handling ---

func TestParseExtraSpaces(t *testing.T) {
	assertParse(t, "1  2", []engine.Value{
		engine.NewInteger(1),
		engine.NewInteger(2),
	})
}

func TestParseLeadingTrailingSpaces(t *testing.T) {
	assertParse(t, "  1  ", []engine.Value{engine.NewInteger(1)})
}

func TestParseTabs(t *testing.T) {
	assertParse(t, "1\t2", []engine.Value{
		engine.NewInteger(1),
		engine.NewInteger(2),
	})
}

func TestParseWhitespaceOnly(t *testing.T) {
	assertParse(t, "   ", nil)
}

// --- Comment handling ---

func TestParseHashComment(t *testing.T) {
	// 1 # this is a comment
	assertParse(t, "1 # this is a comment", []engine.Value{
		engine.NewInteger(1),
	})
}

func TestParseSlashComment(t *testing.T) {
	// 1 // inline comment
	assertParse(t, "1 // inline comment", []engine.Value{
		engine.NewInteger(1),
	})
}

func TestParseCommentOnly(t *testing.T) {
	assertParse(t, "# just a comment", nil)
}

// --- Value keywords disabled (treated as words) ---

func TestParseTrueAsWord(t *testing.T) {
	assertParse(t, "true", []engine.Value{engine.NewWord("true")})
}

func TestParseFalseAsWord(t *testing.T) {
	assertParse(t, "false", []engine.Value{engine.NewWord("false")})
}

func TestParseNullAsWord(t *testing.T) {
	assertParse(t, "null", []engine.Value{engine.NewWord("null")})
}

// --- Full expression tests ---

func TestParseFullPrefixExpression(t *testing.T) {
	// "hello" upper → string then word (engine would call upper on the string)
	assertParse(t, `"hello" upper`, []engine.Value{
		engine.NewString("hello"),
		engine.NewWord("upper"),
	})
}

func TestParseFullInfixWithParens(t *testing.T) {
	// 2 mul (3 add 4)
	assertParse(t, "2 mul (3 add 4)", []engine.Value{
		engine.NewInteger(2),
		engine.NewWord("mul"),
		engine.NewWord("("),
		engine.NewInteger(3),
		engine.NewWord("add"),
		engine.NewInteger(4),
		engine.NewWord(")"),
	})
}

func TestParseForthPrimitives(t *testing.T) {
	// 1 dup swap drop
	assertParse(t, "1 dup swap drop", []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("dup"),
		engine.NewWord("swap"),
		engine.NewWord("drop"),
	})
}

func TestParseStorageSetGet(t *testing.T) {
	// set x 10 end get x
	assertParse(t, "set x 10 end get x", []engine.Value{
		engine.NewWord("set"),
		engine.NewWord("x"),
		engine.NewInteger(10),
		engine.NewWord("end"),
		engine.NewWord("get"),
		engine.NewWord("x"),
	})
}

func TestParseMixedLiteralsAndWords(t *testing.T) {
	// 42 "hello" foo 7
	assertParse(t, `42 "hello" foo 7`, []engine.Value{
		engine.NewInteger(42),
		engine.NewString("hello"),
		engine.NewWord("foo"),
		engine.NewInteger(7),
	})
}

// --- Error cases ---

func TestParseUnterminatedString(t *testing.T) {
	assertParseError(t, `"hello`)
}

func TestParseUnterminatedSingleQuote(t *testing.T) {
	assertParseError(t, `'hello`)
}
