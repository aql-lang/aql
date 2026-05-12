package engine_test

import (
	"github.com/aql-lang/aql/lang/engine"
	"github.com/aql-lang/aql/lang/native"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// await mode:'all (default) tests
// =============================================================================

func TestAwaitAllDefault(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	// await [[1 add 2] [3 add 4]]
	// Both branches succeed → [3, 7]
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("add"), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewInteger(3), engine.NewWord("add"), engine.NewInteger(4)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	list := result[0].AsList()
	if list.IsNil() || list.Len() != 2 {
		t.Fatalf("expected list of 2, got %v", result[0])
	}
	v0 := list.Get(0)
	v1 := list.Get(1)
	i0, _ := v0.AsInteger()
	i1, _ := v1.AsInteger()
	if i0 != 3 || i1 != 7 {
		t.Errorf("expected [3, 7], got [%d, %d]", i0, i1)
	}
}

func TestAwaitAllWithError(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	// await [[1 add 2] [1 div 0]]
	// Second branch errors → result is the error
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("add"), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("div"), engine.NewInteger(0)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	if !result[0].IsError() {
		t.Fatalf("expected error value, got %s", result[0].String())
	}
}

func TestAwaitAllRunsInParallel(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	// Each branch sleeps 50ms. If parallel, total should be ~50ms, not ~150ms.
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewWord("sleep"), engine.NewInteger(50), engine.NewInteger(1)}),
		engine.NewList([]engine.Value{engine.NewWord("sleep"), engine.NewInteger(50), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewWord("sleep"), engine.NewInteger(50), engine.NewInteger(3)}),
	})
	inner.Quoted = true

	start := time.Now()
	result, err := e.Run([]engine.Value{engine.NewWord("await"), inner})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	list := result[0].AsList()
	if list.IsNil() || list.Len() != 3 {
		t.Fatalf("expected list of 3, got %v", result[0])
	}
	// Should complete in roughly 50ms, not 150ms.
	if elapsed > 120*time.Millisecond {
		t.Errorf("expected parallel execution (~50ms), took %v", elapsed)
	}
}

func TestAwaitAllEmpty(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	inner := engine.NewList([]engine.Value{})
	inner.Quoted = true
	result, err := e.Run([]engine.Value{engine.NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}
	list := result[0].AsList()
	if list.Len() != 0 {
		t.Fatalf("expected empty list, got %v", result[0])
	}
}

// =============================================================================
// await mode:'full tests
// =============================================================================

func TestAwaitFull(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	// Build options: make Options {mode:'full}
	optsFields := engine.NewOrderedMap()
	optsFields.Set("mode", engine.NewAtom("full"))
	optsVal := engine.NewOptionsType(optsFields)

	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("add"), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("div"), engine.NewInteger(0)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}

	list := result[0].AsList()
	if list.IsNil() || list.Len() != 2 {
		t.Fatalf("expected list of 2 results, got %v", result[0])
	}

	// First result: {status:ok, value:3}
	r0 := list.Get(0)
	m0 := r0.AsMap()
	if m0 == nil {
		t.Fatalf("expected map for result[0], got %v", r0)
	}
	status0, _ := m0.Get("status")
	s0, _ := status0.AsAtom()
	if s0 != "ok" {
		t.Errorf("expected status 'ok', got %q", s0)
	}
	val0, _ := m0.Get("value")
	iv0, _ := val0.AsInteger()
	if iv0 != 3 {
		t.Errorf("expected value 3, got %d", iv0)
	}

	// Second result: {status:error, value:error(...)}
	r1 := list.Get(1)
	m1 := r1.AsMap()
	if m1 == nil {
		t.Fatalf("expected map for result[1], got %v", r1)
	}
	status1, _ := m1.Get("status")
	s1, _ := status1.AsAtom()
	if s1 != "error" {
		t.Errorf("expected status 'error', got %q", s1)
	}
	val1, _ := m1.Get("value")
	if !val1.IsError() {
		t.Errorf("expected error value, got %v", val1)
	}
}

