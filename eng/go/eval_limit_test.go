package eng

import (
	"errors"
	"strings"
	"testing"
)

// A non-terminating program must fail with an explicit evaluation_limit
// error — NOT the phantom "unmatched opening parenthesis" the old silent
// step-limit break produced — and never hang forever.

// registerSpin installs a 0-arg word whose handler re-emits the word
// itself, so it never terminates.
func registerSpin(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "spin",
		Signatures: []Signature{{
			Args: []*Type{},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewWord("spin")}, nil
			}),
			Returns: []*Type{TAny},
		}},
	})
}

func TestEvalLimitExplicitError(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	registerSpin(r)
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()

	e := NewTop(r)
	e.stepLimit = 5000 // tiny cap so the test trips quickly

	_, err = e.Run([]Value{NewWord("spin")})
	if err == nil {
		t.Fatal("non-terminating program returned nil error, want evaluation_limit")
	}
	var ae *AqlError
	if !errors.As(err, &ae) || ae.Code != "evaluation_limit" {
		t.Fatalf("error = %v, want code evaluation_limit", err)
	}
	// The honest diagnosis must NOT be the old phantom paren error.
	if strings.Contains(ae.Error(), "unmatched opening parenthesis") {
		t.Errorf("step-limit exhaustion reported as a phantom paren error: %v", ae)
	}
}

// A program that finishes within budget must NOT be flagged.
func TestEvalLimitNotTrippedByNormalProgram(t *testing.T) {
	out := runWith(t, registerAdd, []Value{NewWord("add"), NewInteger(2), NewInteger(3)})
	if len(out) != 1 {
		t.Fatalf("got %d results, want 1", len(out))
	}
	if n, _ := AsInteger(out[0]); n != 5 {
		t.Errorf("add 2 3 = %d, want 5", n)
	}
}
