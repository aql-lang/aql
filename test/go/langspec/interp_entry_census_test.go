// interp_entry_census_test.go measures the island the OpFallback ceiling
// cannot see.
//
// `TestCompiledCoverage` reports 0 islanded, and that number is real but
// narrow: it counts programs whose DISASSEMBLY embeds an OpFallback span. It
// cannot see a `CallBoru` made INSIDE a native handler, because no opcode
// records one — and that is where interpretation actually survives. Every
// predicate dispatch takes `InvokeCallback:callboru` today with no ledger row
// and no island flag (design/FULL-COMPILATION.0.md §2.3, §6.3).
//
// So "0 islanded" is a claim about the metric, not about the runtime. This
// census is the claim about the runtime: RUN each corpus row compiled with the
// InterpEntry hook armed — Stage 1 built it for exactly this — and count the
// entries that are UNATTRIBUTED, i.e. interpreter execution the end-state
// invariant does not permit. Check-mode entries are excluded: the compiler
// front end running RunInCheckMode words is attributed by construction.
//
// T2 ("No islands") is not satisfiable while this number is non-zero, whatever
// the OpFallback ceiling says. The ceiling below is a DOWNWARD ratchet, like
// refusalSiteCeiling: it only falls.
package langspec

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// interpEntryRowCeiling is the number of corpus rows that, when RUN compiled,
// re-enter the interpreter through an unattributed seam.
//
// First measured 2026-08-28 at 184 of 7180 rows that run compiled (959
// entries). Now 130, after six changes on the same day. The seam spread — the
// shape of the debt, not a second ceiling — and what each moved:
//
//	                         first    (a)    (b)    (c)    (d)    (e)    (f)
//	Engine.Run                 501    477    453    443    433    439    425
//	CallBoru                   275    251    251    251    251    251    238
//	vm:island                   66     66     48     48     39     39     39
//	runPooledSub                37     37     35     35     35     37     36
//	RunResolved                 31     31     31     31     31     31     31
//	vm:island-resolved          21     21     21     11     10     10     10
//	InvokeCallback:callboru     28      4      4      4      4      4      6
//	                          ----   ----   ----   ----   ----   ----   ----
//	rows                       184    163    151    141    131    131    130
//
// (a) Foreign detached units became hostable mid-run
// (eng/go/vm_foreign_unit.go). The InvokeCallback column is that one: those 24
// entries were predicate bodies that HAD compiled to units and ran on the
// interpreter anyway, because the mid-run nested path declined every ref whose
// Program was not the running one — which a detached ref never is.
//
// (b) The Apply kernel (eng/go/vm_dyn_apply.go + compiler stampFnConst): a
// fn-value CONST carries its compiled unit, and a dynamic apply ENTERS that
// unit as a frame instead of islanding the callee. The `vm:island` column is
// that one, and it is the column the OpFallback ceiling can never see — those
// programs disassemble with `fallbacks=0` and the island lives inside the
// dynamic-apply opcode.
//
// (c) The Apply kernel reached CALL_DYN_FRAME's replay window in its simplest
// shape — an empty resolved prefix and a token region that is a fn followed by
// plain data. That is the `vm:island-resolved` column. A non-empty prefix keeps
// the island by contract: the fn stack-collects from the prefix as well as
// forward-collecting the region, so the frame would bind a different arg set
// than the interpreter assembles.
//
// (d) stampFnConst descends into list and map CONSTS, so a fn read out of a
// container carries its unit too. Measured before it did: of the island rows
// the top-level stamp left, roughly four in five were exactly that shape —
// `def m {f: (fn …)}  m.f 5`, `def ops {f: inc/v}  ops.f 5`, a class field
// method.
//
// (e) Emit-state isolation for the stamp — the one column that goes UP, and it
// should be read as the price of a correctness fix rather than a regression.
// stampFnConst now declines a stamp that would register a dynamic-scope name in
// the enclosing program, so a handful of bodies that used to carry a unit take
// the apply island again: Engine.Run +6, runPooledSub +2, no row moved. The
// alternative was keeping a stamp that MASKED a miscompile (see
// design/FULL-COMPILATION.0.md, "Container descent, and the mask it nearly
// shipped"). A seam count is not a score.
//
// (f) The seven native-callback sites that used CallBoruFn — `filter`'s
// Function form, the map-lambda each/fold bodies, core walk / StructUtil.walk,
// IO.mount's fileops handlers, boru:parse's matcher and action callbacks, and
// the fn-util words — now call InvokeCallbackFn, so a stamped body runs on the
// VM instead of never being offered to it. CallBoruFn is deleted: it existed to
// keep those bodies OFF the VM, which is the fence full compilation removes.
//
// Every drop carried Engine.Run with it, because an island is a nested Run.
//
// WHAT THE CallBoru COLUMN ACTUALLY IS, measured by probe at (f) — because it
// had not moved through four fixes and read as the largest block of debt:
//
//	Test.property's generator/property calls   224
//	core_helpers foreign-registry fn dispatch    8
//	InvokeCallback's own interpreter fallback    6
//
// So it is not broad interpretation debt. It is two or three `module-test.tsv`
// rows amplified by an iteration count — Test.property runs its bodies ~100
// times per row, and the seam counts INVOCATIONS while the ceiling counts ROWS.
// Those bodies are raw QUOTATIONS, not fn values: the module already routes a
// compiled sig through InvokeCallback and only falls back when the property was
// written as `[body]` tokens, which carry no unit to offer. Compiling them is
// Stage 7 (runtime compilation everywhere), not a seam flip, and it is worth
// perhaps three rows. Read the seam counts as a shape, never as a priority
// order: a big number here can be one row in a loop.
//
// Lower it whenever it falls. Raising it means a change put interpretation
// back into compiled programs, which is the one thing the compilation mission
// rules out — so a rise wants a design note, not a bigger number.
const interpEntryRowCeiling = 130

