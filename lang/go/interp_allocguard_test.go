package lang

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// Interpreter-mode allocation guard — the interpreter twin of
// TestCompiledAllocCeilings. Allocations per interpreted execution of a
// pre-parsed value stream are deterministic, so they are a hard
// regression signal for the dispatch machinery (matchSignature, forward
// collection, per-call stacks) that stays on the interpreted path.
// Each shape has a ceiling pinned slightly above its measured
// allocations; a change that adds a per-dispatch allocation trips the
// ceiling here, in `make test`, long before anyone runs the benchmarks.
// Lower a ceiling when an optimization reduces allocations; never raise
// one without a documented reason.
func TestInterpAllocCeilings(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc guard runs a micro-benchmark; skipped under -short")
	}
	// byteCeiling catches the regression class the alloc COUNT misses: one
	// big allocation per op. The pre-engine-pool interpreter allocated a
	// fresh ~1024-entry tape (~164KB) per body invocation — each_list ran
	// at ~17MB/op with barely more allocs than today. A change that
	// removes sub-engine pooling trips the byte ceiling immediately.
	guards := []struct {
		name        string
		setup       string
		src         string
		ceiling     int64
		byteCeiling int64
	}{
		{"arith_chain64", "", arithChain(64), 1770, 600_000},
		{"compare_loop", "", `for 200 [gt i 100]`, 7900, 1_850_000},
		{"if_scalar", "", `for 200 [if (gt i 100) [1] [0]]`, 15300, 3_700_000},
		{"for_tight", "", `for 200 [add (mul i 3) 7]`, 14700, 2_950_000},
		{"each_list", `def xs100 ` + intList(100), `each [mul 2] xs100`, 2740, 850_000},
		{"fold_int", `def xs100b ` + intList(100), `fold [add] xs100b 0`, 2270, 620_000},
		{"map_get", `def mg {a: {b: {c: {d: 1}}}}`, `for 100 [mg.a.b.c.d]`, 26400, 3_300_000},
		{"string_join", `def ws ['alpha' 'beta' 'gamma' 'delta']`, `for 50 [join '-' ws]`, 19000, 2_050_000},
	}
	for _, g := range guards {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if g.setup != "" {
			if _, err := a.Run(g.setup); err != nil {
				t.Fatalf("%s setup: %v", g.name, err)
			}
		}
		vals, err := parser.Parse(g.src)
		if err != nil {
			t.Fatalf("%s parse: %v", g.name, err)
		}
		reg := a.registry
		got, gotBytes := allocStatsPerOp(func() {
			if _, err := native.NewTop(reg).Run(vals); err != nil {
				t.Fatalf("%s: %v", g.name, err)
			}
		})
		if got > g.ceiling {
			t.Errorf("%s: %d allocs/op exceeds ceiling %d — interpreter allocation regressed", g.name, got, g.ceiling)
		} else if gotBytes > g.byteCeiling {
			t.Errorf("%s: %d bytes/op exceeds ceiling %d — interpreter allocation regressed", g.name, gotBytes, g.byteCeiling)
		} else {
			t.Logf("%s: %d allocs/op (ceiling %d), %d bytes/op (ceiling %d)", g.name, got, g.ceiling, gotBytes, g.byteCeiling)
		}
	}
}
