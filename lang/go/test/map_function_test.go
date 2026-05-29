package test

import (
	"github.com/aql-lang/aql/lang/go/native"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
)

// TestMapFunctionAccess verifies that a function stored in a plain map
// can be invoked, both via the dotted accessor and via bare `get`.
//
// To call it via dot (`m.greet arg`), store the function with the `/r`
// ref modifier — `{greet: greet/r}` — so the map holds a Quoted (data)
// Function value. `m.greet arg` groups to `(m get greet) arg`, the value
// stays as data, and the arg calls it.
//
// Stored *bare* (`{greet: greet}`) the value is a dispatchable reference
// to the globally-registered `greet`, so the dotted form self-invokes it
// 0-arg inside the `( … )` group before the arg arrives — for that form
// use bare `m get greet arg`. (Module functions `pkg.fn arg` are
// unaffected; their names are module-scoped, not global.)
func TestMapFunctionAccess(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)

	setup := `
		def greet fn [[s:String] [String] [s add "!"]]
		def m {greet: greet/r}
	`
	vals, _ := parser.Parse(setup)
	eng := native.NewTop(r)
	if _, err := eng.Run(vals); err != nil {
		t.Fatalf("setup: %v", err)
	}

	tests := []struct {
		expr string
		want string
	}{
		// Dotted access works because greet is stored with /r (data value).
		{`"hello" m.greet`, "'hello!'"}, // stack form
		{`m.greet "hello"`, "'hello!'"}, // forward form
		// Bare get works regardless of how the function is stored.
		{`m get greet "hello"`, "'hello!'"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			vals, _ := parser.Parse(tt.expr)
			eng := native.NewTop(r)
			result, err := eng.Run(vals)
			if err != nil {
				t.Fatalf("%s: %v", tt.expr, err)
			}
			if len(result) != 1 || result[0].String() != tt.want {
				t.Errorf("%s = %v, want %s", tt.expr, result, tt.want)
			}
		})
	}
}
