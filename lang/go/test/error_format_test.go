package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// runWithSource parses and runs AQL source, returning the error.
// Sets source on the engine for error reporting.
func runWithSource(t *testing.T, src string) error {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		return err
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(reg)
	reg.Source = src
	eng := native.NewTop(reg)
	eng.SetSource(src)
	_, err = eng.Run(values)
	return err
}

// assertErrorContains checks that err is non-nil and its message contains all substrings.
func assertErrorContains(t *testing.T, err error, substrings ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	for _, sub := range substrings {
		if !strings.Contains(msg, sub) {
			t.Errorf("error message missing %q\n  got: %s", sub, msg)
		}
	}
}

// assertAqlError checks that the error is an *AqlError with the given code.
func assertAqlError(t *testing.T, err error, code string) *native.AqlError {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ae, ok := err.(*native.AqlError)
	if !ok {
		t.Fatalf("expected *AqlError, got %T: %v", err, err)
	}
	if ae.Code != code {
		t.Errorf("expected code %q, got %q", code, ae.Code)
	}
	return ae
}

// =====================================================================
// Signature errors
// =====================================================================

func TestErrorFormatSignatureWrongType(t *testing.T) {
	// 99 upper → signature error (integer doesn't match string)
	err := runWithSource(t, `99 upper`)
	ae := assertAqlError(t, err, "signature_error")

	assertErrorContains(t, err,
		"[aql/signature_error]",
		"cannot call `upper`",
		"no signature matches the arguments",
		"-->",
	)

	// Should have source location
	if ae.Row == 0 {
		t.Error("expected non-zero Row in error")
	}

	// Structured notes: the received argument, the candidate verdict
	// (expected-vs-found), and the stack snapshot.
	if len(ae.Notes) == 0 {
		t.Error("expected structured Notes")
	}
	assertErrorContains(t, err,
		"the argument was 99 (an Integer)",
		"candidate `upper (String)` — argument 1: expected String, got 99 (an Integer)",
	)
	// The dispatch diagnostic carries NO tape-internal "stack:" note —
	// it has no compiled-mode equivalent, so it is deliberately absent
	// (design/DIAGNOSTICS.0.md phase 7 cross-engine parity).
	for _, n := range ae.Notes {
		if strings.HasPrefix(n, "stack:") {
			t.Errorf("dispatch error must not carry a tape-internal stack note, got %q", n)
		}
	}
}

func TestErrorFormatSignatureMissingArg(t *testing.T) {
	// add with no arguments
	err := runWithSource(t, `add`)
	ae := assertAqlError(t, err, "signature_error")
	assertErrorContains(t, err,
		"[aql/signature_error]",
		"cannot call `add`",
		"candidate `add ",
		"were supplied",
	)
	if len(ae.Notes) == 0 {
		t.Error("expected candidate-verdict Notes")
	}
	// add carries many overloads — the error must point at describe
	// rather than listing them all.
	assertErrorContains(t, err, "more signatures", "aql describe add")
}

func TestErrorFormatSignatureMultiLine(t *testing.T) {
	// Multi-line source — error should show source extract.
	src := "def x 1\ndef y 2\n99 upper"
	err := runWithSource(t, src)
	ae := assertAqlError(t, err, "signature_error")

	// Should point to line 3
	if ae.Row != 3 {
		t.Errorf("expected Row=3, got %d", ae.Row)
	}

	// Should include the source extract with surrounding lines
	msg := err.Error()
	if !strings.Contains(msg, "99 upper") {
		t.Error("error extract should contain the source line '99 upper'")
	}
	if !strings.Contains(msg, "def y 2") {
		t.Error("error extract should contain preceding line 'def y 2'")
	}
}

func TestErrorFormatSignatureFnDef(t *testing.T) {
	// Function signature mismatch at call time
	err := runWithSource(t, `def f fn [[n:Integer] Integer [n]] "hello" f`)
	ae := assertAqlError(t, err, "signature_error")
	assertErrorContains(t, err, "cannot call `f`")
	_ = ae
}

