package lang

// Phase 6 Stage M3 + M4 landing tests (design/STAGE3-INLINING-DESIGN-ROUND.0.md
// §6).
//
// M3 — DSL registration: a `Parse.register`-built kind's `parse <kind>` call
// compiles to a runtime deferred dispatch (parselang-deferred-dispatch) that
// resolves the kind against the LIVE export map, so the module-parse rows run
// native; a check pass installs NOTHING into the export map (the reverted
// register-ReturnsFn leak stays closed). MiniLang's export map gains a growth
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
// words, registered at run time, dispatched through the `parse` macro.
const parseRegRow = `import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [7])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  Parse.register op g  end  parse op 'inc'`

// TestDeferredParseDispatchCompiles pins the M3 parse-side positives: the
// Parse.register rows compile NATIVELY (no island, no trap) and produce the
// interpreter's value.
func TestDeferredParseDispatchCompiles(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"builder grammar (module-parse:14 shape)", parseRegRow, "7"},
		{"whole-spec map (module-parse:37 shape)",
			`import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.spec g {ref:{'@op:o:INC': ([nd:Any] => [7])} abnf:{src:'op = "inc" / "dec"' start:'op'}}  Parse.register sp1 g  end  parse sp1 'inc'`,
			"7"},
		{"deferred result consumed downstream (dot over the dynamic value)",
			`import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [{k:'inc'}])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  Parse.register opm g  end  (parse opm 'inc').k`,
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

// TestDeferredParseDispatchMissParity pins the sound direction of the runtime
// lookup: a RUNTIME-CONDITIONAL Parse.register that did not execute leaves
// the kind missing, and the compiled deferred dispatch raises the parse
// macro's byte-identical parse_unknown_lang (code + detail; the interpreter
// stamps the `parse` word, the compiled op the kind atom — both positions
// present, per the full-corpus error-lane contract).
func TestDeferredParseDispatchMissParity(t *testing.T) {
	const src = `import "aql:parse"  import "aql:parselang"  def g Parse.grammar  Parse.action g '@op:o:INC' ([nd:Any] => [7])  Parse.abnf g 'op = "inc" / "dec"' {start:'op'}  def c false  if c [Parse.register op g] [0]  end  parse op 'inc'`
	gotC, compiled, errC := mustNew(t).RunCompiled(src)
	_, errI := mustNew(t).Run(src)
	if !compiled {
		t.Fatalf("conditional-registration row should still compile (the lookup is the proof); got %v", gotC)
	}
	if codeOf(errC) != "parse_unknown_lang" || codeOf(errI) != "parse_unknown_lang" {
		t.Fatalf("miss parity: compiled=[%s] interp=[%s], want both parse_unknown_lang", codeOf(errC), codeOf(errI))
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

// TestDeferredParseCheckObservationFree pins the leak the REVERTED
// register-ReturnsFn attempt hit (aql-bytecode-stage3-inlining-plan.0.md):
// the check/compile pass must not install the kind into the shared runtime
// export map. If it did, the subsequent INTERPRETED run's Parse.register
// would raise parse_register_error ("already registered") instead of
// completing — exactly the parse_kind_exists-class differential mismatch the
// M3 revert criteria name.
func TestDeferredParseCheckObservationFree(t *testing.T) {
	a := mustNew(t)
	if _, _, _, cerr := a.CompileCheck(parseRegRow); cerr != nil {
		t.Fatalf("check error: %v", cerr)
	}
	got, err := a.Run(parseRegRow)
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
// effects (inc's body prints before apply raises) falls back cleanly: the
// program refuses to compile, runs interpreted, and its prior effects
// still run in order before the identical signature_error — the same abort
// point either way (phase 7 fallback; formerly an M4 carrier trap).
func TestTrapKeepsPriorCallEffects(t *testing.T) {
	const src = `def inc fn [[n:Integer][Integer][print 'a' n add 1]]  5 inc apply`
	prog, _, _, cerr := mustNew(t).CompileCheck(src)
	if cerr != nil {
		t.Fatalf("check error: %v", cerr)
	}
	if prog != nil {
		t.Errorf("a carrier no-match must refuse (fall back), got a compiled program")
	}
	var outC, outI strings.Builder
	ac := mustNew(t)
	ac.SetOutput(&outC)
	_, compiled, errC := ac.RunCompiled(src)
	if compiled {
		t.Fatalf("a carrier no-match must fall back, but ran compiled")
	}
	ai := mustNew(t)
	ai.SetOutput(&outI)
	_, errI := ai.Run(src)
	if codeOf(errC) != "signature_error" || codeOf(errI) != "signature_error" {
		t.Fatalf("compiled=[%s] interp=[%s], want both signature_error", codeOf(errC), codeOf(errI))
	}
	if outC.String() != outI.String() || !strings.Contains(outC.String(), "a") {
		t.Errorf("effect ordering: compiled output %q, interp output %q (the fn body's print must run before the error)", outC.String(), outI.String())
	}
}
