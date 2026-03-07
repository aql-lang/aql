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
	// lower/s
	assertParse(t, "lower/s", []engine.Value{
		engine.NewWordModified("lower", -1, false, true),
	})
}

func TestParseForcePrefixModifier(t *testing.T) {
	// lower/p
	assertParse(t, "lower/p", []engine.Value{
		engine.NewWordModified("lower", -1, true, false),
	})
}

func TestParseArgCountAndSuffixModifier(t *testing.T) {
	// lower/1s
	assertParse(t, "lower/1s", []engine.Value{
		engine.NewWordModified("lower", 1, false, true),
	})
}

func TestParseArgCountAndPrefixModifier(t *testing.T) {
	// lower/1p
	assertParse(t, "lower/1p", []engine.Value{
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
	// B lower/s → word then modified word
	assertParse(t, "B lower/s", []engine.Value{
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
	// [:string] → typed list with child type string
	assertParse(t, "[:string]", []engine.Value{
		engine.NewTypedList(engine.NewTypeLiteral(engine.TString)),
	})
}

func TestParseTypedListNumber(t *testing.T) {
	// [:number] → typed list with child type number
	assertParse(t, "[:number]", []engine.Value{
		engine.NewTypedList(engine.NewTypeLiteral(engine.TNumber)),
	})
}

func TestParseTypedListBoolean(t *testing.T) {
	assertParse(t, "[:boolean]", []engine.Value{
		engine.NewTypedList(engine.NewTypeLiteral(engine.TBoolean)),
	})
}

func TestParseTypedListAny(t *testing.T) {
	assertParse(t, "[:any]", []engine.Value{
		engine.NewTypedList(engine.NewTypeLiteral(engine.TAny)),
	})
}

func TestParseTypedListMap(t *testing.T) {
	// [:{x:number}] → typed list with child type {x:number}
	got, err := Parse("[:{x:number}]")
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
	// [:[:string]] → typed list of typed lists of strings
	assertParse(t, "[:[:string]]", []engine.Value{
		engine.NewTypedList(engine.NewTypedList(engine.NewTypeLiteral(engine.TString))),
	})
}

func TestParseTypedListDeepNested(t *testing.T) {
	// [:[:[:number]]] → three levels deep
	assertParse(t, "[:[:[:number]]]", []engine.Value{
		engine.NewTypedList(engine.NewTypedList(engine.NewTypedList(engine.NewTypeLiteral(engine.TNumber)))),
	})
}

func TestParseTypedListInExpression(t *testing.T) {
	// 1 [:string] → integer then typed list
	assertParse(t, "1 [:string]", []engine.Value{
		engine.NewInteger(1),
		engine.NewTypedList(engine.NewTypeLiteral(engine.TString)),
	})
}

func TestParseTypedListMapChild(t *testing.T) {
	// [:{a:string,b:number}] → typed list with multi-key map child
	got, err := Parse("[:{a:string,b:number}]")
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
	// {:string} → typed map with child type string
	assertParse(t, "{:string}", []engine.Value{
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TString)),
	})
}

func TestParseTypedMapNumber(t *testing.T) {
	// {:number} → typed map with child type number
	assertParse(t, "{:number}", []engine.Value{
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TNumber)),
	})
}

func TestParseTypedMapBoolean(t *testing.T) {
	assertParse(t, "{:boolean}", []engine.Value{
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TBoolean)),
	})
}

func TestParseTypedMapAny(t *testing.T) {
	assertParse(t, "{:any}", []engine.Value{
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TAny)),
	})
}

func TestParseTypedMapList(t *testing.T) {
	// {:[:number]} → typed map with child type [:number]
	assertParse(t, "{:[:number]}", []engine.Value{
		engine.NewTypedMap(engine.NewTypedList(engine.NewTypeLiteral(engine.TNumber))),
	})
}

func TestParseTypedMapNested(t *testing.T) {
	// {:{:string}} → typed map of typed maps of strings
	assertParse(t, "{:{:string}}", []engine.Value{
		engine.NewTypedMap(engine.NewTypedMap(engine.NewTypeLiteral(engine.TString))),
	})
}

func TestParseTypedMapDeepNested(t *testing.T) {
	// {:{:{:number}}} → three levels deep
	assertParse(t, "{:{:{:number}}}", []engine.Value{
		engine.NewTypedMap(engine.NewTypedMap(engine.NewTypedMap(engine.NewTypeLiteral(engine.TNumber)))),
	})
}

