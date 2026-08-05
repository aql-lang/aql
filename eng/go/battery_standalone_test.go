package eng_test

// The eng-local control-flow battery (design/ENG-COVERAGE-PARITY.0.md
// stage-4 plan, step 2): crafted branch/loop programs over the specfix
// vocabulary plus the fixture `if`/`for` control words, each run twice —
// interpreted, and through the compile-or-fallback pipeline — with both
// results pinned against the expected render. The battery is the
// standalone twin of lang's bytecode clusters: it drives the kernel's
// branch/loop capture, the lowering's jump/loop emission, and the VM's
// JMP/FOR opcodes with eng's own suite. Rows stay eng-local (not in the
// shared eng/spec corpus) until the TS fixture twin of the control
// words exists.

import (
	"errors"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/eng/go/specfix"
)

func batteryRegistry(t *testing.T) *eng.Registry {
	t.Helper()
	r, err := standaloneRegistry()
	if err != nil {
		t.Fatal(err)
	}
	specfix.RegisterControlWords(r)
	return r
}

// runBatteryInterp runs one row on a fresh interpreter.
func runBatteryInterp(t *testing.T, input string) ([]eng.Value, error) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	return eng.NewTop(batteryRegistry(t)).Run(values)
}

// runBatteryCompiled runs one row through the recorder pipeline and
// the VM when a Program materialises, falling back to a fresh
// interpreter exactly like the corpus compile-or-fallback lane.
// It reports whether the row genuinely executed on the VM.
func runBatteryCompiled(t *testing.T, input string) ([]eng.Value, bool, error) {
	t.Helper()
	values, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	rA := batteryRegistry(t)
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
			if p, _, ok := rA.Check.Recorder().Finalize(residual); ok {
				prog = p
			}
		}
	}
	finish()

	if prog != nil {
		out, vmErr := eng.RunProgram(prog, rA)
		var be *eng.BoruError
		if vmErr == nil || !errors.As(vmErr, &be) || be.Code != "internal_error" {
			return out, true, vmErr
		}
	}
	rB := batteryRegistry(t)
	reparsed, err := parser.Parse(input)
	if err != nil {
		return nil, false, err
	}
	out, ferr := eng.NewTop(rB).Run(reparsed)
	return out, false, ferr
}

type batteryRow struct {
	input   string
	want    string // Canon of the result stack
	wantErr string // substring of the run error (both lanes)
}

func runBattery(t *testing.T, rows []batteryRow) {
	t.Helper()
	compiledCount := 0
	for _, row := range rows {
		t.Run(row.input, func(t *testing.T) {
			iOut, iErr := runBatteryInterp(t, row.input)
			cOut, onVM, cErr := runBatteryCompiled(t, row.input)
			if onVM {
				compiledCount++
			}
			for lane, res := range map[string]struct {
				out []eng.Value
				err error
			}{"interp": {iOut, iErr}, "compiled": {cOut, cErr}} {
				if row.wantErr != "" {
					if res.err == nil {
						t.Errorf("%s: want error containing %q, got %s", lane, row.wantErr, eng.Canon(res.out))
					}
					continue
				}
				if res.err != nil {
					t.Errorf("%s: unexpected error: %v", lane, res.err)
					continue
				}
				if got := eng.Canon(res.out); got != row.want {
					t.Errorf("%s: got %s, want %s", lane, got, row.want)
				}
			}
		})
	}
	t.Logf("battery: %d/%d rows executed on the VM", compiledCount, len(rows))
}

func TestControlBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Constant conditions: the literal-cond reduction arm.
		{input: "if true [1] [2]", want: "1"},
		{input: "if false [1] [2]", want: "2"},

		// List conditions: mark/move at run time, cond fragment when
		// emitting, JMP_IF_FALSE in the lowering.
		{input: "if [1 is Integer] [10] [20]", want: "10"},
		{input: "if [1 is String] [10] [20]", want: "20"},
		{input: "def x 5 if [x is Integer] [x addq 1] [0]", want: "6"},
		{input: "def x 'a' if [x is Integer] [1] [x concatq 'b']", want: "'ab'"},

		// Value arms (no body capture).
		{input: "if [1 is Integer] 7 8", want: "7"},
		{input: "if [1 is String] 7 8", want: "8"},

		// Branches over an enclosing binding.
		{input: "def n 3 if [true] [n addq 1] [n addq 2]", want: "4"},
		{input: "def n 3 if [false] [n addq 1] [n addq 2]", want: "5"},

		// Nested branches.
		{input: "if [true] [if [false] [1] [2]] [3]", want: "2"},

		// Loops: iterator spread, constant body, zero-trip prune.
		{input: "for 3 [i]", want: "0 1 2"},
		{input: "for 3 ['x']", want: "'x' 'x' 'x'"},
		{input: "for 0 [i]", want: ""},
		{input: "for 2 [i addq 10]", want: "10 11"},

		// Nested loops (the inner iterator shadows).
		{input: "for 2 [for 2 [i]]", want: "0 1 0 1"},

		// Branch inside a loop.
		{input: "for 3 [if [i is Integer] [i addq 1] [0]]", want: "1 2 3"},

		// Loop flow control.
		{input: "for 3 [break]", want: ""},
		{input: "for 3 [continue]", want: ""},

		// Errors: a non-list body, count-form taxonomy.
		{input: "for 2 5", wantErr: "no signature matches"},
	})
}

