package lang

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// zzUpMini registers the contract-exercising `up` kind: an identity
// transducer with a Go compile hook that uppercases at expansion — the
// 2026-07-15 flip finding's fixture. The hook is authoritative at the call
// site, so a compile pass must either mirror it or refuse.
func zzUpMini(t *testing.T) *AQL {
	t.Helper()
	a := mustNew(t)
	err := a.RegisterMiniLang(MiniLangSpec{
		Name:    "up",
		Returns: []*Type{TString},
		Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			s, _ := args[0].AsConcreteString()
			return []native.Value{native.NewString(s)}, nil
		},
		Compile: func(src string, _ native.Value, _ *native.Registry) ([]native.Value, error) {
			return []native.Value{native.NewString(strings.ToUpper(src))}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// The Go compile hook runs IN the compile pass over a concrete src, so the
// compiled program splices the hook's expansion exactly as the interpreter
// does — the flip finding's positive twin (compiled [hi] vs interpreted [HI]
// before the fix).
func TestMiniGoHookCompilesIdentically(t *testing.T) {
	const src = `import "aql:minilang" end mini up 'hi'`
	gotC, ran, errC := zzUpMini(t).RunCompiled(src)
	if !ran || errC != nil {
		t.Fatalf("hooked mini must run compiled: ran=%v err=%v", ran, errC)
	}
	gotI, errI := zzUpMini(t).RunInterp(src)
	if errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Fatalf("hook parity: compiled=%v interp=%v (errI=%v)", gotC, gotI, errI)
	}
	if fmt.Sprint(gotC) != "[HI]" {
		t.Fatalf("hook result = %v, want [HI] (the compile hook ran)", gotC)
	}
}

// A hook whose expansion the compile pass cannot mirror REFUSES — never the
// transducer bake. Non-concrete opts (a fn param) is the exercised shape.
func TestMiniGoHookNonConcreteOptsRefuses(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// AQL_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("AQL_COMPILE_FALLBACK", "1")
	const src = `import "aql:minilang" end def f fn [[m:Map][String][mini up 'hi' m]] f {x:1}`
	a := zzUpMini(t)
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck: %v", cerr)
	}
	if prog != nil || !strings.Contains(reason, "concrete opts") {
		t.Fatalf("prog=%v reason=%q — non-concrete opts must refuse the hook bake", prog, reason)
	}
	// Parity via the (transitional-default) fallback.
	gotC, ran, errC := zzUpMini(t).RunCompiled(src)
	gotI, errI := zzUpMini(t).RunInterp(src)
	if ran || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Fatalf("fallback parity: C=%v/%v I=%v/%v ran=%v", gotC, errC, gotI, errI, ran)
	}
}

// An AQL compile hook (a macro) is interpreter-only: its check-mode
// expansion is not the runtime expansion, so a compile pass refuses.
func TestMiniAQLHookRefuses(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// AQL_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("AQL_COMPILE_FALLBACK", "1")
	// register-compiled is not a RunInCheckMode word, so an in-source AQL
	// hook cannot exist during its own compile pass; pre-register it on the
	// instance via the interpreter, then compile a program using it.
	const setup = `import "aql:minilang" end import "aql:string-util" end ` +
		`MiniLang.register lit (fn [[src:String opts:Map] [String] [src]]) end ` +
		`MiniLang.register-compiled lit (macro [[src opts] [ quote [ unquote (StringUtil.upper src) ] ]]) end `
	a := mustNew(t)
	if _, err := a.RunInterp(setup); err != nil {
		t.Fatalf("setup: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(`import "aql:minilang" end mini lit 'hello'`)
	if cerr != nil {
		t.Fatalf("CompileCheck: %v", cerr)
	}
	if prog != nil || !strings.Contains(reason, "AQL compile hook is interpreter-only") {
		t.Fatalf("prog=%v reason=%q — the AQL hook must refuse the compile pass", prog, reason)
	}
	gotC, ran, errC := a.RunCompiled(`import "aql:minilang" end mini lit 'hello'`)
	if ran || errC != nil || fmt.Sprint(gotC) != "[HELLO]" {
		t.Fatalf("fallback must yield the hook's interpreter result: got=%v ran=%v err=%v", gotC, ran, errC)
	}
}
