package native

import (
	"fmt"

	eng "github.com/boru-lang/boru/eng/go"
)

// TKeyVal is the Node/Map/KeyVal type — the entry value a map-iteration body
// receives (the `e` in `m each ([e:KeyVal] => …)`). It is a child of Map, so
// every map word works on it and it renders as a map; it carries exactly four
// fields:
//
//	k : String    the entry's key
//	v : Any       the entry's value
//	i : Integer   the current index in the key iteration (0-based)
//	n : Integer   the total number of keys being iterated
//
// Being a Map subtype means `e.k` / `e.v` / `e.i` / `e.n` read with ordinary
// dotted access, `e is Map` is true, and `typeof e` is `KeyVal`. Matching is
// nominal (DefaultBehavior): a plain `{k:… v:… i:… n:…}` map is NOT a KeyVal —
// only a value the constructor tags is — so the type reliably marks "this came
// from map iteration".
//
// FixedID 5002 is the next free id in the 5000–9999 kernel/language band,
// after Module (5000) and ModuleExport (5001). See eng/go/CLAUDE.md
// "FixedID Allocation".
var TKeyVal = registerKeyValType()

func registerKeyValType() *eng.Type {
	t, err := eng.Builtin.RegisterType("Node/Map/KeyVal", 5002, eng.OwnerKernel, nil)
	if err != nil {
		// Init-time registration error — recorded, not panicked.
		// See ADR-005 and typeinit.go.
		recordTypeInitErr(fmt.Errorf("native_keyval: register Node/Map/KeyVal: %w", err))
	}
	return t
}

// KeyVal field names. A KeyVal map carries exactly these keys, in this order.
const (
	KeyValK = "k" // key
	KeyValV = "v" // value
	KeyValI = "i" // 0-based iteration index
	KeyValN = "n" // total number of keys
)

// NewKeyVal builds a KeyVal entry for key k with value v, at iteration index i
// of n total keys. The result is a Map tagged Node/Map/KeyVal — it is at once a
// KeyVal and a Map. This is the single constructor the map-iteration words use
// to present each entry to a body.
func NewKeyVal(k string, v Value, i, n int64) Value {
	om := NewOrderedMap()
	om.Set(KeyValK, NewString(k))
	om.Set(KeyValV, v)
	om.Set(KeyValI, NewInteger(i))
	om.Set(KeyValN, NewInteger(n))
	return NewValueRaw(TKeyVal, MapPayload{M: om})
}