func TestInterpEntryCensus(t *testing.T) {
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seams := map[string]int{}
	perFile := map[string]int{}
	rows, dirty, ran := 0, 0, 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, ferr := os.Open(filepath.Join(specDir, e.Name()))
		if ferr != nil {
			t.Fatal(ferr)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			rows++
			n, seen, ok := runWithEntryHook(t, strings.TrimSpace(parts[0]))
			if !ok {
				continue // refused or check-error: the interpreter owns it by design
			}
			ran++
			if n == 0 {
				continue
			}
			dirty++
			perFile[e.Name()]++
			for s, c := range seen {
				seams[s] += c
			}
		}
		f.Close()
	}

	t.Logf("interp-entry census: %d rows, %d ran compiled, %d with UNATTRIBUTED interpreter entries (ceiling %d)",
		rows, ran, dirty, interpEntryRowCeiling)
	for _, s := range sortedKeys(seams) {
		t.Logf("   seam %-28s %d", s, seams[s])
	}
	for _, f := range sortedKeys(perFile) {
		if perFile[f] >= 5 {
			t.Logf("   file %-34s %d rows", f, perFile[f])
		}
	}

	if dirty > interpEntryRowCeiling {
		t.Errorf("interp-entry census %d exceeds ceiling %d — a change put interpretation BACK "+
			"into compiled programs. The OpFallback island ceiling cannot see this (it counts "+
			"disassembly spans, not a CallBoru inside a handler), so raising the number here is "+
			"not a bookkeeping fix: it is the invariant T2 forbids", dirty, interpEntryRowCeiling)
	}
	if dirty < interpEntryRowCeiling {
		t.Errorf("interp-entry census %d is BELOW the ceiling %d — the ratchet tightened, "+
			"lower interpEntryRowCeiling to %d", dirty, interpEntryRowCeiling, dirty)
	}
}

// runWithEntryHook runs one row compiled with the interpreter-entry hook armed
// and returns the unattributed entry count. ok is false when the row did not
// run compiled at all — a refusal or a static check error, where the
// interpreter owning the program is the designed behaviour and not an island.
func runWithEntryHook(t *testing.T, src string) (int, map[string]int, bool) {
	t.Helper()
	a, err := lang.New()
	if err != nil {
		t.Fatal(err)
	}
	a.SetClock(specClock)
	var mu sync.Mutex
	seen := map[string]int{}
	total := 0
	disarm := a.ArmInterpEntryHook(func(ev lang.InterpEntry) {
		if ev.CheckMode || ev.Attribution != "" {
			return
		}
		mu.Lock()
		seen[ev.Seam]++
		total++
		mu.Unlock()
	})
	_, compiled, _ := a.RunCompiled(src)
	disarm()
	mu.Lock()
	defer mu.Unlock()
	return total, seen, compiled
}
