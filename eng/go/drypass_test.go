package eng

import "testing"

// Direct pins for the dry-pass plumbing (drypass.go): operand
// canonicalisation and the concreteness gate.

func TestDryPassOperandsCanonicalisesNone(t *testing.T) {
	// A strict none-SHAPE arg becomes the canonical `none` sentinel, so
	// handler payload probes (IsNone, truthiness) agree with runtime;
	// concrete args pass through untouched (same backing slice when no
	// canonicalisation is needed).
	args := []Value{NewTypeLiteral(TNone), NewInteger(1)}
	got := dryPassOperands(args)
	if !IsNone(got[0]) {
		t.Errorf("none literal not canonicalised: %v", got[0])
	}
	if n, err := AsInteger(got[1]); err != nil || n != 1 {
		t.Errorf("concrete arg disturbed: %v", got[1])
	}

	allConcrete := []Value{NewInteger(2)}
	if same := dryPassOperands(allConcrete); &same[0] != &allConcrete[0] {
		t.Error("all-concrete args should pass through without copying")
	}
}

func TestDryPassConcretenessGate(t *testing.T) {
	if allConcreteArgs([]Value{NewInteger(1), NewCarrier(TInteger)}) {
		t.Error("a bare carrier arg must fail the dry-pass gate")
	}
	dynNone := NewCarrier(TNone)
	dynNone.Dynamic = true
	if allConcreteArgs([]Value{dynNone}) {
		t.Error("a DYNAMIC none carrier must fail the dry-pass gate")
	}
	if !allConcreteArgs([]Value{NewInteger(1), NewTypeLiteral(TNone)}) {
		t.Error("concrete + strict none-shape args must pass the gate")
	}
}

func TestEmitIndexOOBCaughtBodyReattributed(t *testing.T) {
	// Inside a `do [...]` body the consuming word's runtime error is
	// trapped, so the provable-OOB finding is re-attributed: downgraded to
	// info with CaughtAtRuntime stamped (never an error verdict), while
	// the same finding outside the region keeps SeverityError.
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Check.Begin())
	r.Check.CaughtBodyDepth = 1
	CheckListIndex(r, NewInteger(5), NewList([]Value{NewInteger(1)}), "getr")
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("caught-body OOB not recorded: %+v", r.Check.Diagnostics)
	}
	if d := r.Check.Diagnostics[0]; d.Severity != SeverityInfo || !d.CaughtAtRuntime || !d.RuntimeMirror {
		t.Fatalf("caught-body OOB not re-attributed (want info + CaughtAtRuntime + RuntimeMirror): %+v", d)
	}
	r.Check.CaughtBodyDepth = 0
	CheckListIndex(r, NewInteger(5), NewList([]Value{NewInteger(1)}), "getr")
	last := r.Check.Diagnostics[len(r.Check.Diagnostics)-1]
	if last.Severity != SeverityError || last.CaughtAtRuntime {
		t.Fatalf("uncaught OOB must keep the error verdict: %+v", last)
	}
}

func TestBeginCompilePassArmsTheRitual(t *testing.T) {
	// Nil receiver: total, like Begin (the host may hold a zero registry).
	if done := (*CheckState)(nil).BeginCompilePass(); done == nil {
		t.Fatal("nil-receiver BeginCompilePass must still return a done func")
	} else {
		done()
	}

	// The shared compile-pass helper must arm every piece of the ritual —
	// a hand-rolled copy missing one (Vm.compile shipped without the
	// Compiling flag) is the bug class this helper retires.
	c := &CheckState{FnSummaries: map[string][]Value{"x": nil}, FnInflight: map[string]bool{"x": true}}
	done := c.BeginCompilePass()
	defer done()
	if !c.Mode || !c.Compiling {
		t.Fatalf("Mode/Compiling not armed: mode=%v compiling=%v", c.Mode, c.Compiling)
	}
	if c.Emit == theInactiveEmit || c.Emit == nil {
		t.Fatal("a real EmitState must be installed")
	}
	if c.FnSummaries != nil || c.FnInflight != nil {
		t.Fatal("fn-body memos must be dropped so bodies re-record under this pass")
	}
	done()
	if c.Mode {
		t.Fatal("done() must switch check mode off")
	}
}