func TestParseTypedMapInExpression(t *testing.T) {
	// 1 {:string} → integer then typed map
	assertParse(t, "1 {:string}", []engine.Value{
		engine.NewInteger(1),
		engine.NewTypedMap(engine.NewTypeLiteral(engine.TString)),
	})
}

func TestParseTypedMapConcreteChild(t *testing.T) {
	// {:{x:number}} → typed map with map child type
	got, err := Parse("{:{x:number}}")
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
	// {a:true,b:false} → map with boolean values (data context)
	got, err := Parse(`{a:true,b:false}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 value, got %d", len(got))
	}
	m := got[0].AsMap()
	aVal, _ := m.Get("a")
	if !aVal.VType.Matches(engine.TBoolean) || !aVal.AsBoolean() {
		t.Errorf("expected true, got %s", aVal)
	}
	bVal, _ := m.Get("b")
	if !bVal.VType.Matches(engine.TBoolean) || bVal.AsBoolean() {
		t.Errorf("expected false, got %s", bVal)
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
	// {x:number} → map with type literal in data context
	got, err := Parse(`{x:number}`)
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
	assertExpand(t, ".", []engine.Value{engine.NewWord("dot")})
}

func TestExpandBangDotStandalone(t *testing.T) {
	assertExpand(t, "!.", []engine.Value{engine.NewWord("dotr")})
}

func TestExpandDottedSimple(t *testing.T) {
	assertExpand(t, "foo.bar", []engine.Value{
		engine.NewWord("get"),
		engine.NewWord("foo"),
		engine.NewWord("bar"),
		engine.NewWordModified("dot", -1, true, false),
	})
}

func TestExpandDottedChain(t *testing.T) {
	assertExpand(t, "foo.a.b", []engine.Value{
		engine.NewWord("get"),
		engine.NewWord("foo"),
		engine.NewWord("a"),
		engine.NewWordModified("dot", -1, true, false),
		engine.NewWord("b"),
		engine.NewWordModified("dot", -1, true, false),
	})
}

func TestExpandDottedLeading(t *testing.T) {
	assertExpand(t, ".a.b", []engine.Value{
		engine.NewWord("a"),
		engine.NewWordModified("dot", -1, true, false),
		engine.NewWord("b"),
		engine.NewWordModified("dot", -1, true, false),
	})
}

func TestExpandDottedIntegerKey(t *testing.T) {
	assertExpand(t, "foo.0", []engine.Value{
		engine.NewWord("get"),
		engine.NewWord("foo"),
		engine.NewInteger(0),
		engine.NewWordModified("dot", -1, true, false),
	})
}

func TestExpandDottedTrailingDot(t *testing.T) {
	assertExpand(t, "foo.", []engine.Value{
		engine.NewWord("get"),
		engine.NewWord("foo"),
	})
}

func TestExpandDottedEmptyMiddle(t *testing.T) {
	// "foo..bar" → empty segment skipped
	assertExpand(t, "foo..bar", []engine.Value{
		engine.NewWord("get"),
		engine.NewWord("foo"),
		engine.NewWord("bar"),
		engine.NewWordModified("dot", -1, true, false),
	})
}

func TestExpandDottedLeadingSingle(t *testing.T) {
	// ".x" → leading dot, just x dot/p
	assertExpand(t, ".x", []engine.Value{
		engine.NewWord("x"),
		engine.NewWordModified("dot", -1, true, false),
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
	// b should be boolean true
	bVal, _ := m.Get("b")
	if !bVal.VType.Matches(engine.TBoolean) || !bVal.AsBoolean() {
		t.Errorf("expected true, got %s", bVal)
	}
	// c should be boolean false
	cVal, _ := m.Get("c")
	if !cVal.VType.Matches(engine.TBoolean) || cVal.AsBoolean() {
		t.Errorf("expected false, got %s", cVal)
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
	// {x:[:number]} → map key 'x' has typed list value
	got, err := Parse("{x:[:number]}")
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
	if !elems[0].VType.Matches(engine.TBoolean) || !elems[0].AsBoolean() {
		t.Errorf("expected true, got %s", elems[0])
	}
	if !elems[1].VType.Matches(engine.TBoolean) || elems[1].AsBoolean() {
		t.Errorf("expected false, got %s", elems[1])
	}
	// null in jsonic data context becomes Text("null") → resolveTextValue → string "null"
	if !elems[2].VType.Matches(engine.TString) {
		t.Errorf("expected string, got %s", elems[2])
	}
}
