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
			aw.ForceStack == bw.ForceStack &&
			aw.ForceForward == bw.ForceForward
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

func TestParseForwardExpression(t *testing.T) {
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

func TestParseForceForwardModifier(t *testing.T) {
	// lower/f
	assertParse(t, "lower/f", []engine.Value{
		engine.NewWordModified("lower", -1, false, true),
	})
}

func TestParseForceStackModifier(t *testing.T) {
	// lower/s
	assertParse(t, "lower/s", []engine.Value{
		engine.NewWordModified("lower", -1, true, false),
	})
}

func TestParseArgCountAndForwardModifier(t *testing.T) {
	// lower/1f
	assertParse(t, "lower/1f", []engine.Value{
		engine.NewWordModified("lower", 1, false, true),
	})
}

func TestParseArgCountAndStackModifier(t *testing.T) {
	// lower/1s
	assertParse(t, "lower/1s", []engine.Value{
		engine.NewWordModified("lower", 1, true, false),
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
	// B lower/f → word then modified word
	assertParse(t, "B lower/f", []engine.Value{
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

// --- Multiline tests ---

func TestParseNewlines(t *testing.T) {
	assertParse(t, "1\nadd\n2", []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
	})
}

func TestParseCRLF(t *testing.T) {
	assertParse(t, "1\r\nadd\r\n2", []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
	})
}

func TestParseBlankLines(t *testing.T) {
	assertParse(t, "1\n\n\nadd\n\n2", []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
	})
}

func TestParseMultilineWithTabs(t *testing.T) {
	assertParse(t, "\t1\n\tadd\n\t2", []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
	})
}

func TestParseMultilineWithComments(t *testing.T) {
	src := "1 # first value\nadd # operator\n2 # second value"
	assertParse(t, src, []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
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
	assertParse(t, src, []engine.Value{
		engine.NewWord("set"),
		engine.NewWord("x"),
		engine.NewInteger(10),
		engine.NewWord("end"),
		engine.NewWord("set"),
		engine.NewWord("y"),
		engine.NewInteger(20),
		engine.NewWord("end"),
		engine.NewWord("get"),
		engine.NewWord("x"),
		engine.NewWord("add"),
		engine.NewWord("get"),
		engine.NewWord("y"),
	})
}

// --- Typed list tests (list.child) ---

func TestParseTypedListString(t *testing.T) {
	// [:String] → typed list with child type string
	assertParse(t, "[:String]", []engine.Value{
		engine.NewTypedList(engine.NewTypeLiteral(engine.TString)),
	})
}

func TestParseTypedListNumber(t *testing.T) {
	// [:Number] → typed list with child type number
	assertParse(t, "[:Number]", []engine.Value{
		engine.NewTypedList(engine.NewTypeLiteral(engine.TNumber)),
	})
}

func TestParseTypedListBoolean(t *testing.T) {
	assertParse(t, "[:Boolean]", []engine.Value{
		engine.NewTypedList(engine.NewTypeLiteral(engine.TBoolean)),
	})
}

func TestParseTypedListAny(t *testing.T) {
	assertParse(t, "[:Any]", []engine.Value{
		engine.NewTypedList(engine.NewTypeLiteral(engine.TAny)),
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
	if !got[0].IsTypedList() {
		t.Fatalf("expected typed list, got %s", got[0])
	}
	child := got[0].AsChildType().Child
	if !child.VType.Equal(engine.TMap) {
		t.Errorf("expected child type map, got %s", child.VType)
	}
	m := child.AsMap()
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatalf("expected key 'x' in child map")
	}
	if !xVal.VType.Equal(engine.TNumber) {
		t.Errorf("expected x to be number type, got %s (TestParseTypedListMap)", xVal.VType)
	}
}

func TestParseTypedListNested(t *testing.T) {
	// [:[:String]] → typed list of typed lists of strings
	assertParse(t, "[:[:String]]", []engine.Value{
		engine.NewTypedList(engine.NewTypedList(engine.NewTypeLiteral(engine.TString))),
	})
}

func TestParseTypedListDeepNested(t *testing.T) {
	// [:[:[:Number]]] → three levels deep
	assertParse(t, "[:[:[:Number]]]", []engine.Value{
		engine.NewTypedList(engine.NewTypedList(engine.NewTypedList(engine.NewTypeLiteral(engine.TNumber)))),
	})
}

func TestParseTypedListInExpression(t *testing.T) {
	// 1 [:String] → integer then typed list
	assertParse(t, "1 [:String]", []engine.Value{
		engine.NewInteger(1),
		engine.NewTypedList(engine.NewTypeLiteral(engine.TString)),
	})
}

func TestParseTypedListMapChild(t *testing.T) {
	// [:{a:String,b:Number}] → typed list with multi-key map child
	got, err := Parse("[:{a:String,b:Number}]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !got[0].IsTypedList() {
		t.Fatalf("expected 1 typed list, got %v", got)
	}
	child := got[0].AsChildType().Child
	m := child.AsMap()
	if m.Len() != 2 {
		t.Errorf("expected 2 keys, got %d", m.Len())
	}
}

// --- Typed map tests (map.child) ---

func TestParseTypedMapString(t *testing.T) {
	// {:String} → typed map with child type string
	assertParse(t, "{:String}", []engine.Value{
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TString)),
	})
}

func TestParseTypedMapNumber(t *testing.T) {
	// {:Number} → typed map with child type number
	assertParse(t, "{:Number}", []engine.Value{
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TNumber)),
	})
}

