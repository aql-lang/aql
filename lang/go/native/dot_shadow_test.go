package native

import (
	"testing"
)

// TestDotNotationRegisteredWordKey verifies that dot notation can access
// map keys that are also registered word names. This is the fix for
// BORU-DX-REPORT Issue 4: registered words shadow map keys in dot notation.
func TestDotNotationRegisteredWordKey(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// Build a map with keys that shadow registered words.
	m := NewOrderedMap()
	m.Set("trace", NewInteger(42))
	m.Set("size", NewInteger(99))
	m.Set("add", NewString("plus"))

	tests := []struct {
		key  string
		want string
	}{
		{"trace", "42"},
		{"size", "99"},
		{"add", "'plus'"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			// Simulate dot notation: map dot key
			// The key is a Word that names a registered function.
			result := runBORU(t, r, []Value{
				NewMap(m), NewWord("dot"), NewWord(tt.key),
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
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	// Simulate a module map with an export named "trace".
	moduleMap := NewOrderedMap()
	moduleMap.Set("trace", NewString("my-trace-fn"))

	// def matrix {trace:"my-trace-fn"}
	// MatrixUtil.trace → "my-trace-fn" (not the debug trace word)
	runBORU(t, r, []Value{
		NewWord("def"), NewWord("Matrix"), NewMap(moduleMap), NewEnd(),
	})

	// matrix get trace — should do map lookup, not execute trace word
	result := runBORU(t, r, []Value{
		NewWord("Matrix"), NewWord("dot"), NewWord("trace"),
	})
	_as0, _ := AsString(result[0])
	if len(result) != 1 || _as0 != "my-trace-fn" {
		t.Errorf("matrix dot trace = %v, want 'my-trace-fn'", result)
	}
}

// TestDotNotationNormalKeysStillWork verifies that normal (non-shadowing)
// dot notation keys continue to work correctly.
func TestDotNotationNormalKeysStillWork(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)

	m := NewOrderedMap()
	m.Set("name", NewString("alice"))
	m.Set("age", NewInteger(30))

	result := runBORU(t, r, []Value{
		NewMap(m), NewWord("dot"), NewWord("name"),
	})
	_as1, _ := AsString(result[0])
	if len(result) != 1 || _as1 != "alice" {
		t.Errorf("dot name = %v, want 'alice'", result)
	}

	result = runBORU(t, r, []Value{
		NewMap(m), NewWord("dot"), NewWord("age"),
	})
	_as2, _ := AsNumber(result[0])
	if len(result) != 1 || _as2 != 30 {
		t.Errorf("dot age = %v, want 30", result)
	}
}
