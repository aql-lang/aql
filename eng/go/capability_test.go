package eng

import (
	"errors"
	"testing"
)

// These tests cover the capability plugin system: how the host
// installs opaque services on the registry and how a word handler
// retrieves them at dispatch time.

func TestCapabilityRoundTrip(t *testing.T) {
	r, _ := NewRegistry()
	if _, ok, err := r.Capabilities.Get("missing"); err != nil || ok {
		t.Errorf("unset capability: (%v, %v), want (_, false, nil)", ok, err)
	}

	if err := r.Capabilities.Set("foo", 42); err != nil {
		t.Fatalf("Set foo: %v", err)
	}
	v, ok, err := r.Capabilities.Get("foo")
	if err != nil {
		t.Fatalf("Get foo: %v", err)
	}
	if !ok {
		t.Fatal("foo capability should be present after set")
	}
	if v.(int) != 42 {
		t.Errorf("got %v, want 42", v)
	}

	// Replace.
	if err := r.Capabilities.Set("foo", "replaced"); err != nil {
		t.Fatalf("Set foo (replace): %v", err)
	}
	v, _, _ = r.Capabilities.Get("foo")
	if v.(string) != "replaced" {
		t.Errorf("after replace: got %v, want \"replaced\"", v)
	}

	// SetCapability(name, nil) STORES nil — it no longer doubles as
	// a delete. Use DeleteCapability for that.
	if err := r.Capabilities.Set("foo", nil); err != nil {
		t.Fatalf("Set foo (nil): %v", err)
	}
	v, ok, _ = r.Capabilities.Get("foo")
	if !ok {
		t.Error("capability should still be present after storing a nil value")
	}
	if v != nil {
		t.Errorf("got %v, want nil", v)
	}

	// Delete and verify.
	present, err := r.Capabilities.Delete("foo")
	if err != nil {
		t.Fatalf("Delete foo: %v", err)
	}
	if !present {
		t.Error("Delete should report true on existing key")
	}
	if _, ok, _ := r.Capabilities.Get("foo"); ok {
		t.Error("capability should be gone after Delete")
	}
	present, _ = r.Capabilities.Delete("foo")
	if present {
		t.Error("Delete should report false on missing key")
	}
}

func TestCapNilSafety(t *testing.T) {
	// Cap on a nil registry surfaces an error rather than panicking
	// or silently returning (zero, false).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Cap panicked on nil registry: %v", r)
		}
	}()
	v, ok, err := Cap[string]((*Registry)(nil), "anything")
	if ok || v != "" {
		t.Errorf("got (%v, %v), want zero value", v, ok)
	}
	if !errors.Is(err, errCapabilityNil) {
		t.Errorf("err = %v, want errCapabilityNil", err)
	}
}

func TestCapTypedSuccess(t *testing.T) {
	type counter struct{ n int }
	r, _ := NewRegistry()
	if err := r.Capabilities.Set("c", &counter{n: 7}); err != nil {
		t.Fatal(err)
	}

	c, ok, err := Cap[*counter](r, "c")
	if err != nil {
		t.Fatalf("Cap: %v", err)
	}
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
	if err := r.Capabilities.Set("answer", "forty-two"); err != nil {
		t.Fatal(err)
	}

	n, ok, err := Cap[int](r, "answer")
	if err != nil {
		t.Fatalf("Cap: %v", err)
	}
	if ok {
		t.Error("Cap[int] should fail when capability holds a string")
	}
	if n != 0 {
		t.Errorf("got %d, want zero value", n)
	}
}

func TestCapabilityNames(t *testing.T) {
	r, _ := NewRegistry()
	names, err := r.Capabilities.Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("fresh registry: %v, want empty", names)
	}

	for _, kv := range []struct {
		k string
		v int
	}{{"a", 1}, {"b", 2}, {"c", 3}} {
		if err := r.Capabilities.Set(kv.k, kv.v); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.Capabilities.Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
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

// TestCapabilityNilReceiver verifies that every method on a nil
// *CapabilityRegistry surfaces a non-nil error rather than silently
// returning a zero value.
func TestCapabilityNilReceiver(t *testing.T) {
	var c *CapabilityRegistry

	v, ok, err := c.Get("x")
	if ok || v != nil {
		t.Errorf("nil Get = (%v, %v, ...), want (nil, false, ...)", v, ok)
	}
	if !errors.Is(err, errCapabilityNil) {
		t.Errorf("nil Get err = %v, want errCapabilityNil", err)
	}

	if err := c.Set("x", 1); !errors.Is(err, errCapabilityNil) {
		t.Errorf("nil Set err = %v, want errCapabilityNil", err)
	}

	present, err := c.Delete("x")
	if present {
		t.Errorf("nil Delete present = true, want false")
	}
	if !errors.Is(err, errCapabilityNil) {
		t.Errorf("nil Delete err = %v, want errCapabilityNil", err)
	}

	names, err := c.Names()
	if names != nil {
		t.Errorf("nil Names = %v, want nil", names)
	}
	if !errors.Is(err, errCapabilityNil) {
		t.Errorf("nil Names err = %v, want errCapabilityNil", err)
	}
}

// CapabilityAvailableToHandler is the central guarantee: a word
// handler receives *Registry and can retrieve capabilities the host
// installed before Run.
func TestCapabilityAvailableToHandler(t *testing.T) {
	type calc struct{ factor int64 }

	r, _ := NewRegistry()
	if err := r.Capabilities.Set("scaler", &calc{factor: 10}); err != nil {
		t.Fatal(err)
	}

	r.RegisterNativeFunc(NativeFunc{
		Name:        "scale",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{TInteger},
			Handler: func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				c, ok, err := Cap[*calc](reg, "scaler")
				if err != nil {
					return nil, err
				}
				if !ok {
					t.Fatal("scaler capability missing inside handler")
				}
				n, _ := AsInteger(args[0])
				return []Value{NewInteger(n * c.factor)}, nil
			},
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})
	r.InitRootContext()

	out, err := NewTop(r).Run([]Value{NewWord("scale"), NewInteger(7)})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := AsInteger(out[0])
	if got != 70 {
		t.Errorf("got %d, want 70", got)
	}
}

func TestCapabilityMissingIsNotFatal(t *testing.T) {
	// A handler that asks for a capability nobody installed should
	// receive (zero, false, nil) and can decide what to do —
	// typically return a meaningful error rather than panic.
	r, _ := NewRegistry()
	r.RegisterNativeFunc(NativeFunc{
		Name:        "needs-cap",
		ForwardArgs: true,
		Signatures: []NativeSig{{
			Args: []*Type{},
			Handler: func(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
				_, ok, err := Cap[string](reg, "ghost")
				if err != nil {
					return nil, err
				}
				if ok {
					t.Fatal("missing capability should not be ok")
				}
				return []Value{NewString("absent")}, nil
			}, BarrierPos: -1,
		}},
	})
	r.InitRootContext()

	out, err := NewTop(r).Run([]Value{NewWord("needs-cap")})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := AsString(out[0])
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
	if err := r.Capabilities.Set("formats", formats); err != nil {
		t.Fatal(err)
	}

	got, ok, err := Cap[map[string]*formatter](r, "formats")
	if err != nil {
		t.Fatalf("Cap: %v", err)
	}
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
	again, _, _ := Cap[map[string]*formatter](r, "formats")
	if again["xml"].tag != "XML" {
		t.Error("in-place format addition not visible on re-lookup")
	}
}
