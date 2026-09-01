// bind_multirun_parity_test.go is the CROSS-REQUEST INTERPRETER-PARITY
// ORACLE for the twin regime's binding installs — the lane the arm-resident
// increment's design review demanded BEFORE any placement work (Codex-class
// lesson from #421: mechanism labels lie until measured; and the
// full-placement gate is purely SYNTACTIC, so a wrong placement could
// graduate a row while silently installing wrong values — the exact class
// the regime exists to eliminate).
//
// No other lane measures this: the regime differential compares
// SAME-REQUEST results on fresh instances, so a binding that the regime
// loses, mis-values, mis-counts, or mis-orders is invisible there. This
// lane runs each shape on a fresh interpreter instance AND a fresh
// regime-compiled instance, then enumerates the full install stack of
// every probed name on BOTH instances with the read/undef alternation
// probe — the only working probe, because `undef` of a missing name is a
// silent no-op:
//
//	read the name     → undefined_word means zero installs remain: stop
//	record the render → this is the current TOP of the name's def stack
//	undef the name    → pop exactly one install
//
// The recorded sequence per name is the install stack top-down — count,
// values, and order in one measurement. Its first catch predates the lane
// landing: `def _ ([1 2] each [1]) 9` then `_` answered undefined_word
// under the regime (the RecordDynBind name gate skipped `_`'s Push-mode
// OpBindGlobal partner while ApplyBindTwin's carrier-class skip consumed
// the twin — a silent cross-request loss), measured against the
// interpreter's [[1 1]].
//
// Every row declares its classification, and graduation is an explicit
// edit of that field reviewed with the mechanism change:
//
//	parity  — compiles under the regime; driving-request results AND every
//	          probe sequence must match the interpreter exactly.
//	refused — must refuse with an error containing the given substring
//	          (the multi-run-body population, pinned until arm-resident
//	          twins land). The interpreter half must still run clean, so a
//	          row can never rot into pinning an ill-formed program.
//
// A shape can therefore never drift from refused to silently-wrong: the
// moment it compiles at all, its row fails until a reviewed edit says
// parity — and parity means measured equality, not a green placement scan.
package langspec

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// parityShape is one oracle row. probes are the names whose full install
// stacks are compared (or, for refused rows, merely enumerated on the
// interpreter to prove the shape is well-formed).
type parityShape struct {
	name    string
	src     string
	probes  []string
	refused string // "" = parity; else the required refusal substring
}

var parityShapes = []parityShape{
	// --- The compiling population: the probe machinery proven on shapes
	// the regime already handles, before it gates anything new.
	{name: "root-literal-def", src: "def q 5 q add 1", probes: []string{"q"}},
	{name: "root-computed-def", src: "def q ([1 2] each [1]) 9", probes: []string{"q"}},
	// The lane's founding catch: an underscore-named root computed def
	// needs its Push-mode partner under the regime (RecordDynBind's name
	// gate is relaxed for root defs under es.twinRegime — the fix landed
	// with this lane).
	{name: "root-underscore-computed-def", src: "def _ ([1 2] each [1]) 9", probes: []string{"_"}},
	// do-body adoption (#421): the adopted twin replays the captured entry
	// after the call; shadowing depth-orders with the root twin.
	{name: "do-adoption", src: "do [def x 5] x add 1", probes: []string{"x"}},
	{name: "do-adoption-shadow", src: "def x 1 do [def x 2] x", probes: []string{"x"}},
	{name: "do-adoption-quoted", src: "quote [def zz 5 zz add 1] do", probes: []string{"zz"}},

	// --- The multi-run-body population (the each-body class,
	// arm-residency's target). GRADUATED rows carry parity — the
	// arm-resident bridge places a per-element runtime-value install
	// (OpBindResident) at each def site inside the compiled unit, so
	// count, values, order, and zero-iteration definedness are measured
	// interpreter-equal. Still-refused rows pin the population the bridge
	// declines: the var-param Pos-0:0 def/undef pair (until the undef
	// seam lands), nested multi-run bodies (the latch's bodyID fence),
	// and any root read of an arm-bound name (body-run-dependent
	// definedness — its own refusal reason, not the placement gate's).
	{name: "each-literal-def", src: "[1 2 3] each [def x 5]",
		probes: []string{"x"}},
	{name: "each-elem-valued-def", src: "[10 20] each [ var [[r] def x r x] ]",
		probes: []string{"x", "r"}},
	{name: "each-zero-iterations", src: "[] each [def x 5]",
		probes: []string{"x"}},
	{name: "each-read-after", src: "[1 2] each [def x 5] x add 1",
		probes: []string{"x"}, refused: "read of `x` after a multi-run body binds it"},
	{name: "each-underscore-def", src: "[1 2] each [def _u 5]",
		probes: []string{"_u"}},
	{name: "each-nested-multirun", src: "[1] each [[2 3] each [def x 5] 0]",
		probes: []string{"x"}, refused: "twin regime:"},
	// The regime's LAST corpus refusal, graduated: the var-param pair
	// places both halves in-arm (RecordDynUndef's teardown event pairs
	// the BindUndef twin), element-dependent defs re-push from
	// force-promoted slots, and every probe — count, values, order, the
	// pair's net zero — is measured interpreter-equal.
	{name: "row-41-verbatim",
		src:    `def xs [{ok:true} {ok:false}] def _ (xs each [ var [[r] def ok (r "ok" get) def res (if ok [1] [2]) def _2 res 0 ] ]) 9`,
		probes: []string{"xs", "_", "ok", "res", "_2", "r"}},

	// --- The sibling multi-run words, graduated on the same mechanism.
	// `each` was flagged first because its body population is the simplest
	// (one run per element); these carry BodyMultiRunKeepsDefs for the same
	// handler-verified reason — each drives InvokeBody per element/pair on
	// the shared registry with no def cleanup — and the rows below MEASURE
	// that the installs land interpreter-identically rather than assuming
	// the family shares a mechanism.
	{name: "fold-literal-def", src: "fold [ def x 5 ] [10 20] 0",
		probes: []string{"x"}},
	{name: "fold-elem-valued-def", src: "fold [ var [[a b] def x b (a add b)] ] [10 20] 0",
		probes: []string{"x", "a", "b"}},
	// The fold ACCUMULATOR FIXED POINT: a list accumulator widens between
	// analysis rounds, so the body is analysed more than once and every
	// round records the var pair's twins afresh. Only the surviving round
	// is placed; the superseded rows describe no runtime transition and the
	// placement gate exempts them (MultiRunBodyGuard). Before that, this
	// shape refused — the row is the regression pin.
	{name: "fold-list-accumulator", src: "fold [ var [[k acc] (push k acc) ]] [1 2] []",
		probes: []string{"k", "acc"}},
	{name: "scan-elem-valued-def", src: "scan [ var [[a b] def x 5 (a add b)] ] [10 20]",
		probes: []string{"x", "a", "b"}},
	// outer's multi-run population is the PAIR GRID, not a flat element
	// list — a different count through the same mechanism.
	{name: "outer-pair-def", src: "outer [ var [[a b] def x 5 (a add b)] ] [1 2] [3 4]",
		probes: []string{"x"}},
	// NOT graduated, and the reason is a FINDING rather than a decision:
	// `ArrayUtil.foldaxis 0 [var [[a b] def x 5 (a add b)]] [[1 2] [3 4]]`
	// installs `x` twice on the interpreter and ZERO times under the regime,
	// with or without the spec flag — no twin is recorded for that body, so
	// the placement gate is blind to the loss and the program compiles
	// silently wrong. It is a flip blocker (today's keep-installs default
	// hides it: the pass's own install answers the read), so it is written
	// up in the handoff rather than pinned here — a row must be measured
	// parity or pinned refusal, and this shape is neither yet.
}

