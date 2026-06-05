package test

import (
	"strings"
	"testing"
)

// TestReservedCoreWordsCannotBeRedefined pins WAT-audit Exhibit M: a
// built-in / kernel word, and the reserved literals true/false/none,
// may not be redefined or undefined. The language is extended by
// defining NEW words, never by shadowing a core one.
func TestReservedCoreWordsCannotBeRedefined(t *testing.T) {
	reserved := []string{
		`def add fn [[x:Number y:Number] [Number] [x sub y]] 5 3 add`, // native word
		`def if 7`,     // control word
		`def each [1]`, // higher-order word
		`def dup 5`,    // stack word
		`def true 99`,  // reserved literal
		`def false 0`,  // reserved literal
		`undef add`,    // can't undef a native
		`undef true`,   // can't undef a literal
	}
	for _, src := range reserved {
		_, err := runNativeSteps(t, nil, []string{src})
		if err == nil {
			t.Errorf("%q: expected [aql/reserved_word], got no error", src)
			continue
		}
		if !strings.Contains(err.Error(), "reserved_word") {
			t.Errorf("%q: expected reserved_word error, got %v", src, err)
		}
	}

	// `none` is the None value literal, so it is rejected one layer
	// earlier (it is not a nameable token) — still illegal to redefine,
	// just via a signature error rather than the reserved-word guard.
	if _, err := runNativeSteps(t, nil, []string{`def none 1`}); err == nil {
		t.Errorf("def none 1: expected an error, got none")
	}
}

// TestUserWordsRemainRedefinable confirms the guard is scoped to core
// words: user words define, shadow (re-def), and undef freely.
func TestUserWordsRemainRedefinable(t *testing.T) {
	cases := []struct{ src, want string }{
		{`def myadd fn [[x:Number y:Number] [Number] [x add y]] 5 3 myadd`, "8"},
		{`def foo 1 def foo 2 foo`, "2"},           // re-def shadows
		{`def bar 5 def bar 6 undef bar bar`, "5"}, // undef pops the shadow
	}
	for _, c := range cases {
		out, err := runNativeSteps(t, nil, []string{c.src})
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.src, err)
			continue
		}
		if len(out) != 1 || out[0].String() != c.want {
			t.Errorf("%q = %v, want %s", c.src, out, c.want)
		}
	}
}
