package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
)

// The `|` token marks the forward/stack boundary in an `fn` parameter
// list. `__SB` (Stack Barrier) is an alias for environments where a
// bare `|` is awkward to type — shell pipelines, command-line one-
// liners, etc. Both forms produce identical dispatch behavior.

// TestStackBarrierAliasMidPosition: barrier between params. `b` must
// come from the stack, `a` may come from forward. Both glyphs encode
// the same rule.
func TestStackBarrierAliasMidPosition(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def with-pipe fn [[a:Integer | b:Integer] [Integer] [a add b]]`,
		`def with-sb   fn [[a:Integer __SB b:Integer] [Integer] [a add b]]`,
		// 3 sits on the stack, the fn collects 4 forward as `a`, then
		// takes 3 from the stack as `b`. Both forms compute 4 + 3 = 7.
		`3 with-pipe 4   3 with-sb 4`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(result), result)
	}
	for i, want := range []int64{7, 7} {
		got, err := eng.AsInteger(result[i])
		if err != nil {
			t.Fatalf("result[%d] AsInteger: %v", i, err)
		}
		if got != want {
			t.Errorf("result[%d] = %d, want %d", i, got, want)
		}
	}
}

// TestStackBarrierAliasTrailingPosition: barrier after every param,
// equivalent to all-forward (`| ` at the end means "no slots are
// stack-only"). Both forms accept the same surface call.
func TestStackBarrierAliasTrailingPosition(t *testing.T) {
	result, err := runNativeSteps(t, nil, []string{
		`def all-fwd-pipe fn [[a:Integer b:Integer |] [Integer] [a add b]]`,
		`def all-fwd-sb   fn [[a:Integer b:Integer __SB] [Integer] [a add b]]`,
		`all-fwd-pipe 2 3   all-fwd-sb 4 5`,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2: %v", len(result), result)
	}
	for i, want := range []int64{5, 9} {
		got, _ := eng.AsInteger(result[i])
		if got != want {
			t.Errorf("result[%d] = %d, want %d", i, got, want)
		}
	}
}
