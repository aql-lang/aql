package stackform

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
)

// Unit tests for stackform — structural operations on StackForm
// values (Append, Len, Equal, Walk, Cost) that don't need a live
// engine. Compile-Eval round-trip and equivalence-with-direct-run
// tests live in lang/go/test/stackform_equivalence_test.go where
// the language-layer registry is available.

func TestStackFormAppendAndLen(t *testing.T) {
	f := &StackForm{}
	if f.Len() != 0 {
		t.Errorf("empty form len = %d, want 0", f.Len())
	}
	f.Append(PushLit{V: eng.NewInteger(1)})
	f.Append(Call{Name: "add", Arity: 2})
	if f.Len() != 2 {
		t.Errorf("after 2 appends len = %d, want 2", f.Len())
	}
}

func TestStackFormEqual(t *testing.T) {
	a := &StackForm{Ops: []Op{
		PushLit{V: eng.NewInteger(1)},
		PushLit{V: eng.NewInteger(2)},
		Call{Name: "add", Arity: 2},
	}}
	b := &StackForm{Ops: []Op{
		PushLit{V: eng.NewInteger(1)},
		PushLit{V: eng.NewInteger(2)},
		Call{Name: "add", Arity: 2},
	}}
	if !Equal(a, b) {
		t.Error("identical forms should be Equal")
	}
	c := &StackForm{Ops: []Op{
		PushLit{V: eng.NewInteger(1)},
		PushLit{V: eng.NewInteger(3)},
		Call{Name: "add", Arity: 2},
	}}
	if Equal(a, c) {
		t.Error("forms differing in a literal should NOT be Equal")
	}
	d := &StackForm{Ops: []Op{
		PushLit{V: eng.NewInteger(1)},
		PushLit{V: eng.NewInteger(2)},
		Call{Name: "sub", Arity: 2},
	}}
	if Equal(a, d) {
		t.Error("forms differing in a Call name should NOT be Equal")
	}
}

func TestStackFormEqualNestedQuote(t *testing.T) {
	q1 := &StackForm{Ops: []Op{PushLit{V: eng.NewInteger(42)}}}
	q2 := &StackForm{Ops: []Op{PushLit{V: eng.NewInteger(42)}}}
	q3 := &StackForm{Ops: []Op{PushLit{V: eng.NewInteger(43)}}}
	a := &StackForm{Ops: []Op{Quote{Body: q1}}}
	b := &StackForm{Ops: []Op{Quote{Body: q2}}}
	c := &StackForm{Ops: []Op{Quote{Body: q3}}}
	if !Equal(a, b) {
		t.Error("Quote bodies with identical Ops should be Equal")
	}
	if Equal(a, c) {
		t.Error("Quote bodies with different Ops should NOT be Equal")
	}
}

func TestStackFormWalk(t *testing.T) {
	inner := &StackForm{Ops: []Op{
		PushLit{V: eng.NewInteger(99)},
		Call{Name: "neg", Arity: 1},
	}}
	form := &StackForm{Ops: []Op{
		PushLit{V: eng.NewInteger(1)},
		Quote{Body: inner},
		Call{Name: "wrap", Arity: 1},
	}}
	var seen []string
	Walk(form, func(_ []int, op Op) bool {
		switch o := op.(type) {
		case PushLit:
			n, _ := eng.AsInteger(o.V)
			seen = append(seen, "lit:"+itoa(n))
		case Call:
			seen = append(seen, "call:"+o.Name)
		case Quote:
			seen = append(seen, "quote")
		}
		return true
	})
	// Pre-order traversal: outer first, then descend into Quote bodies.
	want := []string{"lit:1", "quote", "lit:99", "call:neg", "call:wrap"}
	if len(seen) != len(want) {
		t.Fatalf("Walk visited %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("Walk[%d]=%q, want %q", i, seen[i], want[i])
		}
	}
}

func TestStackFormCost(t *testing.T) {
	form := &StackForm{Ops: []Op{
		PushLit{V: eng.NewInteger(1)}, // 1
		PushLit{V: eng.NewInteger(2)}, // 1
		Call{Name: "add", Arity: 2},   // 2
	}}
	if got := Cost(form); got != 4 {
		t.Errorf("Cost = %d, want 4 (1+1+2)", got)
	}
	nested := &StackForm{Ops: []Op{
		Quote{Body: form}, // 1 + Cost(form) = 1 + 4 = 5
	}}
	if got := Cost(nested); got != 5 {
		t.Errorf("Cost(nested) = %d, want 5", got)
	}
	if got := Cost(nil); got != 0 {
		t.Errorf("Cost(nil) = %d, want 0", got)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
