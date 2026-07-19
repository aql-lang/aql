package lang

// Phase 6 Stage M3 + M4 landing tests (design/STAGE3-INLINING-DESIGN-ROUND.0.md
// §6).
//
// M3 (as re-landed by the namespace freeze) — DSL parsers: a Parse.parser-
// built grammar parser is a ParseLang Function VALUE; its `parse <name>`
// call compiles to a runtime fn dispatch (parselang-fn-dispatch) whose fn
// operand carries provenance from the Parse.parser call itself, so the
// module-parse rows run native. MiniLang's export map keeps a growth
// LEDGER so a PROVABLY-stable missing-key read folds to None
// (module-minilang:320) while every key a register call may install keeps the
// blanket fold decline.
//
// M4 (superseded by phase 7) — a dispatch-recovery ERROR row whose carrier
// operands are non-concrete at compile time can no longer be baked into a
// terminal OpTrap (the baked diagnostic could not match the interpreter's
// runtime, concrete-value one — design/DIAGNOSTICS.0.md). Such a row now
// EITHER compiles to a runtime-re-matching poly call or falls back to the
// interpreter; either way the raised signature_error is byte-identical across
// engines. TestUnmatchedDispatchTrapCarrierDisjoint pins that parity; the
// former carrier-disjointness trap machinery is removed.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	eng "github.com/aql-lang/aql/eng/go"
)

// parseRegRow is the module-parse.tsv:14 shape — grammar built by builder
// words, finalized into a fn value, dispatched through the `parse` macro's
// value form.
const parseRegRow = `import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [7])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  def op (Parse.parser g)  end  parse op 'inc'`

