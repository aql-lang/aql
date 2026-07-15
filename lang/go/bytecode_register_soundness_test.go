package lang

import (
	"fmt"
	"testing"
)

// A parser registered via ParseLang.register is dispatched later through a
// RUNTIME-added export key (`ParseLang.parse_<name>`). Historically the
// bytecode recorder const-folded the `get <name>` over the module export to a
// missing-key None — using the check-time module snapshot, BEFORE register
// installed the key at runtime — then dropped the dispatch and left the call's
// args on the stack, so the compiled program silently DIVERGED from the
// interpreter. The guard required these programs to FALL BACK.
//
// Stage 3 (design/aql-bytecode-stage3-inlining-plan.0.md, module-parselang:23)
// makes them compile SOUNDLY instead: ParseLang.register's check-mode ReturnsFn
// installs `parse_<name>` so the key is present at check time (the runtime
// register handler is idempotent on the source-call identity, so the compiled
// program's re-run does not re-error), and the resolved parser fn value
// dispatches to a CALL_USER unit (parselang.go + execFnDefSigStackMatch's
// check-mode fn-value path). The invariant this test guards is unchanged — the
// compiled result must never diverge from the interpreter — but the mechanism
// is now "compile soundly", not "fall back". Whichever path RunCompiled takes,
// the result (value + error presence) must match the interpreter exactly.
func TestRegisterDispatchFallsBackNotDiverges(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			"parselang bare dispatch (no trailing get)",
			`import "aql:parselang"  import "aql:string-util"  ` +
				`ParseLang.register calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]]) end  ` +
				`ParseLang.parse_calc 'x + y' {} end`,
		},
		{
			"parselang sugar + get (already a spec row)",
			`import "aql:parselang"  import "aql:string-util"  ` +
				`ParseLang.register calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]]) end  ` +
				`(parse calc 'x + y') get 1`,
		},
		{
			"parselang desugared + get",
			`import "aql:parselang"  import "aql:string-util"  ` +
				`ParseLang.register calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]]) end  ` +
				`(ParseLang.parse_calc 'x + y' {} end) get 1`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ia, _ := New()
			want, werr := ia.RunInterp(c.src)

			ca, _ := New()
			got, _, gerr := ca.RunCompiled(c.src)

			// Whether the program compiled or fell back, its result must match
			// the interpreter exactly (value + error presence).
			if fmt.Sprint(want) != fmt.Sprint(got) || (werr == nil) != (gerr == nil) {
				t.Fatalf("compiled result diverged from interpreter\n  interp: %v (err=%v)\n  comp:   %v (err=%v)",
					want, werr, got, gerr)
			}
		})
	}
}

// Negative / control: an out-of-band export that does NOT register a
// runtime-dispatched word (ParseLang.kinds) must still COMPILE — the refusal is
// scoped to register, not to the whole module.
func TestNonRegisterModuleWordStillCompiles(t *testing.T) {
	src := `import "aql:parselang"  ParseLang.kinds`
	a, _ := New()
	got, err := a.RunCompiledStrict(src)
	if err != nil {
		t.Fatalf("ParseLang.kinds should compile, got refusal: %v", err)
	}
	b, _ := New()
	want, _ := b.RunInterp(src)
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Fatalf("kinds compiled result %v != interpreter %v", got, want)
	}
}
