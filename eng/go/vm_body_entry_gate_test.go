package eng

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"
)

// TestVMBodyEntryIsFunnelled pins the invariant enterBodyUnit exists to
// create: the VM re-enters a compiled unit as a nested BODY from exactly one
// place.
//
// This is a source-shape gate rather than a behavioural one because the
// property it protects is structural and has no runtime symptom until
// something is added to the seam. That is precisely how the ground was lost
// the first time: design/verse-report-defects-investigation.0.md §B records a
// context-frame patch that bracketed `invokeClosureOn` alone, passed the full
// gate including every compiled-differential test, and still left `RunUnit`,
// `runUnitNested`, CALL_USER and the inlined forms leaking — because nothing
// in the tree said how many body-entry paths there were. A new `vc.run` call
// site is how that recurs, and it is invisible to every other test we have.
//
// The allowlist is deliberately tiny and each entry is load-bearing:
//
//   - enterBodyUnit — the funnel itself.
//   - runProgram    — the TOP-LEVEL program entry (startUnit -1, the
//     program's own Code rather than a unit). Not a nested body: it is the
//     run the interpreter's own top-level `Engine.Run` corresponds to, so
//     per-body state does not belong to it.
//
// If you are adding a third, you are adding a body-entry path the §B analysis
// does not know about. Route it through enterBodyUnit instead, and if it
// genuinely cannot be routed, extend enterBodyUnit's four-path map so the
// next reader inherits the correct count rather than re-deriving it.
func TestVMBodyEntryIsFunnelled(t *testing.T) {
	const allowFunnel = "enterBodyUnit"
	allowed := map[string]bool{
		allowFunnel:  true,
		"runProgram": true,
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "vm.go", nil, 0)
	if err != nil {
		t.Fatalf("parse vm.go: %v", err)
	}

	callers := map[string]int{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "run" {
				return true
			}
			// vc.run(...) — the receiver is the vmContext local in every
			// caller; match on the method name plus a bare identifier
			// receiver so an unrelated `x.run()` on some other type would
			// still be caught rather than silently skipped.
			if _, ok := sel.X.(*ast.Ident); !ok {
				return true
			}
			callers[name]++
			return true
		})
	}

	if callers[allowFunnel] == 0 {
		t.Fatalf("%s no longer calls vc.run — the funnel is gone, and this "+
			"gate would pass vacuously for every other caller", allowFunnel)
	}

	var extra []string
	for name := range callers {
		if !allowed[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("vc.run is called from %v, outside the body-entry funnel. "+
			"Every nested body-unit entry must go through %s — see its doc "+
			"comment for the four paths and why one of them cannot be "+
			"funnelled at all.", extra, allowFunnel)
	}
}