func TestParseTypedMapBoolean(t *testing.T) {
	assertParse(t, "{:Boolean}", []engine.Value{
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TBoolean)),
	})
}

func TestParseTypedMapAny(t *testing.T) {
	assertParse(t, "{:Any}", []engine.Value{
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TAny)),
	})
}

func TestParseTypedMapList(t *testing.T) {
	// {:[:Number]} → typed map with child type [:Number]
	assertParse(t, "{:[:Number]}", []engine.Value{
		engine.NewTypedMap(engine.NewTypedList(engine.NewTypeLiteral(engine.TNumber))),
	})
}

func TestParseTypedMapNested(t *testing.T) {
	// {:{:String}} → typed map of typed maps of strings
	assertParse(t, "{:{:String}}", []engine.Value{
		engine.NewTypedMap(engine.NewTypedMap(engine.NewTypeLiteral(engine.TString))),
	})
}

func TestParseTypedMapDeepNested(t *testing.T) {
	// {:{:{:Number}}} → three levels deep
	assertParse(t, "{:{:{:Number}}}", []engine.Value{
		engine.NewTypedMap(engine.NewTypedMap(engine.NewTypedMap(engine.NewTypeLiteral(engine.TNumber)))),
	})
}

func TestParseTypedMapInExpression(t *testing.T) {
	// 1 {:String} → integer then typed map
	assertParse(t, "1 {:String}", []engine.Value{
		engine.NewInteger(1),
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TString)),
	})
}