// TestParseFnDispatchCompiles pins the parse-side positives: the
// Parse.parser rows compile NATIVELY (no island, no trap) and produce the
// interpreter's value.
func TestParseFnDispatchCompiles(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"builder grammar (module-parse:14 shape)", parseRegRow, "7"},
		{"whole-spec map (module-parse:37 shape)",
			`import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.spec g {ref:{'@op:o:INC': ([nd:Any] => [7])} abnf:{src:'op = "inc" / "dec"' start:'op'}}  def sp1 (Parse.parser g)  end  parse sp1 'inc'`,
			"7"},
		{"dispatched result consumed downstream (dot over the dynamic value)",
			`import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [{k:'inc'}])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  def opm (Parse.parser g)  end  (parse opm 'inc').k`,
			"inc"},
	}
	for _, c := range cases {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%s: check error %v", c.name, cerr)
		}
		if prog == nil {
			t.Fatalf("%s: refused: %s", c.name, reason)
		}
		if dis := prog.Disassemble(); strings.Contains(dis, "FALLBACK") || strings.Contains(dis, "TRAP") {
			t.Errorf("%s: expected a native program (no island, no trap):\n%s", c.name, dis)
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		if errC != nil || !compiled {
			t.Fatalf("%s: compiled run failed: compiled=%v err=%v", c.name, compiled, errC)
		}
		gotI, errI := mustNew(t).Run(c.src)
		if errI != nil {
			t.Fatalf("%s: interp run failed: %v", c.name, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || !strings.Contains(fmt.Sprint(gotC), c.want) {
			t.Errorf("%s: compiled=%v interp=%v want %q", c.name, gotC, gotI, c.want)
		}
	}
}

// TestParseFnDispatchMissParity pins the sound direction of the runtime fn
// dispatch: a parser binding whose runtime value is NOT a usable parser fn
// (here via the branch-def quirk: the conditional's def leaks a non-fn
// value in BOTH engines identically) raises the byte-identical parse_error
// through the compiled parselang-fn-dispatch and the interpreter's
// parseFnExpand — code + detail + position, per the full-corpus error-lane
// contract. The old kind-name miss (parse_unknown_lang for an unregistered
// atom) is pinned by module-parselang.tsv §4/§10; a def-scoped parser value
// has no "missing kind" — only an unusable value, and the two engines must
// agree on it.
func TestParseFnDispatchMissParity(t *testing.T) {
	const src = `import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [7])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  def c false  if c [def op (Parse.parser g)] [0]  end  parse op 'inc'`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	_, errI := mustNew(t).Run(src)
	if !compiled {
		t.Fatalf("conditional-parser row should still compile (the dispatch is the proof); got %v", gotC)
	}
	if codeOf(errC) != "parse_error" || codeOf(errI) != "parse_error" {
		t.Fatalf("miss parity: compiled=[%s] interp=[%s], want both parse_error", codeOf(errC), codeOf(errI))
	}
	var aeC, aeI *eng.AqlError
	if !errors.As(errC, &aeC) || !errors.As(errI, &aeI) {
		t.Fatalf("non-AQL error: compiled=%v interp=%v", errC, errI)
	}
	if aeC.Detail != aeI.Detail {
		t.Errorf("miss detail divergence:\n  compiled=%q\n  interp=%q", aeC.Detail, aeI.Detail)
	}
	if aeI.Row > 0 && aeC.Row == 0 {
		t.Errorf("position lost in compiled mode (interp at %d:%d)", aeI.Row, aeI.Col)
	}
}

// TestParseFnDispatchCheckObservationFree pins observation-freedom: a
// compile pass over a Parse.parser program must leave no state behind that
// changes a subsequent INTERPRETED run on the same instance (the class of
// leak the historical register-ReturnsFn attempt hit).
func TestParseFnDispatchCheckObservationFree(t *testing.T) {
	a := mustNew(t)
	if _, _, _, cerr := a.CompileCheck(parseRegRow); cerr != nil {
		t.Fatalf("check error: %v", cerr)
	}
	got, err := a.RunInterp(parseRegRow)
	if err != nil {
		t.Fatalf("interpreted run after a compile pass errored (check-pass state leaked into the export map): %v", err)
	}
	if fmt.Sprint(got) != "[7]" {
		t.Errorf("interpreted run after a compile pass: got %v, want [7]", got)
	}
}

// TestMiniLangAbsenceFoldCompiles pins the M3 minilang growth ledger: the
// PROVABLY-stable missing-key reads fold to None and compile natively, while
// every key a register call may install keeps the sound fold decline.
func TestMiniLangAbsenceFoldCompiles(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// AQL_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("AQL_COMPILE_FALLBACK", "1")
	positives := []struct{ name, src string }{
		// module-minilang.tsv:320 — a non-filter kind (2-param sig) never
		// mints a member type, so MiniLang.Gen is None on every run.
		{"non-filter kind mints no type (minilang:320)",
			`import "aql:minilang"  MiniLang.register gen (fn [[src:String opts:Map] [Integer] [1]]) end  MiniLang.Gen`},
		// No register call at all: the ledger is empty, absence is stable.
		{"missing key with no registration",
			`import "aql:minilang"  MiniLang.Nope`},
	}
	for _, c := range positives {
		prog, reason, _, cerr := mustNew(t).CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%s: check error %v", c.name, cerr)
		}
		if prog == nil {
			t.Fatalf("%s: refused: %s", c.name, reason)
		}
		if dis := prog.Disassemble(); strings.Contains(dis, "FALLBACK") {
			t.Errorf("%s: expected a native program:\n%s", c.name, dis)
		}
		gotC, compiled, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).Run(c.src)
		if errC != nil || errI != nil || !compiled {
			t.Fatalf("%s: compiled=%v errC=%v errI=%v", c.name, compiled, errC, errI)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || !strings.Contains(fmt.Sprint(gotC), "None") {
			t.Errorf("%s: compiled=%v interp=%v want None", c.name, gotC, gotI)
		}
	}

	// NEGATIVES — the fold must keep declining for every key the register
	// call may install; baking None for these would be a miscompile (the
	// interpreter produces the minted type / the stored fn).
	negatives := []struct{ name, src string }{
		// A FILTER-shaped kind (3-param sigs) mints the capitalised member
		// type at run time — the interpreter's answer is the Gen2 type
		// literal, never None.
		{"filter-kind minted type stays unfolded",
			`import "aql:minilang"  MiniLang.register gen2 (fn [[src:String opts:Map subject:String] [Map] [{}]]) end  MiniLang.Gen2`},
		// The transducer key itself is installed by the register call.
		{"lang_<kind> key stays unfolded",
			`import "aql:minilang"  MiniLang.register gen (fn [[src:String opts:Map] [Integer] [1]]) end  MiniLang.lang_gen`},
	}
	for _, c := range negatives {
		prog, _, _, _ := mustNew(t).CompileCheck(c.src)
		if prog != nil && !strings.Contains(prog.Disassemble(), "FALLBACK") {
			// Compiling natively is acceptable ONLY if the value agrees with
			// the interpreter (i.e. it did not fold a stale None).
			gotC, _, errC := mustNew(t).RunCompiled(c.src)
			gotI, errI := mustNew(t).Run(c.src)
			if codeOf(errC) != codeOf(errI) || (errC == nil && fmt.Sprint(gotC) != fmt.Sprint(gotI)) {
				t.Errorf("%s: compiled=%v/%v interp=%v/%v (stale absence baked?)",
					c.name, gotC, errC, gotI, errI)
			}
			if errC == nil && strings.Contains(fmt.Sprint(gotC), "None") && !strings.Contains(fmt.Sprint(gotI), "None") {
				t.Errorf("%s: compiled None where interpreter has %v", c.name, gotI)
			}
			continue
		}
		// Refused: the fallback must agree with the interpreter.
		gotC, _, errC := mustNew(t).RunCompiled(c.src)
		gotI, errI := mustNew(t).Run(c.src)
		if codeOf(errC) != codeOf(errI) || (errC == nil && errI == nil && fmt.Sprint(gotC) != fmt.Sprint(gotI)) {
			t.Errorf("%s: fallback=%v/%v interp=%v/%v", c.name, gotC, errC, gotI, errI)
		}
	}
}

