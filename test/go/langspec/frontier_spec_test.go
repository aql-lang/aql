package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go"
	"github.com/boru-lang/boru/eng/go/parser"
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/modules"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/test/go/specrunner"
)

// The shared frontier TSV corpus (lang/spec/frontier/*.tsv — the flat corpus
// glob skips subdirectories, so these rows sit OUTSIDE the live refusal/
// island ratchets). Each row is an ordinary spec row — `input⇥expected` with
// the interpreter as the semantics oracle, exactly the format a TS port will
// run — whose COMPILE status is the frontier: the expected-red ledger below
// pins which rows the compiler refuses today and why, with the same
// stale/drift/bootstrap contract as the lang-package frontier ledger
// (lang/go/frontier_ledger_test.go) and knownRefusals. Graduation = the row
// compiles → delete its ledger entry and (usually) move the row into the
// main lang/spec corpus so the census owns it.
//
// TestFrontierRefusalRowsCompile is the sibling inventory for the 9
// knownRefusals rows: those already live in the MAIN corpus, so their
// frontier cases read the sources straight from the knownRefusals map (one
// source of truth) and assert the TARGET (compile + byte-identical
// error/value parity); graduation is coupled per-row with the knownRefusals
// deletion.

type frontierRow struct {
	file  string
	line  int
	input string
	want  string
}

func loadFrontierRows(t *testing.T) []frontierRow {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "lang", "spec", "frontier")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("frontier spec dir: %v", err)
	}
	var rows []frontierRow
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("open %s: %v", e.Name(), err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := strings.TrimRight(scanner.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				t.Fatalf("%s:%d: malformed row (no tab)", e.Name(), lineNo)
			}
			rows = append(rows, frontierRow{
				file:  e.Name(),
				line:  lineNo,
				input: strings.TrimSpace(parts[0]),
				want:  strings.TrimSpace(parts[1]),
			})
		}
		f.Close()
	}
	if len(rows) == 0 {
		t.Fatal("no frontier rows loaded")
	}
	return rows
}

// runFrontierInterp evaluates a row on the interpreter with the SAME wiring
// as the production spec runner (langspec_test.go's closure) and reports the
// canonical outcome string (eng.Canon of the stack, or ERROR:<text>).
func runFrontierInterp(input string) (string, error) {
	values, err := parser.Parse(input)
	if err != nil {
		return "ERROR:" + err.Error(), nil
	}
	reg, err := native.DefaultRegistry()
	if err != nil {
		return "", err
	}
	specrunner.RegisterQFixtures(reg)
	reg.SetParseFunc(parser.Parse)
	modules.InstallResolver(reg)
	native.SetHostClock(reg, specClock)
	out, rerr := native.NewTop(reg).Run(values)
	if rerr != nil {
		return "ERROR:" + rerr.Error(), nil
	}
	return eng.Canon(out), nil
}

// TestFrontierSpecInterp — the semantics oracle: every frontier row must PASS
// on the interpreter (rows are green semantics whose compile status is red).
// An expected of BOOTSTRAP fails printing the observed outcome verbatim, so
// populating a new row's expected column is enforced, exactly like the
// ledger's failsWith sentinel.
func TestFrontierSpecInterp(t *testing.T) {
	for _, row := range loadFrontierRows(t) {
		got, err := runFrontierInterp(row.input)
		if err != nil {
			t.Errorf("%s:%d: harness: %v", row.file, row.line, err)
			continue
		}
		switch {
		case row.want == "BOOTSTRAP":
			t.Errorf("%s:%d: BOOTSTRAP row — record the interpreter outcome into the expected column. Observed: %s", row.file, row.line, got)
		case strings.HasPrefix(row.want, "ERROR:"):
			if !strings.HasPrefix(got, "ERROR:") || !strings.Contains(got, strings.TrimPrefix(row.want, "ERROR:")) {
				t.Errorf("%s:%d: interpreter outcome %q, want error containing %q", row.file, row.line, got, row.want)
			}
		case got != row.want:
			t.Errorf("%s:%d: interpreter outcome %q, want %q", row.file, row.line, got, row.want)
		}
	}
}

