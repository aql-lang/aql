package eng

import "testing"

// ADR-008 coverage for tryNativeFnApply's foreign-AQL-body decline: a fn
// value whose name resolves in a FOREIGN sub-registry to an AQL-BODIED
// signature must NOT run its body-splicing handler against the dispatching
// registry (the module-scope escape the aql:repl served handler exposed) —
// the fast path declines so callDynamic islands through the interpreter's
// foreign-wrapper branch. A same-registry AQL fn keeps the fast path.
func TestTryNativeFnApplyForeignAQLDeclines(t *testing.T) {
	main, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// An installed AQL-bodied fn in the foreign registry: `greet x = 42`
	// (the body content is irrelevant; what matters is the AQLImpl sig
	// with a body-splicing dispatch handler).
	install := func(r *Registry) {
		InstallFnDef(r, "greet", FnDefInfo{
			Name: "greet",
			Signatures: []Signature{{
				Params:  []FnParam{{Name: "x", Type: TAny}},
				Impl:    AQL([]Value{NewInteger(42)}),
				Returns: []*Type{TAny},
			}},
		})
	}
	install(foreign)
	install(main)

	vc := &vmContext{r: main}
	args := []Value{NewInteger(1)}

	// Foreign AQL body: decline (done=false) so the caller islands.
	_, done, err := vc.tryNativeFnApply(FnDefInfo{Name: "greet", Registry: foreign}, args)
	if err != nil {
		t.Fatalf("foreign decline: unexpected error %v", err)
	}
	if done {
		t.Error("foreign AQL-bodied fn must decline the fast path (done=false)")
	}

	// Same-registry AQL body: the fast path may proceed — the dispatching
	// registry IS the owning registry, so module scope cannot be lost.
	_, done, err = vc.tryNativeFnApply(FnDefInfo{Name: "greet", Registry: main}, args)
	if err != nil {
		t.Fatalf("same-registry apply: %v", err)
	}
	if !done {
		t.Error("same-registry AQL fn should stay on the fast path (done=true)")
	}
}