func TestParseTypedMapConcreteChild(t *testing.T) {
	// {:{x:Number}} → typed map with map child type
	got, err := Parse("{:{x:Number}}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 || !got[0].IsTypedMap() {
		t.Fatalf("expected 1 typed map, got %v", got)
	}
	child := got[0].AsChildType().Child
	if !child.VType.Equal(engine.TMap) {
		t.Errorf("expected child type map, got %s", child.VType)
	}
	m := child.AsMap()
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatalf("expected key 'x' in child map")
	}
	if !xVal.VType.Equal(engine.TNumber) {
		t.Errorf("expected x to be number type, got %s (TestParseTypedMapConcreteChild)", xVal.VType)
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
	if !got[0].VType.Equal(engine.TList) {
		t.Fatalf("expected list, got %s", got[0].VType)
	}
	elems := got[0].AsList()
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
	elems := got[0].AsList()
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
	if !got[0].VType.Equal(engine.TMap) {
		t.Fatalf("expected map, got %s", got[0].VType)
	}
	m := got[0].AsMap()
	xVal, ok := m.Get("x")
	if !ok {
		t.Fatal("expected key 'x'")
	}
	if !xVal.VType.Equal(engine.TList) {
		t.Errorf("expected list, got %s", xVal.VType)
	}
	elems := xVal.AsList()
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
	m := got[0].AsMap()
	aVal, _ := m.Get("a")
	if !aVal.IsWord() || aVal.AsWord().Name != "true" {
		t.Errorf("expected word(true), got %s", aVal)
	}
	bVal, _ := m.Get("b")
	if !bVal.IsWord() || bVal.AsWord().Name != "false" {
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
	m := got[0].AsMap()
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
	m := got[0].AsMap()
	xVal, _ := m.Get("x")
	if !xVal.VType.Equal(engine.TNumber) {
		t.Errorf("expected number type literal, got %s", xVal.VType)
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
	m := got[0].AsMap()
	aVal, _ := m.Get("a")
	if !aVal.VType.Equal(engine.TMap) {
		t.Errorf("expected nested map, got %s", aVal.VType)
	}
}

// --- preprocessParens tests ---

func TestParseParensInString(t *testing.T) {
	// "(hello)" → the parens are inside a string, not structural
	assertParse(t, `"(hello)"`, []engine.Value{engine.NewString("(hello)")})
}

func TestParseNoParens(t *testing.T) {
	// No parens — preprocessParens should be a no-op
	assertParse(t, "1 2 3", []engine.Value{
		engine.NewInteger(1),
		engine.NewInteger(2),
		engine.NewInteger(3),
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
	assertParse(t, "1 add 2; 99", []engine.Value{
		engine.NewInteger(1),
		engine.NewWord("add"),
		engine.NewInteger(2),
		engine.NewWord("end"),
		engine.NewInteger(99),
	})
}

func TestParseSemicolonStandalone(t *testing.T) {
	assertParse(t, ";", []engine.Value{
		engine.NewWord("end"),
	})
}

func TestParseSemicolonAdjacentToWord(t *testing.T) {
	// "foo;bar" — semicolon is a fixed token, so it splits the text
	assertParse(t, "foo;bar", []engine.Value{
		engine.NewWord("foo"),
		engine.NewWord("end"),
		engine.NewWord("bar"),
	})
}

// --- expandDottedWord direct tests ---

func assertExpand(t *testing.T, text string, want []engine.Value) {
	t.Helper()
	got, err := expandDottedWord(text)
	if err != nil {
		t.Fatalf("expandDottedWord(%q) error: %v", text, err)
	}
	if len(got) != len(want) {
		t.Fatalf("expandDottedWord(%q) got %d values, want %d\n  got:  %v\n  want: %v",
			text, len(got), len(want), got, want)
	}
	for i := range got {
		if !valuesEqual(got[i], want[i]) {
			t.Errorf("expandDottedWord(%q)[%d] = %s, want %s", text, i, got[i], want[i])
		}
	}
}

func TestExpandDotStandalone(t *testing.T) {
	assertExpand(t, ".", []engine.Value{engine.NewWord("get")})
}

func TestExpandBangDotStandalone(t *testing.T) {
	assertExpand(t, "!.", []engine.Value{engine.NewWord("getr")})
}

func TestExpandDottedSimple(t *testing.T) {
	assertExpand(t, "foo.bar", []engine.Value{
		engine.NewOpenParen(),
		engine.NewWord("foo"),
		engine.NewWord("get"),
		engine.NewWord("bar"),
		engine.NewWord(")"),
	})
}

func TestExpandDottedChain(t *testing.T) {
	assertExpand(t, "foo.a.b", []engine.Value{
		engine.NewOpenParen(),
		engine.NewWord("foo"),
		engine.NewWord("get"),
		engine.NewWord("a"),
		engine.NewWord("get"),
		engine.NewWord("b"),
		engine.NewWord(")"),
	})
}

func TestExpandDottedLeading(t *testing.T) {
	assertExpand(t, ".a.b", []engine.Value{
		engine.NewWord("get"),
		engine.NewWord("a"),
		engine.NewWord("get"),
		engine.NewWord("b"),
	})
}

func TestExpandDottedIntegerKey(t *testing.T) {
	assertExpand(t, "foo.0", []engine.Value{
		engine.NewOpenParen(),
		engine.NewWord("foo"),
		engine.NewWord("get"),
		engine.NewInteger(0),
		engine.NewWord(")"),
	})
}

func TestExpandDottedTrailingDot(t *testing.T) {
	assertExpand(t, "foo.", []engine.Value{
		engine.NewOpenParen(),
		engine.NewWord("foo"),
		engine.NewWord(")"),
	})
}

func TestExpandDottedEmptyMiddle(t *testing.T) {
	// "foo..bar" → empty segment skipped
	assertExpand(t, "foo..bar", []engine.Value{
		engine.NewOpenParen(),
		engine.NewWord("foo"),
		engine.NewWord("get"),
		engine.NewWord("bar"),
		engine.NewWord(")"),
	})
}

func TestExpandDottedLeadingSingle(t *testing.T) {
	// ".x" → leading dot, just dot x
	assertExpand(t, ".x", []engine.Value{
		engine.NewWord("get"),
		engine.NewWord("x"),
	})
}

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
	m := got[0].AsMap()
	// b should be word "true" (word context)
	bVal, _ := m.Get("b")
	if !bVal.IsWord() || bVal.AsWord().Name != "true" {
		t.Errorf("expected word(true), got %s", bVal)
	}
	// c should be word "false" (word context)
	cVal, _ := m.Get("c")
	if !cVal.IsWord() || cVal.AsWord().Name != "false" {
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
	m := got[0].AsMap()
	xVal, _ := m.Get("x")
	if !xVal.VType.Equal(engine.TMap) {
		t.Fatalf("expected nested map, got %s", xVal.VType)
	}
	inner := xVal.AsMap()
	yVal, _ := inner.Get("y")
	if !yVal.VType.Equal(engine.TList) {
		t.Errorf("expected list, got %s", yVal.VType)
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
	m := got[0].AsMap()
	xVal, _ := m.Get("x")
	elems := xVal.AsList()
	if len(elems) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(elems))
	}
	// With word context in lists, true/false become words (resolved at runtime),
	// and null becomes a word (resolved to atom at runtime).
	if !elems[0].IsWord() || elems[0].AsWord().Name != "true" {
		t.Errorf("expected word(true), got %s", elems[0])
	}
	if !elems[1].IsWord() || elems[1].AsWord().Name != "false" {
		t.Errorf("expected word(false), got %s", elems[1])
	}
	if !elems[2].IsWord() || elems[2].AsWord().Name != "null" {
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
	if !got[0].VType.Matches(engine.TDecimal) {
		t.Errorf("expected decimal type, got %s", got[0].VType)
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
	if !got[0].VType.Matches(engine.TDecimal) {
		t.Errorf("expected decimal, got %s", got[0].VType)
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
	m := got[0].AsMap()
	xVal, _ := m.Get("x")
	if !xVal.VType.Matches(engine.TDecimal) {
		t.Errorf("expected decimal, got %s", xVal.VType)
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
	elems := got[0].AsList()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
	if !elems[0].VType.Equal(engine.TMap) {
		t.Errorf("expected map element, got %s", elems[0].VType)
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
	m := got[0].AsMap()
	xVal, _ := m.Get("x")
	elems := xVal.AsList()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
}

// --- List with dotted word ---

func TestParseListWithDottedWord(t *testing.T) {
	// [foo.bar] → list with dotted word expansion in word context
	got, err := Parse("[foo.bar]")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	elems := got[0].AsList()
	// ( foo dot bar ) = 5 elements
	if len(elems) != 5 {
		t.Fatalf("expected 5 elements (( foo dot bar )), got %d", len(elems))
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
	m := got[0].AsMap()
	if m.Len() != 1 {
		t.Errorf("expected 1 key, got %d", m.Len())
	}
}

// --- Word modifier edge cases ---

func TestParseUnrecognizedModifier(t *testing.T) {
	// foo/x → unrecognized modifier, treated as plain word "foo/x"
	assertParse(t, "foo/x", []engine.Value{engine.NewWord("foo/x")})
}

func TestParseSlashOnly(t *testing.T) {
	// foo/ → trailing slash with no modifier text (idx == len(name)-1),
	// so it's not matched by the modifier parsing
	assertParse(t, "foo/", []engine.Value{engine.NewWord("foo/")})
}

func TestParseEmptyDigitsEmptyRest(t *testing.T) {
	// This covers the case where modifier is just "/" at end of string
	// which doesn't trigger the modifier parsing (idx >= len(name)-1 check)
	assertParse(t, "x/", []engine.Value{engine.NewWord("x/")})
}

// --- Parens within different quote types ---

func TestParseSingleQuoteWithParens(t *testing.T) {
	// '(test)' → parens inside single-quoted string
	assertParse(t, "'(test)'", []engine.Value{engine.NewString("(test)")})
}

func TestParseBacktickWithParens(t *testing.T) {
	// `(test)` → parens inside backtick-quoted string
	assertParse(t, "`(test)`", []engine.Value{engine.NewString("(test)")})
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

// --- Args expansion ---

func TestExpandDottedArgs(t *testing.T) {
	// args.x → resolves "args" as a word directly (not via get), wrapped in parens
	assertExpand(t, "args.x", []engine.Value{
		engine.NewOpenParen(),
		engine.NewWord("args"),
		engine.NewWord("get"),
		engine.NewWord("x"),
		engine.NewWord(")"),
	})
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
	// 1 foo.bar → triggers dotted expansion in convertTopLevel, wrapped in parens
	got, err := Parse("1 foo.bar")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// 1 ( foo dot bar ) = 6 values
	if len(got) != 6 {
		t.Fatalf("expected 6 values (1 ( foo dot bar )), got %d", len(got))
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
	if !got[0].VType.Equal(engine.TMap) {
		t.Errorf("expected map, got %s", got[0].VType)
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
	m := got[0].AsMap()
	xVal, _ := m.Get("x")
	if !xVal.VType.Matches(engine.TString) || xVal.AsString() != "hello" {
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
	assertParse(t, "null", []engine.Value{engine.NewWord("null")})
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
	m := got[0].AsMap()
	// Check type literal resolution in data context
	eVal, _ := m.Get("e")
	if !eVal.VType.Equal(engine.TNumber) {
		t.Errorf("expected Number type literal, got %s", eVal.VType)
	}
	fVal, _ := m.Get("f")
	if !fVal.VType.Equal(engine.TAny) {
		t.Errorf("expected Any type literal, got %s", fVal.VType)
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
	m := got[0].AsMap()
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
	m := got[0].AsMap()
	xVal, _ := m.Get("x")
	elems := xVal.AsList()
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
	if !v.VType.Matches(engine.TInteger) {
		t.Errorf("expected integer, got %s", v.VType)
	}
}

func TestFloatToValueFractional(t *testing.T) {
	// Fractional float → decimal
	v := floatToValue(3.14)
	if !v.VType.Matches(engine.TDecimal) {
		t.Errorf("expected decimal, got %s", v.VType)
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
		check func(engine.Value) bool
	}{
		{"true", func(v engine.Value) bool { return v.VType.Matches(engine.TBoolean) && v.AsBoolean() }},
		{"false", func(v engine.Value) bool { return v.VType.Matches(engine.TBoolean) && !v.AsBoolean() }},
		{"Number", func(v engine.Value) bool { return v.VType.Equal(engine.TNumber) }},
		{"String", func(v engine.Value) bool { return v.VType.Equal(engine.TString) }},
		{"hello", func(v engine.Value) bool { return v.VType.Matches(engine.TAtom) && v.AsString() == "hello" }},
	}
	for _, tt := range tests {
		v := resolveTextValue(tt.input)
		if !tt.check(v) {
			t.Errorf("resolveTextValue(%q) = %s, unexpected", tt.input, v)
		}
	}
}

func TestExpandDottedWordModifier(t *testing.T) {
	// foo/1.bar → first part has modifier, should work
	got, err := expandDottedWord("foo/1.bar")
	if err != nil {
		t.Fatalf("expandDottedWord error: %v", err)
	}
	if len(got) < 3 {
		t.Fatalf("expected at least 3 values, got %d", len(got))
	}
}

// --- Direct unexported function tests for defensive code paths ---

func TestConvertTopLevelValueBool(t *testing.T) {
	v, err := convertTopLevelValue(true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.VType.Matches(engine.TBoolean) || !v.AsBoolean() {
		t.Errorf("expected true, got %s", v)
	}
	v, err = convertTopLevelValue(false)
	if err != nil {
		t.Fatal(err)
	}
	if !v.VType.Matches(engine.TBoolean) || v.AsBoolean() {
		t.Errorf("expected false, got %s", v)
	}
}

func TestConvertTopLevelValueNil(t *testing.T) {
	v, err := convertTopLevelValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.VType.Equal(engine.TNone) {
		t.Errorf("expected none type, got %s", v.VType)
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
	if !v.VType.Matches(engine.TBoolean) || !v.AsBoolean() {
		t.Errorf("expected true, got %s", v)
	}
}

func TestConvertDataValueNil(t *testing.T) {
	v, err := convertDataValue(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !v.VType.Equal(engine.TNone) {
		t.Errorf("expected none type, got %s", v.VType)
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
	if !v.VType.Equal(engine.TMap) {
		t.Errorf("expected map, got %s", v.VType)
	}
}

func TestConvertDataValueRawMapWithChild(t *testing.T) {
	// map[string]any with child$ key → typed map
	v, err := convertDataValue(map[string]any{"child$": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsTypedMap() {
		t.Errorf("expected typed map, got %s", v)
	}
}

func TestConvertTopLevelValueRawMap(t *testing.T) {
	v, err := convertTopLevelValue(map[string]any{"x": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !v.VType.Equal(engine.TMap) {
		t.Errorf("expected map, got %s", v.VType)
	}
}

func TestConvertTopLevelValueRawMapWithChild(t *testing.T) {
	v, err := convertTopLevelValue(map[string]any{"child$": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsTypedMap() {
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
		{"foo/bad", "foo/bad", true},   // unrecognized modifier
		{"foo/", "foo/", true},         // slash at end not processed
	}
	for _, tt := range tests {
		v, err := parseWord(tt.input)
		if tt.ok && err != nil {
			t.Errorf("parseWord(%q) error: %v", tt.input, err)
		} else if !tt.ok && err == nil {
			t.Errorf("parseWord(%q) expected error", tt.input)
		}
		if tt.ok && v.AsWord().Name != tt.name {
			t.Errorf("parseWord(%q) name = %q, want %q", tt.input, v.AsWord().Name, tt.name)
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
	if !list.VType.Equal(engine.TList) {
		t.Fatalf("expected list, got %s", list.VType)
	}
	elems := list.AsList()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element in list, got %d", len(elems))
	}
	if !elems[0].VType.Equal(engine.TMap) {
		t.Fatalf("expected map element, got %s", elems[0].VType)
	}
	m := elems[0].AsMap()
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
	elems := list.AsList()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element in list, got %d", len(elems))
	}
	if !elems[0].VType.Equal(engine.TMap) {
		t.Fatalf("expected map element, got %s", elems[0].VType)
	}
	m := elems[0].AsMap()
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
	if !vals[0].VType.Equal(engine.TMap) {
		t.Fatalf("expected map, got %s", vals[0].VType)
	}
	m := vals[0].AsMap()
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
	m := vals[0].AsMap()
	if m == nil {
		t.Fatalf("AsMap() returned nil, value: %s (data: %T)", vals[0].String(), vals[0].Data)
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "a" {
		t.Errorf("expected key 'a', got %v", keys)
	}
	val, _ := m.Get("a")
	if !val.IsDisjunct() {
		t.Fatalf("expected disjunct for optional field, got %s", val.String())
	}
	alts := val.AsDisjunct().Alternatives
	if len(alts) != 2 {
		t.Fatalf("expected 2 alternatives, got %d", len(alts))
	}
	if !alts[1].VType.Equal(engine.TNone) {
		t.Errorf("expected second alternative to be None, got %s", alts[1].VType)
	}
}

func TestParseOptionalFieldMixed(t *testing.T) {
	// {a:Integer, b?:String} → "a" is plain, "b" is disjunct
	vals, err := Parse("{a:Integer, b?:String}")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	m := vals[0].AsMap()
	if m == nil {
		t.Fatalf("AsMap() returned nil")
	}
	aVal, _ := m.Get("a")
	if aVal.IsDisjunct() {
		t.Errorf("expected 'a' to NOT be a disjunct, got %s", aVal.String())
	}
	bVal, _ := m.Get("b")
	if !bVal.IsDisjunct() {
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
	elems := list.AsList()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d: %s", len(elems), list.String())
	}
	m := elems[0].AsMap()
	if m == nil {
		t.Fatalf("expected map element, got %s (data: %T)", elems[0].String(), elems[0].Data)
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "x" {
		t.Errorf("expected key 'x', got %v", keys)
	}
	val, _ := m.Get("x")
	if !val.IsDisjunct() {
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
	m := vals[0].AsMap()
	if m == nil {
		t.Fatalf("AsMap() returned nil, value: %s (data: %T)", vals[0].String(), vals[0].Data)
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
	m := vals[0].AsMap()
	if m == nil {
		t.Fatalf("AsMap() returned nil")
	}
	keys := m.Keys()
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %v", keys)
	}
}