// docMod is the shared module preamble of the do-catch rows (a value-
// dependently-raising fn and an always-raising one, reached as M.dec/M.boom).
// Must match the TSV rows byte-for-byte — the orphan arm catches drift.
const docMod = `import module [ def dec fn [[bad:Boolean x:Any] [Any] [ if bad [raise bad_input "boom"] [x] ]] def boom fn [[x:Any] [Any] [ raise bad_input "always" ]] export "M" {dec: dec/r, boom: boom/r} ] end `

// frontierCompileLedger pins the frontier rows the compiler REFUSES today,
// keyed by exact input (the knownRefusals convention). failsWith pins the
// refusal reason substring (stable core only); "" is the bootstrap sentinel.
// Signatures transcribed from the 2026-07-13 bootstrap run.
var frontierCompileLedger = map[string]frontierEntryLS{
	// Conditional fn-shadow — a MISCOMPILE (variation sweep,
	// forward-barrier.tsv:73); now a SOUND REFUSAL: a user fn redefined
	// inside a conditionally-reached body overlap-removes the enclosing
	// overload in place, so the branch/loop def rollback cannot restore it and
	// compiled resolution bakes the shadow while the interpreter keeps the
	// outer fn on the not-taken / zero-iteration path. Refused CondBodyDepth-
	// gated (eng/go/core_helpers.go). Full graduation = a runtime dispatch
	// respecting the conditional binding compiles these rows.
	`def g fn [[x:Any] [Integer] [x add 100]] if false [def g fn [[x:Any] [Integer] [x add 1]]] g 1`: {why: "conditional fn redefinition shadows an outer overload; compiled bake would diverge from the interpreter on the not-taken branch", failsWith: "redefined inside a conditional body"},

	// L-DO — plan Phase 5: the body nets N values on no-raise but 1 Error on
	// raise; needs OpStackMark/OpDropToMark variable arity across the catch
	// merge. One entry per fallibility route (Reach raise, no-raise-at-input,
	// always-raise, value-diverging native, user fn body, bare module-export
	// value, branch-arm nesting).
	// L-DO PART 1 LANDED (2026-07-13): fallible multi-value do results now
	// record VARIADIC (the SetCatchVariadic latch) instead of refusing at the
	// ReturnsFn — the div row graduated to the main corpus, and the remaining
	// rows drifted to the DOWNSTREAM refusals below: `error` consuming the
	// variadic region's top needs the part-2 region-top lowering
	// (strip-input over a variadic region; see the L-DO implementation map
	// in the completion plan).
	// Re-diagnosed 2026-07-20 (PR #280 review): the def-bound variadic
	// region now refuses at lowerCall's store-prologue gate — a promoted
	// variadic result's stores pop success-arity values the raise path
	// never delivers — one stage before the "residual shape beyond Stage 1"
	// decline these rows used to surface. Same sound refusal, earlier and
	// truer diagnosis.
	// Re-diagnosed 2026-07-30 (design/FN-VALUE-DISPATCH.0.md): the region's
	// `M.dec` call fails dispatch, which is now an error-severity check
	// diagnostic in the model-undermining class (dispatch did not resolve, so
	// there is no call to compile) — the pipeline therefore refuses on the
	// diagnostics one stage before the promotion gate these rows used to
	// reach. The INTERPRETER runs them: the failure raises at the call, inside
	// the `do`, so the region's own handler catches it (see the note in
	// frontier-do-catch.tsv, and the check-vs-run divergence recorded in the
	// design note's §6). The L-DO promotion work below is still what would
	// graduate the SHAPE; these two rows can no longer witness it.
	docMod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`:  {why: "plan Phase 5 (L-DO part 2): variadic region under a def binding — now check-rejected first (failed fn-value dispatch)", failsWith: "check diagnostics"},
	docMod + `def msg (do [(false 5 M.dec) "no-raise"] error [dot code])  msg`: {why: "plan Phase 5 (L-DO part 2): same shape, no raise at this input — likewise check-rejected first", failsWith: "check diagnostics"},
	// PR #280 review's promotion-gate representative (the variation
	// differential's prefix-stack find): a BRANCH-VARIANT multi-out do body
	// (0-or-2 values per arm) is variadic without any raise in sight, and a
	// dirty-stack prefix forces its promotion — the same exact-arity seat
	// hazard as the fallible regions.
	`7 def b true  do [1 2 (if b [] [9 9])]`: {why: "PR #280 review: branch-variant multi-out do region promoted under a dirty-stack prefix", failsWith: "variadic result promoted to frame slots"},

	// NUR038 statement-seal twin-call matrix (frontier-nur038-seal.tsv):
	// semantically green under the seal + arrival barrier; compile-refused
	// on the pre-existing residual-ordering limitation — two dynamic call
	// results in one program residual exceed the Stage-1 lowering. Sound
	// interpreter fallback. Graduation = multi-dynamic-result residual
	// lowering; the rows then move to lang/spec/fn-value.tsv §6.
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end m.p 5 m.p 7`:                                {why: "NUR038 seal: twin value-call residual", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end m.p 1 m.p 2 m.p 3`:                          {why: "NUR038 seal: triple value-call residual", failsWith: "fn-value-call boundary"},
	`def g fn [[a:Any b:Any] [Any] [(a mul 100) add b]] end def m {g: g/r} end m.g 1 2 m.g 3 4`:      {why: "NUR038 seal: two-arg twin windows", failsWith: "fn-value-call boundary"},
	`import module [def p fn [[x:Any] [Any] [x]] export "M" {p: p/r}] end M.p 5 M.p 7`:               {why: "NUR038 seal: module-export twins (the original shape)", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end 5 m.p m.p 7`:                                {why: "NUR038 seal: stack form then forward form", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end m.p (1 add 2) m.p 7`:                        {why: "NUR038 seal: computed first argument", failsWith: "fn-value-call boundary"},
	`def m {l: ([x:Any] => [x])} end m.l 5 m.l 7`:                                                    {why: "NUR038 seal: lambda twins", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def h fn [[y:Any] [Any] [y]] end def m {p: f/r} end m.p 5 h 7`: {why: "NUR038 seal: value call then bare-word call", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end (m.p 5) (m.p 7)`:                            {why: "NUR038 seal: explicit paren seals", failsWith: "fn-value-call boundary"},
	`def f fn [[x:Any] [Any] [x]] end def m {p: f/r} end m.p 5 end m.p 7`:                            {why: "NUR038 seal: explicit end seals", failsWith: "fn-value-call boundary"},
	`def e fn [[] [Integer] [42] [x:Any] [Any] [x]] end def m {e: e/r} end m.e 5 m.e 7`:              {why: "NUR038 seal: mixed 0/1-arg overload twins (NUR035 guard)", failsWith: "fn value read from a container auto-dispatches"},

	// Namespace capture at a macro-expanded call site (the NUR038 wrapper
	// retirement's re-bucketed refusal — see frontier-capture-namespace.tsv):
	// an inner fn capturing a body-imported module namespace has no bakeable
	// operand home at the `parse` macro's expanded call site. Successor to
	// the graduated "closure captures a runtime-minted value" bucket (the
	// wrapper refused earlier, at capture-slot numbering). Full graduation =
	// a capture-slot lowering that materialises the namespace binding.
	`def zzvfn fn [[] [] [import "boru:parselang"  import "boru:string-util"  def calc (fn [[source:Any opts:Map] [List] [StringUtil.split ' ' (ParseLang.source source)]])  end  (parse calc {trace:true} 'x + y') get 1]] zzvfn`: {why: "inner fn captures the body-imported ParseLang namespace; no bakeable operand home at the macro-expanded call site", failsWith: "capture ParseLang of calc unreachable at a call site"},
	// GRADUATED 2026-07-17 (§9.1): the `do [M 3] error [dot code]` row
	// compiles — an identity-less dyn-body out (the module-export instance)
	// now mints a fresh ID at the record, restoring its tape placement and
	// event linkage, so the mark-window island owns the region as usual.
	// GRADUATED 2026-07-14 (the mark-window island, L-DO part 2b): the
	// error-over-the-variadic-region rows — do [(M.boom 5) "x"] / the [Any]
	// user-fn twin / the branch-arm nesting / the StructUtil.parse chained
	// leaf — compile natively: Finalize's markWindowShape opens an
	// OpStackMark before the region-starting do event and the residual
	// islands verbatim through OpCallDynMixedFromMark (rows moved to
	// lang/spec/bytecode-migrated.tsv; family pinned in
	// lang/go/bytecode_markwindow_test.go). The def-msg rows above and the
	// module-export row keep their sound refusals (a PROMOTED def read /
	// a non-event region entry decline the window).

	// NUR031 Module-descriptor equality (frontier-nur031-module-eq.tsv):
	// semantically green — the descriptor is an identity-equal opaque
	// handle since 2026-08-02 — but a `$module` synthetic read has no
	// bakeable operand home, so the operand reaching eq/deq carries no
	// static provenance. The PRE-fix binary refuses the identical shape:
	// the fix changed the answer, not the compile status. Graduation =
	// a static provenance representation for the module-namespace
	// synthetics (the capture-namespace family above); the rows then
	// move back into edge-modules-1.tsv and compare-restrict.tsv.
	`import module [export "M" {a:1}] M.$module eq M.$module`: {why: "NUR031: descriptor reflexive eq", failsWith: "operand of unknown provenance"},
	`import module [export "M" {a:1}] M.$module deq M.$module`: {why: "NUR031: descriptor reflexive deq", failsWith: "operand of unknown provenance"},
	`import module [export "A" {x:1}] import module [export "B" {y:2}] A.$module eq B.$module`: {why: "NUR031: distinct descriptors are not eq", failsWith: "operand of unknown provenance"},
	`import module [export "A" {x:1}] import module [export "B" {y:2}] A.$module deq B.$module`: {why: "NUR031: distinct descriptors are not deq", failsWith: "operand of unknown provenance"},
	`import "boru:array-util" import module [export "A" {x:1} export "B" {y:2}] import module [export "C" {z:3}] size (ArrayUtil.unique [A.$module B.$module C.$module])`: {why: "NUR031: the Module handle family in unique's DeqIndex", failsWith: "operand of unknown provenance"},
	`import module [export "A" {x:1} export "B" {y:2} export "C" {z:3}] A.$module eq C.$module`: {why: "NUR031: one module, many namespaces — one shared descriptor", failsWith: "operand of unknown provenance"},
	`import module [export "A" {x:1} export "B" {y:2}] A.$module deq B.$module`: {why: "NUR031: sibling namespaces of ONE module share a descriptor (deq mirrors eq)", failsWith: "operand of unknown provenance"},
	`import "boru:test" Test.$module eq Assert.$module`: {why: "NUR031: a native module's sibling namespaces share one descriptor", failsWith: "operand of unknown provenance"},
	`import "boru:string-util" import "boru:string-util" StringUtil.$module eq StringUtil.$module`: {why: "NUR031: repeat import is a cache no-op — same descriptor instance", failsWith: "operand of unknown provenance"},
	`import "boru:string-util" def a StringUtil.$module undef StringUtil import "boru:string-util" a eq StringUtil.$module`: {why: "NUR031: re-import after undef mints a FRESH descriptor (identity is per-import-instance)", failsWith: "operand of unknown provenance"},

	// Net drivers — plan Phase 5: per-iteration mark/collect in the for: lowering.

	// GRADUATED 2026-07-14 (L-EACH, plan Phase 5): the three forward-drift
	// rows compile natively — errorReturnsFn narrows the catch result to
	// dynamic(join(pass-through, handler-residual)), so the String catch-all
	// overload is disjoint and check mode selects the interpreter's forward
	// collection (rows moved to lang/spec/bytecode-migrated.tsv; family
	// pinned in lang/go/bytecode_edge_findings_test.go with the genuinely
	// wide-join negative keeping the drift refusal).

	// do-unit registry replay — was a MISCOMPILE (variation sweep,
	// 2026-07-13); now a SOUND REFUSAL (drift graduated the same day): the
	// bake decision declines a body carrying a capitalised def
	// (bodyHasReplayHazard), so the interpreter owns the shape with full
	// parity. Full graduation = the Phase 6 JIT detached-unit cache compiles
	// these bodies as units (the check-time install becomes the only
	// install), at which point the rows compile and this entry deletes.
	// GRADUATED 2026-07-14: the do-unit registry-replay rows — do-def LEAK
	// fidelity (RunCarrierBodyKeepDefs) lets the closure re-analysis
	// shadow-rebind instead of tripping the parts conflict, so the typed-def
	// bodies compile as closure units with byte-identical results (rows
	// moved to lang/spec/bytecode-migrated.tsv; leak-semantics edges pinned
	// in lang/go/bytecode_replayhazard_test.go).

	// GRADUATED 2026-07-14: the L-JOIN recursive branch-join row — the
	// refusal was the disjunct-distribution recording per-alternative
	// (disjunctPartitionReturns combos under the armed recording); the fix
	// suspends the combo probes and records ONE CALL_USER with the original
	// args (carrier.go, gated by disjunctCombosTakeSig). Row moved to
	// lang/spec/bytecode-migrated.tsv; the family is pinned in
	// lang/go/bytecode_ljoin_test.go.
}

type frontierEntryLS struct {
	why       string
	failsWith string
}

// TestFrontierSpecCompiled — the compile frontier: an unledgered row must
// compile NATIVELY (no island) and run with byte-identical parity; a
// ledgered row must refuse with the pinned reason (stale → graduate; drift →
// re-diagnose).
func TestFrontierSpecCompiled(t *testing.T) {
	for _, row := range loadFrontierRows(t) {
		err := frontierRowCompiles(row.input)
		key := row.input
		entry, ledgered := frontierCompileLedger[key]
		loc := fmt.Sprintf("%s:%d", row.file, row.line)
		switch {
		case !ledgered && err != nil:
			t.Errorf("%s: frontier row must COMPILE (not ledgered): %v\n  input: %s", loc, err, row.input)
		case ledgered && err == nil:
			t.Errorf("%s: stale compile-ledger entry — the row now compiles; graduate it (delete the entry; usually move the row into the main lang/spec corpus).\n  was red because: %s", loc, entry.why)
		case ledgered && entry.failsWith == "":
			t.Errorf("%s: unpinned compile-ledger row — record the failure mode. Observed: %v", loc, err)
		case ledgered && !strings.Contains(err.Error(), entry.failsWith):
			t.Errorf("%s: compile failure MODE drifted:\n  got:    %v\n  pinned: %q\nre-diagnose before editing the ledger", loc, err, entry.failsWith)
		}
	}
	for key := range frontierCompileLedger {
		found := false
		for _, row := range loadFrontierRows(t) {
			if row.input == key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("orphan compile-ledger entry (no such frontier row): %.80s…", key)
		}
	}
}

// frontierRowCompiles asserts the TARGET compile behavior for one row:
// a native (island-free) Program plus byte-identical value/error parity.
func frontierRowCompiles(input string) error {
	a, err := lang.New()
	if err != nil {
		return err
	}
	a.SetClock(specClock)
	prog, reason, _, cerr := a.CompileCheck(input)
	if cerr != nil {
		return fmt.Errorf("check error: %v", cerr)
	}
	if prog == nil {
		return fmt.Errorf("refused: %s", reason)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		return fmt.Errorf("islanded: program embeds an OpFallback span")
	}
	b, err := lang.New()
	if err != nil {
		return err
	}
	b.SetClock(specClock)
	gotC, compiled, errC := b.RunCompiled(input)
	if !compiled {
		return fmt.Errorf("did not run compiled (err=%v)", errC)
	}
	c, err := lang.New()
	if err != nil {
		return err
	}
	c.SetClock(specClock)
	gotI, errI := c.RunInterp(input)
	if fmt.Sprint(errC) != fmt.Sprint(errI) {
		return fmt.Errorf("error parity: compiled %v vs interp %v", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		return fmt.Errorf("value parity: compiled %v vs interp %v", gotC, gotI)
	}
	return nil
}

// refusalRowLedger pins the knownRefusals rows' TARGET failure modes: each
// must eventually compile via the sound runtime re-dispatch mechanism (plan
// Phase 3, OpDispatchRematch) and raise the interpreter-identical error.
// DERIVED from knownRefusals — the single source of truth for the row
// sources — so graduation is auto-coupled: deleting a knownRefusals entry
// drops its ledger row here, flipping this test's assertion for that row to
// the target (compile + byte-identical parity). The failure mode is the
// LEADING CLAUSE of the knownRefusals reason text ("branch leaves extra
// values (…)" pins "branch leaves extra values"): the remaining row's
// dispatch half already records an offset-form rematch, so its refusal
// signature is the branch-residual seat, not the dispatch recovery — a row
// developing a different failure mode trips the drift arm.
var refusalRowLedger = func() map[string]frontierEntryLS {
	m := make(map[string]frontierEntryLS, len(knownRefusals))
	for input, why := range knownRefusals {
		mode := why
		if i := strings.Index(mode, " ("); i > 0 {
			mode = mode[:i]
		}
		m[input] = frontierEntryLS{
			why:       "plan Phase 3/5 (OpDispatchRematch + variadic-region merge): " + why,
			failsWith: mode,
		}
	}
	return m
}()

// TestFrontierRefusalRowsCompile asserts the TARGET for every knownRefusals
// row: CompileCheck yields a Program and RunCompiled matches the
// interpreter's error byte-for-byte. All 9 are expected-red until Phase 3
// lands, ratcheting down row-by-row in lockstep with knownRefusals.
func TestFrontierRefusalRowsCompile(t *testing.T) {
	for input := range knownRefusals {
		err := frontierRowCompiles(input)
		entry, ledgered := refusalRowLedger[input]
		switch {
		case !ledgered && err != nil:
			t.Errorf("knownRefusals row must COMPILE (not ledgered — did Phase 3 graduate it?): %v\n  input: %.100s", err, input)
		case ledgered && err == nil:
			t.Errorf("stale refusal-ledger entry — the row now compiles; graduate BOTH ledgers (delete here AND in knownRefusals).\n  input: %.100s\n  was red because: %s", input, entry.why)
		case ledgered && entry.failsWith == "":
			t.Errorf("unpinned refusal-ledger row — record the failure mode. Observed: %v\n  input: %.100s", err, input)
		case ledgered && !strings.Contains(err.Error(), entry.failsWith):
			t.Errorf("refusal row failure MODE drifted:\n  got:    %v\n  pinned: %q\n  input: %.100s", err, entry.failsWith, input)
		}
	}
	for input := range refusalRowLedger {
		if _, ok := knownRefusals[input]; !ok {
			t.Errorf("orphan refusal-ledger entry (row left knownRefusals): %.80s…", input)
		}
	}
}
