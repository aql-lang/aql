package lang

// NUR123 (measured 2026-09-05). A bare read of a frame binding holding a fn
// is a WORD dispatch on the interpreter — stepWord routes a bound FnDefInfo
// through Registry.Lookup under the binding name: a 0-arg fn fires, an n-arg
// fn collects from the tokens after it and the frame's stack below it, a
// no-match raises `cannot call `g`` — while the check model bound a CARRIER
// the emitter lowered as a slot push: `def f fn [[g:Function][Any][g]]  f
// ([] => [42])` answered `fn` for the interpreter's 42, and the identity fn
// over a 0-arg lambda the same, both on the DEFAULT lane with exit 0. The
// engine now notes each such read (NoteWordRead), the unit's finish arms the
// whole-frame replay over the residual with the binding names, and the VM
// re-steps a fn-valued read as the WORD through the interpreter's own
// dispatch (CompiledFn.DynFrameWords). A fn-typed read the replay cannot
// seat refuses; a gradual one keeps the slot push it always had.

import (
	"fmt"
	"strings"
	"testing"
)

// requireParityHead is requireParity on the error's first line: the
// interpreter's real frame carries the body's DefCleanup marker, which its
// no-match NOTES list as a stray argument (`… and __dc (a __DC)`), while the
// island region re-stepped by the replay holds only the residual — the
// code and message agree, the notes name a marker only one side has.
func requireParityHead(t *testing.T, src string, gotC []any, errC error, gotI []any, errI error) {
	t.Helper()
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || firstErrLine(errC) != firstErrLine(errI) {
		t.Errorf("%q: parity: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
	}
}

const wrF = `def f fn [[g:Function][Any]`

// TestWordReadDispatchParity pins every measured shape that now agrees on
// both lanes, value and error text alike (positions are NUR118's).
func TestWordReadDispatchParity(t *testing.T) {
	rows := []struct{ src, want string }{
		{wrF + `[g]]  f ([] => [42])`, "42 — was fn"},
		{wrF + `[g]]  f (z:Integer => [z])`, "cannot call `g` — was fn (Integer)"},
		{wrF + `[g]]  f add/v`, "cannot call `g` — was fn add(…)"},
		{wrF + `[(g)]]  f ([] => [42])`, "42 — the paren's word dispatches"},
		{wrF + `[(g)]]  f (z:Integer => [z])`, "cannot call `g`"},
		{`def f fn [[g:Function][Any][g] [g:Function][Any][g]]  f ([] => [42])`, "42 — two overloads"},
		{wrF + `[def y 1  g]]  f ([] => [42])`, "42 — a def before the read is not after it"},
		{`def f fn [[g:Function][Integer][g]]  f ([] => [42])`, "42 — the dispatch's result meets the declared type"},
		{wrF + `[g]]  f ([] => [42]) print`, "42"},
		{`def id fn [[x:Any][Any][x]]  id ([] => [42])`, "42 — the gradual identity over a fn"},
		{`def id fn [[x:Any][Any][x]]  id 5`, "5 — and over plain data, no island"},
		{`def mk fn [[] [Function] [([] => [42])]]  def f fn [[g:Function][Any][g]]  f (mk)`, "42 — a returned closure"},
		{wrF + `[g]]  def g0 ([] => [42])  f g0/v`, "42 — a named 0-arg lambda"},
		{wrF + `[g]]  f (quote ([] => [42]))`, "42 — a quoted fn still dispatches as the binding"},
		{`def f fn [[g:Function x:Integer][Any][x g]]  f (z:Integer => [mul 3 z]) 5`, "15 — the word collects the frame's stack"},
		{`def f fn [[g:Function x:Integer][Any][x g]]  f (z:String => [z]) 5`, "cannot call `g`"},
		{`def f fn [[g:Function x:Integer][Any][g x]]  f (z:String => [z]) 5`, "cannot call `g` — was the count error over [fn 5] (NUR122's park)"},
		{`def f fn [[g:Function x:Integer][Any][g x]]  f ([] => [42]) 5`, "f: expected 1, got 2 — [42 5], the 0-arg fn fires (was [fn 5])"},
		{wrF + `[g 3]]  f (z:Integer => [mul 3 z])`, "9"},
		{wrF + `[g 3]]  f ([] => [42])`, "f: expected 1, got 2 — [42 3]"},
		{wrF + `[g 3]]  f (z:String => [z])`, "cannot call `g`"},
		{wrF + `[do [g]]]  f ([] => [42])`, "42 — the do body islands"},
		// a compiled CLOSURE reaching the binding: the bridge dispatches it by
		// name under the lambda's OWN declared params (lamParamContract), so a
		// typed lambda no-matches exactly as the interpreter's frame binding
		{`def mk fn [[k:Integer][Function][([] => [k])]]  def f fn [[g:Function][Any][g]]  [(mk 3)] each [f]`, "[3] — was [fn]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g 2]]  [(mk 3)] each [f]`, "[6]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g "s"]]  [(mk 3)] each [f]`, "cannot call `g` — was [0] (the closure ran over the String)"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function][Any][g]]  [(mk 3)] each [f]`, "cannot call `g`"},
		{`def mk fn [[k:Integer][Function][([a:Integer b:String] => [k])]]  def f fn [[g:Function][Any][g 1 2]]  [(mk 3)] each [f]`, "cannot call `g` — was [3]"},
		{`def mk fn [[k:Integer][Function][([a:Integer b:String] => [k])]]  def f fn [[g:Function][Any][g 1 "b"]]  [(mk 3)] each [f]`, "[3]"},
		{`def mk fn [[k:Integer][Function][(z:Integer => [mul k z])]]  def f fn [[g:Function x:Integer][Any][x g]]  [(mk 3)] each [f 5]`, "[15] — the closure bridged, collecting the frame's x"},
		// value deliveries stay values: a Function-expecting forward, `/v`
		{`def h fn [[k:Function][Any][typeof k/v]]  def f fn [[g:Function][Any][h g]]  f ([] => [42])`, "Function — delivered, not dispatched"},
		{wrF + `[g/v]]  f ([] => [42]) typeof`, "Function"},
		{`[1 2] each ([g:Function] => [g])`, "[fn (Function) fn (Function)] — an unmatched lambda stays data"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.want, errC)
			continue
		}
		requireParityHead(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestWordReadDispatchRefuses pins the reads the replay cannot seat: a
// container member, a branch residual, a stack-collected argument, a read
// mixed with a `/v` read of the same binding — each a sound fallback that
// answers the interpreter's value.
func TestWordReadDispatchRefuses(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	rows := []struct{ src, reason string }{
		{wrF + `[[g]]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[{a: g}]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[if true [g] [0]]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Function][Any][k/v]]  def f fn [[g:Function][Any][g h]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		{wrF + `[g drop  g/v]]  f ([] => [42])`, "read both bare and by /v"},
		{wrF + `[g/v drop  g]]  f ([] => [42])`, "read both bare and by /v"},
		{wrF + `[g typeof]]  f ([] => [42])`, "consumed where the interpreter dispatches it"},
		// a GRADUAL param bound to a fn at the call: the pass re-runs the
		// body under the argument's RUNTIME type (a Function carrier), so
		// the strict accounting applies — these refuse, they never diverged
		// (the map-literal spelling's CHECK pass is the NUR125 pin below).
		{`def h fn [[k:Any][Any][{a: k}]]  h ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Any][Any][{a: (k)}]]  h ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Any][Any][[k]]]  h ([] => [42])`, "consumed where the interpreter dispatches it"},
		{`def h fn [[k:Any][Any][k typeof]]  h ([] => [42])`, "consumed where the interpreter dispatches it"},
	}
	for _, c := range rows {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", c.src, cerr)
		}
		if prog != nil {
			t.Errorf("%q: compiled — the read has no faithful lowering here", c.src)
			continue
		}
		if !strings.Contains(reason, c.reason) {
			t.Errorf("%q: refusal drifted: want %q in %q", c.src, c.reason, reason)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestGradualFnParamMapLiteralCheckIsClean pins NUR125 (resolved 2026-09-05):
// the check pass over a map literal whose value is a bare read of a gradual
// param bound to a fn PANICKED (recovered as internal_error, both lanes,
// exit 1). The check run binds a concrete lambda argument by pushing it as
// a plain def — a FnDefInfo no installDef gave a runner — and the literal's
// const-fold sub-run dispatched the bare read by name into a signature
// whose DispatchHandler is nil. execMatch now raises internal_error on a
// nil runner (ADR-005: an error, never a panic), the fold declines on it,
// and the literal records normally: the check pass is clean, and both lanes
// answer the interpreter's value (the compiled lane through the refusal
// pinned above). Boru.Check is the entry point that reached the fold;
// CompileCheck never did, so the refusal rows alone could not pin it.
func TestGradualFnParamMapLiteralCheckIsClean(t *testing.T) {
	for _, src := range []string{
		`def h fn [[k:Any][Any][{a: k}]]  h ([] => [42])`,
		`def h fn [[k:Any][Any][{a: (k)}]]  h ([] => [42])`,
		`def h fn [[k:Any][Any][{a: k}]]  h 5  h ([] => [42])`,
		`def h fn [[k:Any n:Integer][Any][{a: k, b: n}]]  h ([] => [42]) 1`,
	} {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		res, cerr := a.Check(src)
		if cerr != nil || len(res.Diagnostics) != 0 {
			t.Errorf("%q: check must be clean: err=%v diags=%v", src, cerr, res.Diagnostics)
		}
	}
	// the negative twin: a gradual param whose fn does not match its read
	// stays the interpreter's no_signature diagnostic, not a clean pass
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	src := `def h fn [[k:Any][Any][{a: k}]]  h (z:Integer => [z])`
	res, cerr := a.Check(src)
	if cerr != nil {
		t.Fatalf("%q: check error: %v", src, cerr)
	}
	if len(res.Diagnostics) != 1 || !strings.Contains(fmt.Sprint(res.Diagnostics[0]), "cannot call `k`") {
		t.Errorf("%q: want one no_signature diagnostic naming k, got %v", src, res.Diagnostics)
	}
}

// TestBodyLocalWordReadParity pins NUR123's leftover as far as the residual
// reaches (2026-09-05): a body-local `def` bound to a CONTAINER ELEMENT the
// pass types dynamic(Any) on every run — `def j (m get "f")  j` answered
// `fn` for the interpreter's 42, exit 0 — resolves through its producing
// event, not a slot, so NoteWordRead never counted it. A body-local
// producer of the unit is now the unit's word read (gradual: named, never
// strict), the tail test anchors a word read at its READ rather than at
// the value's producer (an event between the two ran before the dispatch
// on both lanes), and the VM's no-match reads the INSTALLED binding — the
// interpreter's "group the call in parens" help line included. Full-text
// parity (positions, notes and help) on every row.
//
// Still open on the record: the same read consumed OUTSIDE the residual
// (`j typeof`, `{a: j}`, an if/each/do body, the stack-collected `j j add`
// — which the VM's no-match deferral hands back to the interpreter, slow
// and right) or followed by an event (`j  def y 1`, `j  5 drop`), which
// keep the slot push — a gradual read is best effort — and the `/v` render
// (NUR119).
func TestBodyLocalWordReadParity(t *testing.T) {
	const hM = `def h fn [[m:Map][Any]`
	rows := []struct{ src, want string }{
		{hM + `[def j (m get "f")  j]]  h {f: ([] => [42])}`, "42 — was fn"},
		{hM + `[def j (m get "f")  j]]  h {f: 5}`, "5 — plain data, no island"},
		{hM + `[def j (m get "f")  j]]  h {f: (z:Integer => [z])}`, "cannot call `j` — with the interpreter's forward-args help line"},
		{hM + `[def j (m get "f")  j]]  h {f: (fn [[a:Integer] [Integer] [a add 1]])}`, "cannot call `j`"},
		{hM + `[def j (m get "f")  j]]  h {f: add/v}`, "cannot call `j`"},
		{hM + `[def j (m get "f")  j 3]]  h {f: (z:Integer => [mul 3 z])}`, "9"},
		{hM + `[def j (m get "f")  def y 1  j]]  h {f: ([] => [42])}`, "42 — a def between the producer and the read"},
		{hM + `[def j (m get "f")  def y 1  j y]]  h {f: (z:Integer => [mul 3 z])}`, "3"},
		{hM + `[def j (m get "f")  j drop  j]]  h {f: ([] => [42])}`, "42 — the first read consumed, the second seated"},
		{hM + `[def j m.f  j]]  h {f: ([] => [42])}`, "42 — the member spelling"},
		{`def h fn [[m:Map x:Integer][Any][def j (m get "f")  x j]]  h {f: (z:Integer => [mul 3 z])} 5`, "15 — the word collects the frame's stack"},
		{`def h fn [[xs:List][Any][def j (xs get 0)  j]]  h [([] => [42])]`, "42 — a list element"},
		{hM + `[def j (m get "f")  (j)]]  h {f: ([] => [42])}`, "42"},
		{`def mk fn [[] [Function] [([] => [42])]]  def f fn [[] [Any] [def r (mk)  r]]  f`, "42 — a returned closure bound to a body-local"},
		{`def f fn [[x:Integer] [Any] [def r (x add 1)  r]]  f 4`, "5 — a plain computed local"},
	}
	for _, c := range rows {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q (%s): must compile natively; err=%v", c.src, c.want, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
	// the top-level spelling has no frame to seat in: the Stage-3 refusal
	// it always had, and the interpreter's answer
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	src := `def m {f: ([] => [42])}  def j (m get "f")  j`
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog != nil || !strings.Contains(reason, "fn value read from a container auto-dispatches") {
		t.Errorf("%q: want the Stage-3 container refusal, got prog=%v reason=%q err=%v", src, prog != nil, reason, cerr)
	}
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	requireParity(t, src, gotC, errC, gotI, errI)
}
