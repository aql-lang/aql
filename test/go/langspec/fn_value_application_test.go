package langspec

import (
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// TestFnValueApplicationCompiles pins the fn-value application milestone
// (design/STAGE3-INLINING-DESIGN-ROUND.0.md Stage M2, aql-bytecode-plan.0.md
// §2.4b): the `OpCallDynamic`-family lowerings compile every fn-value
// application SHAPE the corpus exercises. Positive rows must produce a native
// Program (no interpreter island); the deliberate miscompile-E auto-dispatch
// guard must keep refusing the 0-arg shaped-method reads. This is the
// regression floor for the feature — a lowering that silently reverts to
// whole-program fallback, or a weakening of the guard, trips here.
//
// It complements the parity gates (TestSpecCompiledOrFallback,
// TestPropertyDifferential) which prove the RESULTS are byte-identical: this
// asserts the COMPILATION DECISION (native vs refuse) for the milestone shapes,
// so the coverage can't erode without a conscious change.
func TestFnValueApplicationCompiles(t *testing.T) {
	// Positive: fn-value application shapes that must compile NATIVELY.
	native := []struct {
		name string
		src  string
	}{
		// M2a — apply over a Function-typed param (`/r` ref + `apply`, and a
		// param-fn threaded through a helper). recursion.tsv:90.
		{"apply-over-function-param",
			`def g ([x:Integer] => [x add 1]) def h fn [[comp:Function v:Integer] [Integer] [v comp/r apply]] def t fn [[comp:Function] [Integer] [def a (5 comp/r h) def b (7 comp/r h) a add b]] (g/r t)`},
		// M2c — shaped instance-method dispatch (l.info) + typed list-of-record
		// element reads. module-log.tsv:53.
		{"instance-method-dispatch",
			`import "aql:log" ; def l (Log.with "http" {svc:"api"}) ; Log.add-sink memory/q ; Log.remove-sink console/q ; l.info "req" ; Log.dump 0 get "logger" get`},
		// M2d — fn-value-as-operand (two-lambda higher-order form).
		// corpus-core.tsv:134.
		{"fn-value-as-operand-walk",
			`walk {mode: "depth"} {a:{x:1}} (m:Any => [m.path print]) (m:Any => [m.path print])`},
	}
	for _, c := range native {
		t.Run(c.name, func(t *testing.T) {
			prog, reason, _, err := compileRow(t, c.src)
			if err != nil {
				t.Fatalf("check error: %v", err)
			}
			if prog == nil {
				t.Fatalf("fn-value application regressed to whole-program fallback (reason %q): %s", reason, c.src)
			}
			if strings.Contains(prog.Disassemble(), "FALLBACK") {
				t.Errorf("fn-value application compiled with an interpreter island; expected fully native:\n%s", prog.Disassemble())
			}
		})
	}

	// Negative: the miscompile-E auto-dispatch guard (emit.go) MUST keep
	// refusing a 0-arg shaped-method read — it auto-dispatches on landing and
	// baking the check-time closure would freeze seed-specific RNG state. These
	// are documented sound refusals (STAGE3-INLINING-DESIGN-ROUND.0.md M2c:
	// "do not weaken the guard to move 2 rows"). If one starts compiling, the
	// guard was weakened — confirm a real runtime auto-dispatch model landed
	// (not a const-fold) before updating this pin.
	guarded := []struct {
		name string
		src  string
	}{
		{"rand-zero-arg-method-bool", `import "aql:rand"  def r (Rand.with-seed 7)  r.bool`},
		{"rand-zero-arg-method-float", `import "aql:rand"  def r (Rand.with-seed 1)  r.float`},
	}
	for _, c := range guarded {
		t.Run(c.name, func(t *testing.T) {
			prog, _, _, err := compileRow(t, c.src)
			if err != nil {
				t.Fatalf("check error: %v", err)
			}
			if prog != nil {
				t.Errorf("a 0-arg shaped-method read compiled — the miscompile-E auto-dispatch guard was weakened; verify a sound runtime model landed before re-pinning: %s", c.src)
			}
		})
	}
}

// compileRow runs one source through CompileCheck against a fresh instance
// under the frozen spec clock (matching the census harness).
func compileRow(t *testing.T, src string) (*lang.Program, string, lang.CheckResult, error) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	a.SetClock(specClock)
	return a.CompileCheck(src)
}
