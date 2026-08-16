package core

import "testing"

// FnHome answers "which registry resolves this fn value's free words, and which
// captures ride along" for every native callback seam
// (design/FUNCTION-VALUE-SCOPE.0.md). Its three arms are the three shapes a
// callback seam can be handed, and each has a different correct answer — so
// each is pinned here rather than left to the integration suites, where a
// wrong arm would surface as a mysterious value rather than as this function.
func TestFnHome(t *testing.T) {
	caller, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	defining, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	caps := []CapturedBinding{{Name: "x", Value: NewInteger(1)}}

	t.Run("no fn value falls back to the caller", func(t *testing.T) {
		// A synthesized carrier sig with no fn behind it: there is no other
		// registry to route to, and no captures to install.
		got, gotCaps := FnHome(caller, nil)
		if got != caller {
			t.Errorf("registry = %p, want the caller %p", got, caller)
		}
		if gotCaps != nil {
			t.Errorf("captures = %v, want nil", gotCaps)
		}
	})

	t.Run("a fn defined in the running scope stays there", func(t *testing.T) {
		// Registry == nil means "defined where it is running"; the caller IS
		// the defining registry, so routing anywhere else would be wrong.
		got, gotCaps := FnHome(caller, &FnDefInfo{Captured: caps})
		if got != caller {
			t.Errorf("registry = %p, want the caller %p", got, caller)
		}
		if len(gotCaps) != 1 || gotCaps[0].Name != "x" {
			t.Errorf("captures = %v, want the fn's own %v", gotCaps, caps)
		}
	})

	t.Run("a fn from another module routes to its definer", func(t *testing.T) {
		// The case the whole fix exists for: the body's free words must resolve
		// where it was WRITTEN, not where a Go word happens to invoke it.
		got, gotCaps := FnHome(caller, &FnDefInfo{Registry: defining, Captured: caps})
		if got != defining {
			t.Errorf("registry = %p, want the DEFINING registry %p (got the caller: %v)",
				got, defining, got == caller)
		}
		if len(gotCaps) != 1 || gotCaps[0].Name != "x" {
			t.Errorf("captures = %v, want the fn's own %v", gotCaps, caps)
		}
	})
}

// CallBoruFn is FnHome wired to the INTERPRETER call, and it is the arm with
// no in-module caller: every consumer (`filter`'s Function form, the map-lambda
// `each`/`fold` bodies, core `walk`, `StructUtil.walk`, `IO.mount`,
// `boru:parse`) lives in lang/go. core/go is gated by its own suite at a 100%
// floor (`make cover-gate-core`), so "covered from above" leaves it reading as
// dead code here — it has held that gate red since the seam landed. Driving it
// directly closes that, and pins the two things the wrapper must not get wrong.
func TestCallBoruFn(t *testing.T) {
	// The defining registry owns a word the caller does not have. If CallBoruFn
	// ran the body anywhere but there, this call would fail undefined_word —
	// which is the pre-seam bug, surfacing as an error rather than a silence.
	defining := covRegistry(t, func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "modonly",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					n, _ := AsInteger(args[0])
					return []Value{NewInteger(n * 1000)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	})
	caller := covRegistry(t, nil)

	sig := &FnSig{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    Boru([]Value{NewOpenParen(), NewWord("modonly"), NewWord("n"), NewCloseParen()}),
	}

	t.Run("runs the body in the DEFINING registry", func(t *testing.T) {
		out, err := CallBoruFn(caller, &FnDefInfo{Registry: defining}, sig, []Value{NewInteger(3)})
		if err != nil {
			t.Fatalf("CallBoruFn: %v (the body resolved in the caller, where `modonly` is unbound)", err)
		}
		if got, _ := AsInteger(out[0]); got != 3000 {
			t.Errorf("CallBoruFn = %v, want 3000", out[0])
		}
	})

	t.Run("captures ride along", func(t *testing.T) {
		capSig := &FnSig{
			Params:  []FnParam{{Name: "n", Type: TInteger}},
			Returns: []*Type{TInteger},
			Impl:    Boru([]Value{NewOpenParen(), NewWord("modonly"), NewWord("base"), NewCloseParen()}),
		}
		out, err := CallBoruFn(caller,
			&FnDefInfo{Registry: defining, Captured: []CapturedBinding{{Name: "base", Value: NewInteger(7)}}},
			capSig, []Value{NewInteger(1)})
		if err != nil {
			t.Fatalf("CallBoruFn with capture: %v", err)
		}
		if got, _ := AsInteger(out[0]); got != 7000 {
			t.Errorf("captured call = %v, want 7000", out[0])
		}
	})

	t.Run("a nil fn value falls back to the caller", func(t *testing.T) {
		// The FnHome nil arm reached through the wrapper: a synthesized carrier
		// sig with no fn behind it must run where it was invoked.
		local := &FnSig{
			Params:  []FnParam{{Name: "n", Type: TInteger}},
			Returns: []*Type{TInteger},
			Impl:    Boru([]Value{NewOpenParen(), NewWord("cadd"), NewWord("n"), NewWord("n"), NewCloseParen()}),
		}
		out, err := CallBoruFn(caller, nil, local, []Value{NewInteger(6)})
		if err != nil {
			t.Fatalf("CallBoruFn with no fn value: %v", err)
		}
		if got, _ := AsInteger(out[0]); got != 12 {
			t.Errorf("CallBoruFn = %v, want 12", out[0])
		}
	})
}
