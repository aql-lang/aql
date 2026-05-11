package test

import (
	"github.com/aql-lang/aql/lang/native"
	"testing"

	"github.com/aql-lang/aql/eng/parser"
	"github.com/aql-lang/aql/lang/engine"
)

func runAQLText(t *testing.T, r *engine.Registry, src string) ([]engine.Value, error) {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	eng := engine.NewTop(r)
	return eng.Run(values)
}

func TestDotVerify(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)

	setup := `def p {x:{y:1}}
def m 'y'`
	values, err := parser.Parse(setup)
	if err != nil {
		t.Fatalf("parse setup: %v", err)
	}
	eng := engine.NewTop(r)
	if _, err := eng.Run(values); err != nil {
		t.Fatalf("run setup: %v", err)
	}

	tests := []struct {
		expr    string
		want    string
		wantErr bool
	}{
		{"p.x.y", "1", false},
		{"p.x.(m)", "1", false},
		{"p.x.m", "None", false},
		{"p!.x!.y", "1", false},
		{"p!.x!.(m)", "1", false},
		{"p!.x!.m", "", true}, // should error: key not found
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			result, err := runAQLText(t, r, tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Errorf("%s: expected error, got %v", tt.expr, result)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tt.expr, err)
			}
			if len(result) != 1 {
				t.Fatalf("%s: expected 1 result, got %d: %v", tt.expr, len(result), result)
			}
			got := result[0].String()
			if got != tt.want {
				t.Errorf("%s = %s, want %s", tt.expr, got, tt.want)
			}
		})
	}
}
