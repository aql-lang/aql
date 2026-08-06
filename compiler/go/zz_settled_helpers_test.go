package compiler

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

// Pure helpers settled at the carve.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Pure helpers settled at the carve.

func w8ArmCompile(t *testing.T, r *core.Registry) func() {
	t.Helper()
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	return done
}

// --- the record-time spec derivation ---------------------------------------

// testRecorderState unwraps the concrete backend for the branch-pipeline
// fixtures; under a plain-check pass (inactive recorder) it returns a nil
// *EmitState whose methods no-op via the kernel's nil-receiver guards —
// the same decline the pre-S2 interface no-ops provided.
func testRecorderState(c *core.CheckState) *EmitState {
	es, _ := c.Recorder().(*EmitState)
	return es
}
