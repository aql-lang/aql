package test

import (
	"github.com/aql-lang/aql/lang/go/native"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
)

// TestMapFunctionAccess verifies that functions stored in plain maps
// (not modules) can be accessed and invoked via get, just like module
// functions.
func TestMapFunctionAccess(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)

	setup := `
		def greet fn [[s:String] [String] [s add "!"]]
		def m {greet: greet}
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
		// Stack form: arg before function
		{`"hello" m.greet`, "'hello!'"},
		// Forward form: function before arg
		{`m.greet "hello"`, "'hello!'"},
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
