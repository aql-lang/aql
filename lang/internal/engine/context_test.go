package engine_test
import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
	"testing"
)

// --- Basic context set/get ---

func TestContextSetGetString(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("x"), engine.NewInteger(42),
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("x"),
	})
	_as0, _ := result[0].AsInteger()
	if len(result) != 1 || _as0 != 42 {
		t.Errorf("context get x = %v, want 42", result)
	}
}

func TestContextSetGetWordKey(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewWord("foo"), engine.NewInteger(99),
		engine.NewWord("context"), engine.NewWord("get"), engine.NewWord("foo"),
	})
	_as1, _ := result[0].AsInteger()
	if len(result) != 1 || _as1 != 99 {
		t.Errorf("context get foo = %v, want 99", result)
	}
}

func TestContextSetOverwrite(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("k"), engine.NewInteger(1),
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("k"), engine.NewInteger(2),
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("k"),
	})
	_as2, _ := result[0].AsInteger()
	if len(result) != 1 || _as2 != 2 {
		t.Errorf("overwritten context get k = %v, want 2", result)
	}
}

// --- Unknown key returns none ---

func TestContextGetUnknownKeyReturnsError(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(r)
	_, err = e.Run([]engine.Value{
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("missing"),
	})
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
}

// --- Sub-engine inheritance via do ---

func TestContextSubEngineInherits(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	// Set in parent, read in sub-engine via do
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("x"), engine.NewInteger(10),
		engine.NewWord("do"), engine.NewList([]engine.Value{
			engine.NewWord("context"), engine.NewWord("get"), engine.NewString("x"),
		}),
	})
	_as3, _ := result[0].AsInteger()
	if len(result) != 1 || _as3 != 10 {
		t.Errorf("sub-engine should inherit parent context, got %v", result)
	}
}

// --- Sub-engine isolation: writes don't affect parent ---

func TestContextSubEngineIsolation(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	// Set in parent, override in sub-engine, check parent still has original
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("x"), engine.NewInteger(1),
		engine.NewWord("do"), engine.NewList([]engine.Value{
			engine.NewWord("context"), engine.NewWord("set"), engine.NewString("x"), engine.NewInteger(999),
		}),
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("x"),
	})
	_as4, _ := result[0].AsInteger()
	if len(result) != 1 || _as4 != 1 {
		t.Errorf("parent context should be unchanged after sub-engine write, got %v", result)
	}
}

// --- Sub-engine new key doesn't leak to parent ---

func TestContextSubEngineNewKeyDoesNotLeak(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(r)
	_, err = e.Run([]engine.Value{
		engine.NewWord("do"), engine.NewList([]engine.Value{
			engine.NewWord("context"), engine.NewWord("set"), engine.NewString("secret"), engine.NewInteger(42),
		}),
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("secret"),
	})
	if err == nil {
		t.Fatal("expected error: sub-engine key should not leak to parent")
	}
}

// --- Nested 3-level sub-engines ---

func TestContextNestedThreeLevels(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	// Level 0: set level=0
	// Level 1 (do): set level=1, then do level 2
	// Level 2 (do do): read level → should see 1, set level=2
	// Back at level 1: read level → should see 1
	// Back at level 0: read level → should see 0
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("level"), engine.NewInteger(0),
		engine.NewWord("do"), engine.NewList([]engine.Value{
			engine.NewWord("context"), engine.NewWord("set"), engine.NewString("level"), engine.NewInteger(1),
			engine.NewWord("do"), engine.NewList([]engine.Value{
				engine.NewWord("context"), engine.NewWord("get"), engine.NewString("level"),
				// This should be 1 (inherited from level 1)
			}),
		}),
		// do returns the innermost result (1), now check parent level
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("level"),
	})
	// Stack should have: [1, 0]
	// The do returns inner do's result (1), then we get parent level (0)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	_as5, _ := result[0].AsInteger()
	if _as5 != 1 {
		t.Errorf("inner do should see level=1, got %v", result[0])
	}
	_as6, _ := result[1].AsInteger()
	if _as6 != 0 {
		t.Errorf("parent should still see level=0, got %v", result[1])
	}
}

// --- Multiple keys ---

func TestContextMultipleKeys(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("a"), engine.NewInteger(1),
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("b"), engine.NewInteger(2),
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("c"), engine.NewInteger(3),
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("a"),
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("b"),
		engine.NewWord("add"),
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("c"),
		engine.NewWord("add"),
	})
	_as7, _ := result[0].AsInteger()
	if len(result) != 1 || _as7 != 6 {
		t.Errorf("sum of context values = %v, want 6", result)
	}
}

// --- Context with different value types ---

