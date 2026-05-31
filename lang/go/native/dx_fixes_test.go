package native

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
)

// dxRun parses and runs source, returning the canonical render of the
// final stack (or fails the test on error).
func dxRun(t *testing.T, src string) string {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return Canon(out)
}

// dxErr parses and runs source, returning the run error (nil if none).
func dxErr(t *testing.T, src string) error {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	toks, err := parser.Parse(src)
	if err != nil {
		return err
	}
	_, runErr := NewTop(r).Run(toks)
	return runErr
}

// §4.1 — a concrete Map matches an `Options`-typed parameter.
func TestDXOptionsParamAcceptsMap(t *testing.T) {
	if got := dxRun(t, `def f fn [[opts:Options] [Any] [opts]]  f {a:1}`); got != "{a:1}" {
		t.Errorf("Options param with map arg = %q, want {a:1}", got)
	}
}

// §4.1 — an Options param still rejects a non-map.
func TestDXOptionsParamRejectsNonMap(t *testing.T) {
	err := dxErr(t, `def f fn [[opts:Options] [Any] [opts]]  f 42`)
	if err == nil || !strings.Contains(err.Error(), "no matching signature") {
		t.Fatalf("expected no-matching-signature for non-map arg, got %v", err)
	}
}

// §6.1 — forward over-collection error hints at parens / end / ;.
func TestDXForwardPrecedenceHint(t *testing.T) {
	err := dxErr(t, `def inc fn [[n:Integer] [Integer] [n add 1]]  inc inc 5`)
	if err == nil {
		t.Fatal("expected a signature error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "parens") || !strings.Contains(msg, "end") {
		t.Errorf("expected a forward-precedence hint mentioning parens/end, got:\n%s", msg)
	}
}

// §5.3 — `def name foo` (foo undefined) hints at (foo) / foo/q.
func TestDXDefBareWordHint(t *testing.T) {
	err := dxErr(t, `def name foo`)
	if err == nil || !strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("expected a def-body hint, got %v", err)
	}
}

// §5.3 — a plain undefined word does NOT get the def-specific hint.
func TestDXPlainUndefinedNoDefHint(t *testing.T) {
	err := dxErr(t, `foo`)
	if err == nil {
		t.Fatal("expected an undefined-word error")
	}
	if strings.Contains(err.Error(), "did you mean") {
		t.Errorf("plain undefined word should not get the def hint:\n%s", err.Error())
	}
}

// §4.3 — `aql check` on a file that imports a sibling must not hard-fail
// with `module "" not found`. In check mode the import path literal is
// stripped to a carrier; the import is treated as opaque and analysis
// continues instead of erroring.
func TestDXCheckSiblingImportDoesNotHardFail(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.Check.Mode = true
	defer func() { r.Check.Mode = false }()

	toks, err := parser.Parse(`"./lib.aql" import  def x 5  x add 3`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, runErr := NewTop(r).Run(toks)
	if runErr != nil && strings.Contains(runErr.Error(), "not found") {
		t.Fatalf("check mode hard-failed on a sibling import: %v", runErr)
	}
}

// §3.3 — a panicking handler surfaces as a clean internal_error, not a
// goroutine stack trace.
func TestDXTopLevelRecover(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: "dx-boom",
		Signatures: []NativeSig{{
			Args: []*Type{},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				var p *int
				_ = *p // nil deref → panic
				return nil, nil
			}, BarrierPos: 0,
		}},
	})
	_, runErr := NewTop(r).Run([]Value{NewWord("dx-boom")})
	if runErr == nil {
		t.Fatal("expected the panic to surface as an error")
	}
	ae, ok := runErr.(*AqlError)
	if !ok || ae.Code != "internal_error" {
		t.Fatalf("expected an internal_error AqlError, got %T: %v", runErr, runErr)
	}
}

// §8.3 — `export` is a no-op at the top level (running a file directly /
// in the REPL), so a single file can both run standalone and export a
// namespace when imported.
func TestDXExportTopLevelNoOp(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	r.Output = &buf
	toks, err := parser.Parse(`export "Lib" {greet: 1}  print "ran"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, runErr := NewTop(r).Run(toks)
	if runErr != nil {
		t.Fatalf("top-level export should be a no-op, got error: %v", runErr)
	}
	if got := strings.TrimSpace(buf.String()); got != "ran" {
		t.Errorf("top-level export no-op: output = %q, want %q", got, "ran")
	}
	// export consumes its (name, map) args and produces nothing.
	if len(out) != 0 {
		t.Errorf("top-level export should leave the stack empty, got %v", out)
	}
}

// §8.3 — the top-level no-op must NOT leak into an import context: a
// module body's `export` still collects exports (the per-module handler
// shadows the top-level no-op). Guards against the no-op's signatures
// being appended-and-preferred over the real collecting handler.
func TestDXExportStillCollectsInModule(t *testing.T) {
	got := dxRun(t, `import module [export "Lib" {answer: 42}] end  Lib.answer`)
	if got != "42" {
		t.Errorf("module export collection = %q, want 42 (top-level no-op must not shadow it)", got)
	}
}
