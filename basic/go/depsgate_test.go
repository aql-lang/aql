package basic

import (
	"os"
	"strings"
	"testing"
)

// TestBasicDependsOnItsRealPieces pins ADR-013 rule 1: basic's go.mod
// requires exactly the modules it uses — core and parser — and NO other boru
// sibling. go.mod is where the rule lives, since Go's import resolution
// enforces what is written there.
//
// eng is deliberately ABSENT. Until the 2026-08-07 amendment this gate
// asserted "eng only", which was true when eng was one module holding the
// kernel, checker, compiler and parser. After those cuts basic reached them
// through eng/go/aliases_*.go — a migration shim — so the gate was pinning a
// dependency that carried no symbols while the real ones were invisible to
// it. basic now imports them directly.
//
// compiler is absent too, and for a different reason: those references were a
// SEAM GAP, not a dependency. basic's control words record branch and loop
// fragments through core.EmitRecorder, but that interface lacked the
// branch/loop group, so recorderState had to downcast to *compiler.EmitState.
// Widening the core interface removed the downcast and the requirement
// together. An inactive recorder is correct behaviour — nothing to record,
// program still runs interpreted — which is what makes it a real seam.
//
// check is absent as of the 2026-08-08 amendment, and this reverses what the
// 2026-08-07 amendment concluded. That amendment read basic's 23 check
// symbols over 63 call sites as STRUCTURAL — every native control word has an
// analysis half, written (it argued) in the checker's vocabulary, with no
// correct inactive default for "don't narrow, don't join". The premise turned
// out to be wrong on inspection: 21 of the 23 were pure functions over CORE
// types — core Values, core Types, r.Defs, and CheckState, which core already
// owns — that merely happened to live in check/go. They were not the
// checker's vocabulary; they were the interpreter's, filed in the wrong
// module. So they moved down rather than being forwarded: the join lattice to
// core/go/carrier_join.go, the body runners to carrier_body.go, guard
// narrowing to guard_narrow.go + guard_predicate.go, dead-overload detection
// to deadsig.go, the spread carrier to carrier_spread.go, the typed-def make
// record to record_typed_def.go, and the deduping emitters into
// check_state.go beside the state they read. That is the change basic/go's own
// guide prescribed for this case — move code to where its types already live,
// not add indirection.
//
// The genuine remainder is TWO symbols, AnalyseFnBody and AnalyseLoopBody, and
// they are the analysis PASS itself — memoised per call shape, recursion-
// bailing, quota-capped, Kleene-iterated to a fixed point. Those stay in check
// and are reached through core-owned S1 slots (core.RunFnBodyAnalysis /
// core.RunLoopBodyAnalysis), which is a seam and not a mailbox for a reason
// the earlier amendment did not have in view: a check-less build runs NO
// analysis at all. AnalysisImpl.ReturnsFn returns nil there, so no ReturnsFunc
// ever executes and neither accessor is reached. The nil default is therefore
// the same no-analysis regime every other S1 slot already defines — and nil is
// in-band regardless, since AnalyseFnBody documents an empty result as "the
// analyser aborted, treat as an Any carrier".
//
// parser stays: basic's register_test.go imports it, and a test import needs a
// require line like any other.
func TestBasicDependsOnItsRealPieces(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	var siblings []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "require ")
		if !strings.HasPrefix(line, "github.com/boru-lang/boru/") {
			continue
		}
		mod := strings.Fields(line)[0]
		siblings = append(siblings, mod)
	}
	allowed := map[string]bool{
		"github.com/boru-lang/boru/core/go":   true,
		"github.com/boru-lang/boru/parser/go": true,
	}
	for _, mod := range siblings {
		if !allowed[mod] {
			t.Errorf("ADR-013: basic/go must depend on core/parser only; go.mod references %s", mod)
		}
	}
	// The negative half, spelled out per module: an allowlist only rejects
	// names it does not know, so a regression that re-adds one of these would
	// otherwise fail with a generic message that hides WHY it is banned.
	banned := map[string]string{
		"github.com/boru-lang/boru/eng/go": "eng is the facade, not a piece — " +
			"import the module that owns the symbol",
		"github.com/boru-lang/boru/compiler/go": "recording goes through " +
			"core.EmitRecorder; if a method is missing, widen the interface " +
			"rather than downcasting to *compiler.EmitState",
		"github.com/boru-lang/boru/check/go": "basic defines language types and " +
			"words, and the interpreter primitives in core are enough for that — " +
			"a word's analysis half belongs in core's carrier vocabulary " +
			"(carrier_join.go, carrier_body.go, guard_narrow.go), and the two " +
			"real pass drivers are reached through core.RunFnBodyAnalysis / " +
			"core.RunLoopBodyAnalysis. If something here seems to need the " +
			"checker, decide which of those two it is",
	}
	for _, mod := range siblings {
		if why, bad := banned[mod]; bad {
			t.Errorf("ADR-013: basic/go must not depend on %s — %s", mod, why)
		}
	}
	if len(siblings) == 0 {
		t.Fatalf("ADR-013: expected core/parser in go.mod, found no boru sibling at all")
	}
}

// TestBasicSourceNamesNoChecker is the source-level twin of the go.mod gate:
// a `replace` directive plus a workspace `use` can make a sibling importable
// even with the require line gone, so the manifest alone is not proof. No
// file in this package may name the check module at all.
func TestBasicSourceNamesNoChecker(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		// This file quotes the path in its own ban message; skip itself.
		if name == "depsgate_test.go" {
			continue
		}
		if strings.Contains(string(src), "boru/check/go") {
			t.Errorf("ADR-013: %s names the check module — see TestBasicDependsOnItsRealPieces", name)
		}
	}
}
