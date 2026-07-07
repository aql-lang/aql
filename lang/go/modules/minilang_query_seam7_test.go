package modules

import (
	"testing"

	"github.com/aql-lang/aql/lang/go/native"
)

// docToAny's Map-branch AsMap-nil fallback (minilang_query.go:36-38) is a
// dead defensive guard: the only concrete TMap-conforming values that make
// AsMap return nil are typed-map carriers ({:T}), and the shared
// native.ValueToAny the fallback delegates to itself panics on those
// (valueToAny's Map case calls m.Len() on the nil map). No non-panicking
// input can reach the arm, so it is unexercisable without provoking the
// shared-helper panic. Left uncovered by proof rather than a fabricated
// Frankenstein value.

// TestS7B_QueryDocToAnyListIdeal drives docToAny's List-branch Ideal
// fallback: a concrete TList value AsList cannot read (an empty typed-list
// carrier) is projected through idealListToAny.
func TestS7B_QueryDocToAnyListIdeal(t *testing.T) {
	v := native.NewTypedList(native.NewTypeLiteral(native.TInteger))
	_ = docToAny(v)
}