// probeInstalls enumerates name's install stack top-down on one instance
// through the given runner, returning the rendered value sequence. The cap
// bounds a runaway (a probe that cannot drain in probeCap steps fails the
// lane rather than looping).
const probeCap = 16

func probeInstalls(t *testing.T, run func(string) ([]any, error), name string) []string {
	t.Helper()
	var seq []string
	for i := 0; i < probeCap; i++ {
		out, err := run(name)
		if err != nil {
			if strings.Contains(err.Error(), "undefined_word") {
				return seq
			}
			t.Fatalf("probe read of %q: unexpected error %v", name, err)
		}
		seq = append(seq, renderAny(out))
		if _, err := run("undef " + name); err != nil {
			t.Fatalf("probe undef of %q: %v", name, err)
		}
	}
	t.Fatalf("probe of %q did not drain in %d reads (sequence so far %v)", name, probeCap, seq)
	return nil
}

func TestMultiRunBindParityOracle(t *testing.T) {
	t.Setenv("BORU_TWIN_REGIME", "1")

	for _, sh := range parityShapes {
		sh := sh
		t.Run(sh.name, func(t *testing.T) {
			ai, err := lang.New()
			if err != nil {
				t.Fatal(err)
			}
			interpRun := func(src string) ([]any, error) { return ai.RunInterp(src) }
			interpOut, interpErr := interpRun(sh.src)
			if interpErr != nil {
				t.Fatalf("the interpreter half must run clean (an oracle row may not pin an ill-formed program): %v", interpErr)
			}

			ac, err := lang.New()
			if err != nil {
				t.Fatal(err)
			}
			regimeRun := func(src string) ([]any, error) { return ac.RunCompiledStrict(src) }
			regimeOut, regimeErr := regimeRun(sh.src)

			if sh.refused != "" {
				if regimeErr == nil || !strings.Contains(regimeErr.Error(), sh.refused) {
					t.Fatalf("classified refused(%q) but the regime answered out=%v err=%v — a compiling "+
						"multi-run shape must be RE-CLASSIFIED to parity in a reviewed edit, never allowed to "+
						"drift (the silent-wrong-binding hazard)", sh.refused, regimeOut, regimeErr)
				}
				// Still exercise the probe on the interpreter so the row
				// documents the semantics graduation must match.
				for _, name := range sh.probes {
					probeInstalls(t, interpRun, name)
				}
				return
			}

			if regimeErr != nil {
				t.Fatalf("parity row refused/errored under the regime: %v", regimeErr)
			}
			if got, want := renderAny(regimeOut), renderAny(interpOut); got != want {
				t.Fatalf("driving-request divergence: regime=%q interpreter=%q", got, want)
			}
			for _, name := range sh.probes {
				iSeq := probeInstalls(t, interpRun, name)
				cSeq := probeInstalls(t, regimeRun, name)
				if strings.Join(iSeq, "\x00") != strings.Join(cSeq, "\x00") {
					t.Fatalf("install-stack divergence for %q (top-down):\n  interpreter: %v\n  regime:      %v\n"+
						"count, value, and order must all match — this is the semantic contract the placement "+
						"gate cannot see", name, iSeq, cSeq)
				}
			}
		})
	}
}
