// dispatch_agreement_census_test.go — the admission-agreement census
// (design/FULL-COMPILATION.0.md §10, the Tier-1 instrument its correction
// note names).
//
// boru has TWO signature matchers. The interpreter plans and selects with
// Engine.MatchSignature (forward collection, barrier splits, /q word
// preference, the deferred-atom fallback); the VM re-matches at runtime
// with the kernel window matcher core.MatchSignature (signature.go), and
// §6.5 specifies OpDispatchGeneric as THAT matcher over the claimed
// window. Today a disagreement between them is at worst a defer; when the
// generic op EXECUTES on match, a disagreement is a wrong answer. This
// census measures the agreement surface over every interpreter dispatch
// the corpus performs, from the probe seam (core.InstallDispatchProbe),
// and holds it to a COMMITTED ledger — the bind-ledger discipline: zero
// unledgered divergence classes, each ledgered class carrying its reason
// and its fate (a §6.5 admission rule to reconcile, or a deliberate
// planner-only behavior the generic lane must re-create).
//
// THE WINDOW MAPPING IS THE POINT, so it is explicit: the two matchers
// consume OPPOSITE conventions. The planner returns positions[] as
// TAPE-ABSOLUTE slots in signature order; the kernel matcher reads
// window[i] as sig position i (exactly how the VM builds its windows:
// window[i] = stack[len-1-i], eng/go/vm.go). So the census rebuilds the
// claimed window as window[i] = Tape.At(positions[i]) and hands it to the
// kernel with the VM's own exact-arity WordInfo{ArgCount: n}. Positions
// the plan filled with tokens that only resolve at ARRIVAL (a forward
// Word bound to a value, a paren group) are not comparable at plan time —
// those windows are skipped and COUNTED, never silently dropped.
package langspec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// agreementLedger is the committed divergence ledger: every observed
// divergence class must appear here with a reason, and every entry here
// must still be observed (a stale entry means the divergence closed —
// delete the entry so the ledger stays exact). Keyed by class alone;
// per-word detail is in the log for diagnosis.
var agreementLedger = map[string]string{
	// Measured 2026-08-31 over 337,975 corpus dispatches: 236,191 of
	// 236,214 comparable windows agree (99.99%); the 23 divergences fall
	// into the two classes below, 11 word-shapes total, every one named in
	// the census log. Each is a §10 T2 reconciliation candidate — one
	// commit and one spec row per closed shape — and none is reachable by
	// today's compiled lanes (no generic dispatch executes yet; the VM's
	// kernel-matcher calls are poly re-matches over recorder-proven
	// windows, none of these shapes).
	"kernel-different-overload": "split-sensitive selection + type-literal admission: for a window " +
		"two overloads accept, the planner's choice depends on the CALL FORM (forward-phase candidate " +
		"order — the two-map Store shapes create/load/remove/update, emit's Function+map) or on bare " +
		"type-node operands tripping the planner-vs-kernel rejectsTypeLiteral scope difference (gt/lt " +
		"over Number/Integer, ProperString/EmptyString). The generic lane must re-create the planner's " +
		"selection from OpCollect's descriptor (which carries the split), never from the bare window.",
	"kernel-no-match": "plan-time artifacts below the window surface and /q-pattern-heavy sigs: a " +
		"container operand holding a still-unresolved word ({a:word(true)} — resolved by arrival at " +
		"runtime, so the plan-time window is not the runtime window), and def/use/remove(1-arg) " +
		"whose QuoteArgs/pattern admission the kernel's pattern loop answers differently from the " +
		"planner's patternsOk.",
}

