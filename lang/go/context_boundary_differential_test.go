package lang_test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// Context-boundary differential: does a `context set` inside a nested body
// stay inside it, and do the two engines agree?
//
// The interpreter runs every nested body by spawning a sub-engine, and
// `Engine.Run` pushes a child context layer — so interpreted, a nested body is
// a context boundary essentially for free. The VM has no such single seam: it
// reaches a body four different ways, and for a long time bracketed none of
// them, so `context set` inside a compiled `do` / `each` escaped into the
// parent scope. See design/verse-report-defects-investigation.0.md §B.
//
// Two things make this test the shape it is.
//
// First, THE DIVERGENCE WAS INVISIBLE TO THE WHOLE SUITE. Every
// compiled-differential gate the repo has passed with the bug present, and the
// two Go tests that assert the intended semantics by name
// (`TestContextSubEngineIsolation`, `TestContextSubEngineNewKeyDoesNotLeak`)
// run interpreter-only. A spec row that would have covered it was deliberately
// written interpreter-side, with a comment explaining that the compiled path
// was safe "as it currently compiles to a fallback island" — a justification
// that stopped being true when `do` gained a CallableSpec and began lowering
// to a closure, and nothing re-examined it. So the table is explicit about
// mode rather than trusting any single-engine assertion.
//
// Second, IT RECORDS WHAT IS STILL BROKEN. The fix brackets the VM's
// re-entrant body entry (vmContext.enterBodyUnit), which covers every
// closure-unit body — `do`, `each`, `fold`, `filter`, `outer`, `scan`, and
// those same words nested inside a fn. It CANNOT cover a body the compiler
// INLINES into the caller's unit, because there is no call to wrap: the `case`
// desugaring to a nested-`if` chain, `otherwise`'s list argument, and list
// auto-evaluation all emit the body's tokens straight into the enclosing code.
// Those rows are marked `wantDiverge` and are pinned as divergences ON
// PURPOSE. An honest inventory of a partial fix beats a suite that quietly
// covers only the fixed half — that is how the original gap survived.
//
// If a `wantDiverge` row starts AGREEING, that is good news and the row should
// move to the agreeing set. If an agreeing row starts diverging, the frame has
// regressed.
func TestContextBoundaryDifferential(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// wantDiverge marks a form the compiled path does NOT yet bracket.
		wantDiverge bool
		// why documents an expected divergence. Required when wantDiverge.
		why string
	}{
		// ---- closure-unit bodies: bracketed at enterBodyUnit ----
		{name: "do", src: `do [ context set y 1 5 ]
context has y`},
		{name: "nested do", src: `do [ do [ context set y 1 5 ] ]
context has y`},
		{name: "each", src: `each [ drop context set y 1 5 ] [1]
context has y`},
		{name: "fold", src: `fold [ drop drop context set y 1 5 ] [1]
context has y`},
		{name: "filter", src: `filter [ drop context set y 1 true ] [1]
context has y`},
		{name: "outer", src: `outer [ drop drop context set y 1 5 ] [1] [2]
context has y`},
		{name: "scan", src: `scan [ drop drop context set y 1 5 ] [1]
context has y`},
		// Nested inside a fn body: a different unit-nesting shape, and the one
		// the reverted patch's differential corpus found most reliably.
		{name: "do inside a fn body", src: `def f fn [[] [Any] [ do [ context set y 1 5 ] ]]
f
context has y`},
		{name: "each inside a fn body", src: `def f fn [[] [Any] [ each [ drop context set y 1 5 ] [1] ]]
f
context has y`},

		// ---- forms that are NOT boundaries in EITHER engine ----
		// Recorded because the note's own Blocker-2 table listed the fn-body
		// case as still leaking, and measurement says otherwise: a named fn
		// body and a called lambda leak in BOTH engines, so CALL_USER needs no
		// bracketing at all. Keeping these rows is what stops a future
		// "complete the fix" pass from bracketing CALL_USER and silently
		// changing the interpreter's answer too.
		{name: "if branch", src: `if true [ context set y 1 5 ] [ 6 ]
context has y`},
		{name: "for body", src: `for 1 [ context set y 1 5 ]
context has y`},
		{name: "named fn body", src: `def f fn [[] [Any] [ context set y 1 5 ]]
f
context has y`},
		{name: "called lambda", src: `def g ([] => [ context set y 1 5 ])
g
context has y`},

		// ---- INLINED bodies: not bracketable by any seam ----
		{name: "case clause body", src: `case 1 [ 1 [ context set y 1 5 ] 2 [ 6 ] ]
context has y`,
			wantDiverge: true,
			why: "the compiler desugars `case` to an inline nested-`if` chain " +
				"(conditional.go), so the clause body is emitted into the caller's " +
				"unit and no closure is ever created"},
		{name: "otherwise list argument", src: `false otherwise [ context set y 1 5 ]
context has y`,
			wantDiverge: true,
			why:         "the list argument is evaluated inline in the caller's unit"},
		{name: "def list auto-evaluation", src: `def b [ context set y 1 5 ]
context has y`,
			wantDiverge: true,
			why: "`def name [list]` auto-evaluates the list; the interpreter does " +
				"it in a sub-engine, the emitter records it as OpMakeList operands " +
				"emitted inline. The write lands at BIND time, before anything runs " +
				"the body — deleting every later reference leaves it unchanged",
		},
		{name: "fn list argument never used", src: `def run fn [[b:List] [Any] [ 0 ]]
run [ context set y 1 5 ]
context has y`,
			wantDiverge: true,
			why:         "same inline list auto-evaluation, reached as a call argument"},
		// PAREN-GROUPED on purpose. Measured, the divergence is
		// source-shape-dependent: `(m.f 1) drop` diverges, while the bare
		// `m.f 1` and `m.f 1 drop` agree (both leak). So the trigger is the
		// paren group taking a different compiled path, not method dispatch as
		// such — a sharper statement than the design note's, which recorded
		// only "a lambda in a map slot".
		{name: "paren-grouped map-slot lambda method", src: `def m {f: ([a:Integer] => [ context set y 1 a ])}
(m.f 1) drop
context has y`,
			wantDiverge: true,
			why: "diverges in the OPPOSITE direction — compiled CONTAINS the write, " +
				"interpreted leaks it. Pre-existing, and evidence the INTERPRETER is " +
				"itself inconsistent about which call forms are boundaries, which is " +
				"why there is no single true boundary list to document yet"},
		// The ungrouped twin of the row above, kept as its own row because the
		// pair IS the finding: same call, same lambda, different answer
		// depending on whether it is wrapped in parens.
		{name: "ungrouped map-slot lambda method", src: `def m {f: ([a:Integer] => [ context set y 1 a ])}
m.f 1 drop
context has y`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.wantDiverge && c.why == "" {
				t.Fatal("an expected divergence must carry its reason")
			}
			a, _ := lang.New()
			compiled, cErr := a.Run(c.src)
			b, _ := lang.New()
			interp, iErr := b.RunInterp(c.src)
			if cErr != nil || iErr != nil {
				t.Fatalf("both engines must run the program — compiled: %v / interpreted: %v",
					cErr, iErr)
			}
			cs, is := fmt.Sprint(compiled), fmt.Sprint(interp)
			agree := cs == is

			switch {
			case c.wantDiverge && agree:
				t.Errorf("this form is recorded as an OPEN divergence but the engines "+
					"now agree (%s). That is progress, not a failure — move the row to "+
					"the agreeing set and drop its `why`.\n  reason on record: %s",
					cs, c.why)
			case !c.wantDiverge && !agree:
				t.Errorf("compiled %s != interpreted %s — the per-body context frame "+
					"has regressed. A `context set` inside this body escapes on one "+
					"engine and not the other, which is a silent wrong answer rather "+
					"than an error.", cs, is)
			}
		})
	}
}

