package basic

import (
	"os"
	"strings"
	"testing"
)

// TestBasicDependsOnEngOnly pins ADR-013 rule 1: basic's go.mod
// requires eng/go (plus parser/go, per the rule's 2026-08-07 amendment)
// and NO other boru sibling — and go.mod is where the rule lives, since
// Go's import resolution enforces what is written there.
//
// parser/go is allowed because it USED to be eng/go/parser, a package
// inside the eng module: "requires eng" already granted it, and the
// parser cut changed the module map without adding any dependency that
// was not already there. Nothing else may join this list.
// The negative direction (nothing upward: basic never imports lang or
// cmd) is the same gate seen from the other side — an upward require
// would have to appear in this file to compile.
func TestBasicDependsOnEngOnly(t *testing.T) {
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
		"github.com/boru-lang/boru/eng/go":    true,
		"github.com/boru-lang/boru/parser/go": true, // ADR-013 amendment, 2026-08-07
	}
	for _, mod := range siblings {
		if !allowed[mod] {
			t.Errorf("ADR-013: basic/go must depend on eng/go and parser/go only; go.mod references %s", mod)
		}
	}
	if len(siblings) == 0 {
		t.Fatalf("ADR-013: expected the eng/go requirement in go.mod, found no boru sibling at all")
	}
}
