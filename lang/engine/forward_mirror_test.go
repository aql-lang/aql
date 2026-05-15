package engine_test

import (
	"fmt"
	"github.com/aql-lang/aql/lang/engine"
	"github.com/aql-lang/aql/lang/native"
	"testing"
)

// TestForwardMirrorPattern verifies the symmetric mirror equivalence for
// forward-collecting words with 1–7 args. For a word f with args a,b,c:
//
//	f a b c  <=>  c f a b  <=>  c b f a  <=>  c b a f
//
// The pattern: each equivalent form moves the last forward arg to the far
// left (deepest stack position). Invalid placements (e.g. a f b for 2 args)
// must produce different results.
//
// Each test word encodes sig-order into the result: for N args with values
// 1..N, it returns sig[0]*10^(N-1) + sig[1]*10^(N-2) + ... + sig[N-1].
// The canonical result for correct ordering is always 123...N.

func RegisterMirrorTestWord(r *engine.Registry, name string, arity int) {
	args := make([]*engine.Type, arity)
	for i := range args {
		args[i] = engine.TInteger
	}
	handler := func(a []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
		result := int64(0)
		for i := 0; i < arity; i++ {
			v, _ := engine.AsInteger(a[i])
			mul := int64(1)
			for j := 0; j < arity-1-i; j++ {
				mul *= 10
			}
			result += v * mul
		}
		return []engine.Value{engine.NewInteger(result)}, nil
	}
	r.Register(name, engine.Signature{
		Args:    args,
		Handler: handler,
	})
}

// mirrorForms generates all N+1 equivalent mirror forms for an N-arg word.
// For N=3 with word "f" and args [a,b,c]:
//
//	f a b c   (0 prefix, 3 forward)
//	c f a b   (1 prefix, 2 forward)
//	c b f a   (2 prefix, 1 forward)
//	c b a f   (3 prefix, 0 forward)
func mirrorForms(word string, args []engine.Value) []struct {
	label string
	input []engine.Value
} {
	n := len(args)
	forms := make([]struct {
		label string
		input []engine.Value
	}, n+1)

	for prefixCount := 0; prefixCount <= n; prefixCount++ {
		fwdCount := n - prefixCount

		input := make([]engine.Value, 0, n+1)

		// Stack args: last prefixCount args in reverse order (deepest first).
		for i := 0; i < prefixCount; i++ {
			input = append(input, args[n-1-i])
		}

		// The word itself.
		input = append(input, engine.NewWord(word))

		// Forward args: first fwdCount args in order.
		for i := 0; i < fwdCount; i++ {
			input = append(input, args[i])
		}

		label := fmt.Sprintf("%d_prefix_%d_forward", prefixCount, fwdCount)
		forms[prefixCount] = struct {
			label string
			input []engine.Value
		}{label, input}
	}
	return forms
}

func TestForwardMirror1Arg(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m1", 1)

	args := []engine.Value{engine.NewInteger(1)}
	canonical := int64(1)

	for _, form := range mirrorForms("m1", args) {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d results, want 1", len(result))
			}
			v, _ := engine.AsInteger(result[0])
			if v != canonical {
				t.Errorf("got %d, want %d", v, canonical)
			}
		})
	}
}

func TestForwardMirror2Args(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m2", 2)

	args := []engine.Value{engine.NewInteger(1), engine.NewInteger(2)}
	canonical := int64(12) // sig[0]=1, sig[1]=2 → 1*10+2=12

	for _, form := range mirrorForms("m2", args) {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d results, want 1", len(result))
			}
			v, _ := engine.AsInteger(result[0])
			if v != canonical {
				t.Errorf("got %d, want %d", v, canonical)
			}
		})
	}
}

func TestForwardMirror2ArgsInvalid(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m2i", 2)

	// a f b should give sig[0]=b, sig[1]=a → 21, NOT 12.
	input := []engine.Value{engine.NewInteger(1), engine.NewWord("m2i"), engine.NewInteger(2)}
	e := engine.New(r)
	result, err := e.Run(input)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d results, want 1", len(result))
	}
	v, _ := engine.AsInteger(result[0])
	if v == 12 {
		t.Errorf("a f b should NOT equal f a b; both gave %d", v)
	}
	if v != 21 {
		t.Errorf("a f b: got %d, want 21 (sig[0]=2, sig[1]=1)", v)
	}
}

func TestForwardMirror3Args(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m3", 3)

	args := []engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}
	canonical := int64(123)

	for _, form := range mirrorForms("m3", args) {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d results, want 1", len(result))
			}
			v, _ := engine.AsInteger(result[0])
			if v != canonical {
				t.Errorf("got %d, want %d", v, canonical)
			}
		})
	}
}