// =============================================================================
// await mode:'first tests
// =============================================================================

func TestAwaitFirst(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)

	// Register a word that records call order.
	var order atomic.Int32
	reg.RegisterStackOnly("testorder", engine.Signature{
		Args: []*engine.Type{},
		Handler: func(_ []engine.Value, _ map[string]engine.Value, _ []engine.Value, _ *engine.Registry) ([]engine.Value, error) {
			return []engine.Value{engine.NewInteger(int64(order.Add(1)))}, nil
		},
	})

	e := engine.NewTop(reg)

	optsFields := engine.NewOrderedMap()
	optsFields.Set("mode", engine.NewAtom("first"))
	optsVal := engine.NewOptionsType(optsFields)

	// First branch is fast (no sleep), second is slow.
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(42)}),
		engine.NewList([]engine.Value{engine.NewWord("sleep"), engine.NewInteger(100), engine.NewInteger(99)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	// The fast branch (42) should win.
	v, _ := result[0].AsInteger()
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestAwaitFirstWithError(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	optsFields := engine.NewOrderedMap()
	optsFields.Set("mode", engine.NewAtom("first"))
	optsVal := engine.NewOptionsType(optsFields)

	// First branch errors immediately, second succeeds but slowly.
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("div"), engine.NewInteger(0)}),
		engine.NewList([]engine.Value{engine.NewWord("sleep"), engine.NewInteger(50), engine.NewInteger(99)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First (the error) should win since it finishes first.
	if len(result) != 1 || !result[0].IsError() {
		t.Fatalf("expected error value from first-to-finish, got %v", result)
	}
}

// =============================================================================
// await mode:'any tests
// =============================================================================

func TestAwaitAny(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	optsFields := engine.NewOrderedMap()
	optsFields.Set("mode", engine.NewAtom("any"))
	optsVal := engine.NewOptionsType(optsFields)

	// First branch errors, second succeeds.
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("div"), engine.NewInteger(0)}),
		engine.NewList([]engine.Value{engine.NewInteger(42)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d: %v", len(result), result)
	}
	// Should get the successful result (42), not the error.
	v, _ := result[0].AsInteger()
	if v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
}

func TestAwaitAnyAllReject(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	optsFields := engine.NewOrderedMap()
	optsFields.Set("mode", engine.NewAtom("any"))
	optsVal := engine.NewOptionsType(optsFields)

	// All branches error.
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("div"), engine.NewInteger(0)}),
		engine.NewList([]engine.Value{engine.NewInteger(2), engine.NewWord("div"), engine.NewInteger(0)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].IsError() {
		t.Fatalf("expected error when all reject, got %v", result)
	}
}

func TestAwaitAnyFirstSuccess(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	optsFields := engine.NewOrderedMap()
	optsFields.Set("mode", engine.NewAtom("any"))
	optsVal := engine.NewOptionsType(optsFields)

	// First branch is slow error, second is fast success.
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewWord("sleep"), engine.NewInteger(100), engine.NewInteger(1), engine.NewWord("div"), engine.NewInteger(0)}),
		engine.NewList([]engine.Value{engine.NewInteger(7)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, _ := result[0].AsInteger()
	if v != 7 {
		t.Errorf("expected 7, got %d", v)
	}
}

// =============================================================================
// await with sleep for timing tests
// =============================================================================

func TestAwaitWithSleep(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	// Two branches that sleep then return a value.
	// sleep 20 consumes 20 as forward arg. Then 1 remains on stack.
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewWord("sleep"), engine.NewInteger(20), engine.NewInteger(1)}),
		engine.NewList([]engine.Value{engine.NewWord("sleep"), engine.NewInteger(40), engine.NewInteger(2)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := result[0].AsList()
	v0 := list.Get(0)
	v1 := list.Get(1)
	i0, _ := v0.AsInteger()
	i1, _ := v1.AsInteger()
	if i0 != 1 || i1 != 2 {
		t.Errorf("expected [1, 2], got [%d, %d]", i0, i1)
	}
}

// =============================================================================
// await error handling edge cases
// =============================================================================

func TestAwaitAllMultipleErrors(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	// All branches error — should return the first error in order.
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewWord("div"), engine.NewInteger(0)}),
		engine.NewList([]engine.Value{engine.NewInteger(2), engine.NewWord("div"), engine.NewInteger(0)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || !result[0].IsError() {
		t.Fatalf("expected error, got %v", result)
	}
}

func TestAwaitFullAllSucceed(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	optsFields := engine.NewOrderedMap()
	optsFields.Set("mode", engine.NewAtom("full"))
	optsVal := engine.NewOptionsType(optsFields)

	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(10)}),
		engine.NewList([]engine.Value{engine.NewInteger(20)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), optsVal, inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := result[0].AsList()
	for i := 0; i < list.Len(); i++ {
		v := list.Get(i)
		m := v.AsMap()
		if m == nil {
			t.Fatalf("expected map at [%d], got %v", i, v)
		}
		st, _ := m.Get("status")
		s, _ := st.AsAtom()
		if s != "ok" {
			t.Errorf("expected status ok at [%d], got %q", i, s)
		}
	}
}

