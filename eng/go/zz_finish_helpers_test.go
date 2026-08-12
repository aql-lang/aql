package eng

import (
	"sync"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// namedFnVal builds a NON-anonymous FnDef value carrying its own authored
// sigs — the shape a fn literal stored in a map / module export has.
func namedFnVal(name string, params []core.FnParam, returns []*core.Type, body []core.Value) core.Value {
	return core.NewFunction(core.FnDefInfo{
		Name: name,
		Signatures: []core.Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       core.Boru(body),
			BarrierPos: core.BarrierAllForward,
		}},
	})
}

func w8ArmCompile(t *testing.T, r *core.Registry) func() {
	t.Helper()
	done := r.Check.Begin()
	r.Check.Emit = compiler.NewEmitState()
	r.Check.Compiling = true
	return done
}

// entryCollector gathers InterpEntry events thread-safely (forks may emit
// concurrently under the -race lanes).
type entryCollector struct {
	mu      sync.Mutex
	entries []core.InterpEntry
}

// armEmit arms a live bytecode recorder on r so es.Active() is true.
func armEmit(r *core.Registry) *compiler.EmitState {
	es := compiler.NewEmitState()
	r.Check.Emit = es
	return es
}

// --- joinedElementCarrier ---------------------------------------------------
