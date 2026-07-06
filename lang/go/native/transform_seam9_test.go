package native

import "testing"

// TestW9ValueToAnyTypedMapNoPanic pins the ADR-005 fix for valueToAny's
// TMap arm: a value that conforms to TMap but has no concrete OrderedMap
// payload (a typed-map / record / options descriptor) makes AsMap return
// nil, and the arm must fall back to the rendered form rather than
// dereferencing nil. This path is reached in production via the
// struct-word conversions (e.g. minilang_query's docToAny) on such a
// value; before the guard it panicked on m.Len().
func TestW9ValueToAnyTypedMapNoPanic(t *testing.T) {
	tm := NewTypedMap(NewTypeLiteral(TNumber)) // TMap-conforming, ChildTypeInfo payload

	var got any
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("valueToAny panicked on a typed-map descriptor: %v", r)
			}
		}()
		got = valueToAny(tm)
	}()

	if _, ok := got.(string); !ok {
		t.Errorf("valueToAny(typed map) = %#v (%T), want a string fallback", got, got)
	}
}
