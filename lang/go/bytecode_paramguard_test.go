package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestGradualArgParamGuard pins the fix for a real compile==interpret VIOLATION
// (found by the Stage-D-2/4 adversarial sweep): a gradual (Dynamic) value — a
// value of static type exactly Any, e.g. laundered through an `[Any]`-returning
// helper — matches a CONCRETE user-fn param OPTIMISTICALLY at check time (the
// gradual-Any allowance). The compiled OpCallUser used to enter the fn frame with
// NO param-type check, while the interpreter runtime-matches the concrete value:
// a List laundered into an `m:Map` param ran the body and returned `m size` = 3
// where the interpreter raises signature_error.
//
// The fix is a runtime param guard at CALL_USER entry (CompiledFn.Params, checked
// via v.Is — the compiled mirror of the RET return-check). So the call now
// COMPILES but raises the SAME signature_error at run time. Pinned with the clean
// `f (id …)` call form so the interpreter's error is the plain signature_error
// (not the forward-collection usage hint a bare `(r f)` produces).
func TestGradualArgParamGuard(t *testing.T) {
	laundered := []struct{ name, src string }{
		{"List laundered into m:Map", `def id fn [[x:Any] [Any] [x]] def f fn [[m:Map] [Integer] [m size]] f (id [10 20 30])`},
		{"Integer laundered into s:String", `def id fn [[x:Any] [Any] [x]] def f fn [[s:String] [Integer] [s size]] f (id 99)`},
		{"List laundered into m:Map via 0-arg [Any] fn", `def g fn [[] [Any] [[1 2 3]]] def f fn [[m:Map] [Integer] [m size]] f (g)`},
	}
	for _, c := range laundered {
		t.Run("guarded/"+c.name, func(t *testing.T) {
			a, _ := New()
			gotC, _, errC := a.RunCompiled(c.src)
			// The miscompile was: compiled RETURNED a value. Now it must ERROR.
			if errC == nil {
				t.Fatalf("compiled must raise (param guard), not return %v", gotC)
			}
			if !strings.Contains(fmt.Sprint(errC), "no matching signature") {
				t.Errorf("compiled error = %v, want a signature_error", errC)
			}
			// And the interpreter raises the SAME thing.
			b, _ := New()
			_, errI := b.Run(c.src)
			if errI == nil || !strings.Contains(fmt.Sprint(errI), "no matching signature") {
				t.Errorf("interpreter error = %v, want a signature_error", errI)
			}
		})
	}

	// NEGATIVE: a directly-typed arg matches its concrete param for real, so it
	// must STILL compile natively to the correct value (the guard is a pass).
	typed := []struct{ name, src, want string }{
		{"Map literal -> m:Map", `def f fn [[m:Map] [Integer] [m size]] (f {a:1 b:2})`, "[2]"},
		{"Integer -> n:Integer", `def f fn [[n:Integer] [Integer] [n add 1]] (f 41)`, "[42]"},
		// A gradual arg whose runtime value DOES match the param compiles AND runs
		// correctly (the guard passes) — the Test.run-spec harness shape in miniature.
		{"gradual arg that matches at runtime", `def id fn [[x:Any] [Any] [x]] def f fn [[n:Integer] [Integer] [n add 1]] f (id 41)`, "[42]"},
		// Options-typed params: the guard must use sigTypeMatches, NOT v.Is — a
		// concrete map satisfies an Options slot (the codebase recommends
		// `opts:Options`). A v.Is guard OVER-RAISED here; these pin that it does not.
		{"Options param + map", `def f fn [[o:Options] [Integer] [o size]] (f {a:1 b:2})`, "[2]"},
		{"optional Options param", `def f fn [[a:Integer o?:Options] [Integer] [a]] (f 7 {a:1 b:2})`, "[7]"},
		{"Options laundered through id", `def id fn [[x:Any] [Any] [x]] def f fn [[o:Options] [Integer] [o size]] f (id {a:1 b:2})`, "[2]"},
	}
	for _, c := range typed {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("must compile and run, error: %v", err)
			}
			b, _ := New()
			want, _ := b.Run(c.src)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("compiled %v != interpreter %v", got, want)
			}
			if fmt.Sprint(got) != c.want {
				t.Errorf("got %v, want %s", got, c.want)
			}
		})
	}
}
