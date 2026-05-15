package eng

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
	list, err := AsList(v)
	if err != nil || list.IsNil() {
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
	m, err := AsMap(v)
	if err != nil || m == nil {
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
	s, err := AsString(v)
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
	n, err := AsInteger(v)
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
	b, err := AsBoolean(v)
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
	f, err := AsDecimal(v)
	if err != nil {
		return 0, false
	}
	return f, true
}

// --- Pos threading ------------------------------------------------------

// WithPos returns v with its Pos copied from src. Use when a handler
// constructs a new Value from an input — error reporting downstream
// then has the source location even though the new value is
// structurally unrelated to the input.
func WithPos(v, src Value) Value {
	v.Pos = src.Pos
	return v
}

// predicateSandbox holds the slice/map state that RunPredicate
// snapshots before invoking a predicate body. DefStacks is NOT
// included — CallAQL handles that itself. r.Check is preserved by
// reference (the entire CheckState struct is copied) so any
// per-call diagnostics or step counters set during the predicate
// don't leak.
type predicateSandbox struct {
	types    *TypeTable
	ctxStack []*StoreInstanceInfo
	check    CheckState
}

func snapshotPredicateState(r *Registry) predicateSandbox {
	if r == nil {
		return predicateSandbox{}
	}
	return predicateSandbox{
		types:    r.Types.Clone(),
		ctxStack: r.Contexts.Snapshot(),
		check:    r.Check,
	}
}

func restorePredicateState(r *Registry, s predicateSandbox) {
	if r == nil {
		return
	}
	r.Types = s.types
	r.Contexts.Restore(s.ctxStack)
	r.Check = s.check
}

// AsConcreteString unwraps a String-typed Value into its Go string,
// returning a clear error if the value is a DepScalar constraint
// payload rather than a concrete String. The lattice override makes
// `DepString.Matches(TString)` true for sig-matching purposes, so
// any code path that sees a TString value and immediately calls
// `AsString` will hit a `DepString → "" + error` silent miscompile
// when the caller swallows the error. Use AsConcreteString in any
// path where the concrete payload is required (display, comparison,
// indexing, …); the error is loud and discoverable.
func (v Value) AsConcreteString() (string, error) {
	if v.IsDepScalar() {
		return "", fmt.Errorf("AsConcreteString: value is a dependent-type constraint (%s), not a concrete String", v.VType.String())
	}
	// Handlers with dual [TString]+[TAtom] signatures (trim, upper,
	// lower, concat, …) call into AsConcreteString from either path;
	// historically they relied on the raw-string payload being
	// shared between strings and atoms. Post Step 5, atoms carry
	// AtomPayload; accept it here so the "textual content" semantic
	// of AsConcreteString is preserved for those handlers.
	if IsAtom(v) {
		return AsAtom(v)
	}
	return AsString(v)
}

// AsConcreteInteger — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteInteger() (int64, error) {
	if v.IsDepScalar() {
		return 0, fmt.Errorf("AsConcreteInteger: value is a dependent-type constraint (%s), not a concrete Integer", v.VType.String())
	}
	return AsInteger(v)
}

// AsConcreteDecimal — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteDecimal() (float64, error) {
	if v.IsDepScalar() {
		return 0, fmt.Errorf("AsConcreteDecimal: value is a dependent-type constraint (%s), not a concrete Decimal", v.VType.String())
	}
	return AsDecimal(v)
}

// AsConcreteBoolean — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteBoolean() (bool, error) {
	if v.IsDepScalar() {
		return false, fmt.Errorf("AsConcreteBoolean: value is a dependent-type constraint (%s), not a concrete Boolean", v.VType.String())
	}
	return AsBoolean(v)
}

// AsConcreteAtom — DepScalar-rejecting accessor. See AsConcreteString.
func (v Value) AsConcreteAtom() (string, error) {
	if v.IsDepScalar() {
		return "", fmt.Errorf("AsConcreteAtom: value is a dependent-type constraint (%s), not a concrete Atom", v.VType.String())
	}
	return AsAtom(v)
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
// Values whose VType is a Disjunct (Type/Disjunct or a subtype such
// as Type/Disjunct/Enum) but whose payload isn't a real DisjunctInfo
// fall back to the single-element slice.
func FlattenDisjunctAlts(v Value) []Value {
	if d, ok := v.Data.(DisjunctInfo); ok && v.VType.Matches(TDisjunct) {
		return d.Alternatives
	}
	return []Value{v}
}
