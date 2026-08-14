package lang_test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestNur054InlineCtxRefusal pins the NUR054 refusal in both directions.
//
// The compiler INLINES some bodies into the caller's unit — the `case`
// desugar's clause fragments, auto-evaluated list arguments, interp-string
// holes — where the interpreter runs each in a fresh sub-engine with its own
// context layer. There is no call to bracket (vm.go's enterBodyUnit map,
// path 5), so an ambient-context write inside one cannot be lowered
// faithfully: it must REFUSE compilation and fall back to the interpreter
// ("slow, not wrong", design/COMPILABLE-SUBSET.md). The refusal keys on
// PROVENANCE — the operand `context` returned while inside the inline
// region, i.e. a handle to the region's own layer — so a handle bound
// OUTSIDE the region (an in-place layer write both engines scope
// identically) and a write inside a closure unit within the region (the VM
// brackets those bodies with its own context frame) both keep compiling.
//
// TestContextBoundaryDifferential / TestDocumentedContextBoundaries assert
// the resulting AGREEMENT and the canonical values; this test pins the
// mechanism, so agreement can never silently become "both engines leak".
func TestNur054InlineCtxRefusal(t *testing.T) {
	const refusalMark = "NUR054"

	refuse := []struct{ name, src string }{
		{"set in case clause body", "case 1 [ 1 [ context set y 1 5 ] 2 [ 6 ] ]\ncontext has y"},
		{"del in case clause body", "context set y 9\ncase 1 [ 1 [ context del y 5 ] 2 [ 6 ] ]\ncontext has y"},
		{"set in otherwise list argument", "false otherwise [ context set y 1 5 ]\ncontext has y"},
		{"set in def list auto-evaluation", "def b [ context set y 1 5 ]\ncontext has y"},
		{"set in fn list argument", "def run fn [[b:List] [Any] [ 0 ]]\nrun [ context set y 1 5 ]\ncontext has y"},
		{"set in each collection list", "each [ drop 0 ] [ context set y 1 1 ]\ncontext has y"},
		{"set in interp-string hole", "`x${context set y 1 5}`\ncontext has y"},
		{"set in nested list element", "false otherwise [ [ context set y 1 1 ] ]\ncontext has y"},
	}
	for _, c := range refuse {
		t.Run("refuses/"+c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			prog, reason, _, cerr := a.CompileCheck(c.src)
			if cerr != nil {
				t.Fatalf("CompileCheck: %v", cerr)
			}
			if prog != nil || !strings.Contains(reason, refusalMark) {
				t.Errorf("must refuse with the NUR054 reason; prog=%v reason=%q",
					prog != nil, reason)
			}
		})
	}

	compile := []struct{ name, src string }{
		// Bracketed closure-unit bodies: contained by enterBodyUnit.
		{"set in do body", "do [ context set y 1 5 ]\ncontext has y"},
		{"set in each body", "each [ drop context set y 1 5 ] [1]\ncontext has y"},
		// Non-boundaries in either engine: the write leaks identically.
		{"set in if branch", "if true [ context set y 1 5 ] [ 6 ]\ncontext has y"},
		{"set in for body", "for 1 [ context set y 1 5 ]\ncontext has y"},
		{"set in named fn body", "def f fn [[] [Any] [ context set y 1 5 ]]\nf\ncontext has y"},
		{"set in if code-body condition", "if [ context set y 1 true ] [ 5 ] [ 6 ]\ncontext has y"},
		// Provenance precision: the handle was read OUTSIDE the inline
		// region, so the write persists identically on both engines.
		{"outside-bound handle written in case arm",
			"def s (context)\ncase 1 [ 1 [ s set y 1 5 ] 2 [ 6 ] ]\ncontext has y"},
		// A case with no context write at all must keep its desugar.
		{"case without context write", "case 1 [ 1 [ 5 ] 2 [ 6 ] ]"},
	}
	for _, c := range compile {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			prog, reason, _, cerr := a.CompileCheck(c.src)
			if cerr != nil {
				t.Fatalf("CompileCheck: %v", cerr)
			}
			if prog == nil {
				t.Errorf("must keep compiling; refused with reason=%q", reason)
			}
		})
	}
}
