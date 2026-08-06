package eng

import "testing"

// bodyFreeForFallback must screen out the context-dependent words
// (args / __pa): an island's sub-engine inside a compiled fn sees an
// EMPTY interpreter args stack where the interpreter sees the enclosing
// call's list. Both spellings decline; an ordinary registered-word body
// stays free (the positive pair).
func TestBodyFreeForFallbackContextWords(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for _, name := range []string{"args", "__pa"} {
		body := NewList([]Value{NewWord(name)})
		if BodyFreeForFallback(r, body) {
			t.Errorf("body [%s]: want island-decline (context-dependent word)", name)
		}
	}
	// Positive pair: a body of known literals stays island-free.
	free := NewList([]Value{NewWord("true")})
	if !BodyFreeForFallback(r, free) {
		t.Errorf("body [true]: want island-free")
	}
}