func TestForwardMirror4Args(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m4", 4)

	args := []engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4)}
	canonical := int64(1234)

	for _, form := range mirrorForms("m4", args) {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d results, want 1", len(result))
			}
			v, _ := engine.AsInteger(result[0])
			if v != canonical {
				t.Errorf("got %d, want %d", v, canonical)
			}
		})
	}
}

func TestForwardMirror5Args(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m5", 5)

	args := []engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4), engine.NewInteger(5)}
	canonical := int64(12345)

	for _, form := range mirrorForms("m5", args) {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d results, want 1", len(result))
			}
			v, _ := engine.AsInteger(result[0])
			if v != canonical {
				t.Errorf("got %d, want %d", v, canonical)
			}
		})
	}
}

func TestForwardMirror6Args(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m6", 6)

	args := []engine.Value{
		engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3),
		engine.NewInteger(4), engine.NewInteger(5), engine.NewInteger(6),
	}
	canonical := int64(123456)

	for _, form := range mirrorForms("m6", args) {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d results, want 1", len(result))
			}
			v, _ := engine.AsInteger(result[0])
			if v != canonical {
				t.Errorf("got %d, want %d", v, canonical)
			}
		})
	}
}

func TestForwardMirror7Args(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m7", 7)

	args := []engine.Value{
		engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4),
		engine.NewInteger(5), engine.NewInteger(6), engine.NewInteger(7),
	}
	canonical := int64(1234567)

	for _, form := range mirrorForms("m7", args) {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d results, want 1", len(result))
			}
			v, _ := engine.AsInteger(result[0])
			if v != canonical {
				t.Errorf("got %d, want %d", v, canonical)
			}
		})
	}
}

// TestForwardMirror3ArgsInvalidPlacements verifies that non-mirror orderings
// produce different (wrong) sig mappings for 3 args.
func TestForwardMirror3ArgsInvalidPlacements(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	RegisterMirrorTestWord(r, "m3x", 3)

	canonical := int64(123)

	// These are NOT mirror forms — they should NOT produce 123.
	invalids := []struct {
		label string
		input []engine.Value
	}{
		// a f b c: 1 prefix(a), 2 forward(b,c) — a goes to sig[2], not sig[0]
		{"a_f_b_c", []engine.Value{engine.NewInteger(1), engine.NewWord("m3x"), engine.NewInteger(2), engine.NewInteger(3)}},
		// a b f c: 2 prefix(a,b), 1 forward(c) — wrong stack order
		{"a_b_f_c", []engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewWord("m3x"), engine.NewInteger(3)}},
	}

	for _, inv := range invalids {
		t.Run(inv.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(inv.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("got %d results, want 1", len(result))
			}
			v, _ := engine.AsInteger(result[0])
			if v == canonical {
				t.Errorf("%s should NOT produce %d (same as canonical mirror form)", inv.label, canonical)
			}
		})
	}
}

// TestForwardMirrorSubVerification cross-checks with the built-in sub word
// (non-commutative) to confirm the mirror pattern matches real arithmetic.
func TestForwardMirrorSubVerification(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// sub is 2-arg, non-commutative: 10 sub 3 = 7, 3 sub 10 = -7.
	// Mirror: sub a b <=> b sub a <=> b a sub.
	forms7 := []struct {
		label string
		input []engine.Value
	}{
		{"sub_3_10", []engine.Value{engine.NewWord("sub"), engine.NewInteger(3), engine.NewInteger(10)}},
		{"10_sub_3", []engine.Value{engine.NewInteger(10), engine.NewWord("sub"), engine.NewInteger(3)}},
		{"10_3_sub", []engine.Value{engine.NewInteger(10), engine.NewInteger(3), engine.NewWord("sub")}},
	}
	for _, form := range forms7 {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			v, _ := engine.AsInteger(result[0])
			if v != 7 {
				t.Errorf("got %d, want 7", v)
			}
		})
	}

	formsMinus7 := []struct {
		label string
		input []engine.Value
	}{
		{"sub_10_3", []engine.Value{engine.NewWord("sub"), engine.NewInteger(10), engine.NewInteger(3)}},
		{"3_sub_10", []engine.Value{engine.NewInteger(3), engine.NewWord("sub"), engine.NewInteger(10)}},
		{"3_10_sub", []engine.Value{engine.NewInteger(3), engine.NewInteger(10), engine.NewWord("sub")}},
	}
	for _, form := range formsMinus7 {
		t.Run(form.label, func(t *testing.T) {
			e := engine.New(r)
			result, err := e.Run(form.input)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			v, _ := engine.AsInteger(result[0])
			if v != -7 {
				t.Errorf("got %d, want -7", v)
			}
		})
	}
}
