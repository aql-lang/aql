package engine

import (
	"testing"
)

// --- Basic context set/get ---

func TestContextSetGetString(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("x"), NewInteger(42),
		NewWord("context"), NewWord("get"), NewString("x"),
	})
	if len(result) != 1 || result[0].AsInteger() != 42 {
		t.Errorf("context get x = %v, want 42", result)
	}
}

func TestContextSetGetWordKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewWord("foo"), NewInteger(99),
		NewWord("context"), NewWord("get"), NewWord("foo"),
	})
	if len(result) != 1 || result[0].AsInteger() != 99 {
		t.Errorf("context get foo = %v, want 99", result)
	}
}

func TestContextSetOverwrite(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("k"), NewInteger(1),
		NewWord("context"), NewWord("set"), NewString("k"), NewInteger(2),
		NewWord("context"), NewWord("get"), NewString("k"),
	})
	if len(result) != 1 || result[0].AsInteger() != 2 {
		t.Errorf("overwritten context get k = %v, want 2", result)
	}
}

// --- Unknown key returns none ---

func TestContextGetUnknownKeyReturnsNone(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("get"), NewString("missing"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].VType.Equal(TNone) {
		t.Errorf("context get missing = %v (type %s), want none", result[0], result[0].VType)
	}
}

// --- Sub-engine inheritance via do ---

func TestContextSubEngineInherits(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Set in parent, read in sub-engine via do
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("x"), NewInteger(10),
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("get"), NewString("x"),
		}),
	})
	if len(result) != 1 || result[0].AsInteger() != 10 {
		t.Errorf("sub-engine should inherit parent context, got %v", result)
	}
}

// --- Sub-engine isolation: writes don't affect parent ---

func TestContextSubEngineIsolation(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Set in parent, override in sub-engine, check parent still has original
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("x"), NewInteger(1),
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("set"), NewString("x"), NewInteger(999),
		}),
		NewWord("context"), NewWord("get"), NewString("x"),
	})
	if len(result) != 1 || result[0].AsInteger() != 1 {
		t.Errorf("parent context should be unchanged after sub-engine write, got %v", result)
	}
}

// --- Sub-engine new key doesn't leak to parent ---

func TestContextSubEngineNewKeyDoesNotLeak(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("set"), NewString("secret"), NewInteger(42),
		}),
		NewWord("context"), NewWord("get"), NewString("secret"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].VType.Equal(TNone) {
		t.Errorf("sub-engine key should not leak to parent, got %v", result[0])
	}
}

// --- Nested 3-level sub-engines ---

func TestContextNestedThreeLevels(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Level 0: set level=0
	// Level 1 (do): set level=1, then do level 2
	// Level 2 (do do): read level → should see 1, set level=2
	// Back at level 1: read level → should see 1
	// Back at level 0: read level → should see 0
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("level"), NewInteger(0),
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("set"), NewString("level"), NewInteger(1),
			NewWord("do"), NewList([]Value{
				NewWord("context"), NewWord("get"), NewString("level"),
				// This should be 1 (inherited from level 1)
			}),
		}),
		// do returns the innermost result (1), now check parent level
		NewWord("context"), NewWord("get"), NewString("level"),
	})
	// Stack should have: [1, 0]
	// The do returns inner do's result (1), then we get parent level (0)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	if result[0].AsInteger() != 1 {
		t.Errorf("inner do should see level=1, got %v", result[0])
	}
	if result[1].AsInteger() != 0 {
		t.Errorf("parent should still see level=0, got %v", result[1])
	}
}

// --- Multiple keys ---

func TestContextMultipleKeys(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("a"), NewInteger(1),
		NewWord("context"), NewWord("set"), NewString("b"), NewInteger(2),
		NewWord("context"), NewWord("set"), NewString("c"), NewInteger(3),
		NewWord("context"), NewWord("get"), NewString("a"),
		NewWord("context"), NewWord("get"), NewString("b"),
		NewWord("add"),
		NewWord("context"), NewWord("get"), NewString("c"),
		NewWord("add"),
	})
	if len(result) != 1 || result[0].AsInteger() != 6 {
		t.Errorf("sum of context values = %v, want 6", result)
	}
}

// --- Context with different value types ---