// TestContextBoundaryAccumulates covers the property a single end-of-body
// check misses: the interpreter's boundary is per INVOCATION, so a compiled
// path that pushed one frame for the whole loop instead of one per element
// would pass every row above and still be wrong.
//
// The note's own repro: each element reads the counter, adds one, writes it
// back. Contained per element, every element sees 0 and the result is [1 1];
// leaking, the writes accumulate and it is [1 2].
func TestContextBoundaryAccumulates(t *testing.T) {
	const src = `context set 'z' 0
each [ drop context set 'z' ((context get 'z') add 1) end (context get 'z') ] [1 2]`
	a, _ := lang.New()
	compiled, cErr := a.Run(src)
	b, _ := lang.New()
	interp, iErr := b.RunInterp(src)
	if cErr != nil || iErr != nil {
		t.Fatalf("compiled: %v / interpreted: %v", cErr, iErr)
	}
	if fmt.Sprint(compiled) != fmt.Sprint(interp) {
		t.Errorf("per-element accumulation diverges: compiled %v != interpreted %v — "+
			"the frame is being pushed once for the loop rather than once per "+
			"element invocation", compiled, interp)
	}
	// And the frame must actually contain the write, not merely agree: two
	// engines that both leaked would pass the comparison above.
	if strings.Contains(fmt.Sprint(compiled), "2") {
		t.Errorf("got %v — the second element saw the first element's write, so the "+
			"body is not a boundary on either engine", compiled)
	}
}
