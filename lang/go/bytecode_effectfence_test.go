package lang

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// The C1 effect fence on RunCompiled's two fallback arms (eng effects.go,
// design/RUNTIME-INDEPENDENCE-COMPLETION-PLAN.0.md): a silent interpreter
// re-run is permitted only while no observable effect escaped since before
// the check pass — RestoreForCompile rolls back registry scopes but cannot
// un-print, so a re-run after an effect DUPLICATES it (the L-DUP class,
// design/VOXGIG-COMPILE-LEAVES.2.md — the whole trie smoke suite printed
// twice). The pure-value differential corpus never exercises
// emit-then-fall-back, so these are the pins.

// --- the runtime-bail arm ---------------------------------------------------

// A compiled run that PRINTS and then hits a runtime internal_error (the
// zz-inst shape-claim violation from bytecode_methodshape_test.go) must
// propagate the annotated internal_error with the output emitted exactly
// once — the pre-fence behaviour re-ran the whole source and printed twice.
func TestRuntimeBailAfterEffectPropagates(t *testing.T) {
	src := `print "once" ; def i (zz-inst) ; i.m 5 ; 42`
	a := zzShapedInstance(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(src)
	if codeOf(err) != "internal_error" {
		t.Fatalf("fenced bail: err=[%s] %v (got=%v compiled=%v); want the propagated internal_error", codeOf(err), err, got, compiled)
	}
	if !strings.Contains(err.Error(), "--no-compile") {
		t.Errorf("fenced bail: error should carry the --no-compile note, got: %v", err)
	}
	if out.String() != "once\n" {
		t.Errorf("fenced bail: output = %q, want exactly one %q (no duplicate from a re-run)", out.String(), "once\n")
	}
}

// The positive twin: the SAME runtime bail with NO effect before it still
// falls back silently to the interpreter with the correct result — the fence
// only blocks a re-run that would duplicate output.
func TestRuntimeBailBeforeEffectStillFallsBack(t *testing.T) {
	src := `def i (zz-inst) ; i.m 5 ; 42`
	a := zzShapedInstance(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("effect-free bail: err=%v; want the silent interpreter fallback", err)
	}
	if compiled {
		t.Error("effect-free bail: ran compiled; want the interpreter fallback")
	}
	if fmt.Sprint(got) != "[7 42]" {
		t.Errorf("effect-free bail: got %v, want [7 42] from the fallback", got)
	}
	if out.String() != "" {
		t.Errorf("effect-free bail: unexpected output %q", out.String())
	}
}

// --- the refusal arm --------------------------------------------------------

// zzCheckEmit registers `zz-emit`, a RunInCheckMode word that WRITES to the
// registry output when it executes — so the CHECK pass itself emits an
// observable effect, the way a module body printing at import time does.
func zzCheckEmit(a *AQL) {
	a.Register("zz-emit", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{},
		BarrierPos: -1,
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, reg *native.Registry) ([]native.Value, error) {
			fmt.Fprint(reg.Output, "E")
			return nil, nil
		}, native.RunInCheck()),
	})
}

// zzRefusingRow is a knownRefusals dispatch-recovery row (the local-add
// overload — its window exceeds the written-tuple bound until DispatchErrCtx
// 3a lands): it compiles to a nil Program with no check error, driving the
// refusal arm, and the interpreter raises its canonical signature_error at
// run time. (The word-splice row that used to sit here graduated 2026-07-14
// to a serialized terminal trap.)
const zzRefusingRow = `def f fn [[x:Boolean] [Boolean] [def add fn [[a:Boolean b:Boolean] [Boolean] [a or b]] add x false]]  (f true) add true false`

// A REFUSAL after the check pass emitted an effect must not re-run the
// source (the re-run would execute zz-emit a second time): it surfaces a
// blocked-fallback internal_error carrying the refusal reason, with the
// effect emitted exactly once.
func TestRefusalFallbackAfterCheckEffectPropagates(t *testing.T) {
	a := mustNew(t)
	zzCheckEmit(a)
	var out bytes.Buffer
	a.SetOutput(&out)

	got, compiled, err := a.RunCompiled(`zz-emit ; ` + zzRefusingRow)
	if codeOf(err) != "internal_error" {
		t.Fatalf("fenced refusal: err=[%s] %v (got=%v compiled=%v); want the blocked-fallback internal_error", codeOf(err), err, got, compiled)
	}
	if !strings.Contains(err.Error(), "--no-compile") {
		t.Errorf("fenced refusal: error should carry the --no-compile note, got: %v", err)
	}
	if out.String() != "E" {
		t.Errorf("fenced refusal: output = %q, want exactly one %q", out.String(), "E")
	}
}

// A STATIC check error after a check-pass effect surfaces AS ITSELF (the
// program is invalid in both engines; the interpreter re-run that would
// normally render the canonical error is blocked, but the check error is the
// truthful verdict) — never masked as a blocked-fallback internal_error.
func TestStaticErrorAfterCheckEffectSurfacesItself(t *testing.T) {
	a := mustNew(t)
	zzCheckEmit(a)
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(`zz-emit ; zz-no-such-word-xyz`)
	if err == nil || compiled {
		t.Fatalf("fenced static error: err=%v compiled=%v; want the check error surfaced", err, compiled)
	}
	if codeOf(err) == "internal_error" {
		t.Errorf("fenced static error: masked as internal_error, want the genuine static diagnostic: %v", err)
	}
	if out.String() != "E" {
		t.Errorf("fenced static error: output = %q, want exactly one %q", out.String(), "E")
	}
}