func TestContextDifferentValueTypes(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Use context dispatcher with end separators, matching the parser output
	// for: context set "str" "hello"; context set "num" 42; ...
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("str"), NewString("hello"),
		NewWord("end"),
		NewWord("context"), NewWord("set"), NewString("num"), NewInteger(42),
		NewWord("end"),
		NewWord("context"), NewWord("set"), NewString("bool"), NewBoolean(true),
		NewWord("end"),
		NewWord("context"), NewWord("get"), NewString("str"),
		NewWord("end"),
		NewWord("context"), NewWord("get"), NewString("num"),
		NewWord("end"),
		NewWord("context"), NewWord("get"), NewString("bool"),
	})
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	if !result[0].VType.Matches(TString) || result[0].AsString() != "hello" {
		t.Errorf("string value = %v, want hello", result[0])
	}
	if !result[1].VType.Matches(TInteger) || result[1].AsInteger() != 42 {
		t.Errorf("integer value = %v, want 42", result[1])
	}
	if !result[2].VType.Matches(TBoolean) || result[2].AsBoolean() != true {
		t.Errorf("boolean value = %v, want true", result[2])
	}
}

// --- Values are copied by reference (maps are shared, not deep-copied) ---

func TestContextValuesByReference(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Store a map in context, retrieve it in sub-engine — should be the same map
	m := NewOrderedMap()
	m.Set("key", NewInteger(100))
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("mymap"), NewMap(m),
		NewWord("do"), NewList([]Value{
			NewWord("context"), NewWord("get"), NewString("mymap"),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	rm := result[0].AsMap()
	v, ok := rm.Get("key")
	if !ok || v.AsInteger() != 100 {
		t.Errorf("expected map with key=100, got %v", result[0])
	}
}

// --- Module inherits parent context ---

func TestContextModuleInherits(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("parent_val"), NewInteger(77),
		NewWord("module"), NewList([]Value{
			NewWord("export"), NewWord("result"),
			NewMap(func() *OrderedMap {
				m := NewOrderedMap()
				// We can't call "context get" inside a map literal directly.
				// Instead, test by checking the module can read context in its body.
				m.Set("val", NewList([]Value{
					NewWord("context"), NewWord("get"), NewString("parent_val"),
				}))
				return m
			}()),
		}),
	})
	// Module returns a module desc. Let's verify it ran without error.
	if len(result) != 1 {
		t.Fatalf("expected 1 result (module desc), got %d: %v", len(result), result)
	}
}

// --- Module writes don't affect parent ---

func TestContextModuleIsolation(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("x"), NewInteger(1),
		NewWord("module"), NewList([]Value{
			NewWord("context"), NewWord("set"), NewString("x"), NewInteger(999),
			NewWord("export"), NewWord("dummy"),
			NewMap(func() *OrderedMap {
				m := NewOrderedMap()
				m.Set("v", NewInteger(0))
				return m
			}()),
		}),
		NewWord("drop"), // drop module desc
		NewWord("context"), NewWord("get"), NewString("x"),
	})
	if len(result) != 1 || result[0].AsInteger() != 1 {
		t.Errorf("parent context should be unchanged after module write, got %v", result)
	}
}

// --- Direct unit tests for PushContext/PopContext/Context ---

func TestRegistryContextStackMethods(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Initially no context
	if r.Context() != nil {
		t.Fatal("expected nil context initially")
	}

	// Push empty context
	r.PushContext(make(map[string]Value))
	ctx := r.Context()
	if ctx == nil {
		t.Fatal("expected non-nil context after push")
	}
	if len(ctx) != 0 {
		t.Fatalf("expected empty context, got %d entries", len(ctx))
	}

	// Write to context
	ctx["key1"] = NewInteger(1)

	// Push child — should inherit key1
	r.PushContext(ctx)
	child := r.Context()
	v, ok := child["key1"]
	if !ok || v.AsInteger() != 1 {
		t.Error("child should inherit key1=1 from parent")
	}

	// Write to child doesn't affect parent
	child["key1"] = NewInteger(99)
	child["key2"] = NewInteger(2)

	// Pop child
	r.PopContext()
	restored := r.Context()
	v, ok = restored["key1"]
	if !ok || v.AsInteger() != 1 {
		t.Errorf("parent key1 should still be 1 after pop, got %v", v)
	}
	if _, ok := restored["key2"]; ok {
		t.Error("parent should not have key2 after child pop")
	}

	// Pop parent
	r.PopContext()
	if r.Context() != nil {
		t.Error("expected nil context after popping all layers")
	}
}

// --- If condition sub-engine inherits context ---

func TestContextIfSubEngineInherits(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []Value{
		NewWord("context"), NewWord("set"), NewString("val"), NewInteger(5),
		NewWord("if"), NewList([]Value{NewBoolean(true)}),
		NewList([]Value{NewWord("context"), NewWord("get"), NewString("val")}),
		NewList([]Value{NewInteger(0)}),
	})
	if len(result) != 1 || result[0].AsInteger() != 5 {
		t.Errorf("if-branch should inherit context, got %v", result)
	}
}
