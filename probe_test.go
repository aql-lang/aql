package aqleng

import (
	"fmt"
	"testing"
)

func TestProbeParens(t *testing.T) {
	cases := []string{
		// Multi-wrap (no-op nesting)
		"( ( 1 addq 2 ) )",
		"( ( ( 1 addq 2 ) ) )",
		"( ( ( ( 1 addq 2 ) ) ) )",
		"( ( ( ( ( 1 addq 2 ) ) ) ) )",
		"( ( ( ( ( ( 1 addq 2 ) ) ) ) ) )",
		"( ( ( ( ( ( ( 1 addq 2 ) ) ) ) ) ) )",
		// Deep functional nesting
		"addq 1 ( addq 2 ( addq 3 ( addq 4 ( addq 5 ( addq 6 7 ) ) ) ) )",
		"addq ( addq ( addq ( addq ( addq ( addq 1 2 ) 3 ) 4 ) 5 ) 6 ) 7",
		"mulq ( mulq ( mulq ( mulq ( mulq ( mulq 2 2 ) 2 ) 2 ) 2 ) 2 ) 2",
		"subq ( subq ( subq ( subq 100 1 ) 1 ) 1 ) 1",
		// Multiple paren siblings
		"addq ( 1 addq 2 ) ( 3 addq 4 )",
		"mulq ( 2 addq 3 ) ( 4 addq 5 )",
		"addq ( 1 addq 2 ) addq ( 3 addq 4 )",  // ERROR? function word boundary
		// Parens-inside-parens at multiple positions
		"( ( 1 addq 2 ) addq ( 3 addq 4 ) )",
		"( ( 1 addq 2 ) mulq ( 3 addq 4 ) )",
		"addq ( ( 1 addq 2 ) mulq ( 3 subq 4 ) ) ( ( 5 mulq 6 ) addq ( 7 subq 8 ) )",
		// Mix of decimal/integer in deep nesting
		"addq 1 ( addq 2.0 ( addq 3 ( addq 4.5 5 ) ) )",
		// Single-value paren just unwraps
		"( 5 )",
		"( ( ( 5 ) ) )",
		// Multi-value paren
		"( 1 2 3 )",
		"addq ( 1 2 dup )",
		// Empty paren
		"( )",
		// Sibling parens in a chain
		"( 1 addq 2 ) ( 3 addq 4 )",
		"( 1 addq 2 ) ( 3 addq 4 ) addq",
		"( 1 addq 2 ) ( 3 addq 4 ) mulq",
		// Parens with negation
		"negq ( negq ( negq 5 ) )",
		"negq ( negq ( negq ( negq ( negq ( negq ( negq 5 ) ) ) ) ) )",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			vs, err := tokenizeSpec(in)
			if err != nil {
				fmt.Printf("%-65q TOKEN_ERR %v\n", in, err); return
			}
			r, _ := NewRegistry()
			registerSpecWords(r); r.InitRootContext()
			out, err := NewTop(r).Run(vs)
			if err != nil {
				fmt.Printf("%-65q ERR %v\n", in, err); return
			}
			fmt.Printf("%-65q -> %q\n", in, renderStack(out))
		})
	}
}