func TestDispatchAdmissionAgreementCensus(t *testing.T) {
	type stats struct {
		agreed, engineNoMatch, speculative int
		skipped                            map[string]int
		diverged                           map[string]int
		examples                           map[string]string
	}
	s := stats{skipped: map[string]int{}, diverged: map[string]int{}, examples: map[string]string{}}
	var mu sync.Mutex

	// The probe RE-ENTERS: the kernel rematch below evaluates predicate-typed
	// slots (RunPredicate), whose boru bodies dispatch — firing this probe
	// again on the same goroutine. A lock held across the rematch therefore
	// self-deadlocks (measured: the first census run hung exactly there), and
	// the nested dispatches are the census's own artifact, not corpus
	// dispatches. One atomic busy flag suppresses them — and any concurrent
	// fork's events while a rematch runs — COUNTED, never silently dropped.
	var busy atomic.Bool
	var busySkips atomic.Int64 // atomic, NOT mu: the outer probe call holds mu
	uninstall := core.InstallDispatchProbe(func(e *core.Engine, fn *core.FnDefInfo, w core.WordInfo, sig *core.Signature, positions []int, specAt int) {
		if !busy.CompareAndSwap(false, true) {
			busySkips.Add(1)
			return
		}
		defer busy.Store(false)
		mu.Lock()
		defer mu.Unlock()
		if sig == nil {
			// The dispatch failed — the error/recovery path follows, not a
			// generic-lane execution. Counted, not compared: the kernel's
			// view of failures is OpDispatchRematch's territory.
			s.engineNoMatch++
			return
		}
		if specAt >= 0 {
			// A forward slot filled with a WORD that dispatches at runtime
			// (the speculation marker): the plan-time window is not the
			// runtime window by the plan's own admission.
			s.speculative++
			return
		}
		if sig.Fallback {
			// A 0-arg catch-all firing to raise "no matching signature" —
			// an error path, not a surface the generic lane executes.
			s.skipped["fallback-sig"]++
			return
		}
		window := make([]core.Value, len(positions))
		for i, p := range positions {
			if p < 0 || p >= e.Tape.Len() {
				s.skipped["position-out-of-tape"]++
				return
			}
			v := e.Tape.At(p)
			if core.IsWord(v) || core.IsForward(v) || core.IsMark(v) || core.IsMove(v) {
				// A TAPE TOKEN, not a runtime value: a forward Word matched
				// by classification (/q capture, Defs.Top resolution)
				// resolves at ARRIVAL — the runtime window holds the Atom or
				// bound value, so the plan-time token is not comparable.
				// (These carry payloads, so IsConcrete alone admits them —
				// the first census run's two headline "divergences" were
				// exactly this contamination.)
				s.skipped["token-at-plan-time"]++
				return
			}
			if !core.IsConcrete(v) && !core.IsBareTypeNode(v) {
				// A paren group or other structure the plan matched by
				// classification — not a value the kernel matcher can be
				// handed at plan time.
				s.skipped["unresolved-at-plan-time"]++
				return
			}
			window[i] = v
		}
		mr := core.MatchSignature(fn.Signatures, window, core.WordInfo{ArgCount: len(window)})
		class := ""
		switch {
		case mr == nil || mr.Sig == nil:
			class = "kernel-no-match"
		case mr.Sig != sig:
			class = "kernel-different-overload"
		default:
			s.agreed++
			return
		}
		s.diverged[class]++
		perWord := class + "/" + fn.Name
		if _, ok := s.examples[perWord]; !ok || len(s.examples) < 40 {
			s.examples[perWord] = fmt.Sprintf("word=%s arity=%d window=%s", fn.Name, len(window), renderWindow(window))
		}
	})
	defer uninstall()

	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	rows := 0
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
			a, aerr := lang.New()
			if aerr != nil {
				continue
			}
			a.SetClock(specClock)
			_, _ = a.RunInterp(strings.TrimSpace(parts[0]))
			rows++
		}
		_ = f.Close()
	}

	mu.Lock()
	defer mu.Unlock()
	s.skipped["probe-busy-reentrant"] = int(busySkips.Load())
	total := s.agreed + s.engineNoMatch + s.speculative
	for _, n := range s.skipped {
		total += n
	}
	for _, n := range s.diverged {
		total += n
	}
	t.Logf("dispatch admission agreement: %d rows, %d dispatches probed — %d agreed, %d engine-no-match, %d speculative, skipped %v, diverged %v",
		rows, total, s.agreed, s.engineNoMatch, s.speculative, s.skipped, s.diverged)
	var perWord []string
	for k := range s.examples {
		perWord = append(perWord, k)
	}
	sort.Strings(perWord)
	for _, k := range perWord {
		t.Logf("    %s  %s", k, s.examples[k])
	}

	if s.agreed == 0 {
		t.Fatal("no dispatch was compared — the probe or the eligibility rules are broken, not the corpus")
	}
	for c := range s.diverged {
		if _, ok := agreementLedger[c]; !ok {
			t.Errorf("UNLEDGERED divergence class %q (×%d): the planner and the kernel matcher disagree on an admission the generic lane will inherit — ledger it with its reason, or reconcile it (one commit, one spec row, per §10's T2)",
				c, s.diverged[c])
		}
	}
	for c, why := range agreementLedger {
		if s.diverged[c] == 0 {
			t.Errorf("stale ledger entry %q (%s): the divergence no longer occurs — delete the entry so the ledger stays exact", c, why)
		}
	}
}

func renderWindow(window []core.Value) string {
	parts := make([]string, 0, len(window))
	for _, v := range window {
		s := v.String()
		if len(s) > 24 {
			s = s[:24] + "…"
		}
		parts = append(parts, s)
	}
	return "[" + strings.Join(parts, " ") + "]"
}
