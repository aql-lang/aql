package native

import (
	"testing"
)

// parkedFn evaluates src and returns its trailing value. Every caller
// parks the fn with `/v`: a bare 0-arg NAMED fn would dispatch on the spot
// (which is half of the asymmetry under test), so only the parked form
// yields the Function VALUE applyHandler is contracted on.
func parkedFn(t *testing.T, r *Registry, src string) Value {
	t.Helper()
	out, err := seam5Run(r, src)
	if err != nil || len(out) == 0 {
		t.Fatalf("build fn value from %q: %v / %v", src, out, err)
	}
	return out[len(out)-1]
}

// TestADR016ZeroArgApplyIgnoresOrigin pins NUR077 §5 Hole 1: `apply` on a
// fn whose only signatures are 0-arg applies it AT THE APPLY SITE, so the
// answer no longer depends on whether the fn is anonymous or named.
//
// Before this, applyHandler unquoted the value and handed it back for the
// engine to re-step — and the re-step reaches execFnDefLiteral's data
// gate, which keeps a 0-arg ANONYMOUS value inert. So `f/v apply` answered
// `fn f` for a lambda and `42` for the named twin: arity AND origin
// deciding behaviour, which ADR-016 forbids.
func TestADR016ZeroArgApplyIgnoresOrigin(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"anonymous lambda", `def f ([] => [42]) end f/v`},
		{"named fn", `def z fn [[] [Integer] [42]] end z/v`},
	} {
		r := seam5Reg(t)
		out, err := applyHandler([]Value{parkedFn(t, r, tc.src)}, nil, nil, r)
		if err != nil {
			t.Fatalf("%s: applyHandler: %v", tc.name, err)
		}
		if len(out) != 1 {
			t.Fatalf("%s: want one result, got %d: %v", tc.name, len(out), out)
		}
		n, aerr := AsInteger(out[0])
		if aerr != nil || n != 42 {
			t.Errorf("%s: want the APPLIED result 42, got %v (err %v) — "+
				"origin must not decide at arity 0", tc.name, out[0], aerr)
		}
	}
}

// TestADR016ApplyKeepsArgTakingFnInert pins the restriction that keeps the
// fix from eclipsing an arg-taking overload (the NUR035 hazard): a fn with
// any arg-taking signature is still handed back for the engine to re-step
// against its stack args, which is what makes `5 inc/v apply` work.
func TestADR016ApplyKeepsArgTakingFnInert(t *testing.T) {
	r := seam5Reg(t)
	for _, src := range []string{
		`def inc fn [[n:Integer] [Integer] [n add 1]] end inc/v`,
		`def ov fn [[] [Integer] [99] [n:Integer] [Integer] [n add 1]] end ov/v`,
	} {
		out, err := applyHandler([]Value{parkedFn(t, r, src)}, nil, nil, r)
		if err != nil {
			t.Fatalf("applyHandler %q: %v", src, err)
		}
		if len(out) != 1 || !out[0].Parent.Equal(TFunction) {
			t.Errorf("%q: an arg-taking fn must come back as the FUNCTION for "+
				"the re-step to bind its args, got %v", src, out)
		}
	}
}
