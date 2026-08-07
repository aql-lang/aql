package basic

import (
	"os"
	"strings"
	"testing"
)

// TestBasicDependsOnItsRealPieces pins ADR-013 rule 1: basic's go.mod
// requires exactly the modules it uses — core, check, parser — and NO other
// boru sibling. go.mod is where the rule lives, since Go's import resolution
// enforces what is written there.
//
// eng is deliberately ABSENT. Until the 2026-08-07 amendment this gate
// asserted "eng only", which was true when eng was one module holding the
// kernel, checker, compiler and parser. After those cuts basic reached them
// through eng/go/aliases_*.go — a migration shim — so the gate was pinning a
// dependency that carried no symbols (measured: 700 core, 45 check, 9
// compiler, 0 eng) while the real ones were invisible to it. basic now
// imports them directly.
//
// compiler is absent too, and for a different reason: those 9 references
// were a SEAM GAP, not a dependency. basic's control words record branch and
// loop fragments through core.EmitRecorder, but that interface lacked the
// branch/loop group, so recorderState had to downcast to *compiler.EmitState.
// Widening the core interface removed the downcast and the requirement
// together. An inactive recorder is correct behaviour — nothing to record,
// program still runs interpreted — which is what makes it a real seam.
//
// check, by contrast, is STRUCTURAL and stays: 23 symbols over 63 call sites
// (RunCarrierBody*, ApplyGuardNarrowing, InstallJoinedDefs, JoinCarriers,
// AnalyseFnBody, AnalyseLoopBody, RecordTypedDefMake, DeadSignatures). basic
// implements if / case / for / fn / def, and every native control word has an
// analysis half written in the checker's vocabulary. Forwarding those through
// a core table is mechanically possible — they are all core-typed — but it
// would be a mailbox, not a seam: there is no correct inactive default for
// "don't narrow, don't join", so basic would still need check at run time
// while go.mod stopped saying so. See basic/go/CLAUDE.md.
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
		"github.com/boru-lang/boru/check/go":  true,
		"github.com/boru-lang/boru/parser/go": true,
	}
	for _, mod := range siblings {
		if !allowed[mod] {
			t.Errorf("ADR-013: basic/go must depend on core/check/parser only; go.mod references %s", mod)
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
	}
	for _, mod := range siblings {
		if why, bad := banned[mod]; bad {
			t.Errorf("ADR-013: basic/go must not depend on %s — %s", mod, why)
		}
	}
	if len(siblings) == 0 {
		t.Fatalf("ADR-013: expected core/check/parser in go.mod, found no boru sibling at all")
	}
}
