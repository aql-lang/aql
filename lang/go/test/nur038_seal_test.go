package test

import (
	"bytes"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	parser "github.com/boru-lang/boru/parser/go"
)

// NUR038: consecutive statements headed by a value-called Any-param fn
// (a dot-access — `IO.printstr "A"` / `m.p "A"`) misfired silently: a
// COMPLETED forward collection re-steps the callee stack-only via the /s
// word rewrite, but a function VALUE has no WordInfo to stamp, so its
// re-step re-planned forward-first and the still-open window swallowed
// the next statement — side effects ran in reverse (B before A), the
// displaced arguments stranded, and the program exited 0. The engine now
// seals a completed value-called collection with a one-shot stack-only
// re-step.
//
// These tests pin the two shapes a TSV row cannot: OUTPUT ORDER for the
// zero-return flavor (the spec runner compares the residual stack, not
// stdout), and the residual being CLEAN (no stranded fn value). The
// value-returning flavors are pinned in lang/spec/fn-value.tsv §6.

// runSealCase parses and interprets src with a captured Output writer,
// returning (printed, residual).
func runSealCase(t *testing.T, src string) (string, []native.Value) {
	t.Helper()
	tokens, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	r.SetParseFunc(parser.Parse)
	modules.InstallResolver(r)
	var out bytes.Buffer
	r.Output = &out
	res, rerr := native.NewTop(r).Run(tokens)
	if rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	return out.String(), res
}

func TestNur038PrintOrderModuleExport(t *testing.T) {
	// The original NUR038 repro: module-exported printstr, two statements.
	printed, res := runSealCase(t,
		`import "boru:io" IO.printstr "A\n" IO.printstr "B\n"`)
	if printed != "A\nB\n" {
		t.Errorf("printed %q, want \"A\\nB\\n\" (source order)", printed)
	}
	if len(res) != 0 {
		t.Errorf("residual = %v, want empty (no stranded fn value)", res)
	}
}

func TestNur038PrintOrderPlainMap(t *testing.T) {
	// The module-independent twin: a def-bound plain map holding a
	// zero-return Any-param fn — proves the seal is not module machinery.
	printed, res := runSealCase(t,
		`import "boru:io" end def pr fn [[x:Any] [] [IO.printstr x]] end def m {p: pr/v} end m.p "A\n" m.p "B\n"`)
	if printed != "A\nB\n" {
		t.Errorf("printed %q, want \"A\\nB\\n\" (source order)", printed)
	}
	if len(res) != 0 {
		t.Errorf("residual = %v, want empty (no stranded fn value)", res)
	}
}

func TestNur038PrintOrderThreeStatements(t *testing.T) {
	printed, res := runSealCase(t,
		`import "boru:io" IO.printstr "A\n" IO.printstr "B\n" IO.printstr "C\n"`)
	if printed != "A\nB\nC\n" {
		t.Errorf("printed %q, want \"A\\nB\\nC\\n\"", printed)
	}
	if len(res) != 0 {
		t.Errorf("residual = %v, want empty", res)
	}
}

func TestNur038SealComposesWithWordStatement(t *testing.T) {
	// A sealed zero-return value call followed by an ordinary word
	// statement: the word's own collection starts fresh.
	printed, res := runSealCase(t,
		`import "boru:io" IO.printstr "A\n" 1 add 2`)
	if printed != "A\n" {
		t.Errorf("printed %q, want \"A\\n\"", printed)
	}
	if len(res) != 1 {
		t.Fatalf("residual = %v, want the single sum", res)
	}
	if n, _ := native.AsInteger(res[0]); n != 3 {
		t.Errorf("sum = %v, want 3", res[0])
	}
}

func TestNur038MidCollectionArrivalCommits(t *testing.T) {
	// A dot-read call head arriving MID-COLLECTION (the collecting word
	// planned an optimistic window over the reach): the arrival gate
	// commits the pending forward with the args it already claimed —
	// here w's smaller 1-arg overload fires, exactly as an explicit
	// `end` before `m.p` would dispatch it — and the fn then runs as
	// its own statement. Pre-fix the fn value was swallowed as w's
	// second Any arg and `7` stranded.
	_, res := runSealCase(t,
		`def f fn [[x:Any] [Any] [x]] end def m {p: f/v} end `+
			`def w fn [[a:Any] [Any] [a] [a:Any b:Any] [List] [[a b]]] end `+
			`w 1 m.p 7`)
	if len(res) != 2 {
		t.Fatalf("residual = %v, want [1 7]", res)
	}
	a, _ := native.AsInteger(res[0])
	b, _ := native.AsInteger(res[1])
	if a != 1 || b != 7 {
		t.Errorf("residual = %v, want [1 7] (w commits at one arg; m.p claims 7)", res)
	}
}