func TestErrorFormatSignaturePointsAtCallSiteNotLastMatch(t *testing.T) {
	// The failing `upper` is on line 1; a *valid* later `upper` on line 3
	// is the LAST textual match. The parser now stamps Value.Pos, so the
	// error must point at the executed (line-1) word — not the last
	// textual occurrence (line 3), which is what findWordInSource returns.
	src := "99 upper\ndef ok 1\nupper \"fine\""
	err := runWithSource(t, src)
	ae := assertAqlError(t, err, "signature_error")
	if ae.Row != 1 {
		t.Errorf("expected Row=1 (the executed `upper`), got %d — "+
			"a wrong row means the text-search fallback fired instead of the parser position", ae.Row)
	}
}

func TestErrorFormatSignatureSameWordTwoBodies(t *testing.T) {
	// `upper` (String-only) appears in two fn bodies. f's body (line 1)
	// applies it to an Integer and fails with a signature error; g's body
	// on line 2 holds the LAST textual `upper`. The error must point at
	// line 1, where the failing call actually executes — not line 2, which
	// is what the findWordInSource text-search fallback would return.
	src := "def f fn [[a:Integer] Integer [a upper]]\n" +
		"def g fn [[b:Integer] Integer [b upper]]\n" +
		"f 5"
	err := runWithSource(t, src)
	ae := assertAqlError(t, err, "signature_error")
	assertErrorContains(t, err, "cannot call `upper`")
	if ae.Row != 1 {
		t.Errorf("expected Row=1 (the failing `upper` in f's body), got %d", ae.Row)
	}
}

func TestErrorFormatSignatureReceivedArg(t *testing.T) {
	// The received-arguments note names the value the failed dispatch
	// actually saw, in plain language — the plain-English successor to
	// the old tape-internal "stack:" dump (which had no compiled-mode
	// equivalent and was removed for cross-engine parity, phase 7).
	// `upper` is 1-arg, so it dispatches on 42; the unrelated 'hello'
	// leftover below it is not the argument and is not reported.
	err := runWithSource(t, `"hello" 42 upper`)
	ae := assertAqlError(t, err, "signature_error")
	argNote := ""
	for _, n := range ae.Notes {
		if strings.HasPrefix(n, "the argument") {
			argNote = n
		}
	}
	// The note names the values in the failing window, top-first — the
	// same context the old stack note gave, in plain language, and
	// reproduced identically by the compiled path.
	if argNote != "the arguments were 42 (an Integer) and 'hello' (a ProperString)" {
		t.Errorf("received-argument note = %q, want the plain-language arg description", argNote)
	}
	// And no tape-internal stack note survives.
	for _, n := range ae.Notes {
		if strings.HasPrefix(n, "stack:") {
			t.Errorf("dispatch error must not carry a stack note, got %q", n)
		}
	}
}

// =====================================================================
// Undefined words + did-you-mean
// =====================================================================

func TestErrorFormatUndefinedWordDidYouMean(t *testing.T) {
	err := runWithSource(t, "def counter 1\ncountr")
	ae := assertAqlError(t, err, "undefined_word")
	assertErrorContains(t, err,
		"undefined word: countr",
		"did you mean `counter`?",
	)
	if len(ae.Suggestions) == 0 {
		t.Error("expected structured Suggestions")
	}
}

func TestErrorFormatUndefinedWordBuiltinDescribe(t *testing.T) {
	// A typo'd BUILTIN also points at its describe entry.
	err := runWithSource(t, `filtr [1 2 3] [true]`)
	assertAqlError(t, err, "undefined_word")
	assertErrorContains(t, err, "did you mean", "aql describe ")
}

func TestErrorFormatUndefinedWordNoWildGuess(t *testing.T) {
	// Nothing plausible nearby → no did-you-mean at all (never guess).
	err := runWithSource(t, `zzqzzqzz`)
	assertAqlError(t, err, "undefined_word")
	if strings.Contains(err.Error(), "did you mean") {
		t.Errorf("far name must not produce a guess:\n%s", err.Error())
	}
}

func TestErrorFormatUndefinedTypeNameDidYouMean(t *testing.T) {
	// A misspelt TYPE name in a binding suggests the type.
	err := runWithSource(t, `def x:Intger 5`)
	assertAqlError(t, err, "def_error")
	assertErrorContains(t, err,
		"type annotation must be a type value",
		"did you mean `Integer`",
	)
}

// =====================================================================
// Strict-read key misses + did-you-mean
// =====================================================================

