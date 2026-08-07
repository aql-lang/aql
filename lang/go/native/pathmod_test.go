package native

import (
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/parser/go"
)

// TestPathModifiers pins the `/`-modifier-on-a-paren/path feature: the
// modifier applies to the whole group's result (engine DispatchMod marker),
// not to the dotted key.
func TestPathModifiers(t *testing.T) {
	run := func(src string) string {
		t.Helper()
		v, err := parser.Parse(src)
		if err != nil {
			return "PARSE:" + err.Error()
		}
		r, _ := DefaultRegistry()
		registerIOWords(r)
		o, e := NewTop(r).Run(v)
		if e != nil {
			return "ERR:" + e.Error()
		}
		return eng.Canon(o)
	}
	cases := []struct{ src, want string }{
		{`def m {a:add/r} end m.a/u 1 2`, "3"},   // usurp
		{`def m {s:sub/r} end m.s/u 10 3`, "7"},  // usurp reversed
		{`def m {s:sub/r} end m.s/f 10 3`, "-7"}, // force forward
		{`def m {s:sub/r} end 10 3 m.s/s`, "7"},  // force stack
		{`def m {a:add/r} end m.a/2 1 2`, "3"},   // argcount
		{`def m {a:add/r} end (m.a)/u 1 2`, "3"}, // paren usurp
		{`(1 add 2)/q`, "3"},                     // /q on non-fn: no-op
	}
	for _, c := range cases {
		if got := run(c.src); got != c.want {
			t.Errorf("%-34s => %s, want %s", c.src, got, c.want)
		}
	}

	// /r keeps the function as DATA: it is NOT invoked, so the trailing
	// args remain separate on the stack (function value + 1 + 2).
	if got := run(`def m {a:add/r} end m.a/r 1 2`); got[:2] != "fn" || got[len(got)-3:] != "1 2" {
		t.Errorf("m.a/r 1 2 should leave the fn as data with 1 2 separate, got %s", got)
	}
}

// TestForceFnComposition pins that the force-* companion words compose:
// each returns a function, so they wrap one another.
func TestForceFnComposition(t *testing.T) {
	run := func(src string) string {
		t.Helper()
		v, err := parser.Parse(src)
		if err != nil {
			return "PARSE:" + err.Error()
		}
		r, _ := DefaultRegistry()
		registerIOWords(r)
		o, e := NewTop(r).Run(v)
		if e != nil {
			return "ERR:" + e.Error()
		}
		return eng.Canon(o)
	}
	cases := []struct{ src, want string }{
		{`def m {s:sub/r} end usurp (forward-args (m.s)) 10 3`, "7"},
		{`def m {s:sub/r} end forward-args (usurp (m.s)) 10 3`, "7"},
		{`def m {s:sub/r} end force-arity 2 (usurp (m.s)) 10 3`, "7"},
		{`def m {s:sub/r} end usurp (force-arity 2 (m.s)) 10 3`, "7"},
		{`def m {s:sub/r} end 10 3 stack-args (usurp (m.s))`, "-7"},
		{`def m {s:sub/r} end 10 3 stack-args (force-arity 2 (m.s))`, "7"},
		{`def m {s:sub/r} end force-arity 2 (stack-args (m.s)) 10 3`, "-7"},
		{`def m {a:add/r} end force-arity 2 (usurp (forward-args (m.a))) 1 2`, "3"},
	}
	for _, c := range cases {
		if got := run(c.src); got != c.want {
			t.Errorf("%-54s => %s, want %s", c.src, got, c.want)
		}
	}
}