func TestNur038MidCollectionPropertyReadDispatches(t *testing.T) {
	// A 0-arg-only dot-read arriving mid-collection is a PROPERTY call:
	// it dispatches in place (consuming nothing, it can never swallow a
	// statement) and its RESULT fills the still-pending window.
	_, res := runSealCase(t,
		`def z fn [[] [Integer] [42]] end def m {z: z/v} end `+
			`def w2 fn [[a:Any b:Any] [List] [[a b]]] end `+
			`w2 1 m.z`)
	if len(res) != 1 {
		t.Fatalf("residual = %v, want the single pair", res)
	}
	lst, err := native.AsList(res[0])
	if err != nil || lst.Len() != 2 {
		t.Fatalf("residual = %v, want [1 42]", res)
	}
	a, _ := native.AsInteger(lst.Get(0))
	b, _ := native.AsInteger(lst.Get(1))
	if a != 1 || b != 42 {
		t.Errorf("pair = %v, want [1 42] (m.z dispatches as a property read)", res[0])
	}
}

func TestNur038MidCollectionMixedArityPropertyRead(t *testing.T) {
	// A MIXED-arity dot-read (a 0-arg overload plus arg-taking ones)
	// arriving mid-collection: the group defers the call (NUR035 — the
	// 0-arg overload must not eclipse the arg-taking sigs inside the
	// group), so the fn value itself arrives at the pending window, where
	// the arrival gate's property arm dispatches it in place — the 0-arg
	// overload fires (nothing to its right belongs to it) and the RESULT
	// fills the window.
	_, res := runSealCase(t,
		`def mx fn [[] [Integer] [42] [x:Integer] [Integer] [x add 1]] end `+
			`def m {x: mx/v} end `+
			`def w2 fn [[a:Any b:Any] [List] [[a b]]] end `+
			`w2 1 m.x`)
	if len(res) != 1 {
		t.Fatalf("residual = %v, want the single pair", res)
	}
	lst, err := native.AsList(res[0])
	if err != nil || lst.Len() != 2 {
		t.Fatalf("residual = %v, want [1 42]", res)
	}
	a, _ := native.AsInteger(lst.Get(0))
	b, _ := native.AsInteger(lst.Get(1))
	if a != 1 || b != 42 {
		t.Errorf("pair = %v, want [1 42] (mx property-dispatches at arrival)", res[0])
	}
}

func TestNur038ZeroArgRefStaysData(t *testing.T) {
	// The 0-arg property-call exception yields to an EXPLICIT /v: the
	// dot-read stays a Function reference, it is not invoked (the
	// PR-review finding — dispatching inside the group consumed the fn
	// before the post-group marker peek could see the /v).
	_, res := runSealCase(t,
		`def z fn [[] [Integer] [42]] end def m {z: z/v} end typeof m.z/v`)
	if len(res) != 1 || res[0].String() != "Function" {
		t.Errorf("residual = %v, want [Function] (the /v read is a reference, not a call)", res)
	}
	// The unmarked twin stays a property CALL.
	_, res = runSealCase(t,
		`def z fn [[] [Integer] [42]] end def m {z: z/v} end m.z`)
	if len(res) != 1 {
		t.Fatalf("residual = %v, want [42]", res)
	}
	if n, _ := native.AsInteger(res[0]); n != 42 {
		t.Errorf("residual = %v, want [42] (the unmarked read dispatches)", res)
	}
}

func TestNur038LambdaTwinCallsSeal(t *testing.T) {
	// The CI-caught regression (frontier-nur038-seal.tsv's lambda row):
	// a single-arity ANONYMOUS value call must seal even though the
	// next token is the second reach (an optimistic probe) — no wider
	// overload exists, so nothing can widen.
	_, res := runSealCase(t,
		`def m {l: ([q:Any] => [q])} end m.l 5 m.l 7`)
	if len(res) != 2 {
		t.Fatalf("residual = %v, want [5 7]", res)
	}
	a, _ := native.AsInteger(res[0])
	b, _ := native.AsInteger(res[1])
	if a != 5 || b != 7 {
		t.Errorf("residual = %v, want [5 7] (each lambda call is its own statement)", res)
	}
}

func TestNur038MidCollectionStrictStaysLoud(t *testing.T) {
	// A value parked mid-collection when a function WORD begins its own
	// dispatch stays LOUD (the strict-forward-barrier rule; the bare
	// no-args data case is fn-value.tsv §3's `m.f` row). Pinned as a Go
	// test rather than a TSV row: the compiled path models this window
	// differently (the pre-existing check-vs-run class fn-value.tsv §5
	// notes), so a row would read as a compile divergence.
	tokens, err := parser.Parse(`def f fn [[x:Any] [Any] [x]] end def m {p: f/v} end m.p typeof`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	r.SetParseFunc(parser.Parse)
	modules.InstallResolver(r)
	_, rerr := native.NewTop(r).Run(tokens)
	if rerr == nil || !strings.Contains(rerr.Error(), "still waiting") {
		t.Fatalf("want the strict still-waiting error, got %v", rerr)
	}
}

func TestNur038CheckReportsCleanResidual(t *testing.T) {
	// `boru check` used to report a `ProperString Function` residual for
	// the misfire with 0 errors — the only visible trace. Post-seal the
	// static model agrees with the fixed runtime: no Function residual.
	a, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := a.Check(`import "boru:io" IO.printstr "A\n" IO.printstr "B\n"`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, s := range res.Stack {
		if strings.Contains(s, "Function") {
			t.Errorf("check residual contains a Function carrier: %v", res.Stack)
		}
	}
}