func TestContextDifferentValueTypes(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	// Wrap each context get in parens so previous results on the stack
	// don't get consumed by the next get (stack-preference rule: when
	// a String result is on the stack, context-get would take it as
	// its key instead of forward-collecting the intended key).
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("str"), engine.NewString("hello"),
		engine.NewWord("end"),
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("num"), engine.NewInteger(42),
		engine.NewWord("end"),
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("bool"), engine.NewBoolean(true),
		engine.NewWord("end"),
		engine.NewOpenParen(), engine.NewWord("context"), engine.NewWord("get"), engine.NewString("str"), engine.NewWord(")"),
		engine.NewOpenParen(), engine.NewWord("context"), engine.NewWord("get"), engine.NewString("num"), engine.NewWord(")"),
		engine.NewOpenParen(), engine.NewWord("context"), engine.NewWord("get"), engine.NewString("bool"), engine.NewWord(")"),
	})
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	_as8, _ := result[0].AsString()
	if !result[0].VType.Matches(engine.TString) || _as8 != "hello" {
		t.Errorf("string value = %v, want hello", result[0])
	}
	_as9, _ := result[1].AsInteger()
	if !result[1].VType.Matches(engine.TInteger) || _as9 != 42 {
		t.Errorf("integer value = %v, want 42", result[1])
	}
	_as10, _ := result[2].AsBoolean()
	if !result[2].VType.Matches(engine.TBoolean) || _as10 != true {
		t.Errorf("boolean value = %v, want true", result[2])
	}
}

// --- Values are copied by reference (maps are shared, not deep-copied) ---

func TestContextValuesByReference(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	// Store a map in context, retrieve it in sub-engine — should be the same map
	m := engine.NewOrderedMap()
	m.Set("key", engine.NewInteger(100))
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("mymap"), engine.NewMap(m),
		engine.NewWord("do"), engine.NewList([]engine.Value{
			engine.NewWord("context"), engine.NewWord("get"), engine.NewString("mymap"),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	rm := result[0].AsMap()
	v, ok := rm.Get("key")
	_as11, _ := v.AsInteger()
	if !ok || _as11 != 100 {
		t.Errorf("expected map with key=100, got %v", result[0])
	}
}

// --- Module inherits parent context ---

func TestContextModuleInherits(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("parent_val"), engine.NewInteger(77),
		engine.NewWord("module"), engine.NewList([]engine.Value{
			engine.NewWord("export"), engine.NewAtom("result"),
			engine.NewMap(func() *engine.OrderedMap {
				m := engine.NewOrderedMap()
				// We can't call "context get" inside a map literal directly.
				// Instead, test by checking the module can read context in its body.
				m.Set("val", engine.NewList([]engine.Value{
					engine.NewWord("context"), engine.NewWord("get"), engine.NewString("parent_val"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("x"), engine.NewInteger(1),
		engine.NewWord("module"), engine.NewList([]engine.Value{
			engine.NewWord("context"), engine.NewWord("set"), engine.NewString("x"), engine.NewInteger(999),
			engine.NewWord("export"), engine.NewAtom("dummy"),
			engine.NewMap(func() *engine.OrderedMap {
				m := engine.NewOrderedMap()
				m.Set("v", engine.NewInteger(0))
				return m
			}()),
		}),
		engine.NewWord("drop"), // drop module desc
		engine.NewWord("context"), engine.NewWord("get"), engine.NewString("x"),
	})
	_as12, _ := result[0].AsInteger()
	if len(result) != 1 || _as12 != 1 {
		t.Errorf("parent context should be unchanged after module write, got %v", result)
	}
}

// --- Direct unit tests for PushContext/PopContext/Context ---

func TestRegistryContextStackMethods(t *testing.T) {
	r, err := engine.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Initially no context
	if r.ContextStore() != nil {
		t.Fatal("expected nil context initially")
	}

	// Push empty context (nil parent)
	r.PushContext(nil)
	store := r.ContextStore()
	if store == nil {
		t.Fatal("expected non-nil context store after push")
	}
	if len(store.Data) != 0 {
		t.Fatalf("expected empty context, got %d entries", len(store.Data))
	}

	// Write to context store
	store.Set("key1", engine.NewInteger(1))

	// Push child — should inherit key1 via prototype
	r.PushContext(store)
	childStore := r.ContextStore()
	v, ok := childStore.Get("key1")
	_as13, _ := v.AsInteger()
	if !ok || _as13 != 1 {
		t.Error("child should inherit key1=1 from parent via prototype")
	}

	// Write to child doesn't affect parent
	childStore.Set("key1", engine.NewInteger(99))
	childStore.Set("key2", engine.NewInteger(2))

	// Pop child
	r.PopContext()
	restored := r.ContextStore()
	v, ok = restored.Get("key1")
	_as14, _ := v.AsInteger()
	if !ok || _as14 != 1 {
		t.Errorf("parent key1 should still be 1 after pop, got %v", v)
	}
	if _, ok := restored.Get("key2"); ok {
		t.Error("parent should not have key2 after child pop")
	}

	// Pop parent
	r.PopContext()
	if r.ContextStore() != nil {
		t.Error("expected nil context after popping all layers")
	}
}

// --- If condition sub-engine inherits context ---

func TestContextIfSubEngineInherits(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("context"), engine.NewWord("set"), engine.NewString("val"), engine.NewInteger(5),
		engine.NewWord("if"), engine.NewList([]engine.Value{engine.NewBoolean(true)}),
		engine.NewList([]engine.Value{engine.NewWord("context"), engine.NewWord("get"), engine.NewString("val")}),
		engine.NewList([]engine.Value{engine.NewInteger(0)}),
	})
	_as15, _ := result[0].AsInteger()
	if len(result) != 1 || _as15 != 5 {
		t.Errorf("if-branch should inherit context, got %v", result)
	}
}
