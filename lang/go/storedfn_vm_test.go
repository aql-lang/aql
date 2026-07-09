package lang

import (
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// A CompileStoresFn word stashes a handler to invoke LATER (like Net.serve-raw's
// connection handler). The compiler compiles a capture-free handler body to its
// own unit and stamps a durable CompiledFnRef on the baked const, so the word
// can run the handler on the VM via RunUnit / InvokeCallback instead of the
// interpreter. These tests drive that edge deterministically with a synthetic
// store-fn word (no network), and pin the fallback for a non-compiling body.

// registerStash adds a CompileStoresFn word `stash {opts} handler` that returns
// its opts map and treats the handler as inert data (the store-fn contract).
func registerStash(t *testing.T) *AQL {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a.Register("stash", Signature{
		Args: []*Type{TMap, TAny},
		Impl: eng.Go(func(args []Value, _ map[string]Value, _ []Value, _ *eng.Registry) ([]Value, error) {
			return []Value{args[0]}, nil
		}),
		Returns:       []*Type{TMap},
		BarrierPos:    -1,
		CompileEffect: eng.CompileStoresFn,
	})
	return a
}

// firstStampedHandler returns the first FnDefInfo const in prog whose own sig
// carries a compiled-unit ref, or nil.
func firstStampedHandler(prog *eng.Program) *eng.Signature {
	for _, c := range prog.Consts {
		fd, ok := c.Data.(eng.FnDefInfo)
		if !ok {
			continue
		}
		for i := range fd.Signatures {
			if fd.Signatures[i].CompiledRef() != nil {
				return &fd.Signatures[i]
			}
		}
	}
	return nil
}

// A capture-free store-fn handler with a compilable body is compiled to a unit,
// stamped with a CompiledFnRef, and Finalize back-stamps the *Program; RunUnit
// then executes it standalone.
func TestStoredFnCompilesAndRunsOnVM(t *testing.T) {
	a := registerStash(t)
	prog, reason, _, err := a.CompileCheck(`stash {a: 1} (fn [[x:Integer] [Integer] [x add 1]])`)
	if err != nil || prog == nil {
		t.Fatalf("compile failed: reason=%q err=%v", reason, err)
	}
	sig := firstStampedHandler(prog)
	if sig == nil {
		t.Fatal("handler was not compiled + stamped with a CompiledFnRef")
	}
	ref := sig.CompiledRef()
	if ref.Prog != prog {
		t.Fatalf("Finalize did not back-stamp ref.Prog (got %p, want %p)", ref.Prog, prog)
	}
	// Run the compiled unit standalone: x=5 → 6.
	out, err := eng.RunUnit(ref, a.NativeRegistry(), []Value{NewInteger(5)})
	if err != nil {
		t.Fatalf("RunUnit: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := out[0].AsConcreteInteger(); n != 6 {
		t.Fatalf("unit result = %v, want 6", out[0])
	}
}

// InvokeCallback runs a stamped handler on the VM when the registry is idle, and
// its result matches CallAQL (the interpreter) on the same body — the fail-safe
// equivalence. A busy registry (or a nil ref) falls back to the interpreter.
func TestInvokeCallbackVMAndFallbackAgree(t *testing.T) {
	a := registerStash(t)
	prog, _, _, err := a.CompileCheck(`stash {a: 1} (fn [[x:Integer] [Integer] [x add 1]])`)
	if err != nil || prog == nil {
		t.Fatalf("compile: %v", err)
	}
	sig := firstStampedHandler(prog)
	if sig == nil {
		t.Fatal("no stamped handler")
	}
	reg := a.NativeRegistry()

	// Idle registry → VM path (RunUnit).
	vmOut, err := eng.InvokeCallback(reg, sig, []Value{NewInteger(5)}, nil)
	if err != nil {
		t.Fatalf("InvokeCallback (VM): %v", err)
	}
	if n, _ := vmOut[0].AsConcreteInteger(); n != 6 {
		t.Fatalf("VM path result = %v, want 6", vmOut[0])
	}

	// Same sig, interpreter fallback (CallAQL directly): byte-identical.
	interpOut, err := reg.CallAQL(sig, []Value{NewInteger(5)}, nil)
	if err != nil {
		t.Fatalf("CallAQL: %v", err)
	}
	if n, _ := interpOut[0].AsConcreteInteger(); n != 6 {
		t.Fatalf("interpreter result = %v, want 6", interpOut[0])
	}
}

// firstHandlerSig returns the first FnDefInfo const's first own sig, stamped or
// not — used to reach an UN-stamped handler for the fallback path.
func firstHandlerSig(prog *eng.Program) *eng.Signature {
	for _, c := range prog.Consts {
		fd, ok := c.Data.(eng.FnDefInfo)
		if !ok {
			continue
		}
		for i := range fd.Signatures {
			if !fd.Signatures[i].Fallback {
				return &fd.Signatures[i]
			}
		}
	}
	return nil
}

// A handler whose body does NOT compile (uses the context-dependent `args` word)
// is left un-stamped: no ref, so InvokeCallback falls back to the interpreter,
// which runs the body and returns the same result.
func TestStoredFnNonCompilingBodyFallsBack(t *testing.T) {
	a := registerStash(t)
	prog, reason, _, err := a.CompileCheck(`stash {a: 1} (fn [[x:Integer] [Integer] [args drop x]])`)
	if err != nil || prog == nil {
		t.Fatalf("compile: reason=%q err=%v", reason, err)
	}
	if sig := firstStampedHandler(prog); sig != nil {
		t.Fatal("a non-compiling handler body must NOT be stamped")
	}
	// The un-stamped sig routes through InvokeCallback's interpreter fallback.
	sig := firstHandlerSig(prog)
	if sig == nil {
		t.Fatal("no handler sig found")
	}
	out, err := eng.InvokeCallback(a.NativeRegistry(), sig, []Value{NewInteger(5)}, nil)
	if err != nil {
		t.Fatalf("InvokeCallback fallback: %v", err)
	}
	if n, _ := out[0].AsConcreteInteger(); n != 5 {
		t.Fatalf("fallback result = %v, want 5", out[0])
	}
}

// A multi-overload handler is not a single stored unit — precheck declines, so
// it is left un-stamped and falls back to the interpreter.
func TestStoredFnMultiOverloadNotStamped(t *testing.T) {
	a := registerStash(t)
	prog, reason, _, err := a.CompileCheck(
		`stash {a: 1} (fn [[x:Integer] [Integer] [x add 1] [s:String] [String] [s]])`)
	if err != nil || prog == nil {
		t.Fatalf("compile: reason=%q err=%v", reason, err)
	}
	if sig := firstStampedHandler(prog); sig != nil {
		t.Fatal("a multi-overload handler must NOT be stamped as a single unit")
	}
}
