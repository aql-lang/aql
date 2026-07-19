package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/test/go/specrunner"
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
	docMod + `def msg (do [(true 5 M.dec) "no-raise"] error [dot code])  msg`:  {why: "plan Phase 5 (L-DO part 2): variadic region under a def binding", failsWith: "residual shape beyond Stage 1"},
	docMod + `def msg (do [(false 5 M.dec) "no-raise"] error [dot code])  msg`: {why: "plan Phase 5 (L-DO part 2): same shape, no raise at this input", failsWith: "residual shape beyond Stage 1"},
	docMod + `do [M 3] error [dot code]`:                                       {why: "plan Phase 5 (L-DO part 2): a module-export VALUE in the variadic region is not event-produced, so the mark-window probe declines and the residual materialise arm refuses", failsWith: "residual value not statically materialisable"},
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
