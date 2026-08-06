package basic

import (
	"os"
	"strings"
	"testing"
)

// TestBasicDependsOnEngOnly pins ADR-013 rule 1: basic's go.mod
// requires eng/go and NO other boru sibling (and go.mod is where the
// rule lives — Go's import resolution enforces what is written here).
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
	for _, mod := range siblings {
		if mod != "github.com/boru-lang/boru/eng/go" {
			t.Errorf("ADR-013: basic/go must depend on eng/go only; go.mod references %s", mod)
		}
	}
	if len(siblings) == 0 {
		t.Fatalf("ADR-013: expected the eng/go requirement in go.mod, found no boru sibling at all")
	}
}
