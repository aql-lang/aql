package aqleng

import (
	"testing"
)

// These tests cover the capability plugin system: how the host
// installs opaque services on the registry and how a word handler
// retrieves them at dispatch time.

func TestCapabilityRoundTrip(t *testing.T) {
	r, _ := NewRegistry()
	if _, ok := r.Capability("missing"); ok {
		t.Error("unset capability should be missing")
	}

	r.SetCapability("foo", 42)
	v, ok := r.Capability("foo")
	if !ok {
		t.Fatal("foo capability should be present after set")
	}
	if v.(int) != 42 {
		t.Errorf("got %v, want 42", v)
	}

	// Replace.
	r.SetCapability("foo", "replaced")
	v, _ = r.Capability("foo")
	if v.(string) != "replaced" {
		t.Errorf("after replace: got %v, want \"replaced\"", v)
	}

	// Pass nil to clear.
	r.SetCapability("foo", nil)
	if _, ok := r.Capability("foo"); ok {
		t.Error("capability should be cleared after SetCapability(name, nil)")
	}
}

func TestCapNilSafety(t *testing.T) {
	// Cap on a nil registry should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Cap panicked on nil registry: %v", r)
		}
	}()
	v, ok := Cap[string]((*Registry)(nil), "anything")
	if ok || v != "" {
		t.Errorf("got (%v, %v), want (\"\", false)", v, ok)
	}
}

func TestCapTypedSuccess(t *testing.T) {
	type counter struct{ n int }
	r, _ := NewRegistry()
	r.SetCapability("c", &counter{n: 7})

	c, ok := Cap[*counter](r, "c")
	if !ok {
		t.Fatal("Cap[*counter] should succeed")
	}
	if c.n != 7 {
		t.Errorf("c.n = %d, want 7", c.n)
	}
}

func TestCapTypedWrongType(t *testing.T) {
	// Capability is stored as a string; asking for an int returns the
	// zero value and false rather than panicking.
	r, _ := NewRegistry()
	r.SetCapability("answer", "forty-two")

	n, ok := Cap[int](r, "answer")
	if ok {
		t.Error("Cap[int] should fail when capability holds a string")
	}
	if n != 0 {
		t.Errorf("got %d, want zero value", n)
	}
}

func TestCapabilityNames(t *testing.T) {
	r, _ := NewRegistry()
	if names := r.CapabilityNames(); len(names) != 0 {
		t.Errorf("fresh registry: %v, want empty", names)
	}

	r.SetCapability("a", 1)
	r.SetCapability("b", 2)
	r.SetCapability("c", 3)
	got := r.CapabilityNames()
	if len(got) != 3 {
		t.Fatalf("got %d names, want 3 (%v)", len(got), got)
	}

	// Set membership only — order is unspecified.
	want := map[string]bool{"a": true, "b": true, "c": true}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected name %q in %v", n, got)
		}
	}
}

// CapabilityAvailableToHandler is the central guarantee: a word
// handler receives *Registry and can retrieve capabilities the host
// installed before Run.
func TestCapabilityAvailableToHandler(t *testing.T) {
	type calc struct{ factor int64 }

	r, _ := NewRegistry()
	r.SetCapability("scaler", &calc{factor: 10})

	r.RegisterNativeFunc(NativeFunc{
		Name:              "scale",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				c, ok := Cap[*calc](reg, "scaler")
				if !ok {
					t.Fatal("scaler capability missing inside handler")
				}
				n, _ := args[0].AsInteger()
				return []Value{NewInteger(n * c.factor)}, nil
			},
			Returns: []Type{TInteger},
		}},
	})
	r.InitRootContext()

	out, err := NewTop(r).Run([]Value{NewWord("scale"), NewInteger(7)})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := out[0].AsInteger()
	if got != 70 {
		t.Errorf("got %d, want 70", got)
	}
}

func TestCapabilityMissingIsNotFatal(t *testing.T) {
	// A handler that asks for a capability nobody installed should
	// receive (zero, false) and can decide what to do — typically
	// return a meaningful error rather than panic.
	r, _ := NewRegistry()
	r.RegisterNativeFunc(NativeFunc{
		Name:              "needs-cap",
		ForwardPrecedence: true,
		Signatures: []NativeSig{{
			Args: []Type{},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				if _, ok := Cap[string](reg, "ghost"); ok {
					t.Fatal("missing capability should not be ok")
				}
				return []Value{NewString("absent")}, nil
			},
		}},
	})
	r.InitRootContext()

	out, err := NewTop(r).Run([]Value{NewWord("needs-cap")})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := out[0].AsString()
	if got != "absent" {
		t.Errorf("got %q, want \"absent\"", got)
	}
}

func TestCapabilityMapPattern(t *testing.T) {
	// The "format registry" pattern: store a map under one capability
	// key, look up entries by name. This is how the host wires its
	// format encoders/decoders without aqleng knowing about formats.
	type formatter struct{ tag string }
	formats := map[string]*formatter{
		"json": {tag: "JSON"},
		"csv":  {tag: "CSV"},
	}

	r, _ := NewRegistry()
	r.SetCapability("formats", formats)

	got, ok := Cap[map[string]*formatter](r, "formats")
	if !ok {
		t.Fatal("formats capability missing")
	}
	if got["json"].tag != "JSON" {
		t.Errorf("formats[json].tag = %q, want JSON", got["json"].tag)
	}

	// In-place mutation via the retrieved reference is visible to a
	// later lookup — the host owns the map, capabilities just hold a
	// reference.
	got["xml"] = &formatter{tag: "XML"}
	again, _ := Cap[map[string]*formatter](r, "formats")
	if again["xml"].tag != "XML" {
		t.Error("in-place format addition not visible on re-lookup")
	}
}