// TestFnBodyControlBattery drives branches and loops INSIDE fn bodies:
// the stored-body compile path, carrier-typed params flowing through
// the branch/loop analysis, and the fn-unit VM execution.
func TestFnBodyControlBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Branch on a Boolean param (carrier cond at check time).
		{input: "def f fn [[b:Boolean] [Any] [if b [99] [0]]] f true", want: "99"},
		{input: "def f fn [[b:Boolean] [Any] [if b [99] [0]]] f false", want: "0"},
		// Branch joining different arm types.
		{input: "def f fn [[b:Boolean] [Any] [if b [1] ['s']]] f false", want: "'s'"},
		// Branch on a list condition over the param.
		{input: "def f fn [[n:Integer] [Any] [if [n is Integer] [n addq 1] [0]]] f 4", want: "5"},
		// A spreading loop violates the fn's declared single return —
		// the return-conformance check fires on both engines.
		{input: "def f fn [[n:Integer] [Any] [for n [i]]] f 3",
			wantErr: "expected 1 return value(s), got 3"},
		// Side-effect loop (body nets 0) with a trailing value.
		{input: "def f fn [[n:Integer] [Any] [for n [5 drop] 0]] f 3", want: "0"},
		// Loop whose body reads the param, netting one value.
		{input: "def f fn [[n:Integer] [Any] [for 1 [n addq i]]] f 10", want: "10"},
		// Nested fn calls with control inside.
		{input: "def g fn [[x:Integer] [Integer] [x addq 1]] def f fn [[n:Integer] [Any] [if [n is Integer] [g n] [0]]] f 7", want: "8"},
		// A fn called from a loop body, netting one value.
		{input: "def g fn [[x:Integer] [Integer] [x mulq 2]] def f fn [[n:Integer] [Any] [for 1 [g n]]] f 3", want: "6"},
	})
}

// TestTruthinessAndBindBattery drives the condition-coercion table and
// computed def bindings (the dyn-bind record/lower path).
func TestTruthinessAndBindBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Truthiness coercions through the fixture if.
		{input: "if 1 [1] [2]", want: "1"},
		{input: "if 0 [1] [2]", want: "2"},
		{input: "if 'x' [1] [2]", want: "1"},
		{input: "if '' [1] [2]", want: "2"},
		// An empty-LIST condition is an empty cond region — an error;
		// an empty MAP is an ordinary falsy value.
		{input: "if [] [1] [2]", wantErr: "condition produced no value"},
		{input: "if {} [1] [2]", want: "2"},

		// Computed defs: the bind is dynamic at compile time.
		{input: "def x (5 addq 1) x", want: "6"},
		{input: "def x (5 addq 1) x addq 2", want: "8"},
		{input: "def x (5 addq 1) def y (x addq 1) y", want: "7"},
		// A computed def read inside a branch and a loop.
		{input: "def x (2 addq 3) if [x is Integer] [x] [0]", want: "5"},
		{input: "def x (1 addq 1) for x [i]", want: "0 1"},
		// Rebinding via undef.
		{input: "def x 1 undef x def x 2 x", want: "2"},
		// Quote / splice interactions with control.
		{input: "def s word 42 if [true] [s] [0]", want: "42"},
	})
}

// TestClosureBattery drives the Callable pipeline through doq — the
// battery's body-runner carrying basic do's CallableSpec — so closure
// dispatch, recording, lowering, and VM execution run standalone.
func TestClosureBattery(t *testing.T) {
	runBattery(t, []batteryRow{
		// Single- and multi-value residual closures.
		{input: "doq [1 addq 2]", want: "3"},
		{input: "doq [10 20 30]", want: "10 20 30"},
		{input: "doq []", want: ""},
		// An enclosing binding read inside the body.
		{input: "def n 4 doq [n addq 1]", want: "5"},
		// A def-leaking body (do keeps body defs bound).
		{input: "doq [def z 5 z] z", want: "5 5"},
		// Nested closures.
		{input: "doq [doq [1] addq 1]", want: "2"},
		// A word-bound body (the deref arm; the computed body declines
		// the closure and rides the fallback).
		{input: "def b [1 addq 2] doq b", want: "3"},
		// Inside a fn body.
		{input: "def f fn [[x:Integer] [Any] [doq [x addq 1]]] f 3", want: "4"},
		// Control flow inside the closure body.
		{input: "doq [if [true] [7] [8]]", want: "7"},
		{input: "doq [for 2 [i]]", want: "0 1"},
		// A raising body surfaces as an Error VALUE.
		{input: "doq [refine 5] typeof", want: "Error"},
	})
}
