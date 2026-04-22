package test

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// TestMapFunctionAccess verifies that functions stored in plain maps
// (not modules) can be accessed and invoked via get, just like module
// functions.
func TestMapFunctionAccess(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)

	setup := `
		def greet fn [[s:String] [String] [s add "!"]]
		def m {greet: greet}
	`
	vals, _ := parser.Parse(setup)
	eng := engine.NewTop(r)
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
			eng := engine.NewTop(r)
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