// A check-pass ERROR (CompileCheck's err return, not the diagnostics
// sentinel) after a check-pass effect also surfaces AS ITSELF: zz-emit-fail
// is a RunInCheckMode word that writes and then errors, so the check pass
// both emits and fails — the fence must return the genuine error, not a
// blocked-fallback wrapper, and must not re-run the source.
func TestCheckErrorAfterCheckEffectSurfacesItself(t *testing.T) {
	a := mustNew(t)
	a.Register("zz-emit-fail", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{},
		BarrierPos: -1,
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, reg *native.Registry) ([]native.Value, error) {
			fmt.Fprint(reg.Output, "E")
			return nil, fmt.Errorf("zz-emit-fail: deliberate check-pass failure")
		}, native.RunInCheck()),
	})
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(`zz-emit-fail`)
	if err == nil || compiled {
		t.Fatalf("fenced check error: err=%v compiled=%v; want the check error surfaced", err, compiled)
	}
	if !strings.Contains(err.Error(), "deliberate check-pass failure") {
		t.Errorf("fenced check error: want the genuine check error, got: %v", err)
	}
	if out.String() != "E" {
		t.Errorf("fenced check error: output = %q, want exactly one %q", out.String(), "E")
	}
}

// --- the foreign-error (non-AqlError) bail class ----------------------------

// zzForeignBoom registers `zz-boom`, a plain native whose handler returns a
// foreign Go error — the non-AqlError class runtimeShouldFallback also
// resolves by re-running the interpreter.
func zzForeignBoom(t *testing.T) *AQL {
	t.Helper()
	a := mustNew(t)
	a.Register("zz-boom", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{},
		BarrierPos: -1,
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			return nil, fmt.Errorf("zz-boom: foreign failure")
		}),
	})
	return a
}

// A compiled run that prints and then fails with a FOREIGN error must also be
// fenced: fenceBlockedFallback wraps the foreign text in an internal_error
// carrying the --no-compile note, with the output emitted exactly once.
func TestForeignErrorBailAfterEffectPropagates(t *testing.T) {
	a := zzForeignBoom(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(`print "once" ; zz-boom`)
	if codeOf(err) != "internal_error" {
		t.Fatalf("fenced foreign bail: err=[%s] %v compiled=%v; want the wrapped internal_error", codeOf(err), err, compiled)
	}
	if !strings.Contains(err.Error(), "zz-boom: foreign failure") || !strings.Contains(err.Error(), "--no-compile") {
		t.Errorf("fenced foreign bail: error should carry the foreign text and the note, got: %v", err)
	}
	if out.String() != "once\n" {
		t.Errorf("fenced foreign bail: output = %q, want exactly one %q", out.String(), "once\n")
	}
}

// The positive twin: the same foreign failure with no prior effect still
// falls back silently, and the interpreter re-run surfaces the same foreign
// error as its own verdict.
func TestForeignErrorBailWithoutEffectFallsBack(t *testing.T) {
	a := zzForeignBoom(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(`zz-boom`)
	if compiled {
		t.Error("effect-free foreign bail: ran compiled; want the interpreter fallback")
	}
	if err == nil || !strings.Contains(err.Error(), "zz-boom: foreign failure") {
		t.Errorf("effect-free foreign bail: err=%v; want the interpreter's own foreign error", err)
	}
	if out.String() != "" {
		t.Errorf("effect-free foreign bail: unexpected output %q", out.String())
	}
}

// The positive twin for the refusal arm: a refusal with NO check-pass effect
// keeps the silent interpreter fallback (this row is an ERROR row — the
// interpreter raises its canonical runtime error, which is the contract the
// fullcorpus gate pins).
func TestRefusalWithoutEffectStillFallsBack(t *testing.T) {
	a := mustNew(t)
	var out bytes.Buffer
	a.SetOutput(&out)

	_, compiled, err := a.RunCompiled(zzRefusingRow)
	if compiled {
		t.Error("effect-free refusal: ran compiled; want the interpreter fallback")
	}
	if err == nil || codeOf(err) == "internal_error" {
		t.Errorf("effect-free refusal: err=[%s] %v; want the interpreter's canonical runtime error", codeOf(err), err)
	}
	if out.String() != "" {
		t.Errorf("effect-free refusal: unexpected output %q", out.String())
	}
}

// --- check-pass effect-freedom (obligation O1) ------------------------------

// The check pass over ordinary programs — value words, defs and calls, a
// module import, and a print in RUN position (check mode analyses it, never
// executes it) — must emit NO observable effect: this is what keeps the
// refusal arm's silent fallback sound for every corpus row, and what makes
// the C2 error-oracle run single-emission.
func TestCheckPassIsEffectFree(t *testing.T) {
	for _, src := range []string{
		`1 add 2`,
		`print "hello"`,
		`def f fn [[x:Integer] [Integer] [x mul 2]] f 21`,
		`import "aql:math-util" ; MathUtil.sqrt 16.0`,
		`def xs [1 2 3] xs each [dup mul]`,
	} {
		a := mustNew(t)
		var out bytes.Buffer
		a.SetOutput(&out)
		disarm := a.registry.ArmEffectFence()
		before := a.registry.Effects.Count()
		_, _, _, err := a.CompileCheck(src)
		after := a.registry.Effects.Count()
		disarm()
		if err != nil {
			t.Errorf("CompileCheck(%q): %v", src, err)
			continue
		}
		if after != before {
			t.Errorf("check pass over %q emitted %d observable effect(s) (output %q) — the check pass must be effect-free (O1)", src, after-before, out.String())
		}
	}
}
