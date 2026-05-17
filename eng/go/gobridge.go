package eng

// Value <-> Go-type conversion helpers.
//
// Host programs (CLIs, REPLs, integrations) frequently need to round-
// trip data between AQL Values and plain Go values: a CLI passes a Go
// `map[string]any` returned by some library down into an AQL handler,
// and renders an AQL Value back as Go data for serialisation. These
// helpers centralise that conversion so callers don't reinvent the
// payload-unwrapping logic per project.
//
// ToGo maps a Value down to a plain Go value:
//   String   → string
//   Integer  → int64
//   Decimal  → float64
//   Boolean  → bool
//   Atom     → string (the atom name)
//   List     → []any  (each element recursively ToGo'd)
//   Map      → map[string]any (likewise)
//   None     → nil
//   anything else → v.String() (best-effort textual fallback)
//
// FromGo lifts a plain Go value up to an AQL Value:
//   nil           → None
//   string        → String
//   bool          → Boolean
//   int / int64   → Integer
//   float64       → Integer if integral-valued, else Decimal
//   []any         → List (each element recursively FromGo'd)
//   map[string]any→ Map (likewise)
//   fmt.Stringer / anything else → String of fmt.Sprintf("%v", x)
//
// FromGo is intentionally lenient — it never errors. Callers passing
// data of an unknown shape get a stringified fallback so the value at
// least surfaces in the AQL stream.

import "fmt"

// ToGo converts an AQL Value into a plain Go value. See the package
// header comment for the mapping.
func ToGo(v Value) any {
	switch {
	case v.VType == nil:
		return nil
	case v.VType.Matches(TNone):
		return nil
	case v.VType.Matches(TString):
		s, _ := AsString(v)
		return s
	case v.VType.Matches(TInteger):
		n, _ := AsInteger(v)
		return n
	case v.VType.Matches(TDecimal):
		f, _ := AsDecimal(v)
		return f
	case v.VType.Matches(TBoolean):
		b, _ := AsBoolean(v)
		return b
	case v.VType.Matches(TAtom):
		a, _ := AsAtom(v)
		return a
	case v.VType.Matches(TMap):
		rm, err := AsMap(v)
		if err != nil {
			return v.String()
		}
		out := make(map[string]any, rm.Len())
		for _, k := range rm.Keys() {
			vv, _ := rm.Get(k)
			out[k] = ToGo(vv)
		}
		return out
	case v.VType.Matches(TList):
		rl, err := AsList(v)
		if err != nil {
			return v.String()
		}
		out := make([]any, rl.Len())
		for i := 0; i < rl.Len(); i++ {
			out[i] = ToGo(rl.Get(i))
		}
		return out
	}
	return v.String()
}

// FromGo lifts a plain Go value to an AQL Value. See the package
// header comment for the mapping. Never returns an error — unknown
// shapes fall back to a stringified Value.
func FromGo(x any) Value {
	switch v := x.(type) {
	case nil:
		return NewNone()
	case Value:
		return v
	case string:
		return NewString(v)
	case bool:
		return NewBoolean(v)
	case int:
		return NewInteger(int64(v))
	case int32:
		return NewInteger(int64(v))
	case int64:
		return NewInteger(v)
	case uint:
		return NewInteger(int64(v))
	case uint32:
		return NewInteger(int64(v))
	case uint64:
		return NewInteger(int64(v))
	case float32:
		return floatToValue(float64(v))
	case float64:
		return floatToValue(v)
	case []any:
		out := make([]Value, len(v))
		for i, e := range v {
			out[i] = FromGo(e)
		}
		return NewList(out)
	case map[string]any:
		m := NewOrderedMap()
		for k, vv := range v {
			m.Set(k, FromGo(vv))
		}
		return NewMap(m)
	}
	return NewString(fmt.Sprintf("%v", x))
}

// floatToValue promotes integer-valued floats to Integer to keep CLI
// output compact (e.g. JSON's `1.0` renders as `1` rather than `1.0`).
// Non-integral floats stay as Decimal.
func floatToValue(f float64) Value {
	if !isFinite(f) {
		return NewDecimal(f)
	}
	if f == float64(int64(f)) {
		return NewInteger(int64(f))
	}
	return NewDecimal(f)
}

func isFinite(f float64) bool {
	return f == f && f-f == 0
}
