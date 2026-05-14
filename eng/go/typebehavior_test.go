package eng

import "testing"

// TestDefaultBehaviorInstalled verifies that every builtin *Type
// carries a non-nil Behavior after init. The Behavior seam relies on
// this invariant — any code path that calls t.Behavior.Match(...)
// without nil-checking would panic on a Type that escaped
// registration without Behavior wiring.
func TestDefaultBehaviorInstalled(t *testing.T) {
	if len(Builtin.byID) == 0 {
		t.Fatal("Builtin TypeTable is empty")
	}
	for id, def := range Builtin.byID {
		if def == nil {
			t.Errorf("Builtin.byID[%q] is nil", id)
			continue
		}
		if def.Behavior == nil {
			t.Errorf("Builtin type %q (%s) has nil Behavior", def.Path(), id)
		}
	}
}

// TestMintTypeInstallsDefaultBehavior verifies that MintType wires a
// non-nil Behavior on dynamically-minted types.
func TestMintTypeInstallsDefaultBehavior(t *testing.T) {
	tt := NewDynamicTypeTable()
	def := tt.MintType("DynamicFoo", TInteger)
	if def.Behavior == nil {
		t.Fatal("MintType returned a Type with nil Behavior")
	}
}

// TestMintTypeWithBehaviorRespectsArg verifies that
// MintTypeWithBehavior installs the supplied Behavior, falling back
// to DefaultBehavior on nil.
func TestMintTypeWithBehaviorRespectsArg(t *testing.T) {
	tt := NewDynamicTypeTable()

	custom := &fakeBehavior{}
	a := tt.MintTypeWithBehavior("WithCustom", TInteger, custom)
	if a.Behavior != custom {
		t.Errorf("WithCustom: got Behavior %T, want custom", a.Behavior)
	}

	b := tt.MintTypeWithBehavior("WithNil", TInteger, nil)
	if b.Behavior != DefaultBehavior {
		t.Errorf("WithNil: got Behavior %T, want DefaultBehavior", b.Behavior)
	}
}

// TestDefaultBehaviorMatchDelegates verifies DefaultBehavior.Match
// agrees with the historical lattice walk on a representative
// hierarchy (Integer ⊂ Number ⊂ Scalar).
func TestDefaultBehaviorMatchDelegates(t *testing.T) {
	v := NewInteger(42)
	cases := []struct {
		target *Type
		want   bool
	}{
		{TInteger, true},
		{TNumber, true},
		{TScalar, true},
		{TAny, true},
		{TString, false},
		{TList, false},
	}
	for _, c := range cases {
		got := DefaultBehavior.Match(v, c.target)
		if got != c.want {
			t.Errorf("DefaultBehavior.Match(NewInteger(42), %s) = %v, want %v",
				c.target, got, c.want)
		}
	}
}

// fakeBehavior is a test-only TypeBehavior used to verify that
// MintTypeWithBehavior installs the supplied Behavior verbatim.
type fakeBehavior struct{}

func (fakeBehavior) Match(Value, *Type) bool { return false }
func (fakeBehavior) Format(Value) string     { return "fake" }
func (fakeBehavior) Equal(Value, Value) bool { return false }
