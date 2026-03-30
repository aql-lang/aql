package engine

import (
	"fmt"
	"testing"
)

// TestSequentialPlanner_BasicInfix tests 2 add 3 with the sequential planner.
func TestSequentialPlanner_BasicInfix(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := New(r)
	out, err := e.Run([]Value{NewInteger(2), NewWord("add"), NewInteger(3)})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 1 || out[0].AsInteger() != 5 {
		t.Fatalf("expected [5], got %v", out)
	}
}

// TestSequentialPlanner_DefForward tests def foo 42 foo.
func TestSequentialPlanner_DefForward(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := NewTop(r)
	out, err := e.Run([]Value{
		NewWord("def"), NewWord("foo"), NewInteger(42),
		NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 1 || out[0].AsInteger() != 42 {
		t.Fatalf("expected [42], got %v", out)
	}
}

// TestSequentialPlanner_DefGreeting tests def greeting "hello" greeting.
func TestSequentialPlanner_DefGreeting(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := NewTop(r)
	out, err := e.Run([]Value{
		NewWord("def"), NewWord("greeting"), NewString("hello"),
		NewWord("greeting"),
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 1 || out[0].AsString() != "hello" {
		t.Fatalf("expected ['hello'], got %v", out)
	}
}

// TestSequentialPlanner_UndefSimple tests def foo 1 foo undef foo foo.
func TestSequentialPlanner_UndefSimple(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := NewTop(r)
	out, err := e.Run([]Value{
		NewWord("def"), NewWord("foo"), NewInteger(1),
		NewWord("foo"),
		NewWord("undef"), NewWord("foo"),
		NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(out), out)
	}
	if out[0].AsInteger() != 1 {
		t.Errorf("result[0]: expected 1, got %v", out[0])
	}
	if out[1].AsAtom() != "foo" {
		t.Errorf("result[1]: expected atom foo, got %v", out[1])
	}
}

// TestSequentialPlanner_SetGet tests set foo 99 get foo.
func TestSequentialPlanner_SetGet(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := NewTop(r)
	out, err := e.Run([]Value{
		NewWord("set"), NewWord("foo"), NewInteger(99),
		NewWord("get"), NewWord("foo"),
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 1 || out[0].AsInteger() != 99 {
		t.Fatalf("expected [99], got %v", out)
	}
}

// TestSequentialPlanner_ContextSetGet tests context set/get with word keys.
func TestSequentialPlanner_ContextSetGet(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := NewTop(r)
	r.PushContext(make(map[string]Value))
	out, err := e.Run([]Value{
		NewWord("context"), NewWord("set"), NewWord("x"), NewInteger(42),
		NewWord("context"), NewWord("get"), NewWord("x"),
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 1 || out[0].AsInteger() != 42 {
		t.Fatalf("expected [42], got %v", out)
	}
}

// TestSequentialPlanner_AddPrefixAndForward tests add in various positions.
func TestSequentialPlanner_AddPrefixAndForward(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true

	tests := []struct {
		name   string
		tokens []Value
		want   int64
	}{
		{"infix", []Value{NewInteger(2), NewWord("add"), NewInteger(3)}, 5},
		{"forward", []Value{NewWord("add"), NewInteger(2), NewInteger(3)}, 5},
		{"prefix", []Value{NewInteger(2), NewInteger(3), NewWord("add")}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(r)
			out, err := e.Run(tt.tokens)
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if len(out) != 1 || out[0].AsInteger() != tt.want {
				t.Fatalf("expected [%d], got %v", tt.want, out)
			}
		})
	}
}

// TestSequentialPlanner_DefFnSquare tests def sq fn [[x:Number] [Number] [x mul x]] 5 sq.
func TestSequentialPlanner_DefFnSquare(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := NewTop(r)

	// Build fn spec list: [[x:Number] [Number] [x mul x]]
	inputSig := NewList([]Value{
		func() Value {
			m := NewOrderedMap()
			m.Set("x", NewTypeLiteral(TNumber))
			return NewImplicitMap(m)
		}(),
	})
	outputSig := NewList([]Value{NewTypeLiteral(TNumber)})
	body := NewList([]Value{NewWord("x"), NewWord("mul"), NewWord("x")})
	fnList := NewList([]Value{inputSig, outputSig, body})

	out, err := e.Run([]Value{
		NewWord("def"), NewWord("sq"),
		NewWord("fn"), fnList,
		NewInteger(5), NewWord("sq"),
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 1 || out[0].AsInteger() != 25 {
		t.Fatalf("expected [25], got %v", out)
	}
}

// TestSequentialPlanner_Quote tests quote word.
func TestSequentialPlanner_Quote(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := New(r)
	out, err := e.Run([]Value{NewWord("quote"), NewWord("hello")})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 1 || out[0].AsAtom() != "hello" {
		t.Fatalf("expected [atom(hello)], got %v", out)
	}
}

// TestSequentialPlanner_Dup tests dup.
func TestSequentialPlanner_Dup(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := New(r)
	out, err := e.Run([]Value{NewInteger(7), NewWord("dup")})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(out) != 2 || out[0].AsInteger() != 7 || out[1].AsInteger() != 7 {
		t.Fatalf("expected [7 7], got %v", out)
	}
}

// TestSequentialPlanner_TypeDef tests type word.
func TestSequentialPlanner_TypeDef(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := NewTop(r)
	// type MyNum Number
	out, err := e.Run([]Value{
		NewWord("type"), NewWord("MyNum"), NewTypeLiteral(TNumber),
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	_ = out // type returns nothing; just check no error
}

// TestSequentialPlanner_PlannerUnit tests the planner function directly.
func TestSequentialPlanner_PlannerUnit(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := New(r)

	// Test: add with [Integer, Integer] on forward stream
	fn := r.Lookup("add")
	if fn == nil {
		t.Fatal("add not found")
	}

	e.stack = []Value{NewWord("add"), NewInteger(2), NewInteger(3)}
	e.pointer = 0

	sig, stackCount := e.plannerSequentialForward(fn, WordInfo{Name: "add", ArgCount: -1}, nil)
	if sig == nil {
		t.Fatal("expected a matching signature")
	}
	fmt.Printf("matched: %v, stackCount=%d\n", sig.Args, stackCount)
	if stackCount != 0 {
		t.Errorf("expected 0 stack args for all-forward, got %d", stackCount)
	}
}

// TestSequentialPlanner_PlannerStackMatch tests stack filling.
func TestSequentialPlanner_PlannerStackMatch(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SequentialPlanner = true
	e := New(r)

	fn := r.Lookup("add")
	if fn == nil {
		t.Fatal("add not found")
	}

	// Stack has one integer, forward has one integer: 5 add 3
	e.stack = []Value{NewInteger(5), NewWord("add"), NewInteger(3)}
	e.pointer = 1
	resolved := []Value{NewInteger(5)}

	sig, stackCount := e.plannerSequentialForward(fn, WordInfo{Name: "add", ArgCount: -1}, resolved)
	if sig == nil {
		t.Fatal("expected a matching signature")
	}
	fmt.Printf("matched: %v, forward+stack, stackCount=%d\n", sig.Args, stackCount)
}
