package engine_test

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// runAQL is a test helper shared by external `package engine_test` tests.
// The `package engine` internal tests have their own copy in integration_test.go.
func runAQL(t *testing.T, r *engine.Registry, tokens []engine.Value) []engine.Value {
	t.Helper()
	e := engine.NewTop(r)
	result, err := e.Run(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return result
}

func runAQLError(t *testing.T, r *engine.Registry, tokens []engine.Value) error {
	t.Helper()
	e := engine.NewTop(r)
	_, err := e.Run(tokens)
	return err
}
