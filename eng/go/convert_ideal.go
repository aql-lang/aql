package eng

import (
	"errors"
	"sort"
	"strconv"
)

// ErrNoConverter lets a Behavior decline the IdealConverter capability so
// the parent-chain walk continues — mirrors ErrNoComparer. A wrapper
// Behavior that structurally satisfies IdealConverter but has no body
// installed returns this from ToMap/ToList.
var ErrNoConverter = errors.New("eng: no converter in this Behavior")

// IdealConverter is the optional capability that lets an Ideal type
// convert its values to a Map or a List. Implement it on a TypeBehavior.
//
// The base Ideal type (idealConvertBehavior, installed below) provides the
// fallback: any Ideal that does not override converts to an empty Map ({})
// or empty List ([]). Concrete Ideals — built-in or user-defined, in Go or
// AQL — may attach a Behavior implementing this to give a meaningful
// projection. Dispatch walks the value's Parent chain (ConvertIdealToMap /
// ConvertIdealToList) exactly like the Comparer walk, so the nearest
// override wins and the Ideal root guarantees a terminal result.
type IdealConverter interface {
	ToMap(v Value) (Value, error)
	ToList(v Value) (Value, error)
}

// ConvertIdealToMap converts an Ideal value to a Map by walking its
// Parent chain for the nearest IdealConverter. Falls back to an empty Map
// when nothing in the chain converts (the Ideal root always does).
func ConvertIdealToMap(v Value) (Value, error) {
	for t := ValueType(v); t != nil; t = t.Parent {
		c, ok := t.Behavior.(IdealConverter)
		if !ok {
			continue
		}
		m, err := c.ToMap(v)
		if errors.Is(err, ErrNoConverter) {
			continue
		}
		return m, err
	}
	return NewMap(NewOrderedMap()), nil
}

// ConvertIdealToList converts an Ideal value to a List by the same walk.
func ConvertIdealToList(v Value) (Value, error) {
	for t := ValueType(v); t != nil; t = t.Parent {
		c, ok := t.Behavior.(IdealConverter)
		if !ok {
			continue
		}
		l, err := c.ToList(v)
		if errors.Is(err, ErrNoConverter) {
			continue
		}
		return l, err
	}
	return NewList(nil), nil
}

// idealConvertBehavior is the base Ideal Behavior. It delegates the core
// TypeBehavior methods to DefaultBehavior and supplies the empty-{}/[]
// IdealConverter fallback for every Ideal that does not override.
type idealConvertBehavior struct{}