// =============================================================================
// await invalid mode
// =============================================================================

func TestAwaitInvalidMode(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	optsFields := engine.NewOrderedMap()
	optsFields.Set("mode", engine.NewAtom("bogus"))
	optsVal := engine.NewOptionsType(optsFields)

	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1)}),
	})
	inner.Quoted = true

	_, err := e.Run([]engine.Value{engine.NewWord("await"), optsVal, inner})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should mention invalid mode, got: %v", err)
	}
}

// =============================================================================
// await type literal no-panic
// =============================================================================

func TestAwaitTypeLiteralNoPanic(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	e := engine.NewTop(reg)
	// Pass a type literal (Data==nil) — should error, not panic.
	_, _ = e.Run([]engine.Value{engine.NewTypeLiteral(engine.TList), engine.NewWord("await")})
}

// =============================================================================
// await non-list elements
// =============================================================================

func TestAwaitNonListElement(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	// Non-list elements are returned as-is.
	inner := engine.NewList([]engine.Value{
		engine.NewInteger(42),
		engine.NewString("hello"),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list := result[0].AsList()
	if list.IsNil() || list.Len() != 2 {
		t.Fatalf("expected list of 2, got %v", result[0])
	}
	v0 := list.Get(0)
	i0, _ := v0.AsInteger()
	if i0 != 42 {
		t.Errorf("expected 42, got %d", i0)
	}
	v1 := list.Get(1)
	s1, _ := v1.AsString()
	if s1 != "hello" {
		t.Errorf("expected 'hello', got %q", s1)
	}
}

// =============================================================================
// await with multi-value branch results
// =============================================================================

func TestAwaitMultiValueBranch(t *testing.T) {
	reg, _ := engine.DefaultRegistry(native.Register)
	e := engine.NewTop(reg)

	// Branch that leaves multiple values on the stack: [1 2 3]
	inner := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}),
		engine.NewList([]engine.Value{engine.NewInteger(42)}),
	})
	inner.Quoted = true

	result, err := e.Run([]engine.Value{engine.NewWord("await"), inner})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := result[0].AsList()
	if list.IsNil() || list.Len() != 2 {
		t.Fatalf("expected list of 2, got %v", result[0])
	}

	// First result: multi-value → wrapped in a list [1,2,3]
	v0 := list.Get(0)
	sublist := v0.AsList()
	if sublist.IsNil() || sublist.Len() != 3 {
		t.Fatalf("expected sub-list of 3 for multi-value branch, got %v", v0)
	}

	// Second result: single value → unwrapped
	v1 := list.Get(1)
	i1, _ := v1.AsInteger()
	if i1 != 42 {
		t.Errorf("expected 42, got %d", i1)
	}
}
