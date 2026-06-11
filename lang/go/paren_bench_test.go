package lang

import "testing"

// Paren-representation benchmark (PAREN-REPRESENTATION.9.md Step 7): measure
// the per-paren cost of the nested-ParenExpr representation vs. the former
// inline-marker form. Uses only the public New()/Run() API so the same file
// runs unchanged on the pre-change baseline commit for an A/B comparison.

var parenBenchSrcs = map[string]string{
	"flat3":    `(1 add 2) (3 add 4) (5 add 6)`,
	"nested3":  `(((1 add 2) mul 3) sub 4)`,
	"deep6":    `(1 add (2 add (3 add (4 add (5 add 6)))))`,
	"forward":  `add (1 add 2) (3 add 4)`,
	"ifparen":  `if (1 lt 2) [(10 add 20)] [(30 add 40)]`,
	"loop":     `for 50 [(1 add 2)]`,
	"dotchain": `m.a.b.c.d`,
}

func benchRun(b *testing.B, setup, src string) {
	b.Helper()
	a, err := New()
	if err != nil {
		b.Fatal(err)
	}
	if setup != "" {
		if _, err := a.Run(setup); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := a.Run(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParens(b *testing.B) {
	for name, src := range parenBenchSrcs {
		setup := ""
		if name == "dotchain" {
			setup = `def m {a: {b: {c: {d: 1}}}}`
		}
		s := src
		su := setup
		b.Run(name, func(b *testing.B) { benchRun(b, su, s) })
	}
}
