package engine_test

import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"github.com/metsitaba/voxgig-exp/lang/internal/native"
	"testing"
)

// TestDotNotationRegisteredWordKey verifies that dot notation can access
// map keys that are also registered word names. This is the fix for
// AQL-DX-REPORT Issue 4: registered words shadow map keys in dot notation.
func TestDotNotationRegisteredWordKey(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Build a map with keys that shadow registered words.
	m := engine.NewOrderedMap()
	m.Set("trace", engine.NewInteger(42))
	m.Set("length", engine.NewInteger(99))
	m.Set("add", engine.NewString("plus"))

	tests := []struct {
		key  string
		want string
	}{
		{"trace", "42"},
		{"length", "99"},
		{"add", "'plus'"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			// Simulate dot notation: map get key
			// The key is a Word that names a registered function.
			result := runAQL(t, r, []engine.Value{
				engine.NewMap(m), engine.NewWord("get"), engine.NewWord(tt.key),
			})
			if len(result) != 1 || result[0].String() != tt.want {
				t.Errorf("{...} get %s = %v, want %s", tt.key, result, tt.want)
			}
		})
	}
}

// TestDotNotationModuleExportShadow verifies that module exports with
// names that shadow registered words can be accessed via dot notation.
func TestDotNotationModuleExportShadow(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a module map with an export named "trace".
	moduleMap := engine.NewOrderedMap()
	moduleMap.Set("trace", engine.NewString("my-trace-fn"))

	// def matrix {trace:"my-trace-fn"}
	// matrix.trace → "my-trace-fn" (not the debug trace word)
	runAQL(t, r, []engine.Value{
		engine.NewWord("def"), engine.NewWord("matrix"), engine.NewMap(moduleMap), engine.NewWord("end"),
	})

	// matrix get trace — should do map lookup, not execute trace word
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("matrix"), engine.NewWord("get"), engine.NewWord("trace"),
	})
	_as0, _ := result[0].AsString()
	if len(result) != 1 || _as0 != "my-trace-fn" {
		t.Errorf("matrix get trace = %v, want 'my-trace-fn'", result)
	}
}

// TestDotNotationNormalKeysStillWork verifies that normal (non-shadowing)
// dot notation keys continue to work correctly.
func TestDotNotationNormalKeysStillWork(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	m := engine.NewOrderedMap()
	m.Set("name", engine.NewString("alice"))
	m.Set("age", engine.NewInteger(30))

	result := runAQL(t, r, []engine.Value{
		engine.NewMap(m), engine.NewWord("get"), engine.NewWord("name"),
	})
	_as1, _ := result[0].AsString()
	if len(result) != 1 || _as1 != "alice" {
		t.Errorf("get name = %v, want 'alice'", result)
	}

	result = runAQL(t, r, []engine.Value{
		engine.NewMap(m), engine.NewWord("get"), engine.NewWord("age"),
	})
	_as2, _ := result[0].AsNumber()
	if len(result) != 1 || _as2 != 30 {
		t.Errorf("get age = %v, want 30", result)
	}
}