// TestUnmatchedDispatchTrapCarrierDisjoint pins the M4 positives: dispatch
// recoveries whose carrier operands are PROVABLY disjoint from every
// overload's slots (assignment feasibility, sigDefinitelyUnmatched) compile
// to a terminal OpTrap with the interpreter's byte-identical taxonomy —
// code, detail, and a position wherever the interpreter carries one.
func TestUnmatchedDispatchTrapCarrierDisjoint(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// AQL_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("AQL_COMPILE_FALLBACK", "1")
	cases := []struct{ name, src string }{
		// apply.tsv:37 — the former "carrier operand declines" negative:
		// inc's Integer result is disjoint from apply's Function slot, and
		// the [Reach Any] overload cannot even fill its window.
		{"Integer carrier vs apply Function slot",
			`def inc fn [[n:Integer][Integer][n add 1]]  5 inc apply`},
		// apply.tsv:38 — same via a ref value on the stack.
		{"ref-value carrier vs apply",
			`def inc fn [[n:Integer][Integer][n add 1]]  5 (ref inc) apply`},
		// open-words.tsv:32 — a Boolean carrier against add's builtin
		// overloads; the 3-arg [Map Any Patrun] overload falls to the
		// zero-edge Map slot (true/false fail it too).
		{"Boolean carrier vs builtin add",
			`def f fn [[x:Boolean] [Boolean] [def add fn [[a:Boolean b:Boolean] [Boolean] [a or b]] add x false]]  (f true) add true false`},
		// open-words.tsv:83 — module-PRIVATE Flag overload not exported:
		// two Flag carriers are disjoint from every builtin add slot.
		{"Flag carriers vs builtin add (module add private)",
			`import module [def Flag (refine Boolean) def add fn [[a:Flag b:Flag] [Boolean] [a and b]] def mk fn [[b:Boolean] [Flag] [def v:Flag b v]] export "M" {mk: mk/r Flag: Flag}]  add (M.mk true) (M.mk true)`},
		// open-words.tsv:100 — the COUNTING case: the merged [Point Point]
		// overload has one Point-compatible candidate for two Point slots,
		// so the Integer must occupy one of them (assignment infeasibility).
		{"one Point candidate for two Point slots",
			`import module [def Point class {x:Integer y:Integer} def add fn [[a:Point b:Point] [Point] [make Point {x:(a.x add b.x) y:(a.y add b.y)}]] export "Pointer" {Point: Point add: add/r}]  def p0 (make Pointer.Point {x:1 y:2})  add p0 1`},
		// generics-sugar.tsv:37 — the design's named example: a Box<String>
		// instance is statically Never against a Box<Integer> param.
		{"Box<String> vs Box<Integer> param",
			`def Box<T> class {value:T} def f fn [[x:Box<Integer>] [Integer] [x dot value]] end f (make Box<String> {value:'s'})`},
		// generics.tsv:60 — the explicit gen spelling of the same proof.
		{"Box of [String] vs Box of [Integer] param",
			`def Box gen [T] class {value:T} def f fn [[x:(Box of [Integer])] [Integer] [x dot value]] end f (make (Box of [String]) {value:'s'})`},
	}
	for _, c := range cases {
		// A carrier no-match reaches the compiled path in one of two ways:
		// a runtime-re-matching poly call (tryRecordPoly), or — where that
		// declines — a whole-program fallback (the no-match trap DECLINES a
		// carrier window, since a carrier is not concrete at compile time so
		// a baked diagnostic could not match the interpreter's runtime one;
		// this supersedes the former M4 carrier-disjointness trap). EITHER
		// way the raised error must be byte-identical to the interpreter's
		// (phase 7): same code, Detail, notes, and suggestions.
		_, _, errC := mustNew(t).RunCompiled(c.src)
		_, errI := mustNew(t).Run(c.src)
		if codeOf(errC) != "signature_error" || codeOf(errI) != "signature_error" {
			t.Fatalf("%s: compiled=[%s] interp=[%s], want both signature_error", c.name, codeOf(errC), codeOf(errI))
		}
		var aeC, aeI *eng.AqlError
		if !errors.As(errC, &aeC) || !errors.As(errI, &aeI) {
			t.Fatalf("%s: non-AQL error: compiled=%v interp=%v", c.name, errC, errI)
		}
		if aeC.Detail != aeI.Detail {
			t.Errorf("%s: detail divergence:\n  compiled=%q\n  interp=%q", c.name, aeC.Detail, aeI.Detail)
		}
		if !diagNotesEqual(aeC, aeI) {
			t.Errorf("%s: note divergence:\n  compiled=%v\n  interp=%v", c.name, aeC.Notes, aeI.Notes)
		}
	}
}

