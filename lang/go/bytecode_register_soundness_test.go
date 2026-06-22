package lang

import (
	"fmt"
	"testing"
)

// A parser registered via MiniLang.register / ParseLang.register is dispatched
// later through a RUNTIME-added export key (`ParseLang.parse_<name>`,
// `MiniLang.<name>`). The bytecode recorder const-folded the `get <name>` over
// the module export to a missing-key None — using the check-time module
// snapshot, BEFORE register installed the key at runtime — then dropped the
// dispatch and left the call's args on the stack. So the compiled program used
// to silently DIVERGE from the interpreter (which sees the registered key).
//
// The fix is in tryFoldModuleConst (eng/go/carrier.go): a `get` that resolves to
// None is a MISSING key, and a module's keyspace can grow at runtime, so the
// missing-key fold is declined — the get stays dynamic and the program falls
// back / islands faithfully. This guards the bare-dispatch hole specifically:
// the spec rows all have a trailing `get` that already refused, but
// `ParseLang.parse_calc 'x + y' {}` with no get had nothing downstream to refuse
// on — it compiled to garbage.
func TestRegisterDispatchFallsBackNotDiverges(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			"parselang bare dispatch (no trailing get)",
			`"aql:parselang" import end  "aql:string-util" import end  ` +
				`ParseLang.register calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]]) end  ` +
				`ParseLang.parse_calc 'x + y' {} end`,
		},
		{
			"parselang sugar + get (already a spec row)",
			`"aql:parselang" import end  "aql:string-util" import end  ` +
				`ParseLang.register calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]]) end  ` +
				`(parse calc 'x + y') get 1`,
		},
		{
			"parselang desugared + get",
			`"aql:parselang" import end  "aql:string-util" import end  ` +
				`ParseLang.register calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]]) end  ` +
				`(ParseLang.parse_calc 'x + y' {} end) get 1`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ia, _ := New()
			want, werr := ia.Run(c.src)

			ca, _ := New()
			got, compiled, gerr := ca.RunCompiled(c.src)

			// A register-using program MUST fall back — the compiled path
			// cannot dispatch the runtime-registered word soundly.
			if compiled {
				t.Fatalf("register-using program took the COMPILED path; it must fall back\n  src: %s", c.src)
			}
			// And the fallback must match the interpreter exactly (value + error).
			if fmt.Sprint(want) != fmt.Sprint(got) || (werr == nil) != (gerr == nil) {
				t.Fatalf("fallback diverged from interpreter\n  interp: %v (err=%v)\n  comp:   %v (err=%v)",
					want, werr, got, gerr)
			}
		})
	}
}

// Negative / control: an out-of-band export that does NOT register a
// runtime-dispatched word (ParseLang.kinds) must still COMPILE — the refusal is
// scoped to register, not to the whole module.
func TestNonRegisterModuleWordStillCompiles(t *testing.T) {
	src := `"aql:parselang" import end  ParseLang.kinds`
	a, _ := New()
	got, err := a.RunCompiledStrict(src)
	if err != nil {
		t.Fatalf("ParseLang.kinds should compile, got refusal: %v", err)
	}
	b, _ := New()
	want, _ := b.Run(src)
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Fatalf("kinds compiled result %v != interpreter %v", got, want)
	}
}
