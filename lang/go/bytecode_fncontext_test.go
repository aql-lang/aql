package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestFnBodyFunctionContext pins the design decision: a NAMED fn always creates
// a function context — its params are in scope for the WHOLE body, INCLUDING a
// returned bare container literal — so the returned list/map closes over the
// param (not a module binding), and the body compiles natively (the residual is
// assembled in-frame, fixed at call time). An anonymous lambda (afn / =>) keeps
// the single-bare-literal "no closures" transparency. compile == interpret on
// both surfaces.
func TestFnBodyFunctionContext(t *testing.T) {
	// NAMED fn: the param wins, and the body compiles natively (no island).
	named := []struct{ src, want string }{
		{`def b 99 def g fn [[a:Integer b:Integer] [Map] [{x:(b)}]] g 1 2`, "x:2"},
		{`def f fn [[a:Integer] [Map] [{x:(a 1 add)}]] f 5`, "x:6"},
		{`def c1 1 def mk fn [[c1:Integer] [List] [[c1]]] mk 9`, "9"},
		// No module binding for the param: it still resolves in-frame (the pre-fix
		// transparency would have errored on the late module-scope eval).
		{`def mk fn [[x:Integer] [List] [[x add 1]]] mk 5`, "6"},
	}
	for _, c := range named {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Fatalf("named fn must compile natively: reason=%q err=%v :: %s", reason, cerr, c.src)
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("named fn function-context must lower native (no island): %s", c.src)
		}
		got, err := mustNew(t).RunCompiledStrict(c.src)
		if err != nil {
			t.Fatalf("strict compiled: %v :: %s", err, c.src)
		}
		gotI, _ := mustNew(t).RunInterp(c.src)
		if fmt.Sprint(got) != fmt.Sprint(gotI) {
			t.Errorf("compiled %v != interp %v (MISCOMPILE) :: %s", got, gotI, c.src)
		}
		if !strings.Contains(fmt.Sprint(got), c.want) {
			t.Errorf("got %v, want it to contain %q :: %s", got, c.want, c.src)
		}
	}

	// AFN keeps the transparency and the closure capture (unchanged).
	afn := []struct{ src, want string }{
		{`def c1 1 def mk ([c1:Integer] => [[c1]]) mk 9`, "1"},                     // single-literal: module binding
		{`def mk fn [[c1:Integer] [Function] [([] => [c1])]] def f (mk 7) f`, "7"}, // captures the enclosing param
	}
	for _, c := range afn {
		gotI, _ := mustNew(t).RunInterp(c.src)
		if !strings.Contains(fmt.Sprint(gotI), c.want) {
			t.Errorf("afn: got %v, want it to contain %q :: %s", gotI, c.want, c.src)
		}
	}
}
