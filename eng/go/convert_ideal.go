package eng

import "errors"

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
