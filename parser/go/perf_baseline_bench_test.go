package parser

import (
	"strconv"
	"strings"
	"testing"
)

// Parser performance baseline (perf-baseline suite). One benchmark per
// representative source shape so a parse-cost regression (grammar rule
// churn, conversion-pass allocation) is visible per shape. Run with:
//
//	go test -bench 'BenchmarkParse' -benchmem -run '^$' .
//
// or `make bench` from the repo root.

func benchIntList(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = strconv.Itoa(i)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

var parseBenchSrcs = []struct {
	name string
	src  string
}{
	{"scalars", `1 2.5 'str' true x/q`},
	{"arith_chain", `1 add 2 mul 3 sub 4 add 5 mul 6 sub 7 add 8`},
	{"list100", benchIntList(100)},
	{"nested_list", `[1 [2 [3 [4 [5 [6]]]]]]`},
	{"map", `{a: 1, b: 'two', c: {d: [3 4 5], e: {f: true}}}`},
	{"parens", `(1 add (2 add (3 add (4 add 5))))`},
	{"fn_def", `def f fn [[a:Integer b:String] [Integer] [a add 1]]`},
	{"dotchain", `m.a.b.c.d`},
	{"interp_string", "`x=${1 add 2} y=${m.a}`"},
	{"lambda", `def double ([x:Integer] => [x mul 2])`},
}

func BenchmarkParse(b *testing.B) {
	for _, bb := range parseBenchSrcs {
		bb := bb
		b.Run(bb.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := Parse(bb.src); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
