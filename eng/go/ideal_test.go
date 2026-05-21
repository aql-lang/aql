package eng

import "testing"

func TestIdealRegistry_RegisterGet(t *testing.T) {
	ir := NewIdealRegistry()
	a := &Ideal{Name: "A", Enabled: true}
	ir.Register(a)
	if got := ir.Get("A"); got != a {
		t.Errorf("Get(A) = %v, want the registered Ideal", got)
	}
	if got := ir.Get("missing"); got != nil {
		t.Errorf("Get(missing) = %v, want nil", got)
	}
}

func TestIdealRegistry_ForResolvesByAccepts(t *testing.T) {
	ir := NewIdealRegistry()
	ir.Register(&Ideal{
		Name: "Stringy", Enabled: true,
		Accepts: func(v Value) bool { return v.VType.Matches(TString) },
	})
	ir.Register(&Ideal{
		Name: "Inty", Enabled: true,
		Accepts: func(v Value) bool { return v.VType.Matches(TInteger) },
	})
	if id := ir.For(NewString("x")); id == nil || id.Name != "Stringy" {
		t.Errorf("For(string) = %v, want Stringy", id)
	}
	if id := ir.For(NewInteger(1)); id == nil || id.Name != "Inty" {
		t.Errorf("For(integer) = %v, want Inty", id)
	}
	if id := ir.For(NewBoolean(true)); id != nil {
		t.Errorf("For(boolean) = %v, want nil (no kind claims it)", id)
	}
}

func TestIdealRegistry_ForSkipsDisabled(t *testing.T) {
	ir := NewIdealRegistry()
	ir.Register(&Ideal{
		Name: "Off", Enabled: false,
		Accepts: func(Value) bool { return true },
	})
	if id := ir.For(NewString("x")); id != nil {
		t.Errorf("For with only a disabled Ideal = %v, want nil", id)
	}
}

func TestIdealRegistry_ForFirstMatchWins(t *testing.T) {
	ir := NewIdealRegistry()
	ir.Register(&Ideal{Name: "First", Enabled: true, Accepts: func(Value) bool { return true }})
	ir.Register(&Ideal{Name: "Second", Enabled: true, Accepts: func(Value) bool { return true }})
	if id := ir.For(NewString("x")); id == nil || id.Name != "First" {
		t.Errorf("For = %v, want First (registration order wins ties)", id)
	}
}

func TestIdealRegistry_ReregisterReplaces(t *testing.T) {
	ir := NewIdealRegistry()
	ir.Register(&Ideal{Name: "K", Enabled: true, Accepts: func(Value) bool { return false }})
	ir.Register(&Ideal{Name: "K", Enabled: true, Accepts: func(Value) bool { return true }})
	got := ir.Get("K")
	if got == nil || !got.Accepts(NewString("x")) {
		t.Error("re-register did not replace the descriptor")
	}
	if n := len(ir.Names()); n != 1 {
		t.Errorf("re-register changed the count to %d, want 1", n)
	}
}

func TestIdealRegistry_Names(t *testing.T) {
	ir := NewIdealRegistry()
	ir.Register(&Ideal{Name: "One", Enabled: true})
	ir.Register(&Ideal{Name: "Two", Enabled: true})
	names := ir.Names()
	if len(names) != 2 || names[0] != "One" || names[1] != "Two" {
		t.Errorf("Names() = %v, want [One Two] in registration order", names)
	}
}

func TestIdealRegistry_NilSafe(t *testing.T) {
	var ir *IdealRegistry
	ir.Register(&Ideal{Name: "X"})
	if ir.Get("X") != nil || ir.For(NewString("x")) != nil ||
		ir.Match(NewString("x")) != nil || ir.Names() != nil {
		t.Error("nil IdealRegistry methods must be safe no-ops")
	}
}