func TestErrorFormatKeyMissDidYouMean(t *testing.T) {
	err := runWithSource(t, `{color:1 size:2} "colour" getr`)
	ae := assertAqlError(t, err, "not_found")
	assertErrorContains(t, err,
		`key "colour" not found in map`,
		"did you mean `color`?",
	)
	_ = ae
}

func TestErrorFormatKeyMissNoWildGuess(t *testing.T) {
	err := runWithSource(t, `{color:1 size:2} "zzz" getr`)
	assertAqlError(t, err, "not_found")
	if strings.Contains(err.Error(), "did you mean") {
		t.Errorf("far key must not produce a guess:\n%s", err.Error())
	}
}

func TestErrorFormatModuleExportMissDidYouMean(t *testing.T) {
	// Native module imports need the full lang.New wiring (module
	// resolver), not the bare DefaultRegistry fixture.
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Run("import \"aql:math-util\"\nMathUtil \"sqrtt\" getr")
	if err == nil {
		t.Fatal("expected an export-miss error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `export "sqrtt" not found in module`) ||
		!strings.Contains(msg, "did you mean `sqrt`?") {
		t.Errorf("missing export did-you-mean:\n%s", msg)
	}
}

// =====================================================================
// Return type errors
// =====================================================================

func TestErrorFormatReturnType(t *testing.T) {
	// Function returns wrong type
	err := runWithSource(t, `def f fn [[n:Integer] String [n]] 42 f`)
	ae := assertAqlError(t, err, "type_error")
	assertErrorContains(t, err,
		"[aql/type_error]",
		"return value 1",
		"expected",
		"got",
	)
	_ = ae
}

func TestErrorFormatReturnTypeTwoSpans(t *testing.T) {
	// The rich report labels BOTH the declaration and the produced
	// value alongside the primary call-site caret (phase 5).
	err := runWithSource(t, "def f fn [[n:Integer] String [n]]\n42 f")
	ae := assertAqlError(t, err, "type_error")
	if len(ae.Spans) == 0 {
		t.Fatalf("expected secondary spans, got none:\n%s", err.Error())
	}
	assertErrorContains(t, err,
		"the declaration says `f` returns String",
		"the returned value was produced here",
	)
}

func TestErrorFormatReturnCountDeclSpan(t *testing.T) {
	err := runWithSource(t, "def f fn [[n:Integer] [Integer Integer] [n]]\n42 f")
	assertAqlError(t, err, "type_error")
	assertErrorContains(t, err, "the declaration of `f` expects 2 return value(s)")
}

// =====================================================================
// Arithmetic error taxonomy
// =====================================================================

func TestErrorFormatArithCode(t *testing.T) {
	err := runWithSource(t, `20 div 0`)
	assertAqlError(t, err, "arith_error")
	assertErrorContains(t, err, "[aql/arith_error]", "division by zero")

	err = runWithSource(t, `20 mod 0`)
	assertAqlError(t, err, "arith_error")
	assertErrorContains(t, err, "modulo by zero")
}

func TestErrorFormatReturnCount(t *testing.T) {
	// Function returns wrong number of values
	err := runWithSource(t, `def f fn [[n:Integer] [Integer Integer] [n]] 42 f`)
	ae := assertAqlError(t, err, "type_error")
	assertErrorContains(t, err,
		"[aql/type_error]",
		"expected 2 return value(s)",
		"got 1",
	)
	_ = ae
}

func TestErrorFormatReturnTypeAnonymousFn(t *testing.T) {
	// An anonymous fn value (no name) returns the wrong type. Its
	// FuncName is the "<fn>" placeholder, which findWordInSource can never
	// match — so without a real position the error had no arrow at all.
	// The fn value now carries its construction position, so the error
	// points at line 3 where the `fn [...]` is written.
	src := "def x 1\n" +
		"def y 2\n" +
		"42 (fn [[Integer] String [dup]])"
	err := runWithSource(t, src)
	ae := assertAqlError(t, err, "type_error")
	assertErrorContains(t, err, "return value 1")
	if ae.Row != 3 {
		t.Errorf("expected Row=3 (the `fn [...]` construction), got %d", ae.Row)
	}
}

func TestErrorFormatReturnTypePointsAtCallSite(t *testing.T) {
	// f's return mismatch fires at the call on line 2. A trailing `def ff`
	// on line 3 contains the substring "f", which findWordInSource would
	// wrongly match as the last occurrence. The call-site Pos now flows
	// through the ReturnCheck marker, so the error points at line 2.
	src := "def f fn [[n:Integer] String [n]]\n" +
		"42 f\n" +
		"def ff 1"
	err := runWithSource(t, src)
	ae := assertAqlError(t, err, "type_error")
	assertErrorContains(t, err, "return value 1")
	if ae.Row != 2 {
		t.Errorf("expected Row=2 (the call `42 f`), got %d — "+
			"a wrong row means the text-search fallback matched a later substring", ae.Row)
	}
}

func TestErrorFormatUnknownPositionSaysSo(t *testing.T) {
	// An unmatched opening paren is a parser-side syntax error with no
	// source token to attribute it to. Rather than guess a location by
	// text-searching, the error explicitly states the position is unknown.
	err := runWithSource(t, "x add (1 2")
	ae := assertAqlError(t, err, "syntax_error")
	if ae.Row != 0 {
		t.Errorf("expected no position (Row 0), got %d", ae.Row)
	}
	assertErrorContains(t, err, "--> source position unknown")
}

// =====================================================================
// Syntax errors
// =====================================================================

func TestErrorFormatSyntaxUnmatchedClose(t *testing.T) {
	err := runWithSource(t, `)`)
	ae := assertAqlError(t, err, "syntax_error")
	assertErrorContains(t, err,
		"[aql/syntax_error]",
		"unmatched closing parenthesis",
	)
	_ = ae
}

func TestErrorFormatSyntaxUnmatchedOpen(t *testing.T) {
	// Unmatched opening paren — comes from parser
	err := runWithSource(t, `(1`)
	assertErrorContains(t, err,
		"syntax_error",
		"unmatched opening parenthesis",
	)
}

func TestErrorFormatSyntaxUnmatchedOpenNested(t *testing.T) {
	err := runWithSource(t, `((1)`)
	assertErrorContains(t, err,
		"syntax_error",
		"unmatched opening parenthesis",
	)
}

// =====================================================================
// Error format structure
// =====================================================================

func TestErrorFormatStructure(t *testing.T) {
	// Verify the overall structure matches jsonic format
	err := runWithSource(t, `99 upper`)
	msg := err.Error()

	lines := strings.Split(msg, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multi-line error, got: %s", msg)
	}

	// Line 1: [aql/<code>]: <detail>
	if !strings.HasPrefix(lines[0], "[aql/") {
		t.Errorf("line 1 should start with '[aql/', got: %s", lines[0])
	}
	if !strings.Contains(lines[0], "]:") {
		t.Errorf("line 1 should contain ']:', got: %s", lines[0])
	}

	// Line 2: --> <row>:<col>
	if !strings.Contains(lines[1], "-->") {
		t.Errorf("line 2 should contain '-->', got: %s", lines[1])
	}
}

func TestErrorFormatSourceExtract(t *testing.T) {
	// Multi-line source with error on line 3
	src := "def a 1\ndef b 2\n99 upper\ndef c 3"
	err := runWithSource(t, src)
	msg := err.Error()

	// Should show the error line
	if !strings.Contains(msg, "99 upper") {
		t.Errorf("should contain error line, got: %s", msg)
	}
	// Should show context lines
	if !strings.Contains(msg, "def b 2") {
		t.Errorf("should contain preceding context line, got: %s", msg)
	}
	if !strings.Contains(msg, "def c 3") {
		t.Errorf("should contain following context line, got: %s", msg)
	}
	// Should show carets
	if !strings.Contains(msg, "^") {
		t.Errorf("should contain caret markers, got: %s", msg)
	}
}

// =====================================================================
// Signature errors inside various contexts
// =====================================================================

func TestErrorFormatSigErrorInList(t *testing.T) {
	// Signature error inside an auto-evaluated list
	err := runWithSource(t, `[99 upper]`)
	assertErrorContains(t, err, "signature_error", "upper")
}

func TestErrorFormatSigErrorInMap(t *testing.T) {
	// Signature error inside a map value (autoEvalMap sub-engine)
	err := runWithSource(t, `{a:[99 upper]}`)
	assertErrorContains(t, err, "upper")
}

func TestErrorFormatSigErrorInFnBody(t *testing.T) {
	// Signature error inside a function body
	err := runWithSource(t, `def f fn [[] Integer [99 upper]] f`)
	assertErrorContains(t, err, "upper")
}
