package eng_test

// The STANDALONE kernel corpus run: every eng/spec row executed by
// eng's own test suite — no other module's tests involved — so the
// kernel's coverage stands on its own (the merged repo-wide cover gate
// remains on top). The registry is eng.NewRegistry + the specfix
// fixture words + a mechanism-only stand-in for the content layer's
// Micron Ideal: Pathon construction is kernel plumbing (MakePathon),
// so its rows run here; the six Emailon/Urlon validator rows skip via
// specfix.ErrSkipRow and stay covered by the full harness in
// test/go/engspec, which installs basic's real family Ideal.

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/eng/go/specfix"
)

// standaloneContentRow reports whether a corpus row needs the content
// layer's validators (the Emailon / Urlon constructions).
func standaloneContentRow(input string) bool {
	return strings.Contains(input, "make Emailon") || strings.Contains(input, "make Urlon")
}

// installStandaloneMicronIdeal registers the mechanism-only stand-in:
// the same Accepts shape the content layer's descriptor has, with
// Instantiate wired to the kernel's own Pathon constructor. A
// non-Pathon kind reaching it is a harness bug — the skip predicate
// keeps those rows out.
func installStandaloneMicronIdeal(r *eng.Registry) {
	r.Ideals.Register(&eng.Ideal{
		Name:    "Micron",
		Enabled: true,
		Accepts: func(v eng.Value) bool {
			return eng.IsMicronType(v) || (eng.IsBareTypeNode(v) && (&v).ConformsTo(eng.TMicron))
		},
		Instantiate: func(typ, data eng.Value, _ *eng.Registry) ([]eng.Value, error) {
			if (&typ).Equal(eng.TPathon) {
				return eng.MakePathon(data, false)
			}
			return nil, fmt.Errorf("standalone corpus: %s needs the content layer", typ.String())
		},
	})
}

func standaloneRegistry() (*eng.Registry, error) {
	r, err := eng.NewRegistry()
	if err != nil {
		return nil, err
	}
	specfix.RegisterSpecWords(r)
	installStandaloneMicronIdeal(r)
	r.InitRootContext()
	return r, nil
}

// TestSpecStandalone is the value-mode corpus, kernel-only.
func TestSpecStandalone(t *testing.T) {
	specfix.RunDir(t, filepath.Join("..", "spec"), func(input string) ([]eng.Value, error) {
		if standaloneContentRow(input) {
			return nil, specfix.ErrSkipRow
		}
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		r, err := standaloneRegistry()
		if err != nil {
			return nil, err
		}
		return eng.NewTop(r).Run(values)
	})
}

// TestCheckSpecStandalone is the check-mode corpus, kernel-only.
func TestCheckSpecStandalone(t *testing.T) {
	specfix.RunDirRendered(t, filepath.Join("..", "spec", "check"), func(input string) (string, error) {
		if standaloneContentRow(input) {
			return "", specfix.ErrSkipRow
		}
		values, err := parser.Parse(input)
		if err != nil {
			return "", err
		}
		r, err := standaloneRegistry()
		if err != nil {
			return "", err
		}
		specfix.RegisterCheckExtras(r)
		r.Source = input

		done := r.Check.Begin()
		out, runErr := eng.NewTop(r).Run(values)
		r.RescueForwardRefDiagnostics()
		r.Check.EmitUnusedDefDiagnostics()
		diags := r.Check.Diagnostics
		done()

		if runErr != nil {
			return "", runErr
		}
		return specfix.RenderCheck(out, diags), nil
	})
}

// TestSpecCompiledStandalone is the compile-or-fallback lane over the
// same corpus, kernel-only: each row is compiled through the recorder
// pipeline (the identical primitives lang's CompileCheck composes —
// BeginCompilePass, the check-mode run, the model-undermining
// diagnostics gate, Recorder().Finalize) and executed on the VM when a
// Program materialises; a refusal falls back to a fresh interpreter
// run. Either way the result must match the row's expected column, so
// the emit/lower/VM pipeline is exercised — and validated — by eng's
// own suite.
func TestSpecCompiledStandalone(t *testing.T) {
	specfix.RunDir(t, filepath.Join("..", "spec"), func(input string) ([]eng.Value, error) {
		if standaloneContentRow(input) {
			return nil, specfix.ErrSkipRow
		}
		values, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		rA, err := standaloneRegistry()
		if err != nil {
			return nil, err
		}
		rA.Source = input

		finish := rA.Check.BeginCompilePass()
		residual, runErr := eng.NewTop(rA).Run(values)
		rA.RescueForwardRefDiagnostics()
		rA.Check.EmitUnusedDefDiagnostics()
		var prog *eng.Program
		if runErr == nil && !rA.Check.SuppressedRuntimeError && !rA.Check.AmbiguousGradualSplit {
			refuse := false
			for _, d := range rA.Check.Diagnostics {
				if !d.RuntimeMirror && (d.Severity == eng.SeverityError || d.CaughtAtRuntime) {
					refuse = true
					break
				}
			}
			if !refuse {
				if p, _, ok := rA.Check.Recorder().(*eng.EmitState).Finalize(residual); ok {
					prog = p
				}
			}
		}
		finish()

		if prog != nil {
			out, vmErr := eng.RunProgram(prog, rA)
			var be *eng.BoruError
			if vmErr == nil || !errors.As(vmErr, &be) || be.Code != "internal_error" {
				return out, vmErr
			}
			// internal_error is the VM's soundness bailout ("defer to the
			// interpreter") — the same contract lang's compiled-by-default
			// entry honours with a silent re-run. Fall through to the
			// interpreter arm below.
		}
		// Refused, check-pass errored, or the VM bailed out: interpreter
		// fallback on a fresh registry, so check-mode side effects never
		// leak into it.
		rB, err := standaloneRegistry()
		if err != nil {
			return nil, err
		}
		reparsed, err := parser.Parse(input)
		if err != nil {
			return nil, err
		}
		return eng.NewTop(rB).Run(reparsed)
	})
}

// TestInterpNestedCarrierNoBake pins the recursive half of the
// interpolation bake probe: a hole whose MAP literal embeds a computed
// value auto-evaluates to a real map holding a check-mode carrier — the
// container's own Carrier flag is false, so only the recursive walk
// keeps the check-time render (a type tag) out of the const pool. The
// compiled result must be the runtime string.
func TestInterpNestedCarrierNoBake(t *testing.T) {
	const input = "`${{a: (addq 1 2)}}`"
	expected := "'{a:3}'"

	values, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rA, err := standaloneRegistry()
	if err != nil {
		t.Fatal(err)
	}
	rA.Source = input
	finish := rA.Check.BeginCompilePass()
	residual, runErr := eng.NewTop(rA).Run(values)
	var prog *eng.Program
	if runErr == nil {
		if p, _, ok := rA.Check.Recorder().(*eng.EmitState).Finalize(residual); ok {
			prog = p
		}
	}
	finish()

	var out []eng.Value
	if prog != nil {
		out, err = eng.RunProgram(prog, rA)
		if err != nil {
			t.Fatalf("vm: %v", err)
		}
	} else {
		rB, _ := standaloneRegistry()
		reparsed, _ := parser.Parse(input)
		out, err = eng.NewTop(rB).Run(reparsed)
		if err != nil {
			t.Fatalf("interp: %v", err)
		}
	}
	if got := eng.Canon(out); got != expected {
		t.Errorf("got %s, want %s (compiled=%v)", got, expected, prog != nil)
	}
}