func (idealConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (idealConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (idealConvertBehavior) Format(v Value) string       { return DefaultBehavior.Format(v) }
func (idealConvertBehavior) ToMap(Value) (Value, error)  { return NewMap(NewOrderedMap()), nil }
func (idealConvertBehavior) ToList(Value) (Value, error) { return NewList(nil), nil }

func init() {
	// Install the fallback converter on the Ideal root. Package vars
	// (incl. TIdeal) are initialised before init() runs — see the
	// compare_scalar_behaviors.go init for the same ordering rationale.
	TIdeal.Behavior = idealConvertBehavior{}
}

// ---- concrete converters for the built-in Ideal kinds ----
//
// Each behavior adds the IdealConverter capability to a kernel Ideal
// without changing its rendering: Format delegates to kernelFormatDefault
// (Value.String calls that Format, so
// error(…)/array/etc. output is unchanged). Match/Equal stay default.

// objectConvertBehavior: an Object instance → its fields (own + inherited).
type objectConvertBehavior struct{}

func (objectConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (objectConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (objectConvertBehavior) Format(v Value) string       { return kernelFormatDefault(v) }
func (objectConvertBehavior) ToMap(v Value) (Value, error) {
	oi, err := AsObjectInstance(v)
	if err != nil {
		return NewMap(NewOrderedMap()), nil
	}
	return NewMap(objectFieldMap(&oi)), nil
}
func (objectConvertBehavior) ToList(v Value) (Value, error) {
	oi, err := AsObjectInstance(v)
	if err != nil {
		return NewList(nil), nil
	}
	return NewList(orderedMapValues(objectFieldMap(&oi))), nil
}

// objectFieldMap flattens an object's prototype chain (base first) then
// its own fields into a fresh OrderedMap.
func objectFieldMap(oi *ObjectInstanceInfo) *OrderedMap {
	out := NewOrderedMap()
	var fill func(o *ObjectInstanceInfo)
	fill = func(o *ObjectInstanceInfo) {
		if o == nil {
			return
		}
		fill(o.Prototype)
		if o.Fields != nil {
			for _, k := range o.Fields.Keys() {
				val, _ := o.Fields.Get(k)
				out.Set(k, val)
			}
		}
	}
	fill(oi)
	return out
}

func orderedMapValues(m *OrderedMap) []Value {
	out := make([]Value, 0, m.Len())
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		out = append(out, v)
	}
	return out
}

// arrayConvertBehavior: an Array instance → its elements (List); ToMap is
// an index→element map.
type arrayConvertBehavior struct{}

func (arrayConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (arrayConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (arrayConvertBehavior) Format(v Value) string       { return kernelFormatDefault(v) }
func (arrayConvertBehavior) ToList(v Value) (Value, error) {
	ai, err := AsArray(v)
	if err != nil || ai == nil {
		return NewList(nil), nil
	}
	return NewList(append([]Value(nil), ai.Elems...)), nil
}
func (arrayConvertBehavior) ToMap(v Value) (Value, error) {
	ai, err := AsArray(v)
	if err != nil || ai == nil {
		return NewMap(NewOrderedMap()), nil
	}
	m := NewOrderedMap()
	for i, e := range ai.Elems {
		m.Set(strconv.Itoa(i), e)
	}
	return NewMap(m), nil
}

// storeConvertBehavior: a Store → its own key/value entries (sorted keys
// for determinism); ToList is the values in that order.
type storeConvertBehavior struct{}

func (storeConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (storeConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (storeConvertBehavior) Format(v Value) string       { return kernelFormatDefault(v) }
func (storeConvertBehavior) ToMap(v Value) (Value, error) {
	return NewMap(storeEntryMap(v)), nil
}
func (storeConvertBehavior) ToList(v Value) (Value, error) {
	return NewList(orderedMapValues(storeEntryMap(v))), nil
}

func storeEntryMap(v Value) *OrderedMap {
	out := NewOrderedMap()
	si, err := AsStore(v)
	if err != nil || si == nil {
		return out
	}
	keys := make([]string, 0, len(si.Data))
	for k := range si.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Set(k, si.Data[k])
	}
	return out
}

// errorConvertBehavior: an Error → {message:…}; ToList is [message].
type errorConvertBehavior struct{}

func (errorConvertBehavior) Match(v Value, t *Type) bool { return DefaultBehavior.Match(v, t) }
func (errorConvertBehavior) Equal(a, b Value) bool       { return DefaultBehavior.Equal(a, b) }
func (errorConvertBehavior) Format(v Value) string       { return kernelFormatDefault(v) }
func (errorConvertBehavior) ToMap(v Value) (Value, error) {
	m := NewOrderedMap()
	if ei, err := AsError(v); err == nil {
		m.Set("message", NewString(ei.Message))
	}
	return NewMap(m), nil
}
func (errorConvertBehavior) ToList(v Value) (Value, error) {
	if ei, err := AsError(v); err == nil {
		return NewList([]Value{NewString(ei.Message)}), nil
	}
	return NewList(nil), nil
}

func init() {
	TObject.Behavior = objectConvertBehavior{}
	TArray.Behavior = arrayConvertBehavior{}
	TStore.Behavior = storeConvertBehavior{}
	TError.Behavior = errorConvertBehavior{}
}