// diagNotesEqual reports whether two errors carry the same notes,
// normalising the incidental value-rendering non-determinism the two
// engines legitimately have (counter-based IDs) — the diagnostic
// STRUCTURE is what parity requires.
func diagNotesEqual(a, b *eng.AqlError) bool {
	if len(a.Notes) != len(b.Notes) {
		return false
	}
	norm := func(s string) string { return regexp.MustCompile(`#\d+`).ReplaceAllString(s, "#N") }
	for i := range a.Notes {
		if norm(a.Notes[i]) != norm(b.Notes[i]) {
			return false
		}
	}
	return true
}

// TestTrapKeepsPriorCallEffects pins that a carrier no-match with prior
// effects (inc's body prints before apply raises) keeps them in order: the
// program compiles to a runtime rematch (OpDispatchRematch — formerly a
// whole-program refusal, before that an M4 carrier trap), inc's unit runs
// natively and PRINTS, then the rematch raises the identical
// signature_error — the same effects and abort point as the interpreter,
// with no fallback re-run to duplicate the print.
func TestTrapKeepsPriorCallEffects(t *testing.T) {
	const src = `def inc fn [[n:Integer][Integer][print 'a' n add 1]]  5 inc apply`
	prog, reason, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check error: %v", cerr)
	}
	if prog == nil {
		t.Errorf("the carrier no-match must compile to a runtime rematch (reason %q)", reason)
	}
	var outC, outI strings.Builder
	ac := mustNew(t)
	ac.SetOutput(&outC)
	_, compiled, errC := ac.RunCompiled(src)
	if !compiled {
		t.Fatalf("the rematch program must run compiled")
	}
	ai := mustNew(t)
	ai.SetOutput(&outI)
	_, errI := ai.RunInterp(src)
	if codeOf(errC) != "signature_error" || codeOf(errI) != "signature_error" {
		t.Fatalf("compiled=[%s] interp=[%s], want both signature_error", codeOf(errC), codeOf(errI))
	}
	if outC.String() != outI.String() || !strings.Contains(outC.String(), "a") {
		t.Errorf("effect ordering: compiled output %q, interp output %q (the fn body's print must run before the error)", outC.String(), outI.String())
	}
}
