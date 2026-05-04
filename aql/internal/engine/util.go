package engine

import "fmt"

// Shared utility helpers consolidating duplicated patterns spread
// across the engine package. The goal is a single, well-tested
// surface for the half-dozen interactions every handler repeats —
// type-literal vs. concrete-value distinction, panic-safe map/list
// access, type-checked map-field lookups, and DefStacks resolution.
//
// Each helper is independently testable; the test file
// (util_test.go) targets 100% coverage. New duplicated patterns
// found in future code should land here, not be re-inlined.

// IsTypeLiteral reports whether v is a bare type literal — a Value
// whose VType identifies a type but carries no concrete payload and
// is not a CheckMode carrier. Type literals have Data == nil and
// Carrier == false; the convention is established in CLAUDE.md and
// scattered through the codebase as raw `v.Data == nil` checks.
//
// The single exception is None: Value{VType:TNone, Data:nil}
// represents both the unit type AND the unique None value, so
// IsTypeLiteral returns false for None — the caller would otherwise
// treat None as a "type" rather than a value.
func IsTypeLiteral(v Value) bool {
	if v.Data != nil || v.Carrier {
		return false
	}
	if v.VType.Equal(TNone) {
		return false
	}
	return true
}

// IsConcrete reports whether v carries a real payload (not a type
// literal, not a Carrier). Mirrors the negation of IsTypeLiteral
// without the None exception — None values are concrete in the
// sense that they're real values flowing through the program.
func IsConcrete(v Value) bool {
	return v.Data != nil && !v.Carrier
}

// RequireConcreteList unwraps a list-typed Value into its ReadList,
// returning an error if the value is a type literal, a carrier, or
// otherwise lacks a concrete list payload. Use this at native-handler
// entry points that take a list arg via [TList] or [TAny]; it
// replaces the recurring `if args[i].Data == nil { return error }`
// preamble.
//
// The op string is included in the error so callers can identify
// their site without wrapping again.
func RequireConcreteList(v Value, op string) (ReadList, error) {
	if v.Data == nil {
		return ReadList{}, fmt.Errorf("%s: expected a concrete list, got type literal %s", op, v.VType.String())
	}
	if v.Carrier {
		return ReadList{}, fmt.Errorf("%s: expected a concrete list, got carrier %s", op, v.VType.String())
	}
	list := v.AsList()
	if list.IsNil() {
		return ReadList{}, fmt.Errorf("%s: value is not a list (got %s)", op, v.VType.String())
	}
	return list, nil
}

// RequireConcreteMap unwraps a map-typed Value into its ReadMap. As
// with RequireConcreteList, type literals, carriers, and map-subtypes
// (RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo) that lack a
// concrete OrderedMap payload return an error.
func RequireConcreteMap(v Value, op string) (ReadMap, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("%s: expected a concrete map, got type literal %s", op, v.VType.String())
	}
	if v.Carrier {
		return nil, fmt.Errorf("%s: expected a concrete map, got carrier %s", op, v.VType.String())
	}
	m := v.AsMap()
	if m == nil {
		return nil, fmt.Errorf("%s: value is not a concrete map (got %s)", op, v.VType.String())
	}
	return m, nil
}

// MapFieldString fetches a String-valued field from a ReadMap.
// Returns the string and true on hit; "" and false when the key is
// absent OR the value's type is not String. Replaces the
// `if v, ok := m.Get(k); ok && v.VType.Matches(TString) { s, _ := v.AsString(); … }`
// pattern that appeared in fileio.go, native_string_helpers.go, and
// native_type_make.go.
//
// Nil map → false (panic-safe).
func MapFieldString(m ReadMap, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m.Get(key)
	if !ok || !v.VType.Matches(TString) {
		return "", false
	}
	s, err := v.AsString()
	if err != nil {
		return "", false
	}
	return s, true
}

// MapFieldInteger fetches an Integer-valued field. Same shape as
// MapFieldString.
func MapFieldInteger(m ReadMap, key string) (int64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m.Get(key)
	if !ok || !v.VType.Matches(TInteger) || v.IsDepScalar() {
		return 0, false
	}
	n, err := v.AsInteger()
	if err != nil {
		return 0, false
	}
	return n, true
}

// MapFieldBoolean fetches a Boolean-valued field.
func MapFieldBoolean(m ReadMap, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	v, ok := m.Get(key)
	if !ok || !v.VType.Matches(TBoolean) {
		return false, false
	}
	b, err := v.AsBoolean()
	if err != nil {
		return false, false
	}
	return b, true
}

// MapFieldDecimal fetches a Decimal-valued field.
func MapFieldDecimal(m ReadMap, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m.Get(key)
	if !ok || !v.VType.Matches(TDecimal) || v.IsDepScalar() {
		return 0, false
	}
	f, err := v.AsDecimal()
	if err != nil {
		return 0, false
	}
	return f, true
}

// TopOfDefStack returns the most recent binding for a name in the
// def stack, or zero Value and false if the stack is empty / name
// unbound. Replaces the `if ds := r.DefStacks[name]; len(ds) > 0
// { top := ds[len(ds)-1]; … }` indexing dance that appears 16+
// times across carrier.go, forloop.go, native_definition_def.go,
// native_help.go, native_type_is.go, native_type_typedef.go,
// native_control_do.go, and match.go.
func (r *Registry) TopOfDefStack(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	ds := r.DefStacks[name]
	if len(ds) == 0 {
		return Value{}, false
	}
	return ds[len(ds)-1], true
}

// ResolveTypedName resolves a name to its type value through the
// type-resolution chain used by the typed-def handler and `is`:
// r.Types first (the dedicated type registry), then DefStacks (legacy
// path for record/object/DepScalar definitions). Returns the resolved
// value and true if found; zero Value and false otherwise.
//
// Centralises the lookup so future namespace changes (a single
// type/def store, scoped types) only need to update one site.
func (r *Registry) ResolveTypedName(name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	if tv, ok := r.Types[name]; ok {
		return tv, true
	}
	return r.TopOfDefStack(name)
}

// FlattenDisjunctAlts returns the alternatives of a disjunct value
// or a single-element slice containing v if it isn't a disjunct.
// Replaces the
// `if v.IsDisjunct() { d, _ := v.AsDisjunct(); alts = append(alts,
// d.Alternatives...) } else { alts = append(alts, v) }` pattern in
// `tor`, `tand`, `tany`, and several unify branches.
//
// Inlines the type-assertion the IsDisjunct/AsDisjunct pair would
// otherwise duplicate so there's no unreachable error branch.
// Values whose VType is TDisjunct but whose payload isn't a real
// DisjunctInfo fall back to the single-element slice.
func FlattenDisjunctAlts(v Value) []Value {
	if d, ok := v.Data.(DisjunctInfo); ok && v.VType.Equal(TDisjunct) {
		return d.Alternatives
	}
	return []Value{v}
}