// Match reports the first claiming Ideal regardless of Enabled; For
// additionally requires it to be enabled. The pair lets a caller tell
// a disabled kind apart from an unknown base.
func TestIdealRegistry_Match(t *testing.T) {
	ir := NewIdealRegistry()
	ir.Register(&Ideal{
		Name: "Off", Enabled: false,
		Accepts: func(v Value) bool { return v.VType.Matches(TString) },
	})
	ir.Register(&Ideal{
		Name: "On", Enabled: true,
		Accepts: func(v Value) bool { return v.VType.Matches(TInteger) },
	})
	if id := ir.Match(NewString("x")); id == nil || id.Name != "Off" {
		t.Errorf("Match(string) = %v, want Off (Match reports disabled kinds)", id)
	}
	if id := ir.For(NewString("x")); id != nil {
		t.Errorf("For(string) = %v, want nil (For skips disabled kinds)", id)
	}
	if id := ir.Match(NewInteger(1)); id == nil || id.Name != "On" {
		t.Errorf("Match(integer) = %v, want On", id)
	}
	if id := ir.For(NewInteger(1)); id == nil || id.Name != "On" {
		t.Errorf("For(integer) = %v, want On", id)
	}
	if id := ir.Match(NewBoolean(true)); id != nil {
		t.Errorf("Match(boolean) = %v, want nil (no kind claims it)", id)
	}
}

func TestRegisterKernelIdeals(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Object", "Record", "Table"} {
		id := r.Ideals.Get(name)
		if id == nil {
			t.Fatalf("kernel Ideal %q not registered by NewRegistry", name)
		}
		if id.Accepts == nil {
			t.Errorf("kernel Ideal %q has no Accepts predicate", name)
		}
		if id.Instantiate == nil {
			t.Errorf("kernel Ideal %q has no Instantiate", name)
		}
	}
}

// A refinement is available for dispatch only while its whole Refines
// chain is enabled; Match reports it regardless of the chain state.
func TestIdeal_Refines(t *testing.T) {
	ir := NewIdealRegistry()
	base := &Ideal{
		Name: "Tensor", Enabled: true,
		Accepts: func(v Value) bool { return v.VType.Matches(TInteger) },
	}
	ref := &Ideal{
		Name: "Matrix", Enabled: true, Refines: base,
		Accepts: func(v Value) bool { return v.VType.Matches(TString) },
	}
	ir.Register(base)
	ir.Register(ref)
	if id := ir.For(NewString("x")); id == nil || id.Name != "Matrix" {
		t.Fatalf("For(string) = %v, want Matrix", id)
	}
	// Disabling the base kind makes the refinement unavailable too.
	base.Enabled = false
	if id := ir.For(NewString("x")); id != nil {
		t.Errorf("For with base disabled = %v, want nil (refinement follows its base)", id)
	}
	if id := ir.Match(NewString("x")); id == nil || id.Name != "Matrix" {
		t.Errorf("Match with base disabled = %v, want Matrix (Match ignores the chain)", id)
	}
	// Re-enabling the base restores the refinement.
	base.Enabled = true
	if id := ir.For(NewString("x")); id == nil || id.Name != "Matrix" {
		t.Errorf("For after re-enabling base = %v, want Matrix", id)
	}
	// Disabling the refinement itself also removes it.
	ref.Enabled = false
	if id := ir.For(NewString("x")); id != nil {
		t.Errorf("For with refinement disabled = %v, want nil", id)
	}
}

type fakeHostType struct{ HostTypeBody }

// A host module's constructed type — an ExtensionPayload whose Body
// embeds HostTypeBody — is recognised by the kernel's type machinery
// without the kernel inspecting the opaque payload.
func TestIsHostTypeBody(t *testing.T) {
	typeVal := NewExtension(TAny, fakeHostType{})
	if !IsHostTypeBody(typeVal) {
		t.Error("IsHostTypeBody(marked extension) = false, want true")
	}
	if !IsTypeBody(typeVal) {
		t.Error("IsTypeBody(host type body) = false, want true")
	}
	// An ExtensionPayload whose Body does not embed HostTypeBody is an
	// instance, not a type.
	instVal := NewExtension(TAny, struct{ n int }{1})
	if IsHostTypeBody(instVal) {
		t.Error("IsHostTypeBody(plain extension) = true, want false")
	}
	if IsTypeBody(instVal) {
		t.Error("IsTypeBody(plain extension instance) = true, want false")
	}
	if IsHostTypeBody(NewInteger(1)) {
		t.Error("IsHostTypeBody(integer) = true, want false")
	}
}
