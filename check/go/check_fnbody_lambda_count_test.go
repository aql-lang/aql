package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// LambdaCountContract is a COUNT-only contract: n Any slots, nothing for a
// zero or negative count (a fn declaring no returns has no contract).
func TestLambdaCountContract(t *testing.T) {
	if got := LambdaCountContract(0); got != nil {
		t.Errorf("0 returns: want nil, got %v", got)
	}
	if got := LambdaCountContract(-1); got != nil {
		t.Errorf("negative: want nil, got %v", got)
	}
	got := LambdaCountContract(2)
	if len(got) != 2 {
		t.Fatalf("want 2 slots, got %d", len(got))
	}
	for i, ty := range got {
		if !ty.Equal(core.TAny) {
			t.Errorf("slot %d: want Any, got %v", i, ty)
		}
	}
}
