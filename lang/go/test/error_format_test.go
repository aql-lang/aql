package test

import (
	"github.com/aql-lang/aql/lang/go/native"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
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
		"no matching signature for upper",
		"-->",
	)

	// Should have source location
	if ae.Row == 0 {
		t.Error("expected non-zero Row in error")
	}

	// Should include hint with expected signatures and stack
	if ae.Hint == "" {
		t.Error("expected non-empty Hint")
	}
	assertErrorContains(t, err, "expected:", "stack:")
}

func TestErrorFormatSignatureMissingArg(t *testing.T) {
	// add with no arguments
	err := runWithSource(t, `add`)
	ae := assertAqlError(t, err, "signature_error")
	assertErrorContains(t, err,
		"[aql/signature_error]",
		"no matching signature for add",
	)
	if ae.Hint == "" {
		t.Error("expected non-empty Hint")
	}
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
	assertErrorContains(t, err, "no matching signature for f")
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
	assertErrorContains(t, err, "no matching signature for upper")
	if ae.Row != 1 {
		t.Errorf("expected Row=1 (the failing `upper` in f's body), got %d", ae.Row)
	}
}

func TestErrorFormatSignatureStackContext(t *testing.T) {
	// The hint should describe what types are actually on the stack
	err := runWithSource(t, `"hello" 42 upper`)
	ae := assertAqlError(t, err, "signature_error")
	// Stack should show the types around the word
	if !strings.Contains(ae.Hint, "stack:") {
		t.Error("expected 'stack:' in hint")
	}
	// Should mention the actual types present
	if !strings.Contains(ae.Hint, "'hello'") || !strings.Contains(ae.Hint, "42") {
		t.Errorf("stack hint should describe the actual values, got: %s", ae.Hint)
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
