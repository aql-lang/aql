package eng

import (
	"sync"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// namedFnVal builds a NON-anonymous FnDef value carrying its own authored
// sigs — the shape a fn literal stored in a map / module export has.
func namedFnVal(name string, params []FnParam, returns []*Type, body []Value) Value {
	return NewFunction(FnDefInfo{
		Name: name,
		Signatures: []Signature{{
			Params:     params,
			Returns:    returns,
			Impl:       Boru(body),
			BarrierPos: BarrierAllForward,
		}},
	})
}

func w8ArmCompile(t *testing.T, r *core.Registry) func() {
	t.Helper()
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	return done
}

// entryCollector gathers InterpEntry events thread-safely (forks may emit
// concurrently under the -race lanes).
type entryCollector struct {
	mu      sync.Mutex
	entries []InterpEntry
}

// armEmit arms a live bytecode recorder on r so es.Active() is true.
func armEmit(r *core.Registry) *EmitState {
	es := NewEmitState()
	r.Check.Emit = es
	return es
}

// --- joinedElementCarrier ---------------------------------------------------
