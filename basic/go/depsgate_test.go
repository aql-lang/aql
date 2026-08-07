package basic

import (
	"os"
	"strings"
	"testing"
)

// TestBasicDependsOnItsRealPieces pins ADR-013 rule 1: basic's go.mod
// requires exactly the modules it uses — core, check, compiler, parser —
// and NO other boru sibling. go.mod is where the rule lives, since Go's
// import resolution enforces what is written there.
//
// eng is deliberately ABSENT. Until the 2026-08-07 amendment this gate
// asserted "eng only", which was true when eng was one module holding the
// kernel, checker, compiler and parser. After those cuts basic reached them
// through eng/go/aliases_*.go — a migration shim — so the gate was pinning a
// dependency that carried no symbols (measured: 700 core, 45 check, 9
// compiler, 0 eng) while the three real ones were invisible to it. basic now
// imports them directly.
//
// check and compiler are not incidental: basic implements if / case / for /
// fn / def, and control flow has to join carriers and record branch and loop
// fragments. That is not expressible against core alone.
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
		"github.com/boru-lang/boru/core/go":     true,
		"github.com/boru-lang/boru/check/go":    true,
		"github.com/boru-lang/boru/compiler/go": true,
		"github.com/boru-lang/boru/parser/go":   true,
	}
	for _, mod := range siblings {
		if !allowed[mod] {
			t.Errorf("ADR-013: basic/go must depend on core/check/compiler/parser only; go.mod references %s", mod)
		}
	}
	// The negative half: eng must NOT come back. A future edit that reaches
	// for the facade instead of the piece would otherwise pass silently,
	// since the allowlist above only rejects names it does not know.
	for _, mod := range siblings {
		if mod == "github.com/boru-lang/boru/eng/go" {
			t.Error("ADR-013: basic/go must not depend on eng/go — import the piece that owns the symbol")
		}
	}
	if len(siblings) == 0 {
		t.Fatalf("ADR-013: expected the eng/go requirement in go.mod, found no boru sibling at all")
	}
}
